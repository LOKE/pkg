// Package unleash provides a featureflags.Provider backed by Unleash, with
// LOKE's custom activation strategies registered.
package unleash

import (
	"context"
	"errors"
	"fmt"

	"github.com/LOKE/pkg/featureflags"
	unleashsdk "github.com/Unleash/unleash-go-sdk/v5"
	unleashcontext "github.com/Unleash/unleash-go-sdk/v5/context"
	"github.com/Unleash/unleash-go-sdk/v5/strategy"
)

// Client is a featureflags.Provider. Create it with New and Close it when done.
type Client struct{ client *unleashsdk.Client }

// New creates an Unleash client for the API at url and waits until the first
// feature fetch succeeds or ctx is done. ctx doesn't bound client creation,
// which registers the instance synchronously unless metrics are disabled.
// opts are applied after the defaults, so passing WithStrategies replaces
// LOKE's strategies.
func New(ctx context.Context, appName, url string, opts ...unleashsdk.ConfigOption) (*Client, error) {
	client, err := unleashsdk.NewClient(append([]unleashsdk.ConfigOption{
		unleashsdk.WithAppName(appName),
		unleashsdk.WithUrl(url),
		unleashsdk.WithStrategies(strategies()...),
	}, opts...)...)
	if err != nil {
		return nil, fmt.Errorf("creating unleash client: %w", err)
	}
	// Ready() is also read by the SDK's own event loop, so only WaitForReady reliably observes readiness.
	ready := make(chan struct{})
	go func() {
		client.WaitForReady()
		close(ready)
	}()
	select {
	case <-ready:
		return &Client{client}, nil
	case <-ctx.Done():
		return nil, errors.Join(fmt.Errorf("waiting for unleash: %w", ctx.Err()), client.Close())
	}
}

// IsEnabled reports whether flag is enabled for ctx.
func (c *Client) IsEnabled(flag string, ctx featureflags.Context) bool {
	return c.client.IsEnabled(flag, unleashsdk.WithContext(convertContext(ctx)))
}

// Close stops polling Unleash and sending metrics.
func (c *Client) Close() error {
	return c.client.Close()
}

// A flag using a strategy missing from this list is silently disabled.
func strategies() []strategy.Strategy {
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
