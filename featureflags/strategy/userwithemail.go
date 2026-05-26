package strategy

import "github.com/Unleash/unleash-go-sdk/v5/context"

type UserWithEmail struct{}

func (UserWithEmail) Name() string {
	return "userWithEmail"
}

func (UserWithEmail) IsEnabled(params map[string]any, ctx *context.Context) bool {
	if ctx == nil {
		return false
	}
	return contextHasID(ctx.Properties, "email", stringParam(params, "emails"))
}
