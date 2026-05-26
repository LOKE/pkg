package strategy

import (
	"time"

	"github.com/Unleash/unleash-go-sdk/v5/context"
)

type Timestamp struct{}

func (Timestamp) Name() string {
	return "timestamp"
}

func (Timestamp) IsEnabled(params map[string]any, _ *context.Context) bool {
	enableAfter := stringParam(params, "enableAfter")
	if enableAfter == "" {
		return false
	}

	t, err := time.Parse(time.RFC3339Nano, enableAfter)
	if err != nil {
		return false
	}
	return time.Now().After(t)
}
