// Package unleash provides a featureflags.Provider backed by an Unleash client
// with LOKE's custom activation strategies registered.
package unleash

import (
	"context"
	"fmt"

	"github.com/LOKE/pkg/featureflags"
	unleashsdk "github.com/Unleash/unleash-go-sdk/v5"
	unleashcontext "github.com/Unleash/unleash-go-sdk/v5/context"
)

// NewProvider creates an unleash client with all strategies registered, blocks until ready
// (or ctx is cancelled), and returns the provider and a close function.
func NewProvider(ctx context.Context, appName, url string) (featureflags.Provider, func() error, error) {
	client, err := unleashsdk.NewClient(
		unleashsdk.WithAppName(appName),
		unleashsdk.WithUrl(url),
		unleashsdk.WithStrategies(
			BlacklistOrgWithID{},
			ClientWithID{},
			ClientWithIDPrefix{},
			Currency{},
			DeviceID{},
			LocationWithID{},
			OrgWithID{},
			Timestamp{},
			UserWithEmail{},
		),
	)
	if err != nil {
		return nil, nil, err
	}
	select {
	case <-client.Ready():
	case <-ctx.Done():
		_ = client.Close()
		return nil, nil, fmt.Errorf("waiting for unleash: %w", ctx.Err())
	}
	return AsProvider(client), client.Close, nil
}

// AsProvider adapts an existing unleash client. The client must have the
// strategies its flags use registered.
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
