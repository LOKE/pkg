package strategy

import "github.com/Unleash/unleash-go-sdk/v5/context"

type ClientWithID struct{}

func (ClientWithID) Name() string {
	return "clientWithId"
}

func (ClientWithID) IsEnabled(params map[string]any, ctx *context.Context) bool {
	if ctx == nil {
		return false
	}
	clientID := contextProperty(ctx.Properties, "clientId")
	if clientID == "" {
		return false
	}
	return containsID(stringParam(params, "clientIds"), clientID)
}
