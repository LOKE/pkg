# lokerpc

`lokerpc` exposes typed Go services over JSON RPC and publishes JSON Type
Definition (JTD) metadata for client generation.

It can generate RPC help, type and field descriptions, and string enums from
Go source. The generated metadata is ordinary compact Go data: source files are
not embedded in the service binary.

## Define a service

Document the service interface and its methods with normal Go doc comments.
Named string constants with supported local expressions, reachable from request
and response types, are exposed as JTD enums.

```go
package widgets

import "context"

//go:generate go run github.com/LOKE/pkg/lokerpc/cmd/lokerpcgen -service Service

// State is the lifecycle state of a widget.
type State string

const (
	StateActive   State = "active"
	StateArchived State = "archived"
)

// CreateWidgetRequest contains the data required to create a widget.
type CreateWidgetRequest struct {
	// Name is shown to users.
	Name string `json:"name"`
}

type DeleteWidgetRequest struct {
	ID string `json:"id"`
}

type Widget struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State State  `json:"state"`
}

// Service creates and manages widgets.
type Service interface {
	// CreateWidget creates a widget.
	CreateWidget(context.Context, CreateWidgetRequest) (*Widget, error)

	// DeleteWidget permanently deletes a widget.
	DeleteWidget(context.Context, DeleteWidgetRequest) error
}
```

Run the generator and commit its output:

```sh
go generate ./path/to/widgets
```

By default it writes `lokerpc_metadata.gen.go` beside the source files. Service
startup intentionally fails with an actionable error when generated metadata is
missing, so a forgotten generation step is caught early.

Use one generator invocation per package. Each generated file registers one
metadata bundle for its import path, so multiple generated files for the same
package would be a configuration error.

## Register endpoints

Pass `GeneratedDocs[Service]()` as the second argument to `NewService`. Generated
endpoint constructors do not require empty help placeholders.

```go
func NewRPCService(service Service) *lokerpc.Service {
	return lokerpc.NewService(
		"widgets",
		lokerpc.GeneratedDocs[Service](),
		lokerpc.EndpointCodecMap{
			"createWidget": lokerpc.MakeGeneratedStandardEndpointCodec(
				service.CreateWidget,
				lokerpc.NoNilResponse(),
			),
			"deleteWidget": lokerpc.MakeGeneratedVoidEndpointCodec(
				service.DeleteWidget,
			),
		},
	)
}
```

Endpoint map keys must be the lower-camel form of their interface method names:
for example, `CreateWidget` is registered as `createWidget`. Generated help is
matched using that convention.

`NewService` is type-safe: its description argument accepts only string-like
manual help or `GeneratedServiceDocs`. Request, response, and endpoint option
types remain statically checked by the generic constructors.

Mount one or more services on an HTTP mux:

```go
mux := http.NewServeMux()
lokerpc.MountHandlers(logger, mux, NewRPCService(widgetService))
```

This exposes:

- `GET /rpc` — metadata for all mounted services
- `GET /rpc/widgets` — metadata for the widget service
- `POST /rpc/widgets/createWidget` — the `createWidget` endpoint

## Generated metadata

The generator reads:

- the selected interface's leading Go doc comment as service help;
- leading Go doc comments on interface methods as endpoint help;
- type and struct-field comments as JTD `description` metadata; and
- resolvable local constants belonging to named string types as JTD enums.

It follows request and response types declared in the service package. Repeat
`-service` within the same generator invocation to generate metadata for
multiple interfaces. Embedded local service interfaces are followed as well.

Generated metadata improves both client generators:

- TypeScript clients receive JSDoc, documented fields, and string-literal
  unions for enums;
- Go clients receive Go doc comments, named string types, and typed enum
  constants.

Run `go generate` again whenever relevant interfaces, comments, fields, or
constants change.

### Generator flags

| Flag | Purpose |
| --- | --- |
| `-service NAME` | Include a service interface and its reachable local request and response types. Repeatable. |
| `-type NAME` | Include a local root type and other local types reachable from it. Repeatable. |
| `-output FILE` | Change the generated filename from `lokerpc_metadata.gen.go`. |

## Shared DTO packages

Imported types cannot be discovered solely by parsing the service package.
Register roots in the package that owns shared request or response types:

```go
//go:generate go run github.com/LOKE/pkg/lokerpc/cmd/lokerpcgen -type State -type Widget
```

`-type` is repeatable and recursively follows other local types reachable from
each root. Importing the shared package registers that metadata at startup.

## Manual descriptions

Existing services can continue supplying descriptions directly. The original
typed APIs remain supported:

```go
lokerpc.NewService(
	"widgets",
	"Widget service",
	lokerpc.EndpointCodecMap{
		"createWidget": lokerpc.MakeStandardEndpointCodec(
			service.CreateWidget,
			"Create a widget",
			lokerpc.NoNilResponse(),
		),
	},
)
```

Manual help on an endpoint is retained when a generated service interface does
not document that method, which allows incremental migration.

## Limitations

- Enum generation currently targets named string types.
- Duplicate enum values are collapsed while preserving declaration order.
- Enum values may use local string literals, identifiers, parentheses,
  conversions, and `+`. Other constant expression forms are skipped.
- Method help comes from leading comments on the selected interface, not
  implementation methods or trailing comments.
- Endpoint keys must be lower-camel versions of their interface method names.
- Request types must currently be non-pointer structs so their JSON parameter
  names can be reflected during service construction.
- Imported DTOs need metadata generation in their owning package via `-type`.
- Generated files must be refreshed and committed when their source changes.
