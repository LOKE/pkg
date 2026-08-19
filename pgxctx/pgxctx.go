package pgxctx

import (
	"context"
	"time"
)

const ctxDataKey ctxKey = "data"

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

func WithSlowQueryThreshold(threshold time.Duration) ContextOption {
	return func(data *ctxData) {
		data.slowQueryThreshold = threshold
	}
}
