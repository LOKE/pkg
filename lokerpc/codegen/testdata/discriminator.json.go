package activity

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/LOKE/pkg/lokerpc"
)

type AddActivityRequest struct {
	CustomerUid string                 `json:"customerUid"`
	Item        AddActivityRequestItem `json:"item"`
}

type AddActivityResponse struct {
	Value AddActivityResponseVariant
}

type AddActivityRequestItemVariant interface{ isAddActivityRequestItemVariant() }

type AddActivityRequestItemCreditExpired struct {
	Amount   int32  `json:"amount"`
	CreditID string `json:"creditId"`
	Title    string `json:"title"`
}

func (AddActivityRequestItemCreditExpired) isAddActivityRequestItemVariant() {}

type AddActivityRequestItemMessage struct {
	Body         string `json:"body"`
	Title        string `json:"title"`
	LocationName string `json:"locationName,omitempty"`
}

func (AddActivityRequestItemMessage) isAddActivityRequestItemVariant() {}

type AddActivityRequestItemPoints struct {
	Points       int32  `json:"points"`
	Title        string `json:"title"`
	LocationName string `json:"locationName,omitempty"`
}

func (AddActivityRequestItemPoints) isAddActivityRequestItemVariant() {}

type AddActivityRequestItemUnknown struct {
	Tag string
	Raw json.RawMessage
}

func (AddActivityRequestItemUnknown) isAddActivityRequestItemVariant() {}

func (v AddActivityRequestItem) MarshalJSON() ([]byte, error) {
	switch value := v.Value.(type) {
	case AddActivityRequestItemCreditExpired:
		return json.Marshal(struct {
			Tag string `json:"type"`
			AddActivityRequestItemCreditExpired
		}{"CREDIT_EXPIRED", value})
	case AddActivityRequestItemMessage:
		return json.Marshal(struct {
			Tag string `json:"type"`
			AddActivityRequestItemMessage
		}{"MESSAGE", value})
	case AddActivityRequestItemPoints:
		return json.Marshal(struct {
			Tag string `json:"type"`
			AddActivityRequestItemPoints
		}{"POINTS", value})
	case AddActivityRequestItemUnknown:
		var tag struct {
			Tag *string `json:"type"`
		}
		if err := json.Unmarshal(value.Raw, &tag); err != nil {
			return nil, err
		}
		if tag.Tag == nil || *tag.Tag != value.Tag {
			return nil, fmt.Errorf("AddActivityRequestItem: unknown variant tag does not match payload")
		}
		return value.Raw, nil
	case *AddActivityRequestItemCreditExpired:
		if value != nil {
			return (AddActivityRequestItem{Value: *value}).MarshalJSON()
		}
	case *AddActivityRequestItemMessage:
		if value != nil {
			return (AddActivityRequestItem{Value: *value}).MarshalJSON()
		}
	case *AddActivityRequestItemPoints:
		if value != nil {
			return (AddActivityRequestItem{Value: *value}).MarshalJSON()
		}
	case *AddActivityRequestItemUnknown:
		if value != nil {
			return (AddActivityRequestItem{Value: *value}).MarshalJSON()
		}
	}
	return nil, fmt.Errorf("AddActivityRequestItem: no variant set")
}

func (v *AddActivityRequestItem) UnmarshalJSON(b []byte) error {
	var tag struct {
		Tag *string `json:"type"`
	}
	if err := json.Unmarshal(b, &tag); err != nil {
		return err
	}
	if tag.Tag == nil {
		return fmt.Errorf("AddActivityRequestItem: missing type")
	}
	switch *tag.Tag {
	case "CREDIT_EXPIRED":
		var value AddActivityRequestItemCreditExpired
		if err := json.Unmarshal(b, &value); err != nil {
			return err
		}
		v.Value = value
	case "MESSAGE":
		var value AddActivityRequestItemMessage
		if err := json.Unmarshal(b, &value); err != nil {
			return err
		}
		v.Value = value
	case "POINTS":
		var value AddActivityRequestItemPoints
		if err := json.Unmarshal(b, &value); err != nil {
			return err
		}
		v.Value = value
	default:
		v.Value = AddActivityRequestItemUnknown{Tag: *tag.Tag, Raw: append(json.RawMessage(nil), b...)}
	}
	return nil
}

type AddActivityRequestItem struct {
	Value AddActivityRequestItemVariant
}

type AddActivityResponseVariant interface{ isAddActivityResponseVariant() }

type AddActivityResponseCreditExpired struct {
	Amount    int32     `json:"amount"`
	CreditID  string    `json:"creditId"`
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Title     string    `json:"title"`
}

func (AddActivityResponseCreditExpired) isAddActivityResponseVariant() {}

type AddActivityResponseMessage struct {
	Body         string    `json:"body"`
	ID           string    `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	Title        string    `json:"title"`
	LocationName string    `json:"locationName,omitempty"`
}

func (AddActivityResponseMessage) isAddActivityResponseVariant() {}

type AddActivityResponsePoints struct {
	ID           string    `json:"id"`
	Points       int32     `json:"points"`
	PointsText   string    `json:"pointsText"`
	Timestamp    time.Time `json:"timestamp"`
	Title        string    `json:"title"`
	LocationName string    `json:"locationName,omitempty"`
}

func (AddActivityResponsePoints) isAddActivityResponseVariant() {}

type AddActivityResponseUnknown struct {
	Tag string
	Raw json.RawMessage
}

func (AddActivityResponseUnknown) isAddActivityResponseVariant() {}

func (v AddActivityResponse) MarshalJSON() ([]byte, error) {
	switch value := v.Value.(type) {
	case AddActivityResponseCreditExpired:
		return json.Marshal(struct {
			Tag string `json:"type"`
			AddActivityResponseCreditExpired
		}{"CREDIT_EXPIRED", value})
	case AddActivityResponseMessage:
		return json.Marshal(struct {
			Tag string `json:"type"`
			AddActivityResponseMessage
		}{"MESSAGE", value})
	case AddActivityResponsePoints:
		return json.Marshal(struct {
			Tag string `json:"type"`
			AddActivityResponsePoints
		}{"POINTS", value})
	case AddActivityResponseUnknown:
		var tag struct {
			Tag *string `json:"type"`
		}
		if err := json.Unmarshal(value.Raw, &tag); err != nil {
			return nil, err
		}
		if tag.Tag == nil || *tag.Tag != value.Tag {
			return nil, fmt.Errorf("AddActivityResponse: unknown variant tag does not match payload")
		}
		return value.Raw, nil
	case *AddActivityResponseCreditExpired:
		if value != nil {
			return (AddActivityResponse{Value: *value}).MarshalJSON()
		}
	case *AddActivityResponseMessage:
		if value != nil {
			return (AddActivityResponse{Value: *value}).MarshalJSON()
		}
	case *AddActivityResponsePoints:
		if value != nil {
			return (AddActivityResponse{Value: *value}).MarshalJSON()
		}
	case *AddActivityResponseUnknown:
		if value != nil {
			return (AddActivityResponse{Value: *value}).MarshalJSON()
		}
	}
	return nil, fmt.Errorf("AddActivityResponse: no variant set")
}

func (v *AddActivityResponse) UnmarshalJSON(b []byte) error {
	var tag struct {
		Tag *string `json:"type"`
	}
	if err := json.Unmarshal(b, &tag); err != nil {
		return err
	}
	if tag.Tag == nil {
		return fmt.Errorf("AddActivityResponse: missing type")
	}
	switch *tag.Tag {
	case "CREDIT_EXPIRED":
		var value AddActivityResponseCreditExpired
		if err := json.Unmarshal(b, &value); err != nil {
			return err
		}
		v.Value = value
	case "MESSAGE":
		var value AddActivityResponseMessage
		if err := json.Unmarshal(b, &value); err != nil {
			return err
		}
		v.Value = value
	case "POINTS":
		var value AddActivityResponsePoints
		if err := json.Unmarshal(b, &value); err != nil {
			return err
		}
		v.Value = value
	default:
		v.Value = AddActivityResponseUnknown{Tag: *tag.Tag, Raw: append(json.RawMessage(nil), b...)}
	}
	return nil
}

type ActivityService interface {
	AddActivity(context.Context, AddActivityRequest) (*AddActivityResponse, error)
}

type ActivityRPCClient struct {
	lokerpc.Client
}

func (c ActivityRPCClient) AddActivity(ctx context.Context, req AddActivityRequest) (*AddActivityResponse, error) {
	var res AddActivityResponse
	err := c.DoRequest(ctx, "addActivity", req, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}
