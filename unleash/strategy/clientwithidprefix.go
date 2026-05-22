package strategy

import (
	"strings"

	"github.com/Unleash/unleash-go-sdk/v5/context"
)

type ClientWithIDPrefix struct{}

func (ClientWithIDPrefix) Name() string {
	return "clientWithIdPrefix"
}

func (ClientWithIDPrefix) IsEnabled(params map[string]any, ctx *context.Context) bool {
	if ctx == nil {
		return false
	}
	clientID := contextProperty(ctx.Properties, "clientId")
	if clientID == "" {
		return false
	}

	for _, prefix := range IDsFromString(stringParam(params, "prefixes")) {
		if strings.HasPrefix(clientID, prefix) {
			return true
		}
	}
	return false
}
