package strategy

import "github.com/Unleash/unleash-go-sdk/v5/context"

type DeviceID struct{}

func (DeviceID) Name() string {
	return "deviceId"
}

func (DeviceID) IsEnabled(params map[string]any, ctx *context.Context) bool {
	if ctx == nil {
		return false
	}
	deviceID := contextProperty(ctx.Properties, "deviceId")
	if deviceID == "" {
		return false
	}
	return containsID(stringParam(params, "deviceIds"), deviceID)
}
