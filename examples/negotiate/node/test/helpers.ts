import { type ChildProcess, execFileSync, spawn } from "node:child_process";
import { once } from "node:events";
import { createServer, request, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { createErrorHandler, createRequestHandler, serviceWithSchema } from "@loke/http-rpc";
import express from "express";
import { negotiate } from "../src/server.ts";
import type { Codec, Defs, Schema } from "../src/schema.ts";
import { formatTimestamp, parseTimestamp } from "../src/timestamp.ts";

const goDir = resolve(import.meta.dirname, "../..");
let goBinary: string | undefined;

// GO overrides the toolchain command, for example GO="go".
export function buildGo(): string {
  if (goBinary) return goBinary;
  const [command, ...prefix] = (process.env.GO ?? "mise exec go@1.27.2 -- go").split(" ");
  goBinary = join(mkdtempSync(join(tmpdir(), "negotiate-")), "negotiate");
  execFileSync(command, [...prefix, "build", "-o", goBinary, "."], { cwd: goDir, stdio: "inherit" });
  return goBinary;
}

export interface Dump {
  schemas: Record<string, { schema: Schema; defs: Defs }>;
  cases: { name: string; schema: string; json: unknown; proto: string; msgpack: string }[];
}

export const goDump = (): Dump => JSON.parse(execFileSync(buildGo(), ["dump"], { maxBuffer: 64 << 20 }).toString());

export interface GoCall {
  results: { call: string; value?: unknown; error?: string }[];
  exchanges: { method: string; path: string; status: number; requestType: string; responseType: string; ifMatch: string }[];
}

export function goCall(url: string, codec: string): Promise<GoCall> {
  return new Promise((done, fail) => {
    const child = spawn(buildGo(), ["call", "-url", url, "-codec", codec]);
    const out: Buffer[] = [];
    child.stdout.on("data", (chunk) => out.push(chunk));
    child.stderr.pipe(process.stderr);
    child.on("close", (code) => (code === 0 ? done(JSON.parse(Buffer.concat(out).toString())) : fail(new Error(`go call exited ${code}`))));
  });
}

export async function startGoServer(args: string[] = []): Promise<{ url: string; stop: () => void }> {
  const child: ChildProcess = spawn(buildGo(), ["serve", ...args]);
  child.stderr!.pipe(process.stderr);
  const [line] = (await once(child.stdout!, "data")) as [Buffer];
  const match = /listening (http:\/\/\S+)/.exec(line.toString());
  if (!match) throw new Error(`unexpected go server output: ${line}`);
  return { url: match[1], stop: () => child.kill() };
}

export interface Exchange {
  method: string;
  path: string;
  status: number;
  requestType: string;
  responseType: string;
  ifMatch?: string;
}

// A transparent proxy that records what crossed the wire.
export async function startRecorder(target: string) {
  const exchanges: Exchange[] = [];
  const targetUrl = new URL(target);
  const server = createServer((incoming, outgoing) => {
    const upstream = request(
      { host: targetUrl.hostname, port: targetUrl.port, path: incoming.url, method: incoming.method, headers: incoming.headers },
      (response) => {
        exchanges.push({
          method: incoming.method!,
          path: incoming.url!,
          status: response.statusCode!,
          requestType: String(incoming.headers["content-type"] ?? ""),
          responseType: String(response.headers["content-type"] ?? ""),
          ifMatch: incoming.headers["if-match"] as string | undefined,
        });
        outgoing.writeHead(response.statusCode!, response.headers);
        response.pipe(outgoing);
      },
    );
    incoming.pipe(upstream);
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  return {
    url: `http://127.0.0.1:${(server.address() as AddressInfo).port}`,
    exchanges,
    take: () => exchanges.splice(0),
    stop: () => {
      server.closeAllConnections();
      server.close();
    },
  };
}

type Implementation = Record<string, (args: any) => unknown>;
interface MethodDefs {
  help?: string;
  requestTypeDef?: Schema;
  responseTypeDef?: Schema;
}

export function makeService(name: string, definitions: Defs, methods: Record<string, MethodDefs>, impl: Implementation) {
  return serviceWithSchema(impl, {
    name,
    definitions: definitions as never,
    methods: methods as never,
    strictResponseValidation: true,
    logger: { error: () => undefined },
  });
}

// Builds a service whose schemas are the ones a Go server serves for it.
export async function goService(goUrl: string, name: string, impl: Implementation) {
  const meta = (await (await fetch(`${goUrl}/rpc/${name}`)).json()) as {
    definitions?: Defs;
    interfaces: { methodName: string; requestTypeDef?: Schema; responseTypeDef?: Schema }[];
  };
  const methods = Object.fromEntries(meta.interfaces.map((i) => [i.methodName, { requestTypeDef: i.requestTypeDef, responseTypeDef: i.responseTypeDef }]));
  return makeService(name, meta.definitions ?? {}, methods, impl);
}

export interface RunningServer {
  url: string;
  app: express.Express;
  stop: () => void;
}

export async function listen(app: express.Express): Promise<RunningServer> {
  const server: Server = app.listen(0, "127.0.0.1");
  await once(server, "listening");
  return {
    url: `http://127.0.0.1:${(server.address() as AddressInfo).port}`,
    app,
    stop: () => {
      server.closeAllConnections();
      server.close();
    },
  };
}

export const rpcRouter = (services: ReturnType<typeof makeService>[], codecs?: Codec[]) => {
  const router = express.Router();
  if (codecs) router.use(negotiate(services, { codecs }));
  router.use(express.json({ limit: "20mb" }), createRequestHandler(services), (_req: express.Request, res: express.Response) => res.status(404).json({ message: "Not Found" }), createErrorHandler() as never);
  return router;
};

export async function serveNode(services: ReturnType<typeof makeService>[], codecs?: Codec[]) {
  const app = express();
  app.use("/rpc", rpcRouter(services, codecs));
  return listen(app);
}

// Removes what the binary formats cannot tell apart (null, empty lists and maps)
// and writes every timestamp in UTC, since Go decodes some formats into local time.
export function normalize(value: unknown): unknown {
  if (typeof value === "string" && /^\d{4}-\d\d-\d\dT[\d:.]+(Z|[+-]\d\d:\d\d)$/.test(value)) {
    const { seconds, nanos } = parseTimestamp(value);
    return formatTimestamp(seconds, nanos);
  }
  if (Array.isArray(value)) return value.length === 0 ? null : value.map(normalize);
  if (value && typeof value === "object") {
    const entries = Object.entries(value).map(([key, item]) => [key, normalize(item)] as const);
    return entries.length === 0 ? null : Object.fromEntries(entries.filter(([, item]) => item !== null));
  }
  return value;
}
