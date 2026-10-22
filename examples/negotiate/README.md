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

`BenchmarkMenuNegotiationHTTP` (`menu_bench_test.go`) sends a 200-product menu over loopback HTTP, with and without gzip, through seven paths: `legacy` (service without codecs, plain client), `negotiated-json` (codec server, plain JSON client), `negotiated-proto` (codec server, protobuf client, reflection), `generated-proto` (the same with generated code on both ends), `jsonv2` and `gojson` (the JSON option on both ends, no negotiation) and `msgpack` (negotiated MessagePack, msgp generated code on both ends). It reports `<path>-ms/op` per case, runs the paths in adjacent blocks and reverses their order each `-count` round, and logs wire sizes before timing.

Results on an Apple M2 Pro with Go 1.27.2, `-cpu 1 -count 9`, median ms per request. Bold beat legacy in at least 7 of 9 rounds. The Connect proto column is `BenchmarkMenuBridgeHTTP` from the `lokerpc/v2` PR, built with the same Go and run back to back on the same machine.

| Case | Legacy | Negotiated JSON | json/v2 | go-json | Negotiated proto | Generated proto | MessagePack | Connect proto (v2) |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| Server, large response | 0.650 | 0.632 | 0.754 | **0.269** | 0.686 | **0.284** | **0.227** | 0.257 |
| Server, large request | 2.367 | 2.360 | **1.175** | **0.588** | **0.733** | **0.600** | **0.519** | 0.608 |
| Server, small | 0.041 | 0.043 | 0.041 | 0.040 | 0.043 | 0.042 | 0.041 | 0.047 |
| End to end, large, gzip off | 2.254 | **2.162** | **1.699** | **0.691** | **1.222** | **0.803** | **0.581** | 0.660 |
| End to end, large, gzip on | 2.987 | 3.016 | **2.468** | **1.459** | **1.859** | **1.450** | **1.331** | 1.355 |
| End to end, small | 0.052 | **0.051** | 0.050 | **0.045** | 0.047 | 0.047 | **0.047** | 0.045 |

Response bytes (gzipped in brackets): JSON 417,905 (5,026), protobuf 365,894 (3,772), MessagePack 391,903 (4,591).

On Go 1.26 the legacy large request took 5.1 ms and end to end 3.4 ms; Go 1.27 runs encoding/json v1 on the v2 engine, which halves the decode cost for free. go-json on both ends, with no wire change and no negotiation, lands level with generated protobuf and Connect. MessagePack with msgp generated code is the fastest path in every large case.

A Cap'n Proto codec was also tried and removed. Unpacked and with a pooled arena it still lost to protobuf on a 200-item round trip (median about 800,000 ns/op against 638,000, and 63,240 wire bytes against 35,666), because go-capnp's schema-less API and the larger payload cost more than the reflection work it saves.

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
