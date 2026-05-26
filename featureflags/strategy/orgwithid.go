package strategy

import "github.com/Unleash/unleash-go-sdk/v5/context"

type OrgWithID struct{}

func (OrgWithID) Name() string {
	return "orgWithId"
}

func (OrgWithID) IsEnabled(params map[string]any, ctx *context.Context) bool {
	if ctx == nil {
		return false
	}
	return contextHasID(ctx.Properties, "orgId", stringParam(params, "orgIds"))
}
