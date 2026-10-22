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

## Opting in

```go
proto := protocodec.New()

service := lokerpc.NewService("menu", "Menus", endpoints, lokerpc.ServeCodecs(proto))
lokerpc.MountHandlers(logger, mux, service)

client := MenuClient{lokerpc.NewClient(url+"/rpc/menu", lokerpc.ClientCodecs(proto))}
```

`lokerpc.WithHTTPClient(*http.Client)` sets the HTTP client.

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

`BenchmarkMenuNegotiationHTTP` (`menu_bench_test.go`) sends a 200-product menu over loopback HTTP, with and without gzip, through four paths: `legacy` (service without codecs, plain client), `negotiated-json` (codec server, plain JSON client), `negotiated-proto` (codec server, protobuf client, reflection) and `generated-proto` (the same with generated code on both ends). It reports `<path>-ms/op` per case, runs the paths in adjacent blocks and reverses their order each `-count` round, and logs wire sizes before timing.

Results on an Apple M2 Pro, `-cpu 1 -count 9`, median ms per request. Bold beat legacy in at least 7 of 9 rounds. The Connect proto column is `BenchmarkMenuBridgeHTTP` from the `lokerpc/v2` PR run back to back on the same machine, for comparison.

| Case | Legacy | Negotiated JSON | Negotiated proto | Generated proto | Connect proto (v2) |
|---|---:|---:|---:|---:|---:|
| Server, large response | 0.605 | 0.602 | **0.584** | **0.223** | 0.243 |
| Server, large request | 5.115 | 5.149 | **0.711** | **0.559** | 0.528 |
| Server, small | 0.038 | 0.038 | 0.039 | 0.039 | 0.042 |
| End to end, large, gzip off | 3.448 | 3.532 | **1.149** | **0.627** | 0.619 |
| End to end, large, gzip on | 4.444 | 4.426 | **1.835** | **1.407** | 1.471 |
| End to end, small | 0.044 | 0.044 | 0.043 | 0.042 | 0.040 |

Response bytes: legacy JSON 417,905 (4,186 gzipped), negotiated proto 365,894 (3,690 gzipped), Connect proto 365,690 (3,693 gzipped).

With reflection, decoding is where the win is; encoding runs at about the same speed as `encoding/json`. Generated code removes that gap and lands level with Connect's generated protobuf in every case.

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
