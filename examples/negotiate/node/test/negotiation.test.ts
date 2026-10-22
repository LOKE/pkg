import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { after, before, describe, it } from "node:test";
import { RpcHTTPError } from "@loke/http-rpc-client";
import express from "express";
import { NegotiatingClient } from "../src/client.ts";
import { MsgpackCodec } from "../src/msgpack.ts";
import { ProtobufCodec } from "../src/protobuf.ts";
import type { Defs, Schema } from "../src/schema.ts";
import { type RunningServer, listen, makeService, rpcRouter, serveNode, startGoServer, startRecorder } from "./helpers.ts";

const proto = new ProtobufCodec();
const msgpack = new MsgpackCodec();
const codecs = [proto, msgpack];

const definitions: Defs = { Thing: { properties: { id: { type: "int32" }, name: { type: "string" }, tags: { elements: { type: "string" } } } } };
const idRequest: Schema = { properties: { id: { type: "int32" } } };

const makeThings = (help?: string) =>
  makeService(
    "things",
    definitions,
    {
      get: { requestTypeDef: idRequest, responseTypeDef: { ref: "Thing" }, help },
      fail: { requestTypeDef: idRequest, responseTypeDef: { ref: "Thing" } },
      broken: { requestTypeDef: idRequest, responseTypeDef: { ref: "Thing" } },
      ping: { requestTypeDef: { properties: {} }, responseTypeDef: { metadata: { void: true } } },
      noRequest: { responseTypeDef: { ref: "Thing" } },
    },
    {
      get: ({ id }: { id: number }) => ({ id, name: `thing ${id}`, tags: ["a", "b"] }),
      fail: () => {
        const err = Object.assign(new Error(), { type: "https://errors.example.test/thing_failed", code: "thing_failed", expose: true });
        throw Object.defineProperty(err, "message", { enumerable: true, value: "no thing for you" });
      },
      broken: () => ({ id: "not a number" }),
      ping: () => undefined,
      noRequest: () => ({ id: 1, name: "x", tags: [] }),
    },
  );

const things = makeThings();
const wanted = { id: 7, name: "thing 7", tags: ["a", "b"] };
const types = (wire: { method: string; status: number; requestType: string; responseType: string }[]) =>
  wire.map((e) => [e.method, e.status, e.requestType.split(";")[0], e.responseType.split(";")[0]]);

describe("server", () => {
  let negotiated: RunningServer;
  let plain: RunningServer;
  before(async () => {
    negotiated = await serveNode([things], codecs);
    plain = await serveNode([things]);
  });
  after(() => {
    negotiated.stop();
    plain.stop();
  });

  const post = (path: string, headers: Record<string, string>, body: BodyInit = "{}") =>
    fetch(`${negotiated.url}/rpc/things/${path}`, { method: "POST", headers, body });
  const etag = async () => (await fetch(`${negotiated.url}/rpc/things`, { method: "OPTIONS" })).headers.get("etag")!;
  const protoBody = (value: unknown, schema: Schema = idRequest) => proto.encode(value, schema, definitions) as BodyInit;

  describe("OPTIONS", () => {
    it("advertises the codecs and the schema ETag", async () => {
      const options = await fetch(`${negotiated.url}/rpc/things`, { method: "OPTIONS" });
      const get = await fetch(`${negotiated.url}/rpc/things`);
      const body = await get.text();

      assert.equal(options.status, 200);
      assert.equal(options.headers.get("allow"), "GET, POST, OPTIONS");
      assert.equal(options.headers.get("accept-post"), "application/json, application/x-protobuf, application/vnd.msgpack");
      assert.equal(options.headers.get("cache-control"), "max-age=300");
      assert.equal(options.headers.get("etag"), `"${createHash("sha256").update(body).digest("hex")}"`);
      assert.equal(await options.text(), body);
    });

    it("changes the ETag when the service meta changes", async () => {
      const other = await serveNode([makeThings("different help")], codecs);
      try {
        const before = (await fetch(`${negotiated.url}/rpc/things`, { method: "OPTIONS" })).headers.get("etag");
        const after = (await fetch(`${other.url}/rpc/things`, { method: "OPTIONS" })).headers.get("etag");
        assert.notEqual(before, after);
      } finally {
        other.stop();
      }
    });

    it("is not answered by a server without codecs", async () => {
      const res = await fetch(`${plain.url}/rpc/things`, { method: "OPTIONS" });
      assert.equal(res.status, 404);
    });
  });

  describe("JSON clients", () => {
    it("see the same GET and POST bytes with or without codecs", async () => {
      for (const path of ["", "/get"]) {
        const [a, b] = await Promise.all(
          [negotiated, plain].map((server) =>
            fetch(`${server.url}/rpc/things${path}`, path ? { method: "POST", headers: { "Content-Type": "application/json" }, body: '{"id":7}' } : {}).then(async (res) => ({
              status: res.status,
              type: res.headers.get("content-type"),
              body: await res.text(),
            })),
          ),
        );
        assert.deepEqual(a, b, path || "meta");
      }
    });
  });

  describe("If-Match", () => {
    it("is required for protobuf requests and responses", async () => {
      const requests: [string, Record<string, string>][] = [
        ["protobuf request without If-Match", { "Content-Type": proto.contentType }],
        ["protobuf request with a stale If-Match", { "Content-Type": proto.contentType, "If-Match": '"stale"' }],
        ["protobuf response to a JSON request", { "Content-Type": "application/json", Accept: proto.contentType }],
      ];
      for (const [name, headers] of requests) {
        const res = await post("get", headers, headers["Content-Type"] === proto.contentType ? protoBody({ id: 7 }) : '{"id":7}');
        assert.equal(res.status, 412, name);
        assert.equal(res.headers.get("content-type")?.split(";")[0], "application/json", name);
        assert.deepEqual(await res.json(), { message: "schema changed" }, name);
      }
    });

    it("is not needed for msgpack, JSON, or when the ETag matches", async () => {
      const current = await etag();
      const ok: [string, Record<string, string>, BodyInit][] = [
        ["msgpack both ways", { "Content-Type": msgpack.contentType, Accept: msgpack.contentType }, msgpack.encode({ id: 7 }, idRequest, definitions) as BodyInit],
        ["plain JSON", { "Content-Type": "application/json" }, '{"id":7}'],
        ["protobuf with the right ETag", { "Content-Type": proto.contentType, Accept: proto.contentType, "If-Match": current }, protoBody({ id: 7 })],
      ];
      for (const [name, headers, body] of ok) {
        const res = await post("get", headers, body);
        assert.equal(res.status, 200, name);
      }
    });
  });

  describe("responses", () => {
    it("use the first accepted codec and say so", async () => {
      const res = await post("get", { "Content-Type": "application/json", Accept: "text/html, application/vnd.msgpack, application/x-protobuf", }, '{"id":7}');
      assert.equal(res.headers.get("content-type"), msgpack.contentType);
      assert.equal(res.headers.get("vary"), "Accept");
      assert.deepEqual(msgpack.decode(Buffer.from(await res.arrayBuffer())), wanted);
    });

    it("fall back to JSON when no accepted type is supported", async () => {
      const res = await post("get", { "Content-Type": "application/json", Accept: "application/xml" }, '{"id":7}');
      assert.equal(res.headers.get("content-type")?.split(";")[0], "application/json");
      assert.deepEqual(await res.json(), wanted);
    });

    it("are empty for a void method", async () => {
      const res = await post("ping", { "Content-Type": proto.contentType, Accept: proto.contentType, "If-Match": await etag() }, protoBody({}, { properties: {} }));
      assert.equal(res.status, 200);
      assert.equal(res.headers.get("content-type"), proto.contentType);
      assert.equal((await res.arrayBuffer()).byteLength, 0);
    });

    it("stay JSON for application errors and response validation errors", async () => {
      for (const [method, code] of [
        ["fail", "thing_failed"],
        ["broken", "response-validation"],
      ]) {
        const res = await post(method, { "Content-Type": proto.contentType, Accept: proto.contentType, "If-Match": await etag() }, protoBody({ id: 1 }));
        assert.equal(res.status, 400, method);
        assert.equal(res.headers.get("content-type")?.split(";")[0], "application/json", method);
        assert.equal(((await res.json()) as { code: string }).code, code, method);
      }
    });
  });

  describe("bad binary requests", () => {
    const binary = async (body: BodyInit, contentType = proto.contentType) =>
      post("get", { "Content-Type": contentType, Accept: contentType, "If-Match": await etag() }, body);

    for (const [name, body] of [
      ["truncated", Buffer.from([0x0a, 0x05, 0x08])],
      ["wrong wire type", Buffer.from([0x0a, 0x02, 0x0a, 0x00])],
      ["out of range", Buffer.from([0x0a, 0x06, 0x08, 0xff, 0xff, 0xff, 0xff, 0x1f])],
    ] as const) {
      it(`answer 400 JSON for protobuf that is ${name}`, async () => {
        const res = await binary(body);
        assert.equal(res.status, 400);
        assert.equal(res.headers.get("content-type")?.split(";")[0], "application/json");
        assert.match(((await res.json()) as { message: string }).message, /^Invalid request: /);
      });
    }

    it("answer 400 JSON for msgpack that is truncated", async () => {
      const res = await binary(Buffer.from([0x81, 0xa2, 0x69]), msgpack.contentType);
      assert.equal(res.status, 400);
      assert.match(((await res.json()) as { message: string }).message, /^Invalid request: /);
    });

    it("answer a schema validation error when decoded msgpack does not fit the schema", async () => {
      const res = await binary(msgpack.encode({ id: "seven" }, { properties: { id: { type: "string" } } }, {}) as BodyInit, msgpack.contentType);
      assert.equal(res.status, 400);
      assert.equal(((await res.json()) as { code: string }).code, "validation");
    });

    it("answer 415 for a method that takes no request", async () => {
      const res = await post("noRequest", { "Content-Type": proto.contentType, "If-Match": await etag() }, Buffer.alloc(0));
      assert.equal(res.status, 415);
      assert.deepEqual(await res.json(), { message: `endpoint does not support ${proto.contentType}` });
    });

    it("leave unknown methods to http-rpc", async () => {
      const res = await post("nope", { "Content-Type": proto.contentType, "If-Match": await etag() }, Buffer.alloc(0));
      assert.equal(res.status, 404);
    });

    it("treat an empty body as an empty message", async () => {
      const res = await binary(Buffer.alloc(0));
      assert.equal(res.status, 200);
      assert.deepEqual(proto.decode(Buffer.from(await res.arrayBuffer()), { ref: "Thing" }, definitions), { id: 0, name: "thing 0", tags: ["a", "b"] });
    });
  });
});

describe("client", () => {
  const servers: { stop: () => void }[] = [];
  after(() => servers.forEach((s) => s.stop()));

  const through = async (server: { url: string }) => {
    const recorder = await startRecorder(server.url);
    servers.push(recorder);
    return recorder;
  };
  const clientFor = (url: string, ...with_: typeof codecs) => new NegotiatingClient(`${url}/rpc/things`, "things", { codecs: with_ });
  const probes = (recorder: { exchanges: { method: string }[] }) => recorder.exchanges.filter((e) => e.method === "OPTIONS").length;

  describe("negotiates", () => {
    let recorder: Awaited<ReturnType<typeof startRecorder>>;
    before(async () => {
      const server = await serveNode([things], codecs);
      servers.push(server);
      recorder = await through(server);
    });

    it("with the first codec it prefers that the server lists", async () => {
      for (const [preference, want] of [
        [[msgpack, proto], msgpack.contentType],
        [[proto, msgpack], proto.contentType],
      ] as const) {
        recorder.take();
        await clientFor(recorder.url, ...preference).request("get", { id: 7 });
        assert.equal(recorder.take().at(-1)!.requestType, want);
      }
    });

    it("probing once for concurrent first calls", async () => {
      recorder.take();
      const client = clientFor(recorder.url, proto);
      const results = await Promise.all(Array.from({ length: 5 }, () => client.request("get", { id: 7 })));
      assert.deepEqual(results, Array(5).fill(wanted));
      assert.equal(probes(recorder), 1);
    });

    it("and resolves a void method", async () => {
      assert.equal(await clientFor(recorder.url, proto).request("ping", {}), undefined);
    });
  });

  describe("falls back to JSON", () => {
    it("against a server without codecs, probing once", async () => {
      const recorder = await through(await serveNode([things]).then((s) => (servers.push(s), s)));
      const client = clientFor(recorder.url, proto, msgpack);
      for (let i = 0; i < 3; i++) assert.deepEqual(await client.request("get", { id: 7 }), wanted);
      assert.deepEqual(types(recorder.take()), [
        ["OPTIONS", 404, "", "application/json"],
        ...Array(3).fill(["POST", 200, "application/json", "application/json"]),
      ]);
    });

    it("against a Go server without codecs", async () => {
      const go = await startGoServer(["-plain"]);
      servers.push(go);
      const recorder = await through(go);
      const client = new NegotiatingClient(`${recorder.url}/rpc/menu`, "menu", { codecs });
      assert.equal(await client.request("deleteMenu", { menuId: "x" }), null);
      assert.deepEqual(types(recorder.take()), [
        ["OPTIONS", 405, "", "text/plain"],
        ["POST", 200, "application/json", "application/json"],
      ]);
    });

    const stub = async (options: (req: express.Request, res: express.Response) => void) => {
      const app = express();
      app.use("/rpc", (req, res, next) => (req.method === "OPTIONS" ? options(req, res) : next()), rpcRouter([things]));
      const server = await listen(app);
      servers.push(server);
      return through(server);
    };

    for (const [name, handle] of [
      ["an OPTIONS that answers 405", (_req: express.Request, res: express.Response) => res.status(405).end()],
      ["an OPTIONS with no ETag", (_req: express.Request, res: express.Response) => res.set("Accept-Post", "application/x-protobuf").json({ interfaces: [] })],
      ["no codec in common", (_req: express.Request, res: express.Response) => res.set({ "Accept-Post": "application/json, application/cbor", ETag: '"x"' }).json({ interfaces: [] })],
      ["an OPTIONS body that is not meta", (_req: express.Request, res: express.Response) => res.set({ "Accept-Post": "application/x-protobuf", ETag: '"x"' }).send("not json")],
    ] as const) {
      it(`on ${name}`, async () => {
        const recorder = await stub(handle);
        const client = clientFor(recorder.url, proto);
        await client.request("get", { id: 7 });
        await client.request("get", { id: 7 });
        assert.deepEqual(
          types(recorder.take()).map((e) => [e[0], e[2]]),
          [["OPTIONS", ""], ["POST", "application/json"], ["POST", "application/json"]],
        );
      });
    }

    it("when the server cannot be reached, remembering for a while", async () => {
      const closed = await listen(express());
      closed.stop();
      const original = globalThis.fetch;
      let optionsCalls = 0;
      globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
        if (init?.method === "OPTIONS") optionsCalls++;
        return original(input, init);
      }) as typeof fetch;
      try {
        const client = clientFor(closed.url, proto);
        await assert.rejects(client.request("get", { id: 7 }), RpcHTTPError);
        await assert.rejects(client.request("get", { id: 7 }), RpcHTTPError);
      } finally {
        globalThis.fetch = original;
      }
      assert.equal(optionsCalls, 1);
    });
  });

  describe("retries once as JSON", () => {
    it("when the schema changed under a cached ETag, then probes again", async () => {
      const app = express();
      let router = rpcRouter([makeThings("v1")], codecs);
      app.use("/rpc", (req, res, next) => router(req, res, next));
      const server = await listen(app);
      servers.push(server);
      const recorder = await through(server);
      const client = clientFor(recorder.url, proto);

      assert.deepEqual(await client.request("get", { id: 7 }), wanted);
      router = rpcRouter([makeThings("v2")], codecs);
      recorder.take();

      assert.deepEqual(await client.request("get", { id: 7 }), wanted);
      assert.deepEqual(types(recorder.take()), [
        ["POST", 412, proto.contentType, "application/json"],
        ["POST", 200, "application/json", "application/json"],
      ]);

      assert.deepEqual(await client.request("get", { id: 7 }), wanted);
      assert.deepEqual(types(recorder.take()), [
        ["OPTIONS", 200, "", "application/json"],
        ["POST", 200, proto.contentType, proto.contentType],
      ]);
    });

    for (const status of [400, 415]) {
      it(`when a replica without codec support answers ${status} with a body that is not JSON`, async () => {
        const app = express();
        app.use("/rpc", (req, res, next) => {
          const binary = req.method === "POST" && req.get("Content-Type") === proto.contentType;
          if (binary) res.status(status).type("text/plain").send("unsupported");
          else next();
        }, rpcRouter([things], codecs));
        const server = await listen(app);
        servers.push(server);
        const recorder = await through(server);

        assert.deepEqual(await clientFor(recorder.url, proto).request("get", { id: 7 }), wanted);
        assert.deepEqual(types(recorder.take()), [
          ["OPTIONS", 200, "", "application/json"],
          ["POST", status, proto.contentType, "text/plain"],
          ["POST", 200, "application/json", "application/json"],
        ]);
      });
    }
  });

  describe("does not retry", () => {
    let recorder: Awaited<ReturnType<typeof startRecorder>>;
    before(async () => {
      const server = await serveNode([things], codecs);
      servers.push(server);
      recorder = await through(server);
    });

    it("an application error, which stays JSON", async () => {
      recorder.take();
      await assert.rejects(clientFor(recorder.url, proto).request("fail", { id: 1 }), (err: Error) => {
        assert.match(String(err), /no thing for you/);
        return true;
      });
      assert.deepEqual(types(recorder.take()), [
        ["OPTIONS", 200, "", "application/json"],
        ["POST", 400, proto.contentType, "application/json"],
      ]);
    });

    it("a JSON 400 for a response that fails validation", async () => {
      recorder.take();
      await assert.rejects(clientFor(recorder.url, msgpack).request("broken", { id: 1 }), /response|received/i);
      assert.equal(recorder.take().filter((e) => e.method === "POST").length, 1);
    });
  });

  it("does not cache probes whose max-age is zero", async () => {
    const app = express();
    app.use("/rpc", (req, res, next) => {
      const setHeader = res.setHeader.bind(res);
      res.setHeader = (name: string, value: any) => setHeader(name, name.toLowerCase() === "cache-control" ? "max-age=0" : value);
      next();
    }, rpcRouter([things], codecs));
    const server = await listen(app);
    servers.push(server);
    const recorder = await through(server);
    const client = clientFor(recorder.url, proto);
    await client.request("get", { id: 7 });
    await client.request("get", { id: 7 });
    assert.equal(probes(recorder), 2);
  });

  it("refuses to send values outside the schema, without sending", async () => {
    const server = await serveNode([things], codecs);
    servers.push(server);
    const recorder = await through(server);
    const client = clientFor(recorder.url, proto);
    await assert.rejects(client.request("get", { id: 2 ** 31 }), /out of range/);
    assert.equal(recorder.take().filter((e) => e.method === "POST").length, 0);
  });
});
