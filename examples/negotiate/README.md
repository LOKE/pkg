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

## Benchmark

`BenchmarkMenuNegotiationHTTP` (`menu_bench_test.go`) sends a 200-product menu over loopback HTTP, with and without gzip, through three paths: `legacy` (service without codecs, plain client), `negotiated-json` (codec server, plain JSON client) and `negotiated-proto` (codec server, protobuf client). It reports `<path>-ms/op` per case, runs the paths in adjacent blocks and reverses their order each `-count` round, and logs wire sizes before timing.

Results on an Apple M2 Pro, `-cpu 1 -count 9`, median ms per request. Bold beat legacy in at least 7 of 9 rounds. The Connect proto column is `BenchmarkMenuBridgeHTTP` from the `lokerpc/v2` PR run back to back on the same machine, for comparison.

| Case | Legacy | Negotiated JSON | Negotiated proto | Connect proto (v2) |
|---|---:|---:|---:|---:|
| Server, large response | 0.602 | 0.603 | 0.594 | 0.239 |
| Server, large request | 5.018 | **4.944** | **0.681** | 0.490 |
| Server, small | 0.038 | 0.038 | 0.039 | 0.039 |
| End to end, large, gzip off | 3.471 | 3.484 | **1.104** | 0.598 |
| End to end, large, gzip on | 4.322 | 4.340 | **1.925** | 1.436 |
| End to end, small | 0.044 | **0.042** | 0.043 | 0.040 |

Response bytes: legacy JSON 417,905 (4,186 gzipped), negotiated proto 365,894 (3,690 gzipped), Connect proto 365,690 (3,693 gzipped).

Decoding is where the win is. Encoding by reflection runs at about the same speed as `encoding/json`, so the server-side large response doesn't improve, while generated protobuf code is about 2.5x faster there.

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
