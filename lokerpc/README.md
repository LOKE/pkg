# lokerpc

HTTP JSON-RPC framework for LOKE services. Endpoints are type-safe Go generics, schemas are generated from reflection, and TypeScript/Go clients are generated from the schema.

## Wire format

```
POST /rpc/<service>/<method>   — invoke a method (JSON body in, JSON body out)
GET  /rpc/<service>            — JTD schema for that service
GET  /rpc                      — JTD schema for all mounted services
```

Errors are returned as HTTP 400 with a JSON body. 200 means success.

---

## Server

### Defining a service

```go
svc, err := lokerpc.NewService("payments", "Payment processing", lokerpc.EndpointCodecMap{
    "createPayment": lokerpc.MakeStandardEndpointCodec(
        s.CreatePayment,
        "Create a new payment",
        lokerpc.NoNilResponse(),
    ),
    "cancelPayment": lokerpc.MakeVoidEndpointCodec(
        s.CancelPayment,
        "Cancel a payment",
    ),
})
```

- `MakeStandardEndpointCodec[Req, Res]` — for methods that return a value
- `MakeVoidEndpointCodec[Req]` — for methods that only return an error
- `NoNilResponse()` — returns a 500 if the handler returns a nil response without an error

### Mounting

```go
lokerpc.MountHandlers(logger, mux, svc1, svc2, svc3)
```

This mounts all endpoints and the schema endpoints. Pass any `http.ServeMux` or compatible router.

### Handler signature

```go
// Standard: returns a value
func (s *Service) CreatePayment(ctx context.Context, req CreatePaymentRequest) (*Payment, error)

// Void: no return value
func (s *Service) CancelPayment(ctx context.Context, req CancelPaymentRequest) error
```

---

## Client

```go
client := lokerpc.NewClient("http://payments-service/rpc/payments")

var result Payment
err := client.DoRequest(ctx, "createPayment", req, &result)

// For void methods, pass nil as result
err = client.DoRequest(ctx, "cancelPayment", req, nil)
```

Pass `X-Request-ID` and `X-Request-Deadline` headers are forwarded automatically from the context when present.

---

## Schema generation

Schemas are generated automatically from Go types during `MountHandlers`. No manual schema writing required.

### Type mapping

| Go type | JTD schema |
|---|---|
| `string` | `{type: "string"}` |
| `int`, `int32` | `{type: "int32"}` |
| `float64` | `{type: "float64"}` |
| `bool` | `{type: "boolean"}` |
| `time.Time` | `{type: "timestamp"}` |
| `*T` | `{...T, nullable: true}` |
| `[]T` | `{elements: T, nullable: true}` |
| `map[string]T` | `{values: T, nullable: true}` |
| named struct | definition + `{ref: "Name"}` |
| `MarshalText()` | `{type: "string"}` |
| `MarshalJSON()` | `{}` (any) |
| named string implementing `EnumProvider` | enum definition + `{ref: "Name"}` |

### Enums

Declare a named string type's values as `const`s and have the type implement `EnumProvider` (`EnumValues() []string`, **value receiver**). Schema generation resolves the values by reflection, and every field of that type (requests and responses alike) references the same enum definition with no per-field tag:

```go
type Currency string

const (
    CurrencyAUD Currency = "AUD"
    CurrencyNZD Currency = "NZD"
    CurrencyUSD Currency = "USD"
)

// lokerpc.Enum keeps the returned values in sync with the declared consts.
func (Currency) EnumValues() []string {
    return lokerpc.Enum(CurrencyAUD, CurrencyNZD, CurrencyUSD)
}

type CreatePaymentRequest struct {
    Currency Currency `json:"currency" validate:"enum"`
    Amount   int32    `json:"amount" validate:"required,gt=0"`
}
```

Use a value receiver: schema generation relies on `reflect.Zero(t).Interface().(EnumProvider)`, which a pointer receiver fails silently.

This produces:

```json
{
  "definitions": {
    "Currency": { "enum": ["AUD", "NZD", "USD"] }
  },
  "properties": {
    "currency": { "ref": "Currency" },
    "amount":   { "type": "int32" }
  }
}
```

To enforce the values on incoming requests, register a custom `enum` validator that looks the field's type up with `lokerpc.EnumValuesFor(field.Type())`, and tag the field with `validate:"enum"`. The [`loke` golangci-lint plugin](../lint) (`enumtag` analyzer) statically checks that every enum-typed struct field carries that rule.

---

## Code generation

The `lokerpc/codegen` subpackage generates typed clients from a service's schema JSON.

### TypeScript

```go
import "github.com/LOKE/pkg/lokerpc/codegen"

var meta lokerpc.Meta
// ... decode schema JSON into meta

err := codegen.GenTypescriptClient(w, meta)
```

Output:

```typescript
export type Currency = "AUD" | "NZD" | "USD";

export type CreatePaymentRequest = {
  currency: Currency;
  amount: number;
};

export class PaymentsService extends RPCContextClient {
  createPayment(ctx: Context, req: CreatePaymentRequest): Promise<Payment> {
    return this.request(ctx, "createPayment", req);
  }
}
```

### Go

```go
err := codegen.GenGoClient(w, meta)
```

Output:

```go
type Currency string

const (
    CurrencyAUD Currency = "AUD"
    CurrencyNZD Currency = "NZD"
    CurrencyUSD Currency = "USD"
)

type PaymentsService interface {
    CreatePayment(context.Context, CreatePaymentRequest) (*Payment, error)
}

type PaymentsRPCClient struct{ lokerpc.Client }

func (c PaymentsRPCClient) CreatePayment(ctx context.Context, req CreatePaymentRequest) (*Payment, error) {
    var res Payment
    err := c.DoRequest(ctx, "createPayment", req, &res)
    if err != nil {
        return nil, err
    }
    return &res, nil
}
```

---

## Error handling

Return any `error` from a handler and it is serialised as a 400 response:

```json
{ "message": "something went wrong" }
```

Use `github.com/LOKE/pkg/errors` to expose structured errors to callers:

```go
// Public: exposes code, type, and message to the caller
return nil, errors.NewPublicError("payment_declined", "Card was declined")

// Private: only "message" is sent (no code/type)
return nil, fmt.Errorf("db query failed: %w", err)
```

Public error response:

```json
{ "message": "Card was declined", "expose": true, "code": "payment_declined", "type": "payment_declined" }
```

Upstream RPC errors (received from another lokerpc client) are passed through to the caller as-is, preserving `instance`, `namespace`, and `type`.

---

## Advanced: low-level schema API

If you need to generate schemas outside of `MountHandlers`:

```go
tdefs := map[reflect.Type]*lokerpc.NamedSchema{}

// Build schema — EnumProvider types are detected automatically by reflection
schema := lokerpc.TypeSchema(reflect.TypeOf(MyRequest{}), tdefs)

// Resolve definitions
defs := lokerpc.TypeDefs(tdefs)
```
