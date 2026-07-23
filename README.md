# Go pkgs for LOKE services

A collection of small, general-purpose Go packages shared across LOKE services.

Current packages:

- **`errors`** — error handling helpers.
- **`log`** — logging helpers (e.g. systemd integration).
- **`lokerpc`** — LOKE's JSON-RPC server/client, schema, and code generation.
- **`requestid`** — request ID generation and context propagation.

## ⚠️ This is a public repository

**This repository is public. Anything you commit here — code, comments, commit
messages, tests, examples, and documentation — is visible to the entire
internet.**

Before you push, please make sure you are **not** committing:

- Secrets or credentials of any kind — API keys, tokens, passwords, private
  keys, connection strings, `.env` files.
- Internal hostnames, IP addresses, or infrastructure details.
- Customer or personal data.
- Proprietary or commercially sensitive business logic.
- Internal-only URLs, ticket references, or anything that would leak how our
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
