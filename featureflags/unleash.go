package featureflags

import (
	"context"

	sdk "github.com/Unleash/unleash-go-sdk/v5"
	unleashcontext "github.com/Unleash/unleash-go-sdk/v5/context"
	sdkstrategy "github.com/Unleash/unleash-go-sdk/v5/strategy"
)

type Context = unleashcontext.Context

type Client interface {
	IsEnabled(name string, ctx Context) bool
	Close() error
	WaitForReady(ctx context.Context) error
}

type client struct {
	client *sdk.Client
}

func NewClient(appName string, unleashURL string, strategies ...sdkstrategy.Strategy) (Client, error) {
	if unleashURL == "" {
		return DisabledClient{}, nil
	}

	c, err := sdk.NewClient(
		sdk.WithAppName(appName),
		sdk.WithUrl(unleashURL),
		sdk.WithStrategies(
			strategies...,
		),
	)
	if err != nil {
		return nil, err
	}

	return client{client: c}, nil
}

func (c client) IsEnabled(name string, ctx Context) bool {
	return c.client.IsEnabled(name, sdk.WithContext(ctx))
}

func (c client) Close() error {
	return c.client.Close()
}

func (c client) WaitForReady(ctx context.Context) error {
	c.client.WaitForReady()
	return nil
}

type DisabledClient struct{}

func (DisabledClient) IsEnabled(string, Context) bool { return false }

func (DisabledClient) Close() error { return nil }

func (DisabledClient) WaitForReady(context.Context) error { return nil }
