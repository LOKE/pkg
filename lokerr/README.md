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
err.Meta = map[string]any{"attemptCount": 3}
```

### Checking errors

```go
// Type-assert from any error in the chain
if lErr, ok := lokerr.As(err); ok {
    fmt.Println(lErr.ErrorCode(), lErr.ErrorID())
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
  "instance": "a3f2c1...",
  "message": "Payment declined.",
  "code": "payment_declined",
  "expose": true,
  "namespace": "payments",
  "type": "https://example.com/errors/payments/payment_declined",
  "attemptCount": 3
}
```

- `expose` is omitted when `false`
- `code`, `namespace`, `type` are omitted when empty
- `Meta` keys are flattened to the top level (struct fields take precedence over meta keys with the same name)
- `instance` is always present — a random 32-char hex string generated at construction time

## Reusable error types

For errors that recur across a service, define constructor functions that pre-set the stable fields (`code`, `namespace`, `type`, `expose`) and accept only what varies at call time.

```go
// errors.go in your service package

const typePrefix = "https://example.com/errors/payments/"

func ErrPaymentDeclined(reason string) *lokerr.Error {
    e := lokerr.New("Your payment was declined.", "payment_declined")
    e.Namespace = "payments"
    e.Type = typePrefix + "payment_declined"
    e.Meta = map[string]any{"reason": reason}
    return e
}

func ErrInsufficientFunds(available, required int64) *lokerr.Error {
    e := lokerr.New("Insufficient funds.", "insufficient_funds")
    e.Namespace = "payments"
    e.Type = typePrefix + "insufficient_funds"
    e.Meta = map[string]any{"available": available, "required": required}
    return e
}

func ErrDBQuery(err error) *lokerr.Error {
    return lokerr.WrapPublic(err, "db_query_failed", "Something went wrong.")
}
```

### Matching reusable error types

Each `lokerr.Error` has a unique `instance`, so `errors.Is` won't match across calls. Match on `ErrorCode()` or `Type` instead:

```go
if lErr, ok := lokerr.As(err); ok {
    switch lErr.ErrorCode() {
    case "payment_declined":
        // handle declined
    case "insufficient_funds":
        // handle insufficient funds
    }
}
```

To support `errors.Is` matching, wrap a package-level sentinel:

```go
var ErrInsufficientFunds = errors.New("insufficient funds")

func NewInsufficientFundsError(available, required int64) *lokerr.Error {
    e := lokerr.WrapPublic(ErrInsufficientFunds, "insufficient_funds", "Insufficient funds.")
    e.Namespace = "payments"
    e.Type = typePrefix + "insufficient_funds"
    e.Meta = map[string]any{"available": available, "required": required}
    return e
}

// Caller can now use errors.Is:
if errors.Is(err, ErrInsufficientFunds) { ... }
```

Note: the wrapped sentinel is never serialized — it is only present on the server side before the error crosses the wire. Deserialized errors on the client side will have `Unwrap() == nil`.

## Public vs private errors

| Constructor | `expose` | Use case |
|---|---|---|
| `New` / `Errorf` | `true` | User-facing validation errors, business rule violations |
| `Wrap` | `false` | Internal failures (DB errors, network errors) — logged, not shown to users |
| `WrapPublic` | `true` | Internal failure with a safe message to show the user |

When a `lokerr.Error` reaches `lokerpc`, it is serialized in full (all fields) at HTTP 400. Plain Go errors produce `{"message":"..."}` for backward compatibility.
