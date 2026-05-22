package unleash

import (
	"github.com/LOKE/pkg/unleash/strategy"
	unleashsdk "github.com/Unleash/unleash-go-sdk/v5"
	unleashsdkcontext "github.com/Unleash/unleash-go-sdk/v5/context"
)

type Context = unleashsdkcontext.Context

type Client interface {
	IsEnabled(name string, ctx Context) bool
	Close() error
}

type client struct {
	client *unleashsdk.Client
}

func NewClient(appName string, unleashURL string) (Client, error) {
	if unleashURL == "" {
		return DisabledClient{}, nil
	}

	c, err := unleashsdk.NewClient(
		unleashsdk.WithAppName(appName),
		unleashsdk.WithUrl(unleashURL),
		unleashsdk.WithStrategies(
			strategy.UserWithEmail{},
			strategy.OrgWithID{},
			strategy.Timestamp{},
			strategy.DeviceID{},
			strategy.LocationWithID{},
			strategy.ClientWithID{},
			strategy.ClientWithIDPrefix{},
		),
	)
	if err != nil {
		return nil, err
	}

	return client{client: c}, nil
}

func (c client) IsEnabled(name string, ctx Context) bool {
	return c.client.IsEnabled(name, unleashsdk.WithContext(ctx))
}

func (c client) Close() error {
	return c.client.Close()
}

type DisabledClient struct{}

func (DisabledClient) IsEnabled(string, Context) bool { return false }

func (DisabledClient) Close() error { return nil }
