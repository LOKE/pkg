# lint — LOKE's golangci-lint plugin

`package lint` is a [golangci-lint module plugin](https://golangci-lint.run/docs/plugins/module-plugins/)
registered under the name **`loke`**. It bundles LOKE-specific analyzers so every
LOKE module can enforce the same checks from one custom golangci-lint build.

## Analyzers

| Analyzer  | Checks |
|-----------|--------|
| `enumtag` | Exported struct fields whose type implements `lokerpc.EnumProvider` (`EnumValues() []string`) must carry a `validate:"enum"` rule. This is the static replacement for the old runtime `lokerpc.AuditEnumValidatorTags`. |

## Building and running

The plugin must be compiled into a custom golangci-lint binary:

```sh
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
golangci-lint custom          # reads .custom-gcl.yml, writes ./bin/golangci-lint-loke
./bin/golangci-lint-loke run  # reads .golangci.yml
```

The analyzer logic itself is exercised by ordinary `go test ./lint/...` (via
`analysistest`), so it runs in normal CI without the custom binary.

## Adding a check

1. Add `myrule.go` to this package with `var MyRuleAnalyzer = &analysis.Analyzer{...}`.
2. Append it to `Analyzers` in `loke.go`.
3. Add `analysistest` fixtures under `testdata/src/...` and a test.

Every module that enables the `loke` plugin then gets the new check for free.
