# Go pkgs for LOKE services

A collection of small, general-purpose Go packages shared across LOKE services.

Current packages:

- **`errors`** — helpers for inspecting errors, including whether an error is safe to expose publicly (`IsPublic`) and reading a machine-readable `ErrorCode` off an error value.
- **`log`** — a systemd/syslog logger that prefixes go-kit log lines with the appropriate syslog priority level, so log levels are picked up correctly by the journal.
- **`lokerpc`** — LOKE's JSON-RPC HTTP server and client, including request schemas, Prometheus request metrics, and code generation for typed clients.
- **`requestid`** — request ID generation and propagation, carrying an ID (from the incoming `X-Request-ID` header or newly generated) through `context.Context`.

## ⚠️ This is a public repository

**This repository is public. Anything you commit here — code, comments, commit
messages, tests, examples, and documentation — is visible to the entire
internet.**

The usual hygiene applies (no secrets, credentials, or customer data — same as
any repo). On top of that, because this one is public, don't commit things that
would be fine in a private repo but shouldn't be seen outside LOKE:

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

## Licence

[MIT](./LICENCE) © LOKE
