package service1

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/LOKE/pkg/lokerpc"
)

type Hello1Request struct {
	Thing Hello1RequestThing `json:"thing"`
}

type Hello1Response struct {
	Value Hello1ResponseVariant
}

type Hello1RequestThingVariant interface{ isHello1RequestThingVariant() }

type Hello1RequestThingUserCreated struct {
	ID string `json:"id"`
}

func (Hello1RequestThingUserCreated) isHello1RequestThingVariant() {}

type Hello1RequestThingUserDeleted struct {
	ID         string `json:"id"`
	SoftDelete bool   `json:"softDelete"`
}

func (Hello1RequestThingUserDeleted) isHello1RequestThingVariant() {}

type Hello1RequestThingUserPaymentPlanChanged struct {
	ID   string `json:"id"`
	Plan string `json:"plan"`
}

func (Hello1RequestThingUserPaymentPlanChanged) isHello1RequestThingVariant() {}

type Hello1RequestThingUnknown struct {
	Tag string
	Raw json.RawMessage
}

func (Hello1RequestThingUnknown) isHello1RequestThingVariant() {}

func (v Hello1RequestThing) MarshalJSON() ([]byte, error) {
	switch value := v.Value.(type) {
	case Hello1RequestThingUserCreated:
		return json.Marshal(struct {
			Tag string `json:"eventType"`
			Hello1RequestThingUserCreated
		}{"USER_CREATED", value})
	case Hello1RequestThingUserDeleted:
		return json.Marshal(struct {
			Tag string `json:"eventType"`
			Hello1RequestThingUserDeleted
		}{"USER_DELETED", value})
	case Hello1RequestThingUserPaymentPlanChanged:
		return json.Marshal(struct {
			Tag string `json:"eventType"`
			Hello1RequestThingUserPaymentPlanChanged
		}{"USER_PAYMENT_PLAN_CHANGED", value})
	case Hello1RequestThingUnknown:
		var tag struct {
			Tag *string `json:"eventType"`
		}
		if err := json.Unmarshal(value.Raw, &tag); err != nil {
			return nil, err
		}
		if tag.Tag == nil || *tag.Tag != value.Tag {
			return nil, fmt.Errorf("Hello1RequestThing: unknown variant tag does not match payload")
		}
		return value.Raw, nil
	case *Hello1RequestThingUserCreated:
		if value != nil {
			return (Hello1RequestThing{Value: *value}).MarshalJSON()
		}
	case *Hello1RequestThingUserDeleted:
		if value != nil {
			return (Hello1RequestThing{Value: *value}).MarshalJSON()
		}
	case *Hello1RequestThingUserPaymentPlanChanged:
		if value != nil {
			return (Hello1RequestThing{Value: *value}).MarshalJSON()
		}
	case *Hello1RequestThingUnknown:
		if value != nil {
			return (Hello1RequestThing{Value: *value}).MarshalJSON()
		}
	}
	return nil, fmt.Errorf("Hello1RequestThing: no variant set")
}

func (v *Hello1RequestThing) UnmarshalJSON(b []byte) error {
	var tag struct {
		Tag *string `json:"eventType"`
	}
	if err := json.Unmarshal(b, &tag); err != nil {
		return err
	}
	if tag.Tag == nil {
		return fmt.Errorf("Hello1RequestThing: missing eventType")
	}
	switch *tag.Tag {
	case "USER_CREATED":
		var value Hello1RequestThingUserCreated
		if err := json.Unmarshal(b, &value); err != nil {
			return err
		}
		v.Value = value
	case "USER_DELETED":
		var value Hello1RequestThingUserDeleted
		if err := json.Unmarshal(b, &value); err != nil {
			return err
		}
		v.Value = value
	case "USER_PAYMENT_PLAN_CHANGED":
		var value Hello1RequestThingUserPaymentPlanChanged
		if err := json.Unmarshal(b, &value); err != nil {
			return err
		}
		v.Value = value
	default:
		v.Value = Hello1RequestThingUnknown{Tag: *tag.Tag, Raw: append(json.RawMessage(nil), b...)}
	}
	return nil
}

type Hello1RequestThing struct {
	Value Hello1RequestThingVariant
}

type Hello1ResponseVariant interface{ isHello1ResponseVariant() }

type Hello1ResponseUserCreated struct {
	ID string `json:"id"`
}

func (Hello1ResponseUserCreated) isHello1ResponseVariant() {}

type Hello1ResponseUserDeleted struct {
	ID         string `json:"id"`
	SoftDelete bool   `json:"softDelete"`
}

func (Hello1ResponseUserDeleted) isHello1ResponseVariant() {}

type Hello1ResponseUserPaymentPlanChanged struct {
	ID   string `json:"id"`
	Plan string `json:"plan"`
}

func (Hello1ResponseUserPaymentPlanChanged) isHello1ResponseVariant() {}

type Hello1ResponseUnknown struct {
	Tag string
	Raw json.RawMessage
}

func (Hello1ResponseUnknown) isHello1ResponseVariant() {}

func (v Hello1Response) MarshalJSON() ([]byte, error) {
	switch value := v.Value.(type) {
	case Hello1ResponseUserCreated:
		return json.Marshal(struct {
			Tag string `json:"eventType"`
			Hello1ResponseUserCreated
		}{"USER_CREATED", value})
	case Hello1ResponseUserDeleted:
		return json.Marshal(struct {
			Tag string `json:"eventType"`
			Hello1ResponseUserDeleted
		}{"USER_DELETED", value})
	case Hello1ResponseUserPaymentPlanChanged:
		return json.Marshal(struct {
			Tag string `json:"eventType"`
			Hello1ResponseUserPaymentPlanChanged
		}{"USER_PAYMENT_PLAN_CHANGED", value})
	case Hello1ResponseUnknown:
		var tag struct {
			Tag *string `json:"eventType"`
		}
		if err := json.Unmarshal(value.Raw, &tag); err != nil {
			return nil, err
		}
		if tag.Tag == nil || *tag.Tag != value.Tag {
			return nil, fmt.Errorf("Hello1Response: unknown variant tag does not match payload")
		}
		return value.Raw, nil
	case *Hello1ResponseUserCreated:
		if value != nil {
			return (Hello1Response{Value: *value}).MarshalJSON()
		}
	case *Hello1ResponseUserDeleted:
		if value != nil {
			return (Hello1Response{Value: *value}).MarshalJSON()
		}
	case *Hello1ResponseUserPaymentPlanChanged:
		if value != nil {
			return (Hello1Response{Value: *value}).MarshalJSON()
		}
	case *Hello1ResponseUnknown:
		if value != nil {
			return (Hello1Response{Value: *value}).MarshalJSON()
		}
	}
	return nil, fmt.Errorf("Hello1Response: no variant set")
}

func (v *Hello1Response) UnmarshalJSON(b []byte) error {
	var tag struct {
		Tag *string `json:"eventType"`
	}
	if err := json.Unmarshal(b, &tag); err != nil {
		return err
	}
	if tag.Tag == nil {
		return fmt.Errorf("Hello1Response: missing eventType")
	}
	switch *tag.Tag {
	case "USER_CREATED":
		var value Hello1ResponseUserCreated
		if err := json.Unmarshal(b, &value); err != nil {
			return err
		}
		v.Value = value
	case "USER_DELETED":
		var value Hello1ResponseUserDeleted
		if err := json.Unmarshal(b, &value); err != nil {
			return err
		}
		v.Value = value
	case "USER_PAYMENT_PLAN_CHANGED":
		var value Hello1ResponseUserPaymentPlanChanged
		if err := json.Unmarshal(b, &value); err != nil {
			return err
		}
		v.Value = value
	default:
		v.Value = Hello1ResponseUnknown{Tag: *tag.Tag, Raw: append(json.RawMessage(nil), b...)}
	}
	return nil
}

type Service1Service interface {
	Hello1(context.Context, Hello1Request) (*Hello1Response, error)
}

type Service1RPCClient struct {
	lokerpc.Client
}

func (c Service1RPCClient) Hello1(ctx context.Context, req Hello1Request) (*Hello1Response, error) {
	var res Hello1Response
	err := c.DoRequest(ctx, "hello1", req, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}
