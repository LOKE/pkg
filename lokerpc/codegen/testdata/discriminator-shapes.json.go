package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/LOKE/pkg/lokerpc"
)

type Change struct {
	Value ChangeVariant
}

type ChangeAlias = Change

type Event *EventUnion

type EventsByID map[string]EventsByIDValue

type ListResponse []ListResponseItem

type PublishRequest struct {
	ID    string               `json:"id"`
	Event *PublishRequestEvent `json:"event,omitempty"`
}

type ChangeVariant interface {
	isChangeVariant()
	marshalJSON() ([]byte, error)
}

type ChangeCreated struct {
	ID string `json:"id"`
}

func (*ChangeCreated) isChangeVariant() {}

func (v *ChangeCreated) marshalJSON() ([]byte, error) {
	if v == nil {
		return nil, fmt.Errorf("Change: no variant set")
	}
	return json.Marshal(struct {
		Tag string `json:"type"`
		*ChangeCreated
	}{"CREATED", v})
}

type ChangeUnknown struct {
	Tag string
	Raw json.RawMessage
}

func (*ChangeUnknown) isChangeVariant() {}

func (v *ChangeUnknown) marshalJSON() ([]byte, error) {
	if v == nil {
		return nil, fmt.Errorf("Change: no variant set")
	}
	var tag struct {
		Tag *string `json:"type"`
	}
	if err := json.Unmarshal(v.Raw, &tag); err != nil {
		return nil, err
	}
	if tag.Tag == nil || *tag.Tag != v.Tag {
		return nil, fmt.Errorf("Change: unknown variant tag does not match payload")
	}
	return v.Raw, nil
}

func (v Change) MarshalJSON() ([]byte, error) {
	if v.Value == nil {
		return nil, fmt.Errorf("Change: no variant set")
	}
	return v.Value.marshalJSON()
}

func (v *Change) UnmarshalJSON(b []byte) error {
	var tag struct {
		Tag *string `json:"type"`
	}
	if err := json.Unmarshal(b, &tag); err != nil {
		return err
	}
	if tag.Tag == nil {
		return fmt.Errorf("Change: missing type")
	}
	switch *tag.Tag {
	case "CREATED":
		var value ChangeCreated
		if err := json.Unmarshal(b, &value); err != nil {
			return err
		}
		v.Value = &value
	default:
		v.Value = &ChangeUnknown{Tag: *tag.Tag, Raw: append(json.RawMessage(nil), b...)}
	}
	return nil
}

type EventUnionVariant interface {
	isEventUnionVariant()
	marshalJSON() ([]byte, error)
}

type EventUnionCreated struct {
	ID string `json:"id"`
}

func (*EventUnionCreated) isEventUnionVariant() {}

func (v *EventUnionCreated) marshalJSON() ([]byte, error) {
	if v == nil {
		return nil, fmt.Errorf("EventUnion: no variant set")
	}
	return json.Marshal(struct {
		Tag string `json:"type"`
		*EventUnionCreated
	}{"CREATED", v})
}

type EventUnionDeleted struct {
	ID string `json:"id"`
}

func (*EventUnionDeleted) isEventUnionVariant() {}

func (v *EventUnionDeleted) marshalJSON() ([]byte, error) {
	if v == nil {
		return nil, fmt.Errorf("EventUnion: no variant set")
	}
	return json.Marshal(struct {
		Tag string `json:"type"`
		*EventUnionDeleted
	}{"DELETED", v})
}

type EventUnionUnknown struct {
	Tag string
	Raw json.RawMessage
}

func (*EventUnionUnknown) isEventUnionVariant() {}

func (v *EventUnionUnknown) marshalJSON() ([]byte, error) {
	if v == nil {
		return nil, fmt.Errorf("EventUnion: no variant set")
	}
	var tag struct {
		Tag *string `json:"type"`
	}
	if err := json.Unmarshal(v.Raw, &tag); err != nil {
		return nil, err
	}
	if tag.Tag == nil || *tag.Tag != v.Tag {
		return nil, fmt.Errorf("EventUnion: unknown variant tag does not match payload")
	}
	return v.Raw, nil
}

func (v EventUnion) MarshalJSON() ([]byte, error) {
	if v.Value == nil {
		return nil, fmt.Errorf("EventUnion: no variant set")
	}
	return v.Value.marshalJSON()
}

func (v *EventUnion) UnmarshalJSON(b []byte) error {
	var tag struct {
		Tag *string `json:"type"`
	}
	if err := json.Unmarshal(b, &tag); err != nil {
		return err
	}
	if tag.Tag == nil {
		return fmt.Errorf("EventUnion: missing type")
	}
	switch *tag.Tag {
	case "CREATED":
		var value EventUnionCreated
		if err := json.Unmarshal(b, &value); err != nil {
			return err
		}
		v.Value = &value
	case "DELETED":
		var value EventUnionDeleted
		if err := json.Unmarshal(b, &value); err != nil {
			return err
		}
		v.Value = &value
	default:
		v.Value = &EventUnionUnknown{Tag: *tag.Tag, Raw: append(json.RawMessage(nil), b...)}
	}
	return nil
}

type EventUnion struct {
	Value EventUnionVariant
}

type EventsByIDValueVariant interface {
	isEventsByIDValueVariant()
	marshalJSON() ([]byte, error)
}

type EventsByIDValueCreated struct {
	ID string `json:"id"`
}

func (*EventsByIDValueCreated) isEventsByIDValueVariant() {}

func (v *EventsByIDValueCreated) marshalJSON() ([]byte, error) {
	if v == nil {
		return nil, fmt.Errorf("EventsByIDValue: no variant set")
	}
	return json.Marshal(struct {
		Tag string `json:"type"`
		*EventsByIDValueCreated
	}{"CREATED", v})
}

type EventsByIDValueUnknown struct {
	Tag string
	Raw json.RawMessage
}

func (*EventsByIDValueUnknown) isEventsByIDValueVariant() {}

func (v *EventsByIDValueUnknown) marshalJSON() ([]byte, error) {
	if v == nil {
		return nil, fmt.Errorf("EventsByIDValue: no variant set")
	}
	var tag struct {
		Tag *string `json:"type"`
	}
	if err := json.Unmarshal(v.Raw, &tag); err != nil {
		return nil, err
	}
	if tag.Tag == nil || *tag.Tag != v.Tag {
		return nil, fmt.Errorf("EventsByIDValue: unknown variant tag does not match payload")
	}
	return v.Raw, nil
}

func (v EventsByIDValue) MarshalJSON() ([]byte, error) {
	if v.Value == nil {
		return nil, fmt.Errorf("EventsByIDValue: no variant set")
	}
	return v.Value.marshalJSON()
}

func (v *EventsByIDValue) UnmarshalJSON(b []byte) error {
	var tag struct {
		Tag *string `json:"type"`
	}
	if err := json.Unmarshal(b, &tag); err != nil {
		return err
	}
	if tag.Tag == nil {
		return fmt.Errorf("EventsByIDValue: missing type")
	}
	switch *tag.Tag {
	case "CREATED":
		var value EventsByIDValueCreated
		if err := json.Unmarshal(b, &value); err != nil {
			return err
		}
		v.Value = &value
	default:
		v.Value = &EventsByIDValueUnknown{Tag: *tag.Tag, Raw: append(json.RawMessage(nil), b...)}
	}
	return nil
}

type EventsByIDValue struct {
	Value EventsByIDValueVariant
}

type ListResponseItemVariant interface {
	isListResponseItemVariant()
	marshalJSON() ([]byte, error)
}

type ListResponseItemCreated struct {
	ID string `json:"id"`
}

func (*ListResponseItemCreated) isListResponseItemVariant() {}

func (v *ListResponseItemCreated) marshalJSON() ([]byte, error) {
	if v == nil {
		return nil, fmt.Errorf("ListResponseItem: no variant set")
	}
	return json.Marshal(struct {
		Tag string `json:"type"`
		*ListResponseItemCreated
	}{"CREATED", v})
}

type ListResponseItemDeleted struct {
	ID string `json:"id"`
}

func (*ListResponseItemDeleted) isListResponseItemVariant() {}

func (v *ListResponseItemDeleted) marshalJSON() ([]byte, error) {
	if v == nil {
		return nil, fmt.Errorf("ListResponseItem: no variant set")
	}
	return json.Marshal(struct {
		Tag string `json:"type"`
		*ListResponseItemDeleted
	}{"DELETED", v})
}

type ListResponseItemUnknown struct {
	Tag string
	Raw json.RawMessage
}

func (*ListResponseItemUnknown) isListResponseItemVariant() {}

func (v *ListResponseItemUnknown) marshalJSON() ([]byte, error) {
	if v == nil {
		return nil, fmt.Errorf("ListResponseItem: no variant set")
	}
	var tag struct {
		Tag *string `json:"type"`
	}
	if err := json.Unmarshal(v.Raw, &tag); err != nil {
		return nil, err
	}
	if tag.Tag == nil || *tag.Tag != v.Tag {
		return nil, fmt.Errorf("ListResponseItem: unknown variant tag does not match payload")
	}
	return v.Raw, nil
}

func (v ListResponseItem) MarshalJSON() ([]byte, error) {
	if v.Value == nil {
		return nil, fmt.Errorf("ListResponseItem: no variant set")
	}
	return v.Value.marshalJSON()
}

func (v *ListResponseItem) UnmarshalJSON(b []byte) error {
	var tag struct {
		Tag *string `json:"type"`
	}
	if err := json.Unmarshal(b, &tag); err != nil {
		return err
	}
	if tag.Tag == nil {
		return fmt.Errorf("ListResponseItem: missing type")
	}
	switch *tag.Tag {
	case "CREATED":
		var value ListResponseItemCreated
		if err := json.Unmarshal(b, &value); err != nil {
			return err
		}
		v.Value = &value
	case "DELETED":
		var value ListResponseItemDeleted
		if err := json.Unmarshal(b, &value); err != nil {
			return err
		}
		v.Value = &value
	default:
		v.Value = &ListResponseItemUnknown{Tag: *tag.Tag, Raw: append(json.RawMessage(nil), b...)}
	}
	return nil
}

type ListResponseItem struct {
	Value ListResponseItemVariant
}

type PublishRequestEventVariant interface {
	isPublishRequestEventVariant()
	marshalJSON() ([]byte, error)
}

type PublishRequestEventCreated struct {
	ID string `json:"id"`
}

func (*PublishRequestEventCreated) isPublishRequestEventVariant() {}

func (v *PublishRequestEventCreated) marshalJSON() ([]byte, error) {
	if v == nil {
		return nil, fmt.Errorf("PublishRequestEvent: no variant set")
	}
	return json.Marshal(struct {
		Tag string `json:"type"`
		*PublishRequestEventCreated
	}{"CREATED", v})
}

type PublishRequestEventUnknown struct {
	Tag string
	Raw json.RawMessage
}

func (*PublishRequestEventUnknown) isPublishRequestEventVariant() {}

func (v *PublishRequestEventUnknown) marshalJSON() ([]byte, error) {
	if v == nil {
		return nil, fmt.Errorf("PublishRequestEvent: no variant set")
	}
	var tag struct {
		Tag *string `json:"type"`
	}
	if err := json.Unmarshal(v.Raw, &tag); err != nil {
		return nil, err
	}
	if tag.Tag == nil || *tag.Tag != v.Tag {
		return nil, fmt.Errorf("PublishRequestEvent: unknown variant tag does not match payload")
	}
	return v.Raw, nil
}

func (v PublishRequestEvent) MarshalJSON() ([]byte, error) {
	if v.Value == nil {
		return nil, fmt.Errorf("PublishRequestEvent: no variant set")
	}
	return v.Value.marshalJSON()
}

func (v *PublishRequestEvent) UnmarshalJSON(b []byte) error {
	var tag struct {
		Tag *string `json:"type"`
	}
	if err := json.Unmarshal(b, &tag); err != nil {
		return err
	}
	if tag.Tag == nil {
		return fmt.Errorf("PublishRequestEvent: missing type")
	}
	switch *tag.Tag {
	case "CREATED":
		var value PublishRequestEventCreated
		if err := json.Unmarshal(b, &value); err != nil {
			return err
		}
		v.Value = &value
	default:
		v.Value = &PublishRequestEventUnknown{Tag: *tag.Tag, Raw: append(json.RawMessage(nil), b...)}
	}
	return nil
}

type PublishRequestEvent struct {
	Value PublishRequestEventVariant
}

type EventsService interface {
	Latest(context.Context, any) (Event, error)
	List(context.Context, any) (*ListResponse, error)
	Publish(context.Context, PublishRequest) error
}

type EventsRPCClient struct {
	lokerpc.Client
}

func (c EventsRPCClient) Latest(ctx context.Context, req any) (Event, error) {
	var res Event
	err := c.DoRequest(ctx, "latest", req, &res)
	if err != nil {
		return nil, err
	}
	return res, nil
}
func (c EventsRPCClient) List(ctx context.Context, req any) (*ListResponse, error) {
	var res ListResponse
	err := c.DoRequest(ctx, "list", req, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}
func (c EventsRPCClient) Publish(ctx context.Context, req PublishRequest) error {
	return c.DoRequest(ctx, "publish", req, nil)
}
