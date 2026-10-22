# negotiate

A prototype of binary wire formats for `lokerpc`. A server and client use protobuf when both sides support it, and plain JSON otherwise. Values are encoded directly between Go structs and wire bytes by reflection, with no JSON step in between. Generated clients do not need regenerating.

This is a nested module so the protobuf dependency stays out of the root `go.mod`. The root `go test ./...` does not cover it; run the commands below from this directory.

## How it works

The JTD schema served at `GET /rpc/<service>` is the IDL. Field numbers and layouts are derived from it deterministically (properties and optional properties, sorted by name). The server's schema is authoritative; the client maps fields by JSON name onto its own, possibly older, structs. Unknown fields are skipped and missing ones are zero.

Negotiation, only when the service has codecs and the client was given codecs:

1. The client sends `OPTIONS /rpc/<service>` once per base URL. A codec server answers 200 with `Accept-Post` (JSON plus its codec content types), `ETag` (sha256 of the service meta), `Cache-Control: max-age=300` and the service meta as the body.
2. The client picks the first of its preferred codecs the server lists and caches that, the ETag and the method schemas for `max-age`.
3. Calls then send the body in the codec's content type with `If-Match: <etag>` and `Accept: <codec>, application/json;q=0.5`. The response is decoded by its `Content-Type`.
4. Errors and non-200 responses are always JSON.

Fallbacks:

- OPTIONS answers 405, 404, any non-200, lists no shared codec, or has no `ETag`: JSON, remembered for `max-age` (default 5 minutes).
- Network error on the probe: JSON, remembered for 30 seconds.
- The schema changed (server answers 412 `{"message":"schema changed"}`): the client drops its cache, retries once as JSON, and re-probes on the next call.
- Mixed fleet during a rolling deploy: a binary POST that lands on a replica without codec support gets a non-JSON 400 or 415. The client treats it like a 412.
- Old clients keep sending JSON and get JSON. `GET /rpc` and `GET /rpc/<service>` are byte-identical with or without codecs.

## MessagePack

`application/vnd.msgpack` is negotiated like protobuf (same OPTIONS probe, `Accept-Post`, `Accept`, and fallbacks) but it is schema free: fields are keyed by their JSON names, so a schema change cannot corrupt a message. A codec says so by implementing `lokerpc.SchemaFree`; the client then sends no `If-Match`, the server does not require one, and a schema change never produces a 412. Protobuf is unchanged.

`msgpackcodec` uses tinylib/msgp generated `MarshalMsg`/`UnmarshalMsg` when a type has them (`//msgp:tag json`, `//msgp:newtime` for the MessagePack timestamp extension, `//msgp:timezone utc`, and a shim that carries `netip.Addr` as raw bytes) and falls back to vmihailenco/msgpack reflection with the `json` struct tag. The two read each other's bytes. The reflection encoder writes nil slices and maps as msgpack nil, which generated decoders reject, so the codec retries a failed generated decode by reflection. Generated code is in `fixtures/msgp_gen.go` and `fixtures/menuclient/msgp_gen.go`; `go generate ./...` rebuilds it and `gen/main_test.go` checks it is current.

## Pluggable JSON

`lokerpc.ServeJSON` and `lokerpc.ClientJSON` swap the JSON library behind request and response bodies. The wire format stays JSON and nothing is negotiated. Without the option the server and client run exactly the code they always did. With it, the server decodes the body straight into the endpoint's request type with the supplied implementation (endpoints without a request type keep the old path) and encodes responses with it.

`jsonengines` provides two: `V2` (`encoding/json/v2`, default options) and `GoJSON` (github.com/goccy/go-json). Differences from `encoding/json` v1, pinned in `jsonengines/jsonengines_test.go`:

- `V2` writes nil slices as `[]` and nil maps as `{}`; v1 and go-json write `null`. A client that tells nil from empty sees a difference.
- `V2` matches member names case-sensitively; `{"NAME":"x"}` no longer fills `name`.
- `V2` rejects duplicate member names and invalid UTF-8; v1 takes the last duplicate and replaces bad bytes with U+FFFD.
- `V2` does not escape `<`, `>` and `&`; v1 and go-json write `\u003c` and friends.
- Unknown members are ignored, `null` clears containers, and type mismatches and trailing data are errors, in all three.

go-json follows v1 on every point above.

Sonic was measured against go-json and dropped. Marshal and unmarshal of the 200-product menu on Apple M2 Pro, Go 1.27.2, median of 3, `-cpu 1`:

| engine | marshal | unmarshal | unmarshal allocs |
|---|---:|---:|---:|
| encoding/json v1 (on v2 engine in 1.27) | 0.568 ms | 1.053 ms | 5,410 |
| encoding/json/v2 | 0.566 ms | 0.797 ms | 5,410 |
| sonic 1.15.4 | 0.469 ms | 0.321 ms | 806 |
| go-json 0.11.2 | 0.117 ms | 0.267 ms | 606 |

Sonic's native code does run on darwin/arm64 under Go 1.27: its build tags (`arm64 && go1.20 && !go1.28`) select the JIT-backed `sonic.go`, not the `compat.go` fallback to encoding/json, and the 806 allocations against v1's 5,410 confirm it. On darwin/arm64 its encoder is barely ahead of the standard library. On linux/amd64 sonic's SIMD paths are the primary target and would be expected to do better, but that is unmeasured here. Sonic stops compiling its native path at Go 1.28.

## Opting in

```go
proto := protocodec.New()

service := lokerpc.NewService("menu", "Menus", endpoints, lokerpc.ServeCodecs(proto))
lokerpc.MountHandlers(logger, mux, service)

client := MenuClient{lokerpc.NewClient(url+"/rpc/menu", lokerpc.ClientCodecs(proto))}
```

`lokerpc.WithHTTPClient(*http.Client)` sets the HTTP client.

## Go version

The example module needs Go 1.27 (`encoding/json/v2`); the root module stays on 1.26. Run it with a 1.27 toolchain, for example `mise exec go@1.27.2 -- go test ./...`. Under 1.27 `encoding/json` v1 is implemented on the v2 engine, so the `legacy` benchmark path is faster than it is on 1.26.

## Running

```
go run .
go test ./...
go test -run '^$' -bench BenchmarkMenuNegotiationHTTP -benchtime 20x -count 5 -cpu 1 ./
```

`go run .` calls a menu service with a plain JSON client, a protobuf client, and a protobuf client against a server without codecs, logging the negotiated content types and response bytes.

## Generated code

`protocodec` encodes by reflection. For hot types, `protogen` generates reflection-free code from a Go type plus its JTD schema: `MarshalProto`, `UnmarshalProto` and `ProtoFingerprint` methods on the root structs, with unexported put/read functions for everything nested. The schema decides field numbers and wire shapes through `protocodec.Compile`, the same plan the reflection codec runs, so the bytes are identical. The generated writer fills the buffer back to front, so no length prefix is ever widened or recomputed.

- Server side: generate from the Go types and `lokerpc.TypeSchema`, as in `gen/main.go` (output in `fixtures/proto_gen.go`).
- Client side: generate from the JTD the client was generated from, not from `TypeSchema` of the client structs, since `lokerpc/codegen` emits tags `TypeSchema` does not read. `gen/main.go` does this from `fixtures/menuclient/menu.meta.json` into `fixtures/menuclient/proto_gen.go`.
- `go generate ./...` regenerates both; `gen/main_test.go` fails if the committed files are stale.

At run time `protocodec` compares the generated fingerprint (a hash of the schema structure and the defs it reaches, ignoring def names) with the negotiated schema. If the generated code is missing or was built for a different schema, the reflection path runs instead. Stale generated code costs speed, never correctness: no JSON fallback and no 412.

Limitations of the generator: map keys must be string kinds, promoted fields through embedded pointers and anonymous struct types are rejected.

## Benchmark

`BenchmarkMenuNegotiationHTTP` (`menu_bench_test.go`) sends a 200-product menu over loopback HTTP, with and without gzip, through eight paths: `legacy` (service without codecs, plain client), `negotiated-json` (codec server, plain JSON client), `negotiated-proto` (codec server, protobuf client, reflection), `generated-proto` (the same with generated code on both ends), `jsonv2`, `gojson` and `gojson-ordered` (the JSON option on both ends, no negotiation; the last uses go-json's `OptimizeFieldOrder`) and `msgpack` (negotiated MessagePack, msgp generated code on both ends). It reports `<path>-ms/op` per case, runs the paths in adjacent blocks and reverses their order each `-count` round, and logs wire sizes before timing.

Results on an Apple M2 Pro with Go 1.27.2, `-cpu 1 -count 9`, median ms per request. Bold beat legacy in at least 7 of 9 rounds. The Connect proto column is `BenchmarkMenuBridgeHTTP` from the `lokerpc/v2` PR, built with the same Go and run back to back on the same machine.

| Case | Legacy | json/v2 | go-json | go-json ordered | Negotiated proto | Generated proto | MessagePack | Connect proto (v2) |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| Server, large response | 0.597 | 0.700 | **0.256** | **0.258** | 0.670 | **0.257** | **0.209** | 0.244 |
| Server, large request | 2.334 | **1.270** | **0.601** | **0.623** | **0.767** | **0.619** | **0.522** | 0.513 |
| Server, small | 0.041 | 0.043 | 0.042 | 0.042 | 0.043 | 0.042 | 0.044 | 0.041 |
| End to end, large, gzip off | 2.258 | **1.747** | **0.730** | **0.689** | **1.312** | **0.775** | **0.615** | 0.598 |
| End to end, large, gzip on | 3.113 | **2.516** | **1.494** | **1.472** | **1.993** | **1.469** | **1.368** | 1.264 |
| End to end, small | 0.044 | 0.044 | **0.041** | **0.041** | 0.044 | 0.044 | **0.042** | 0.042 |

`negotiated-json` (plain JSON client against a server with codecs) stays within noise of legacy in every case.

Response bytes (gzipped in brackets): JSON 417,905 (5,026), protobuf 365,894 (3,772), MessagePack 391,903 (4,591).

On Go 1.26 the legacy large request took 5.1 ms and end to end 3.4 ms; Go 1.27 runs encoding/json v1 on the v2 engine, which halves the decode cost for free. go-json on both ends, with no wire change and no negotiation, lands level with generated protobuf and close to Connect. `OptimizeFieldOrder` makes no measurable difference on this fixture, which is mostly strings. MessagePack with msgp generated code is the fastest of ours and level with Connect.

A Cap'n Proto codec was also tried and removed. Unpacked and with a pooled arena it still lost to protobuf on a 200-item round trip (median about 800,000 ns/op against 638,000, and 63,240 wire bytes against 35,666), because go-capnp's schema-less API and the larger payload cost more than the reflection work it saves.

## Node.js

`node/` is the same negotiation for `@loke/http-rpc` servers and `@loke/http-rpc-client` clients, in TypeScript. It speaks the protocol above, so a Node client works against a Go server and a Go client against a Node server. It is a separate package with its own `package-lock.json`; run everything below from `node/` after `npm ci`.

- `src/server.ts` `negotiate(services, { codecs })` is express middleware that goes in front of the unchanged http-rpc handler: `app.use("/rpc", negotiate([service], { codecs }), express.json(), createRequestHandler([service]))`. It answers `OPTIONS`, decodes binary requests into the parsed body so http-rpc validates and dispatches as usual, and replaces `res.json` to encode successful results. Errors are still http-rpc's JSON.
- `src/client.ts` `NegotiatingClient` extends `RPCClient`. JSON calls go through the stock client. The stock client hard-codes JSON in its fetch call, so binary calls make their own request and reuse its `RpcHTTPError` and `RpcResponseError`; they do not record its Prometheus metrics.
- `src/protobuf.ts` follows the Go `protocodec` mapping and writes the same bytes. `src/msgpack.ts` wraps `@msgpack/msgpack` and carries timestamps as the MessagePack timestamp extension and `float32` properties as float32, as Go requires.
- Values are the JSON-shaped objects http-rpc already handles: timestamps are RFC 3339 strings and integers are numbers.

Differences from the Go side:

- An absent required list or map decodes as `[]` or `{}` even when its schema is nullable, since the wire cannot tell nil from empty. Other absent required properties decode as their zero value (`null` if nullable), and absent optional properties stay absent.
- Timestamps decode in UTC, and `float32` values decode as the nearest float64.
- `any` and discriminator values are carried as `JSON.stringify` output, so object key order is not sorted as Go's is.
- Go carries `netip.Addr` over MessagePack as raw bytes, but its schema says string, so Node receives a `Uint8Array` and a Go client's `netip.Addr` fails Node's validation. Protobuf is unaffected.
- Go's reflection MessagePack decoder returns `time.Time` in the local zone, so JSON re-encoded from it carries an offset. The instant is the same.
- A Go zero-valued `encoding.TextMarshaler` field is written as an empty field; Node omits an empty string.

Tests build the Go example with `mise exec go@1.27.2 -- go build` (set `GO="go"` to use another toolchain) and run it as a server and as a client, so they need Go 1.27 on the path:

```
npm test
```

They check that the TypeScript protobuf bytes are identical to Go's for the menu fixtures and the edge values in `interop.go`, that Node and Go clients and servers negotiate protobuf and MessagePack with each other and return the same values as JSON, and the fallbacks: old server, changed schema (412), mixed fleet (non-JSON 400 or 415), unreachable server, and errors staying JSON.

`go run . serve [-plain]`, `dump` and `call` in `interop.go` are what the tests drive.

The benchmark mirrors the `lokerpc/v2` PR's TypeScript benchmark of http-rpc against Connect: the same 200-product, 600-choice menu (about 424 KB as JSON), cases, rounds, gzip handling and JSON output (a summary table goes to stderr). Its paths are `legacy` (plain http-rpc and `RPCClient`), `negotiated-json` (negotiating server, plain client), `negotiated-proto` and `msgpack`.

```
mise exec node@26 -- node --import tsx bench/menu.ts > node.json
bun bench/menu.ts > bun.json
BENCH_WARMUP=2 BENCH_ITERATIONS=3 BENCH_ROUNDS=2 bun bench/menu.ts   # smoke run
```

Run with Node v26.12.1 and Bun 1.4.2. `BENCH_WARMUP`, `BENCH_ITERATIONS` and `BENCH_ROUNDS` default to 30, 100 and 9.

Results on an Apple M2 Pro, default knobs, median ms per request. The Connect columns are the `lokerpc/v2` PR's TypeScript benchmark run back to back on the same machine; its legacy numbers match ours within noise.

| Case | Node legacy | Node negotiated proto | Node MessagePack | Node Connect proto | Bun legacy | Bun negotiated proto | Bun MessagePack | Bun Connect proto |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| Server, large response | 1.035 | 1.184 | 2.368 | 3.650 | 0.718 | 1.005 | 1.429 | 2.600 |
| Server, large request | 1.015 | 1.298 | 1.351 | 2.195 | 0.894 | 0.860 | 1.535 | 2.200 |
| Server, small | 0.110 | 0.121 | 0.118 | 0.134 | 0.102 | 0.103 | 0.088 | 0.129 |
| End to end, large, gzip off | 1.846 | 2.381 | 3.319 | 5.851 | 1.155 | 1.555 | 2.594 | 4.835 |
| End to end, large, gzip on | 3.193 | 3.305 | 4.569 | 6.766 | 1.406 | 1.728 | 2.800 | 5.848 |
| End to end, small | 0.198 | 0.199 | 0.200 | 0.186 | 0.118 | 0.112 | 0.112 | 0.156 |

Response bytes: JSON 423,734, protobuf 300,304, MessagePack 383,368.

Native `JSON.parse` and `JSON.stringify` beat every binary codec in JS, so JS services and clients should stay on JSON. The JTD-driven protobuf codec is still 1.7 to 3x faster than Connect's generated protobuf on large messages, and negotiation means a Go client can talk protobuf to a Node server that opted in while JS callers keep JSON.

## Limitations

- Decoding a binary response into an `any` result is an error. Use a concrete result type.
- Protobuf collapses nil and empty slices and maps to nil.
- Times decode in UTC. The zero time is omitted.
- A Go `int` outside int32 is an encode error.
- Map keys must be string-kind or text-marshaling Go types.
- `any` and discriminator fields are carried as JSON bytes inside the binary message.
- Field lookup follows encoding/json rules, which differs slightly from the schema generator for ambiguous embedded fields.
- A non-struct type with `MarshalJSON` is encoded by its schema kind, not its marshaler.
- Compiled plans are cached per schema and never evicted.
- The first probe holds a lock, so concurrent first callers wait for one probe.
