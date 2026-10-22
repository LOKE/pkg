# Go pkgs for LOKE services

A collection of small, general-purpose Go packages shared across LOKE services.

[![Go Reference](https://pkg.go.dev/badge/github.com/LOKE/pkg.svg)](https://pkg.go.dev/github.com/LOKE/pkg)

See the [package documentation on pkg.go.dev](https://pkg.go.dev/github.com/LOKE/pkg)
for the full list of packages and what each one is for.

These packages are open source under the [MIT licence](./LICENCE), so anyone is
welcome to use them in their own projects — you don't need to be at LOKE. The
guidance below is aimed at LOKE engineers contributing to this repo.

## ⚠️ This is a public repository

**This repository is public. Anything committed here — code, comments, commit
messages, tests, examples, and documentation — is visible to the entire
internet.**

If you're contributing, the usual hygiene applies (no secrets, credentials, or
customer data — same as any repo). On top of that, because this one is public,
don't commit things that would be fine in a private repo but shouldn't be seen
outside LOKE:

- Internal hostnames, IP addresses, or infrastructure details.
- Proprietary or commercially sensitive business logic.
- Internal-only URLs, ticket references, or anything that reveals how our
  private systems work.

If you're ever unsure whether something is safe to make public, **don't push
it** — ask first.

## What this repo should and shouldn't contain

**Should contain:**

- Small, reusable, general-purpose Go packages that are safe to be open source.
- Utilities with no dependency on LOKE's private/internal systems.
- Code that other LOKE services can import via `github.com/LOKE/pkg/...`.

**Shouldn't contain:**

- Anything secret, sensitive, or internal-only (see the warning above).
- Business logic specific to a single service — that belongs in the service's
  own (private) repository.
- Application code, config for private infrastructure, or anything that only
  makes sense inside LOKE's private network.

If a package would only ever be useful inside LOKE and can't safely be public,
it belongs in a private repo instead.

## Migrating a service to `lokerpc/v2`

For existing services, use paired mode: retain the v1 service unchanged and add
a separate proto-generated Connect implementation calling the same domain service.
Replace the v1 mount call with one v2 mount call:

```go
import (
    "net/http"
    "connectrpc.com/connect"
    rpcv2 "github.com/LOKE/pkg/lokerpc/v2"
)

err := rpcv2.MountHandlers(logger, mux, []rpcv2.Service{{
    Name: "menu",
    Handler: func(options ...connect.HandlerOption) (string, http.Handler) {
        return examplev1connect.NewMenuServiceHandler(menu, options...)
    },
    NoBridge: true,
}}, existingV1Service)
if err != nil {
    return err
}
```

Here `menu` implements your generated handler interface and `existingV1Service`
is the original `*lokerpc.Service` named `"menu"`. `NoBridge: true` leaves all
`POST /rpc/menu/<method>` routes, validation, responses and errors with that v1
service. Connect clients use the generated service path. `GET /rpc` and
`GET /rpc/menu` retain the original legacy metadata and add only `proto` and
`protoService`, so legacy client generation remains unchanged while proto-aware
clients can generate Connect clients.

Exactly one v1 service and one `NoBridge` proto service may share a name; all
other duplicate names are mount errors. A `NoBridge` service without a v1 pair
is also allowed: its metadata comes from the proto and it has no legacy method
routes. Keep other unchanged v1 services in the same mount call. The required
`Name` must match `^[A-Za-z0-9][A-Za-z0-9_.-]*$`. The generated Connect path resolves
its registered service descriptor automatically. Do not also call the v1
`MountHandlers` on this mux: v2 owns the merged `/rpc` metadata.

For new services without legacy callers, omit `NoBridge` to enable the bridge.
Unary methods then serve `POST /rpc/<name>/<lowerCamelMethod>` in-process through
the Connect handler, preserving its interceptors and options. Metadata is derived
from the proto, not a historical v1 schema; streaming methods remain Connect-only.
The bridge is not a drop-in replacement for an existing service's wire contract.

The bridge accepts identity and gzip request encoding; other encodings return
415 `{"message":"Unsupported Content-Encoding"}`. Custom compression remains
available on the native Connect routes.

The bridge uses a dedicated `loke-legacy-json` codec to write default-valued
protobuf JSON directly into Connect's reusable response buffer, avoiding a
dynamic-message decode and second encode. Field names are cached per descriptor;
maps remain sorted and protobuf JSON scalar formatting is preserved. Well-known
types and proto2 messages use `protojson` for their specialized semantics. Requests
retain `protojson` validation and unknown-field handling, with a fast path for
empty ordinary messages. Native Connect JSON, handler options and compression
behavior are unchanged; the bridge codec is not exposed on native Connect routes.
As in the TypeScript bridge, send-size limits apply to the default-filled JSON
bytes actually returned by the bridge, rather than the smaller native Connect
JSON representation.

`MountHandlers` builds each proto handler with an automatic standard error-detail
interceptor; prepend the supplied options before any service-specific handler options.
The handler factory is called separately for native and bridge codecs. Register
collectors and construct stateful interceptors outside the factory so both routes
share the same instances.
Errors implementing `ErrorCode() string`, including wrapped causes, receive one
`loke.rpc.v1.Error` detail. Existing details and Connect status codes are preserved;
ordinary coded errors use Connect's `unknown` code. Optional `Public() bool`,
`ErrorID() string`, `ErrorType() string`, `ErrorNamespace() string`, `ErrorHelp() string`,
and `ErrorMeta() map[string]any` populate the remaining fields from the same coded
cause. JSON-compatible typed collections and structs are normalized; unencodable
metadata values are omitted without changing the error's status or identity.
Reserved error fields (`message`, `instance`, `expose`, `code`, `namespace`, `type`,
`help`, `name`, `stack`) are excluded from newly extracted metadata and stripped
when rendering bridge metadata, including existing details. Metadata cannot override
the error's identity. Errors without a machine code receive no detail.

Clients can call `detail, ok := rpcv1.FromError(err)` by importing only
`github.com/LOKE/pkg/lokerpc/v2/rpcv1`. This dependency-light package does not import
the RPC server or register its metrics. The bridge renders errors
with this detail as HTTP 400 in canonical BaseError order: `message`, `instance`,
`expose` (only when true), `code`, `namespace` (only when nonempty), `type` (falling
back to code), then flattened metadata. Named metadata keys are sorted in JavaScript
UTF-16 order; integer-index keys come first in numeric order at every object level.
`help` is omitted. Without a detail, existing Connect-to-legacy error mapping applies.
Paired v1 routes are unchanged.

The canonical schema is `lokerpc/v2/proto/loke/rpc/v1/error.proto`. Regenerate its Go
types with `go generate ./lokerpc/v2` (requires `protoc`); the checked-in plugin wrapper
uses `go run google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11`.

## Licence

[MIT](./LICENCE) © LOKE
