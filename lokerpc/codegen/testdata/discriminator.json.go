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
	UserCreated            *Hello1ResponseUserCreated
	UserDeleted            *Hello1ResponseUserDeleted
	UserPaymentPlanChanged *Hello1ResponseUserPaymentPlanChanged
}

type Hello1RequestThingUserCreated struct {
	ID string `json:"id"`
}

type Hello1RequestThingUserDeleted struct {
	ID         string `json:"id"`
	SoftDelete bool   `json:"softDelete"`
}

type Hello1RequestThingUserPaymentPlanChanged struct {
	ID   string `json:"id"`
	Plan string `json:"plan"`
}

func (v Hello1RequestThing) MarshalJSON() ([]byte, error) {
	switch {
	case v.UserCreated != nil:
		return json.Marshal(struct {
			EventType string `json:"eventType"`
			*Hello1RequestThingUserCreated
		}{"USER_CREATED", v.UserCreated})
	case v.UserDeleted != nil:
		return json.Marshal(struct {
			EventType string `json:"eventType"`
			*Hello1RequestThingUserDeleted
		}{"USER_DELETED", v.UserDeleted})
	case v.UserPaymentPlanChanged != nil:
		return json.Marshal(struct {
			EventType string `json:"eventType"`
			*Hello1RequestThingUserPaymentPlanChanged
		}{"USER_PAYMENT_PLAN_CHANGED", v.UserPaymentPlanChanged})
	}
	return nil, fmt.Errorf("Hello1RequestThing: no variant set")
}

func (v *Hello1RequestThing) UnmarshalJSON(b []byte) error {
	var tag struct {
		EventType string `json:"eventType"`
	}
	if err := json.Unmarshal(b, &tag); err != nil {
		return err
	}
	*v = Hello1RequestThing{}
	switch tag.EventType {
	case "USER_CREATED":
		v.UserCreated = &Hello1RequestThingUserCreated{}
		return json.Unmarshal(b, v.UserCreated)
	case "USER_DELETED":
		v.UserDeleted = &Hello1RequestThingUserDeleted{}
		return json.Unmarshal(b, v.UserDeleted)
	case "USER_PAYMENT_PLAN_CHANGED":
		v.UserPaymentPlanChanged = &Hello1RequestThingUserPaymentPlanChanged{}
		return json.Unmarshal(b, v.UserPaymentPlanChanged)
	}
	return fmt.Errorf("Hello1RequestThing: unknown eventType %q", tag.EventType)
}

type Hello1RequestThing struct {
	UserCreated            *Hello1RequestThingUserCreated
	UserDeleted            *Hello1RequestThingUserDeleted
	UserPaymentPlanChanged *Hello1RequestThingUserPaymentPlanChanged
}

type Hello1ResponseUserCreated struct {
	ID string `json:"id"`
}

type Hello1ResponseUserDeleted struct {
	ID         string `json:"id"`
	SoftDelete bool   `json:"softDelete"`
}

type Hello1ResponseUserPaymentPlanChanged struct {
	ID   string `json:"id"`
	Plan string `json:"plan"`
}

func (v Hello1Response) MarshalJSON() ([]byte, error) {
	switch {
	case v.UserCreated != nil:
		return json.Marshal(struct {
			EventType string `json:"eventType"`
			*Hello1ResponseUserCreated
		}{"USER_CREATED", v.UserCreated})
	case v.UserDeleted != nil:
		return json.Marshal(struct {
			EventType string `json:"eventType"`
			*Hello1ResponseUserDeleted
		}{"USER_DELETED", v.UserDeleted})
	case v.UserPaymentPlanChanged != nil:
		return json.Marshal(struct {
			EventType string `json:"eventType"`
			*Hello1ResponseUserPaymentPlanChanged
		}{"USER_PAYMENT_PLAN_CHANGED", v.UserPaymentPlanChanged})
	}
	return nil, fmt.Errorf("Hello1Response: no variant set")
}

func (v *Hello1Response) UnmarshalJSON(b []byte) error {
	var tag struct {
		EventType string `json:"eventType"`
	}
	if err := json.Unmarshal(b, &tag); err != nil {
		return err
	}
	*v = Hello1Response{}
	switch tag.EventType {
	case "USER_CREATED":
		v.UserCreated = &Hello1ResponseUserCreated{}
		return json.Unmarshal(b, v.UserCreated)
	case "USER_DELETED":
		v.UserDeleted = &Hello1ResponseUserDeleted{}
		return json.Unmarshal(b, v.UserDeleted)
	case "USER_PAYMENT_PLAN_CHANGED":
		v.UserPaymentPlanChanged = &Hello1ResponseUserPaymentPlanChanged{}
		return json.Unmarshal(b, v.UserPaymentPlanChanged)
	}
	return fmt.Errorf("Hello1Response: unknown eventType %q", tag.EventType)
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
