package strategy

import "github.com/Unleash/unleash-go-sdk/v5/context"

type LocationWithID struct{}

func (LocationWithID) Name() string {
	return "locationWithId"
}

func (LocationWithID) IsEnabled(params map[string]any, ctx *context.Context) bool {
	if ctx == nil {
		return false
	}
	return contextHasID(ctx.Properties, "locationId", stringParam(params, "locationIds"))
}
