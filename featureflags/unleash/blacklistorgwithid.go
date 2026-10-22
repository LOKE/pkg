package unleash

import "github.com/Unleash/unleash-go-sdk/v5/context"

type BlacklistOrgWithID struct{}

func (BlacklistOrgWithID) Name() string {
	return "blacklistOrgWithId"
}

// Inverse of OrgWithID: enabled for every org except the listed ones.
func (BlacklistOrgWithID) IsEnabled(params map[string]any, ctx *context.Context) bool {
	if ctx == nil {
		return true
	}
	return !contextHasID(ctx.Properties, "orgId", stringParam(params, "orgIds"))
}
