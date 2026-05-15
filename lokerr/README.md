# lokerr

Structured, RPC-serializable errors for Go services. Wire-format compatible with the [`@loke/errors`](https://github.com/LOKE/errors) Node.js package.

## Usage

```go
import "github.com/LOKE/pkg/lokerr"
```

### Creating errors

```go
// Public error — safe to display in a UI (expose: true)
err := lokerr.New("The value provided was null.", "null_value")

// Public error with formatted message
err := lokerr.Errorf("validation_failed", "field %q is required", "email")

// Internal error — logged only, not shown to users (expose: false)
err := lokerr.Wrap(dbErr, "db_query_failed")

// Internal error with a separate public message
err := lokerr.WrapPublic(dbErr, "db_query_failed", "Something went wrong.")
```

### Optional fields

```go
err := lokerr.New("Payment declined.", "payment_declined")
err.Namespace = "payments"
err.Type = "https://example.com/errors/payments/payment_declined"
```

### Checking errors

```go
// Type-assert from any error in the chain
if lErr, ok := lokerr.As(err); ok {
    fmt.Println(lErr.ErrorCode())
}

// Check if safe to show to end users
if lokerr.IsPublic(err) {
    respondWithMessage(w, err.Error())
}

// Standard Go error chain
errors.Is(err, sentinel)
errors.As(err, &target)
```

## Wire format

Errors serialize to JSON compatible with `@loke/errors`:

```json
{
  "message": "Payment declined.",
  "code": "payment_declined",
  "expose": true,
  "namespace": "payments",
  "type": "https://example.com/errors/payments/payment_declined"
}
```

- `expose` is omitted when `false`
- `code`, `namespace`, `type` are omitted when empty

## Public vs private errors

| Constructor | `expose` | Use case |
|---|---|---|
| `New` / `Errorf` | `true` | User-facing validation errors, business rule violations |
| `Wrap` | `false` | Internal failures (DB errors, network errors) — logged, not shown to users |
| `WrapPublic` | `true` | Internal failure with a safe message to show the user |

When a `lokerr.Error` reaches `lokerpc`, it is serialized in full (all fields) at HTTP 400. Plain Go errors produce `{"message":"..."}` for backward compatibility.

## Reusable error types

For errors that recur across a service, define constructor functions that pre-set the stable fields (`code`, `namespace`, `type`, `expose`) and accept only what varies at call time.

```go
const typePrefix = "https://example.com/errors/payments/"

func ErrPaymentDeclined() *lokerr.Error {
    e := lokerr.New("Your payment was declined.", "payment_declined")
    e.Namespace = "payments"
    e.Type = typePrefix + "payment_declined"
    return e
}

func ErrDBQuery(err error) *lokerr.Error {
    return lokerr.WrapPublic(err, "db_query_failed", "Something went wrong.")
}
```

### Matching reusable error types

Match on `ErrorCode()` or wrap a package-level sentinel for `errors.Is` support:

```go
// Match by code
if lErr, ok := lokerr.As(err); ok {
    switch lErr.ErrorCode() {
    case "payment_declined":
        // handle
    }
}

// Or wrap a sentinel so errors.Is works
var ErrPaymentDeclined = errors.New("payment declined")

func NewPaymentDeclinedError() *lokerr.Error {
    e := lokerr.WrapPublic(ErrPaymentDeclined, "payment_declined", "Your payment was declined.")
    e.Namespace = "payments"
    return e
}

if errors.Is(err, ErrPaymentDeclined) { ... }
```

Note: the wrapped sentinel is never serialized — it is only present server-side before the error crosses the wire.

## Extending the error type

`lokerr.Error` is intentionally minimal. If a specific situation calls for extra fields (correlation IDs, structured metadata), embed it in your own type — the standard JSON encoder will merge the fields automatically.

### Adding a correlation ID

```go
type TracedError struct {
    *lokerr.Error
    RequestID string `json:"requestId,omitempty"`
}

// Wire format: {"message":"...","code":"...","requestId":"abc123"}
func NewTracedError(msg, code, requestID string) *TracedError {
    return &TracedError{
        Error:     lokerr.New(msg, code),
        RequestID: requestID,
    }
}
```

### Adding typed metadata

Rather than a generic map, define explicit fields so the wire format stays self-documenting:

```go
type ValidationError struct {
    *lokerr.Error
    Field  string `json:"field"`
    Reason string `json:"reason,omitempty"`
}

// Wire format: {"message":"...","code":"validation_failed","field":"email","reason":"invalid format"}
func NewValidationError(field, reason string) *ValidationError {
    return &ValidationError{
        Error:  lokerr.New("Validation failed.", "validation_failed"),
        Field:  field,
        Reason: reason,
    }
}
```

`lokerr.As` still works on embedded types since `*lokerr.Error` is in the chain:

```go
if lErr, ok := lokerr.As(err); ok {
    // lErr is the embedded *lokerr.Error
}
```
