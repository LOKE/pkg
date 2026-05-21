# lokerr

Structured, RPC-serializable errors for Go services. Wire-format compatible with the [`@loke/errors`](https://github.com/LOKE/errors) Node.js package.

## Design

`lokerr` is interface-based. The `lokerr.Error` interface is what `lokerpc` and application code work against — any struct that implements it will be serialized in full over RPC. `BaseError` is provided as a convenience type for straightforward cases; define your own struct when you need extra fields.

## The interface

```go
type Error interface {
    error
    Public() bool      // true = safe to show in a UI; false = log only
    ErrorCode() string // machine-readable code, e.g. "validation_failed"
}
```

Any type implementing these three methods integrates with `lokerpc` automatically.

## Using BaseError

```go
import "github.com/LOKE/pkg/lokerr"

// Public error — expose: true, safe to show in a UI
err := lokerr.New("The value provided was null.", "null_value")

// Internal error — expose: false, logged but not shown to users
err := lokerr.Wrap(dbErr, "db_query_failed")

// Internal error with a separate safe public message
err := lokerr.WrapPublic(dbErr, "db_query_failed", "Something went wrong.")
```

Optional fields on `BaseError`:

```go
err := lokerr.New("Payment declined.", "payment_declined")
err.Namespace = "payments"
err.Type = "https://example.com/errors/payments/payment_declined"
```

## Custom error types

When an error needs extra fields on the wire, embed `*lokerr.BaseError` and add your own exported fields. `lokerpc` serializes the concrete type directly, so the promoted `BaseError` fields and your custom fields all appear at the top level.

```go
type ValidationError struct {
    *lokerr.BaseError
    Field string `json:"field"`
}

func NewValidationError(field string) *ValidationError {
    return &ValidationError{
        BaseError: lokerr.New("Validation failed.", "validation_failed"),
        Field:       field,
    }
}
```

Wire output:

```json
{"message":"Validation failed.","code":"validation_failed","expose":true,"field":"email"}
```

The same pattern works for any additional context — amounts, limits, resource IDs — just add exported fields with JSON tags.

## Reusable error definitions

Define constructor functions that fix the stable fields and accept only what varies per call:

```go
const typePrefix = "https://example.com/errors/payments/"

func ErrPaymentDeclined() *lokerr.BaseError {
    e := lokerr.New("Your payment was declined.", "payment_declined")
    e.Namespace = "payments"
    e.Type = typePrefix + "payment_declined"
    return e
}

func ErrDBQuery(err error) *lokerr.BaseError {
    return lokerr.WrapPublic(err, "db_query_failed", "Something went wrong.")
}
```

To support `errors.Is` matching, wrap a package-level sentinel:

```go
var ErrInsufficientFunds = errors.New("insufficient funds")

func NewInsufficientFundsError() *lokerr.BaseError {
    e := lokerr.WrapPublic(ErrInsufficientFunds, "insufficient_funds", "Insufficient funds.")
    e.Namespace = "payments"
    return e
}

// Caller:
if errors.Is(err, ErrInsufficientFunds) { ... }
```

Note: the wrapped sentinel is never serialized — it is only present server-side before the error crosses the wire.

## Helper functions

```go
// Extract the lokerr.Error interface from any error in the chain
if lErr, ok := lokerr.As(err); ok {
    fmt.Println(lErr.ErrorCode())
}

// Check expose flag without a type assertion
if lokerr.IsPublic(err) {
    respondToUser(w, err.Error())
}

// Extract code from any error (returns "" if not a lokerr.Error)
code := lokerr.ErrorCode(err)
```

## Wire format

`BaseError` serializes to JSON compatible with `@loke/errors`:

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
- Custom types serialize their own exported fields alongside or instead of these

## lokerpc integration

When a `lokerr.Error` is returned from an RPC handler, `lokerpc` serializes the concrete type in full at HTTP 400. Plain Go errors fall back to `{"message":"..."}` for backward compatibility.

On the client side, error responses are deserialized into `*lokerr.BaseError`, giving access to `Public()`, `ErrorCode()`, and `Error()`.
