// Binary codec negotiation for @loke/http-rpc services mounted in express. It is
// middleware in front of the unchanged http-rpc handler: decoded binary requests
// are handed over as the parsed body, and binary responses replace res.json.
import { createHash } from "node:crypto";
import { type ServiceSet, createRequestHandler } from "@loke/http-rpc";
import express, { type NextFunction, type Request, type Response } from "express";
import { type Codec, type Defs, type Schema, isVoid } from "./schema.ts";

const metaMaxAgeSeconds = 300;

interface MethodState {
  request?: Schema;
  response?: Schema;
}

interface ServiceState {
  defs: Defs;
  etag: string;
  metaJson: string;
  methods: Map<string, MethodState>;
}

export interface NegotiateOptions {
  codecs: Codec[];
  // Largest binary request body, in express.raw's limit syntax.
  limit?: string;
}

function mediaType(header: string | undefined): string {
  return (header ?? "").split(";")[0].trim().toLowerCase();
}

function serviceState(service: ServiceSet<any>): ServiceState {
  // Serve http-rpc's own GET through a throwaway handler so the ETag covers exactly the bytes it serves.
  let meta: unknown;
  const fakeRes = { json: (body: unknown) => (meta = body) };
  void createRequestHandler([service])(
    { method: "GET", path: `/${service.meta.service}`, headers: {}, body: {} } as never,
    fakeRes as never,
    (err) => {
      throw err;
    },
  );
  if (meta === undefined) throw new Error(`http-rpc did not provide metadata for ${service.meta.service}`);

  const metaJson = JSON.stringify(meta);
  const methods = new Map<string, MethodState>();
  for (const method of service.meta.expose) {
    methods.set(method.methodName, {
      request: method.requestTypeDef as Schema | undefined,
      response: method.responseTypeDef as Schema | undefined,
    });
  }
  return {
    defs: (service.meta.definitions ?? {}) as Defs,
    etag: `"${createHash("sha256").update(metaJson).digest("hex")}"`,
    metaJson,
    methods,
  };
}

// Mount before express.json() and the http-rpc handler:
//   app.use("/rpc", negotiate([service], { codecs }), express.json(), createRequestHandler([service]))
export function negotiate(services: ServiceSet<any>[], options: NegotiateOptions) {
  const states = new Map(services.map((service) => [service.meta.service, serviceState(service)]));
  const acceptPost = ["application/json", ...options.codecs.map((codec) => codec.contentType)].join(", ");
  const readRaw = express.raw({ type: () => true, limit: options.limit ?? "20mb" });

  const findCodec = (contentType: string) => options.codecs.find((codec) => codec.contentType === contentType);

  return (req: Request, res: Response, next: NextFunction) => {
    const [serviceName, methodName, ...rest] = req.path.split("/").filter(Boolean);
    const state = states.get(serviceName);
    if (!state || rest.length > 0) return next();

    if (req.method === "OPTIONS" && methodName === undefined) {
      res.set({
        Allow: "GET, POST, OPTIONS",
        "Accept-Post": acceptPost,
        "Cache-Control": `max-age=${metaMaxAgeSeconds}`,
        ETag: state.etag,
      });
      res.type("json").send(state.metaJson);
      return;
    }

    const method = state.methods.get(methodName);
    if (req.method !== "POST" || !method) return next();

    const requestCodec = findCodec(mediaType(req.get("Content-Type")));
    let responseCodec: Codec | undefined;
    if (method.response) {
      for (const accepted of (req.get("Accept") ?? "").split(",")) {
        responseCodec = findCodec(mediaType(accepted));
        if (responseCodec) break;
      }
    }

    const needsMatch = (requestCodec && !requestCodec.schemaFree) || (responseCodec && !responseCodec.schemaFree);
    if (needsMatch && req.get("If-Match") !== state.etag) {
      res.status(412).json({ message: "schema changed" });
      return;
    }

    if (responseCodec) respondWith(res, responseCodec, method.response!, state.defs);
    if (!requestCodec) return next();

    if (!method.request) {
      res.status(415).json({ message: `endpoint does not support ${requestCodec.contentType}` });
      return;
    }
    readRaw(req, res, (err?: unknown) => {
      if (err) return next(err);
      const body = Buffer.isBuffer(req.body) ? req.body : Buffer.alloc(0);
      try {
        req.body = requestCodec.decode(body, method.request!, state.defs) ?? {};
      } catch (decodeErr) {
        res.status(400).json({ message: `Invalid request: ${(decodeErr as Error).message}` });
        return;
      }
      next();
    });
  };
}

// Successful results are encoded with the codec; errors keep going out as JSON.
function respondWith(res: Response, codec: Codec, schema: Schema, defs: Defs) {
  const json = res.json.bind(res);
  res.json = (result: unknown) => {
    if (res.statusCode !== 200) return json(result);

    let body: Uint8Array;
    try {
      body = isVoid(schema) ? Buffer.alloc(0) : codec.encode(result, schema, defs);
    } catch {
      res.status(500);
      return json({ message: "response could not be encoded" });
    }
    res.type(codec.contentType).vary("Accept");
    return res.send(Buffer.from(body.buffer, body.byteOffset, body.byteLength));
  };
}
