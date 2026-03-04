package requestid

import (
	"context"
	"net/http"
)

// ctxKey is an unexported type for context keys defined in this package.
// This prevents collisions with keys defined in other packages.
type ctxKey int

// requestidKey is the key for requestid.RequestID values in Contexts. It is
// unexported; clients use requestid.NewContext and requestid.FromContext
// instead of using this key directly.
var requestidKey ctxKey

// NewContext returns a new Context that carries id.
func NewContext(ctx context.Context, id RequestID) context.Context {
	return context.WithValue(ctx, requestidKey, id)
}

// FromContext returns the RequestID value stored in ctx, if any.
func FromContext(ctx context.Context) (RequestID, bool) {
	u, ok := ctx.Value(requestidKey).(RequestID)
	return u, ok
}

// NewContextFromRequest returns a new context that carries a RequestID. This ID
// is either from X-Request-ID, or newly generated
func NewContextFromRequest(r *http.Request) context.Context {
	ctx := r.Context()
	if id := r.Header.Get("X-Request-ID"); id != "" {
		return NewContext(ctx, RequestID{id})
	}

	return NewContext(ctx, NewRequestID())
}
