import assert from "node:assert/strict";
import { after, before, describe, it } from "node:test";
import { RpcResponseError } from "@loke/http-rpc-client";
import { NegotiatingClient } from "../src/client.ts";
import { MsgpackCodec } from "../src/msgpack.ts";
import { ProtobufCodec } from "../src/protobuf.ts";
import type { Codec } from "../src/schema.ts";
import {
  type Dump,
  type RunningServer,
  goCall,
  goDump,
  goService,
  normalize,
  serveNode,
  startGoServer,
  startRecorder,
} from "./helpers.ts";

const proto = new ProtobufCodec();
const msgpack = new MsgpackCodec();
const codecs: [string, Codec][] = [
  ["protobuf", proto],
  ["msgpack", msgpack],
];

type Json = Record<string, any>;

let dump: Dump;
let go: { url: string; stop: () => void };
const edgeCases = () => dump.cases.filter((c) => c.schema === "edge");
const seedMenu = () => dump.cases.find((c) => c.name === "menu/seed5")!.json as Json;

before(async () => {
  dump = goDump();
  go = await startGoServer();
});
after(() => go.stop());

// Go's JSON, where msgpack carries netip.Addr as bytes the schema calls a string.
const withoutAddr = (edge: unknown) => {
  const { addr: _addr, ...rest } = edge as Json;
  return rest;
};

describe("Node client against the Go server", () => {
  for (const [name, codec] of codecs) {
    describe(name, () => {
      let recorder: Awaited<ReturnType<typeof startRecorder>>;
      before(async () => {
        recorder = await startRecorder(go.url);
      });
      after(() => recorder.stop());

      it("negotiates once and sends binary both ways", async () => {
        const client = new NegotiatingClient(`${recorder.url}/rpc/menu`, "menu", { codecs: [codec] });
        const saved = seedMenu();
        recorder.take();

        const revision = await client.request("saveMenu", saved);
        const got = await client.request("getMenu", { menuId: "lunch" });
        assert.equal(typeof revision.revision, "number");
        assert.deepEqual(normalize(got), normalize(saved));

        const wire = recorder.take();
        assert.deepEqual(
          wire.map((e) => [e.method, e.status, e.requestType, e.responseType.split(";")[0]]),
          [
            ["OPTIONS", 200, "", "application/json"],
            ["POST", 200, codec.contentType, codec.contentType],
            ["POST", 200, codec.contentType, codec.contentType],
          ],
        );
        for (const post of wire.filter((e) => e.method === "POST")) {
          assert.equal(Boolean(post.ifMatch), !codec.schemaFree, "If-Match only for schema bound formats");
        }
      });

      it("returns what the JSON client returns", async () => {
        const binary = new NegotiatingClient(`${go.url}/rpc/menu`, "menu", { codecs: [codec] });
        const json = new NegotiatingClient(`${go.url}/rpc/menu`, "menu", { codecs: [] });
        await binary.request("saveMenu", seedMenu());
        assert.deepEqual(normalize(await binary.request("getMenu", { menuId: "lunch" })), normalize(await json.request("getMenu", { menuId: "lunch" })));
      });

      it("keeps errors as JSON", async () => {
        const client = new NegotiatingClient(`${recorder.url}/rpc/menu`, "menu", { codecs: [codec] });
        recorder.take();
        await assert.rejects(client.request("getMenu", { menuId: "missing" }), (err: Error) => {
          assert.ok(err instanceof RpcResponseError || err.message.includes("missing"), String(err));
          assert.match(String((err as { message?: string }).message ?? err), /missing/);
          return true;
        });
        const post = recorder.take().find((e) => e.method === "POST")!;
        assert.equal(post.status, 400);
        assert.match(post.responseType, /application\/json/);
      });

      it("resolves a void method", async () => {
        const client = new NegotiatingClient(`${go.url}/rpc/menu`, "menu", { codecs: [codec] });
        assert.equal(await client.request("deleteMenu", { menuId: "lunch" }), undefined);
      });

      it("echoes every edge value like JSON does", async () => {
        const binary = new NegotiatingClient(`${go.url}/rpc/edge`, "edge", { codecs: [codec] });
        const json = new NegotiatingClient(`${go.url}/rpc/edge`, "edge", { codecs: [] });
        const strip = codec.schemaFree ? withoutAddr : (edge: unknown) => edge;
        for (const c of edgeCases()) {
          const request = strip(c.json) as Json;
          assert.deepEqual(normalize(strip(await binary.request("echo", request))), normalize(strip(await json.request("echo", request))), c.name);
        }
      });
    });
  }

  it("gets msgpack netip.Addr back as bytes, not the schema's string", async () => {
    const client = new NegotiatingClient(`${go.url}/rpc/edge`, "edge", { codecs: [msgpack] });
    const result = await client.request("echo", withoutAddr(edgeCases()[0].json));
    assert.ok(result.addr instanceof Uint8Array);
  });

  it("sends a 424 KB request to Go and reads the count back", async () => {
    const client = new NegotiatingClient(`${go.url}/rpc/bench`, "bench", { codecs: [proto] });
    const menu = dump.cases.find((c) => c.name === "bench/large")!.json as Json;
    assert.equal(await client.request("decode", menu), 200);
    assert.deepEqual(await client.request("encode", {}), menu);
  });
});

describe("Go client against the Node server", () => {
  let node: RunningServer;
  before(async () => {
    const stored = new Map<string, Json>();
    const revisions = new Map<string, number>();
    const menu = await goService(go.url, "menu", {
      saveMenu: (m: Json) => {
        stored.set(m.id, m);
        revisions.set(m.id, (revisions.get(m.id) ?? 0) + 1);
        return { revision: revisions.get(m.id), savedAt: "2026-03-14T09:26:53.589793Z" };
      },
      getMenu: ({ menuId }: Json) => {
        const found = stored.get(menuId);
        if (!found) {
          const err = Object.assign(new Error(), {
            type: "https://errors.example.test/menu_not_found",
            code: "menu_not_found",
            expose: true,
          });
          // http-rpc serializes the error object, and message is not enumerable on a plain Error.
          throw Object.defineProperty(err, "message", { enumerable: true, value: `no menu "${menuId}"` });
        }
        return found;
      },
      deleteMenu: () => undefined,
    });
    const edge = await goService(go.url, "edge", { echo: (edgeValue: Json) => edgeValue });
    node = await serveNode([menu, edge], [proto, msgpack]);
  });
  after(() => node.stop());

  const runs = new Map<string, Awaited<ReturnType<typeof goCall>>>();
  before(async () => {
    for (const codec of ["json", "protobuf", "msgpack"]) runs.set(codec, await goCall(node.url, codec));
  });

  for (const codec of ["protobuf", "msgpack"]) {
    describe(codec, () => {
      it("saves, reads back and reports errors like the JSON run", () => {
        const menuCalls = (run: string) => runs.get(run)!.results.filter((r) => !r.call.startsWith("echo/"));
        const withoutRevision = (results: ReturnType<typeof menuCalls>) => results.filter((r) => r.call !== "saveMenu");
        assert.deepEqual(normalize(withoutRevision(menuCalls(codec))), normalize(withoutRevision(menuCalls("json"))));
        assert.match(menuCalls(codec).find((r) => r.call === "getMenu/missing")!.error!, /no menu "missing"/);
        assert.equal(menuCalls(codec).find((r) => r.call === "saveMenu")!.value, codec === "protobuf" ? 2 : 3);
      });

      it("uses the negotiated type on the wire and JSON for the error", () => {
        const posts = runs.get(codec)!.exchanges.filter((e) => e.method === "POST" && e.path.startsWith("/rpc/menu/"));
        const contentType = codec === "protobuf" ? "application/x-protobuf" : "application/vnd.msgpack";
        assert.deepEqual(
          posts.map((e) => [e.path, e.status, e.requestType, e.responseType.split(";")[0]]),
          [
            ["/rpc/menu/saveMenu", 200, contentType, contentType],
            ["/rpc/menu/getMenu", 200, contentType, contentType],
            ["/rpc/menu/getMenu", 400, contentType, "application/json"],
          ],
        );
        assert.equal(posts.every((e) => Boolean(e.ifMatch) === (codec === "protobuf")), true);
      });
    });
  }

  it("echoes every edge value over protobuf like JSON does", () => {
    const echoes = (codec: string) => runs.get(codec)!.results.filter((r) => r.call.startsWith("echo/"));
    assert.equal(echoes("protobuf").length, edgeCases().length);
    assert.deepEqual(echoes("protobuf").map((r) => r.error), echoes("protobuf").map(() => undefined));
    assert.deepEqual(normalize(echoes("protobuf")), normalize(echoes("json")));
  });

  it("rejects msgpack netip.Addr because it arrives as bytes where the schema says string", () => {
    const failures = runs.get("msgpack")!.results.filter((r) => r.call.startsWith("echo/") && r.error);
    assert.ok(failures.length > 0);
    assert.match(failures[0].error!, /addr/);
  });
});
