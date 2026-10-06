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

type ChangeVariant interface{ isChangeVariant() }

type ChangeCreated struct {
	ID string `json:"id"`
}

func (ChangeCreated) isChangeVariant() {}

type ChangeUnknown struct {
	Tag string
	Raw json.RawMessage
}

func (ChangeUnknown) isChangeVariant() {}

func (v Change) MarshalJSON() ([]byte, error) {
	switch value := v.Value.(type) {
	case ChangeCreated:
		return json.Marshal(struct {
			Tag string `json:"type"`
			ChangeCreated
		}{"CREATED", value})
	case ChangeUnknown:
		var tag struct {
			Tag *string `json:"type"`
		}
		if err := json.Unmarshal(value.Raw, &tag); err != nil {
			return nil, err
		}
		if tag.Tag == nil || *tag.Tag != value.Tag {
			return nil, fmt.Errorf("Change: unknown variant tag does not match payload")
		}
		return value.Raw, nil
	case *ChangeCreated:
		if value != nil {
			return (Change{Value: *value}).MarshalJSON()
		}
	case *ChangeUnknown:
		if value != nil {
			return (Change{Value: *value}).MarshalJSON()
		}
	}
	return nil, fmt.Errorf("Change: no variant set")
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
		v.Value = value
	default:
		v.Value = ChangeUnknown{Tag: *tag.Tag, Raw: append(json.RawMessage(nil), b...)}
	}
	return nil
}

type EventUnionVariant interface{ isEventUnionVariant() }

type EventUnionCreated struct {
	ID string `json:"id"`
}

func (EventUnionCreated) isEventUnionVariant() {}

type EventUnionDeleted struct {
	ID string `json:"id"`
}

func (EventUnionDeleted) isEventUnionVariant() {}

type EventUnionUnknown struct {
	Tag string
	Raw json.RawMessage
}

func (EventUnionUnknown) isEventUnionVariant() {}

func (v EventUnion) MarshalJSON() ([]byte, error) {
	switch value := v.Value.(type) {
	case EventUnionCreated:
		return json.Marshal(struct {
			Tag string `json:"type"`
			EventUnionCreated
		}{"CREATED", value})
	case EventUnionDeleted:
		return json.Marshal(struct {
			Tag string `json:"type"`
			EventUnionDeleted
		}{"DELETED", value})
	case EventUnionUnknown:
		var tag struct {
			Tag *string `json:"type"`
		}
		if err := json.Unmarshal(value.Raw, &tag); err != nil {
			return nil, err
		}
		if tag.Tag == nil || *tag.Tag != value.Tag {
			return nil, fmt.Errorf("EventUnion: unknown variant tag does not match payload")
		}
		return value.Raw, nil
	case *EventUnionCreated:
		if value != nil {
			return (EventUnion{Value: *value}).MarshalJSON()
		}
	case *EventUnionDeleted:
		if value != nil {
			return (EventUnion{Value: *value}).MarshalJSON()
		}
	case *EventUnionUnknown:
		if value != nil {
			return (EventUnion{Value: *value}).MarshalJSON()
		}
	}
	return nil, fmt.Errorf("EventUnion: no variant set")
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
		v.Value = value
	case "DELETED":
		var value EventUnionDeleted
		if err := json.Unmarshal(b, &value); err != nil {
			return err
		}
		v.Value = value
	default:
		v.Value = EventUnionUnknown{Tag: *tag.Tag, Raw: append(json.RawMessage(nil), b...)}
	}
	return nil
}

type EventUnion struct {
	Value EventUnionVariant
}

type EventsByIDValueVariant interface{ isEventsByIDValueVariant() }

type EventsByIDValueCreated struct {
	ID string `json:"id"`
}

func (EventsByIDValueCreated) isEventsByIDValueVariant() {}

type EventsByIDValueUnknown struct {
	Tag string
	Raw json.RawMessage
}

func (EventsByIDValueUnknown) isEventsByIDValueVariant() {}

func (v EventsByIDValue) MarshalJSON() ([]byte, error) {
	switch value := v.Value.(type) {
	case EventsByIDValueCreated:
		return json.Marshal(struct {
			Tag string `json:"type"`
			EventsByIDValueCreated
		}{"CREATED", value})
	case EventsByIDValueUnknown:
		var tag struct {
			Tag *string `json:"type"`
		}
		if err := json.Unmarshal(value.Raw, &tag); err != nil {
			return nil, err
		}
		if tag.Tag == nil || *tag.Tag != value.Tag {
			return nil, fmt.Errorf("EventsByIDValue: unknown variant tag does not match payload")
		}
		return value.Raw, nil
	case *EventsByIDValueCreated:
		if value != nil {
			return (EventsByIDValue{Value: *value}).MarshalJSON()
		}
	case *EventsByIDValueUnknown:
		if value != nil {
			return (EventsByIDValue{Value: *value}).MarshalJSON()
		}
	}
	return nil, fmt.Errorf("EventsByIDValue: no variant set")
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
		v.Value = value
	default:
		v.Value = EventsByIDValueUnknown{Tag: *tag.Tag, Raw: append(json.RawMessage(nil), b...)}
	}
	return nil
}

type EventsByIDValue struct {
	Value EventsByIDValueVariant
}

type ListResponseItemVariant interface{ isListResponseItemVariant() }

type ListResponseItemCreated struct {
	ID string `json:"id"`
}

func (ListResponseItemCreated) isListResponseItemVariant() {}

type ListResponseItemDeleted struct {
	ID string `json:"id"`
}

func (ListResponseItemDeleted) isListResponseItemVariant() {}

type ListResponseItemUnknown struct {
	Tag string
	Raw json.RawMessage
}

func (ListResponseItemUnknown) isListResponseItemVariant() {}

func (v ListResponseItem) MarshalJSON() ([]byte, error) {
	switch value := v.Value.(type) {
	case ListResponseItemCreated:
		return json.Marshal(struct {
			Tag string `json:"type"`
			ListResponseItemCreated
		}{"CREATED", value})
	case ListResponseItemDeleted:
		return json.Marshal(struct {
			Tag string `json:"type"`
			ListResponseItemDeleted
		}{"DELETED", value})
	case ListResponseItemUnknown:
		var tag struct {
			Tag *string `json:"type"`
		}
		if err := json.Unmarshal(value.Raw, &tag); err != nil {
			return nil, err
		}
		if tag.Tag == nil || *tag.Tag != value.Tag {
			return nil, fmt.Errorf("ListResponseItem: unknown variant tag does not match payload")
		}
		return value.Raw, nil
	case *ListResponseItemCreated:
		if value != nil {
			return (ListResponseItem{Value: *value}).MarshalJSON()
		}
	case *ListResponseItemDeleted:
		if value != nil {
			return (ListResponseItem{Value: *value}).MarshalJSON()
		}
	case *ListResponseItemUnknown:
		if value != nil {
			return (ListResponseItem{Value: *value}).MarshalJSON()
		}
	}
	return nil, fmt.Errorf("ListResponseItem: no variant set")
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
		v.Value = value
	case "DELETED":
		var value ListResponseItemDeleted
		if err := json.Unmarshal(b, &value); err != nil {
			return err
		}
		v.Value = value
	default:
		v.Value = ListResponseItemUnknown{Tag: *tag.Tag, Raw: append(json.RawMessage(nil), b...)}
	}
	return nil
}

type ListResponseItem struct {
	Value ListResponseItemVariant
}

type PublishRequestEventVariant interface{ isPublishRequestEventVariant() }

type PublishRequestEventCreated struct {
	ID string `json:"id"`
}

func (PublishRequestEventCreated) isPublishRequestEventVariant() {}

type PublishRequestEventUnknown struct {
	Tag string
	Raw json.RawMessage
}

func (PublishRequestEventUnknown) isPublishRequestEventVariant() {}

func (v PublishRequestEvent) MarshalJSON() ([]byte, error) {
	switch value := v.Value.(type) {
	case PublishRequestEventCreated:
		return json.Marshal(struct {
			Tag string `json:"type"`
			PublishRequestEventCreated
		}{"CREATED", value})
	case PublishRequestEventUnknown:
		var tag struct {
			Tag *string `json:"type"`
		}
		if err := json.Unmarshal(value.Raw, &tag); err != nil {
			return nil, err
		}
		if tag.Tag == nil || *tag.Tag != value.Tag {
			return nil, fmt.Errorf("PublishRequestEvent: unknown variant tag does not match payload")
		}
		return value.Raw, nil
	case *PublishRequestEventCreated:
		if value != nil {
			return (PublishRequestEvent{Value: *value}).MarshalJSON()
		}
	case *PublishRequestEventUnknown:
		if value != nil {
			return (PublishRequestEvent{Value: *value}).MarshalJSON()
		}
	}
	return nil, fmt.Errorf("PublishRequestEvent: no variant set")
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
		v.Value = value
	default:
		v.Value = PublishRequestEventUnknown{Tag: *tag.Tag, Raw: append(json.RawMessage(nil), b...)}
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
