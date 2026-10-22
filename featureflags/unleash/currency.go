package unleash

import "github.com/Unleash/unleash-go-sdk/v5/context"

type Currency struct{}

func (Currency) Name() string {
	return "currency"
}

func (Currency) IsEnabled(params map[string]any, ctx *context.Context) bool {
	if ctx == nil {
		return false
	}
	return contextHasID(ctx.Properties, "currency", stringParam(params, "currencies"))
}
