package pgxctx

import (
	"context"
	"time"
)

const ctxDataKey ctxKey = "data"

// WithContext returns a new context with the given name and options. It should
// be passed directly to Exec, Query, or QueryRow. Queries executed with this
// context will be logged with the given name on metrics and slow query logs.
func WithContext(ctx context.Context, name string, opts ...ContextOption) context.Context {
	data := ctxData{name: name}
	for _, opt := range opts {
		opt(&data)
	}

	return context.WithValue(ctx, ctxDataKey, data)
}

type ContextOption func(*ctxData)

type ctxData struct {
	name               string
	slowQueryThreshold time.Duration
}

// WithSlowQueryThreshold sets the slow query threshold for the context.
// Queries that take longer than this threshold will be logged as slow queries.
func WithSlowQueryThreshold(threshold time.Duration) ContextOption {
	return func(data *ctxData) {
		data.slowQueryThreshold = threshold
	}
}
