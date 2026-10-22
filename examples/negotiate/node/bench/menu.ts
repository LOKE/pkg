// Run from this directory with node --import tsx bench/menu.ts or bun bench/menu.ts.
// Mirrors an ordering-service benchmark of @loke/http-rpc against a Connect bridge:
// same menu, cases, sampling and output, with negotiated binary formats in place of Connect.
import { deepStrictEqual, ok, strictEqual } from "node:assert";
import { once } from "node:events";
import { Agent, createServer, request } from "node:http";
import type { AddressInfo } from "node:net";
import { performance } from "node:perf_hooks";
import { promisify } from "node:util";
import { gunzipSync, gzip } from "node:zlib";
import { createRequestHandler, serviceWithSchema } from "@loke/http-rpc";
import { RPCClient } from "@loke/http-rpc-client";
import express from "express";
import { NegotiatingClient } from "../src/client.ts";
import { MsgpackCodec } from "../src/msgpack.ts";
import { ProtobufCodec } from "../src/protobuf.ts";
import type { Codec, Defs, Schema } from "../src/schema.ts";
import { negotiate } from "../src/server.ts";

const fulfillmentTypes = ["FULFILLMENT_TYPE_UNSPECIFIED", "FULFILLMENT_TYPE_PICK_UP", "FULFILLMENT_TYPE_DELIVERY"];
const text = (type: string) => ({ type });
const refTo = (ref: string) => ({ ref });
const int32 = { type: "int32" };

const definitions = {
  Price: { properties: { amount: int32, taxIncluded: int32 } },
  Product: {
    properties: {
      id: text("string"),
      title: text("string"),
      description: text("string"),
      available: text("boolean"),
      price: refTo("Price"),
      tags: { elements: text("string") },
    },
    optionalProperties: { plu: text("string"), imageUrl: text("string"), options: refTo("ProductOptions") },
  },
  ProductOptions: { properties: { items: { elements: refTo("Option") } } },
  Option: {
    properties: {
      id: text("string"),
      title: text("string"),
      minQuantity: int32,
      maxQuantity: int32,
      choices: { elements: refTo("Choice") },
    },
  },
  Choice: { properties: { minQuantity: int32, maxQuantity: int32, defaultQuantity: int32, product: refTo("Product") } },
  Category: {
    properties: { id: text("string"), title: text("string"), tags: { elements: text("string") }, products: { elements: refTo("Product") } },
  },
  Menu: {
    properties: {
      id: text("string"),
      title: text("string"),
      fulfillmentTypes: { elements: { enum: fulfillmentTypes } },
      categories: { elements: refTo("Category") },
    },
  },
  GetMenuResponse: { properties: { currency: text("string"), menus: { elements: refTo("Menu") } } },
  GetMenuRequest: {
    properties: {
      organizationUid: text("string"),
      locationUid: text("string"),
      fulfillmentType: { enum: fulfillmentTypes },
      includeOptions: text("boolean"),
    },
  },
} as unknown as Defs;

const description =
  "Prepared fresh to order with seasonal local ingredients, roasted vegetables, fragrant herbs and our house-made dressing. Please tell our team about dietary requirements; our kitchen handles gluten, dairy, eggs, sesame and nuts. ";
const products = Array.from({ length: 200 }, (_, index) => ({
  id: `product-${index}`,
  plu: `PLU-${1000 + index}`,
  title: `Seasonal kitchen plate ${index + 1}`,
  description: description.repeat(2),
  imageUrl: `https://images.example.com/menu/product-${index}.jpg`,
  available: true,
  price: { amount: 1800 + index, taxIncluded: 164 },
  tags: ["fresh", "popular"],
  options: [
    {
      id: `option-${index}`,
      title: "Choose your accompaniments",
      minQuantity: 0,
      maxQuantity: 3,
      choices: Array.from({ length: 3 }, (_, choice) => ({
        minQuantity: 0,
        maxQuantity: 2,
        defaultQuantity: choice === 0 ? 1 : 0,
        product: {
          id: `side-${index}-${choice}`,
          title: ["Garden salad", "Roasted potatoes", "Seasonal greens"][choice],
          description: description.repeat(1),
          available: true,
          price: { amount: 350, taxIncluded: 32 },
          tags: ["vegetarian"],
        },
      })),
    },
  ],
}));
const plainMenu = {
  currency: "AUD",
  menus: [
    {
      id: "all-day",
      title: "All day dining",
      fulfillmentTypes: ["FULFILLMENT_TYPE_PICK_UP", "FULFILLMENT_TYPE_DELIVERY"],
      categories: Array.from({ length: 10 }, (_, index) => ({
        id: `category-${index}`,
        title: `Kitchen favourites ${index + 1}`,
        tags: [],
        products: products.slice(index * 20, (index + 1) * 20).map((product) => ({ ...product, options: { items: product.options } })),
      })),
    },
  ],
};
const plainSmall = {
  organizationUid: "org-123456",
  locationUid: "location-123456",
  fulfillmentType: "FULFILLMENT_TYPE_PICK_UP",
  includeOptions: true,
};

let decoded: unknown;
const menuService = serviceWithSchema(
  {
    encode: (_request: object) => plainMenu,
    decode: (request: unknown) => {
      decoded = request;
      return {};
    },
    small: (request: unknown) => request,
  },
  {
    name: "menu-bench",
    definitions: definitions as never,
    strictResponseValidation: true,
    logger: {
      error(message: string) {
        throw new Error(message);
      },
    },
    methods: {
      encode: { requestTypeDef: { properties: {} }, responseTypeDef: refTo("GetMenuResponse") },
      decode: { requestTypeDef: refTo("GetMenuResponse"), responseTypeDef: { properties: {} } },
      small: { requestTypeDef: refTo("GetMenuRequest"), responseTypeDef: refTo("GetMenuRequest") },
    } as never,
  },
);
const methodSchemas = {
  encode: { request: { properties: {} }, response: refTo("GetMenuResponse") },
  decode: { request: refTo("GetMenuResponse"), response: { properties: {} } },
  small: { request: refTo("GetMenuRequest"), response: refTo("GetMenuRequest") },
} as Record<string, { request: Schema; response: Schema }>;

const proto = new ProtobufCodec();
const msgpack = new MsgpackCodec();

const compress = promisify(gzip);
const app = express();
const routes = express.Router();
const gzipRoutes = express.Router();
// The JSON and binary routes need the same negotiated, asynchronous gzip step.
// Never compress or cache the fixture early.
app.use("/gzip", (req, res, next) => {
  if (
    (req.path.startsWith("/legacy/") || req.path.startsWith("/rpc/")) &&
    req.get("accept-encoding")?.split(/,\s*/).includes("gzip")
  ) {
    const send = res.send.bind(res);
    res.send = (body: unknown) => {
      if (typeof body !== "string" && !Buffer.isBuffer(body)) return send(body);
      res.vary("Accept-Encoding");
      res.set("Content-Encoding", "gzip");
      void compress(body).then((compressed) => {
        res.removeHeader("Content-Length");
        send(compressed);
      }, next);
      return res;
    };
  }
  next();
});
app.use("/gzip", gzipRoutes);
app.use(routes);
for (const router of [routes, gzipRoutes]) {
  router.use("/legacy", express.json({ limit: "20mb" }), createRequestHandler([menuService]));
  router.use("/rpc", negotiate([menuService], { codecs: [proto, msgpack] }), express.json({ limit: "20mb" }), createRequestHandler([menuService]));
}

const warmup = Number(process.env.BENCH_WARMUP ?? 30);
const iterations = Number(process.env.BENCH_ITERATIONS ?? 100);
const rounds = Number(process.env.BENCH_ROUNDS ?? 9);
if (![warmup, iterations, rounds].every((value) => Number.isInteger(value) && value > 0)) {
  throw new Error("Benchmark counts must be positive integers");
}
const server = app.listen(0, "127.0.0.1");
const agent = new Agent({ keepAlive: true, maxSockets: 1 });

const variants = [
  { name: "legacy", prefix: "/legacy/menu-bench/", codec: undefined },
  { name: "negotiated-json", prefix: "/rpc/menu-bench/", codec: undefined },
  { name: "negotiated-proto", prefix: "/rpc/menu-bench/", codec: proto as Codec },
  { name: "msgpack", prefix: "/rpc/menu-bench/", codec: msgpack as Codec },
] as const;
type Variant = (typeof variants)[number];
type Direction = "encode" | "decode" | "small";

const decodeResponse = (variant: Variant, bytes: Buffer, direction: Direction) =>
  variant.codec
    ? variant.codec.decode(bytes, methodSchemas[direction].response, definitions)
    : JSON.parse(bytes.toString());

function assertOutput(value: unknown, direction: "encode" | "small") {
  deepStrictEqual(value, direction === "encode" ? plainMenu : plainSmall);
}

function realCalls(baseUrl: string, variant: Variant) {
  const url = baseUrl + variant.prefix;
  const client = variant.codec
    ? new NegotiatingClient(url.slice(0, -1), variant.name, { codecs: [variant.codec] })
    : new RPCClient(url, variant.name);
  return {
    encode: (): Promise<unknown> => client.request("encode", {}),
    small: (): Promise<unknown> => client.request("small", plainSmall),
  };
}

async function main() {
  await once(server, "listening");
  const port = (server.address() as AddressInfo).port;
  const baseUrl = `http://127.0.0.1:${port}`;
  const etag = (await fetch(`${baseUrl}/rpc/menu-bench`, { method: "OPTIONS" })).headers.get("etag")!;
  const legacyBody = Buffer.from(JSON.stringify(plainMenu));
  ok(legacyBody.length > 400_000);
  strictEqual(products.length, 200);
  strictEqual(
    plainMenu.menus[0].categories.flatMap((category) => category.products).length,
    200,
  );

  const requestBody = (variant: Variant, direction: Direction) => {
    const value = direction === "encode" ? {} : direction === "small" ? plainSmall : plainMenu;
    return variant.codec
      ? Buffer.from(variant.codec.encode(value, methodSchemas[direction].request, definitions))
      : Buffer.from(JSON.stringify(value));
  };

  function rawCall(path: string, variant: Variant, body: Buffer, collect = false): Promise<Buffer | number> {
    const { promise, resolve, reject } = Promise.withResolvers<Buffer | number>();
    const contentType = variant.codec?.contentType ?? "application/json";
    const req = request(
      {
        host: "127.0.0.1",
        port,
        path,
        method: "POST",
        agent,
        headers: {
          "content-type": contentType,
          "content-length": body.length,
          "accept-encoding": "identity",
          "x-request-id": "menu-benchmark",
          ...(variant.codec ? { accept: contentType, ...(variant.codec.schemaFree ? {} : { "if-match": etag }) } : {}),
        },
      },
      (response) => {
        let bytes = 0;
        const chunks: Buffer[] = [];
        response.on("data", (chunk: Buffer) => {
          bytes += chunk.length;
          if (collect || response.statusCode !== 200) chunks.push(chunk);
        });
        response.on("error", reject);
        response.on("end", () => {
          if (response.statusCode !== 200) reject(new Error(`${response.statusCode}: ${Buffer.concat(chunks).toString()}`));
          else if (collect && response.headers["content-encoding"]) reject(new Error("Server-only response unexpectedly compressed"));
          else if (collect && response.headers["content-type"]?.split(";")[0] !== (variant.codec?.contentType ?? "application/json")) {
            reject(new Error(`Unexpected response type ${response.headers["content-type"]}`));
          } else resolve(collect ? Buffer.concat(chunks) : bytes);
        });
      },
    );
    req.on("error", reject);
    req.end(body);
    return promise;
  }

  // This transparent preflight proxy observes actual client HTTP bodies before
  // either client's decompressor. It is never in the timed loop.
  let observed:
    | {
        requestBody: Buffer;
        responseBody: Buffer;
        acceptEncoding: string;
        requestEncoding: string;
        responseEncoding: string;
      }
    | undefined;
  function capturedWire() {
    const capture = observed;
    ok(capture, "Wire preflight did not observe the real client request");
    return capture;
  }
  const proxy = createServer((incoming, outgoing) => {
    const requestChunks: Buffer[] = [];
    incoming.on("data", (chunk: Buffer) => requestChunks.push(chunk));
    const upstream = request(
      {
        host: "127.0.0.1",
        port,
        path: incoming.url,
        method: incoming.method,
        headers: { ...incoming.headers, host: `127.0.0.1:${port}` },
        agent,
      },
      (response) => {
        const responseChunks: Buffer[] = [];
        outgoing.writeHead(response.statusCode ?? 500, response.headers);
        response.on("data", (chunk: Buffer) => responseChunks.push(chunk));
        response.on("end", () => {
          // The negotiation probe is an OPTIONS; only the call itself is wire data.
          if (incoming.method !== "POST") return;
          observed = {
            requestBody: Buffer.concat(requestChunks),
            responseBody: Buffer.concat(responseChunks),
            acceptEncoding: String(incoming.headers["accept-encoding"] ?? ""),
            requestEncoding: String(incoming.headers["content-encoding"] ?? "identity"),
            responseEncoding: String(response.headers["content-encoding"] ?? "identity"),
          };
        });
        response.on("error", (error) => outgoing.destroy(error));
        response.pipe(outgoing);
      },
    );
    upstream.on("error", (error) => outgoing.destroy(error));
    incoming.on("error", (error) => upstream.destroy(error));
    incoming.pipe(upstream);
  });
  proxy.listen(0, "127.0.0.1");
  await once(proxy, "listening");
  const proxyUrl = `http://127.0.0.1:${(proxy.address() as AddressInfo).port}`;
  const wire: {
    case: number;
    variant: string;
    requestBytes: number;
    responseBytes: number;
    responseDecodedBytes: number;
    requestEncoding: string;
    responseEncoding: string;
    acceptEncoding: string;
  }[] = [];
  try {
    for (const compressed of [false, true]) {
      for (const variant of variants) {
        observed = undefined;
        const calls = realCalls(proxyUrl + (compressed ? "/gzip" : ""), variant);
        assertOutput(await calls.encode(), "encode");
        // The response end handler runs before the client has decoded its body.
        const capture = capturedWire();
        strictEqual(capture.requestEncoding, "identity");
        strictEqual(capture.responseEncoding, compressed ? "gzip" : "identity");
        if (compressed) {
          ok(capture.acceptEncoding.split(/,\s*/).includes("gzip"));
          strictEqual(capture.responseBody[0], 0x1f);
          strictEqual(capture.responseBody[1], 0x8b);
        }
        const decodedBody = compressed ? gunzipSync(capture.responseBody) : capture.responseBody;
        if (compressed) ok(capture.responseBody.length < decodedBody.length);
        deepStrictEqual(decodeResponse(variant, decodedBody, "encode"), plainMenu);
        if (variant.codec) deepStrictEqual(variant.codec.decode(capture.requestBody, methodSchemas.encode.request, definitions), {});
        else deepStrictEqual(JSON.parse(capture.requestBody.toString()), {});
        wire.push({
          case: compressed ? 5 : 4,
          variant: variant.name,
          requestBytes: capture.requestBody.length,
          responseBytes: capture.responseBody.length,
          responseDecodedBytes: decodedBody.length,
          requestEncoding: capture.requestEncoding,
          responseEncoding: capture.responseEncoding,
          acceptEncoding: capture.acceptEncoding,
        });
      }
    }
  } finally {
    proxy.closeAllConnections();
    proxy.close();
  }

  type Cell = {
    case: number;
    name: string;
    variant: Variant;
    direction: Direction;
    run: () => Promise<unknown>;
    verify: () => Promise<void>;
    samples: number[];
  };
  const cells: Cell[] = [];
  const directions = ["encode", "decode", "small"] as const;
  const caseNames = [
    "server-only large response",
    "server-only large request",
    "server-only small call",
    "end-to-end large response gzip off",
    "end-to-end large response gzip on",
    "end-to-end small call",
  ];
  for (const [index, direction] of directions.entries()) {
    for (const variant of variants) {
      const body = requestBody(variant, direction);
      const path = variant.prefix + direction;
      cells.push({
        case: index + 1,
        name: caseNames[index],
        variant,
        direction,
        samples: [],
        run: () => rawCall(path, variant, body),
        verify: async () => {
          decoded = undefined;
          const bytes = await rawCall(path, variant, body, true);
          ok(Buffer.isBuffer(bytes));
          if (direction === "decode") {
            deepStrictEqual(decoded, plainMenu);
            deepStrictEqual(decodeResponse(variant, bytes, "decode"), {});
          } else {
            assertOutput(decodeResponse(variant, bytes, direction), direction);
          }
        },
      });
    }
  }
  for (const caseNumber of [4, 5, 6]) {
    for (const variant of variants) {
      const compressed = caseNumber === 5;
      const direction = caseNumber === 6 ? "small" : "encode";
      const calls = realCalls(baseUrl + (compressed ? "/gzip" : ""), variant);
      const run = calls[direction];
      cells.push({
        case: caseNumber,
        name: caseNames[caseNumber - 1],
        variant,
        direction,
        samples: [],
        run,
        verify: async () => assertOutput(await run(), direction),
      });
    }
  }
  for (const cell of cells) {
    await cell.verify();
    for (let index = 0; index < warmup; index++) await cell.run();
  }
  for (let round = 0; round < rounds; round++) {
    for (let offset = 0; offset < 6; offset++) {
      const caseIndex = (round + offset) % 6;
      const order = round % 2 ? [3, 2, 1, 0] : [0, 1, 2, 3];
      for (const index of order) {
        const cell = cells[caseIndex * 4 + index];
        const start = performance.now();
        for (let iteration = 0; iteration < iterations; iteration++) await cell.run();
        cell.samples.push((performance.now() - start) / iterations);
      }
    }
  }
  for (const cell of cells) await cell.verify();

  const results = cells.map((cell) => {
    const sorted = [...cell.samples].sort((a, b) => a - b);
    const middle = Math.floor(sorted.length / 2);
    const medianMs = sorted.length % 2 ? sorted[middle] : (sorted[middle - 1] + sorted[middle]) / 2;
    const baseline = cells[(cell.case - 1) * 4];
    return {
      case: cell.case,
      name: cell.name,
      variant: cell.variant.name,
      medianMs,
      roundMs: cell.samples,
      pairedWins: cell.samples.filter((value, index) => value < baseline.samples[index]).length,
      pairedRounds: rounds,
    };
  });

  console.error(`legacy request body ${legacyBody.length} bytes`);
  console.error(["case", ...variants.map((v) => v.name)].join("\t"));
  for (let caseNumber = 1; caseNumber <= 6; caseNumber++) {
    const row = results.filter((r) => r.case === caseNumber);
    console.error([caseNames[caseNumber - 1], ...row.map((r) => r.medianMs.toFixed(3))].join("\t"));
  }
  console.error(["wire case/variant", "request", "response", "decoded"].join("\t"));
  for (const w of wire) console.error([`${w.case} ${w.variant}`, w.requestBytes, w.responseBytes, w.responseDecodedBytes].join("\t"));

  console.log(
    JSON.stringify(
      {
        runtime: {
          name: process.versions.bun ? "Bun" : "Node",
          version: process.versions.bun ?? process.version,
          versions: process.versions,
          platform: process.platform,
          arch: process.arch,
        },
        products: products.length,
        nestedChoices: products.length * 3,
        warmup,
        iterations,
        rounds,
        scope: {
          serverOnly:
            "Pre-encoded request; server validation/dispatch/serialization and HTTP response drain; no client parsing; gzip off",
          endToEnd:
            "Actual @loke/http-rpc-client (legacy, negotiated-json) and NegotiatingClient over it (negotiated-proto, msgpack); encode, loopback server, full client decode; requests uncompressed",
          legacySchema:
            "Hand-written JTD of the same menu shape; actual serviceWithSchema request and strict response validation",
          wire: "Actual real-client HTTP request/response body bytes captured by an untimed transparent proxy; excludes headers, chunk framing, TCP/IP; responseDecodedBytes is informational, not gzip wire size",
          compression:
            "Off: no Express compression; on: native fetch negotiation plus async zlib gzip for every path; all on responses verified gzip magic and gunzip fixture equality",
          sampling:
            "Four paths adjacent per case; complete order reversed each round; case order rotated; milliseconds/request; paired comparisons use the matching legacy round",
        },
        results,
        wire,
      },
      null,
      2,
    ),
  );
}
main()
  .finally(() => {
    agent.destroy();
    server.closeAllConnections();
    server.close();
  })
  .catch((error: unknown) => {
    console.error(error);
    process.exitCode = 1;
  });
