// Package unleash adapts an Unleash client to featureflags.Provider and
// provides LOKE's custom activation strategies.
package unleash

import (
	"github.com/LOKE/pkg/featureflags"
	unleashsdk "github.com/Unleash/unleash-go-sdk/v5"
	unleashcontext "github.com/Unleash/unleash-go-sdk/v5/context"
	"github.com/Unleash/unleash-go-sdk/v5/strategy"
)

// Strategies returns every custom strategy in this package. Register them all
// with unleashsdk.WithStrategies: a flag using an unregistered strategy is
// silently disabled.
func Strategies() []strategy.Strategy {
	return []strategy.Strategy{
		BlacklistOrgWithID{},
		ClientWithID{},
		ClientWithIDPrefix{},
		Currency{},
		DeviceID{},
		LocationWithID{},
		OrgWithID{},
		Timestamp{},
		UserWithEmail{},
	}
}

// AsProvider adapts client, which should be created with Strategies registered.
func AsProvider(client *unleashsdk.Client) featureflags.Provider {
	return resolver{client}
}

type resolver struct{ client *unleashsdk.Client }

func (r resolver) IsEnabled(flag string, ctx featureflags.Context) bool {
	return r.client.IsEnabled(flag, unleashsdk.WithContext(convertContext(ctx)))
}

func convertContext(ctx featureflags.Context) unleashcontext.Context {
	return unleashcontext.Context{
		UserId:        ctx.UserID,
		RemoteAddress: ctx.RemoteAddress,
		Properties: map[string]string{
			"orgId":      ctx.OrgID,
			"email":      ctx.Email,
			"deviceId":   ctx.DeviceID,
			"locationId": ctx.LocationID,
			"clientId":   ctx.ClientID,
			"currency":   ctx.Currency,
		},
	}
}
