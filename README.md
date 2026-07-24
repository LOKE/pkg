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

## Licence

[MIT](./LICENCE) © LOKE
