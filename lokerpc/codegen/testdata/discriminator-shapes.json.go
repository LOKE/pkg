package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/LOKE/pkg/lokerpc"
)

type Change struct {
	Created *ChangeCreated
}

type ChangeAlias = Change

type Event *EventUnion

type EventsByID map[string]EventsByIDValue

type ListResponse []ListResponseItem

type PublishRequest struct {
	ID    string               `json:"id"`
	Event *PublishRequestEvent `json:"event,omitempty"`
}

type ChangeCreated struct {
	ID string `json:"id"`
}

func (v Change) MarshalJSON() ([]byte, error) {
	set := 0
	if v.Created != nil {
		set++
	}
	if set > 1 {
		return nil, fmt.Errorf("Change: %d variants set, want one", set)
	}
	switch {
	case v.Created != nil:
		return json.Marshal(struct {
			Type string `json:"type"`
			*ChangeCreated
		}{"CREATED", v.Created})
	}
	return nil, fmt.Errorf("Change: no variant set")
}

func (v *Change) UnmarshalJSON(b []byte) error {
	var tag struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(b, &tag); err != nil {
		return err
	}
	if tag.Type == "" {
		return fmt.Errorf("Change: missing type")
	}
	*v = Change{}
	switch tag.Type {
	case "CREATED":
		v.Created = &ChangeCreated{}
		return json.Unmarshal(b, v.Created)
	}
	return nil
}

type EventUnionCreated struct {
	ID string `json:"id"`
}

type EventUnionDeleted struct {
	ID string `json:"id"`
}

func (v EventUnion) MarshalJSON() ([]byte, error) {
	set := 0
	if v.Created != nil {
		set++
	}
	if v.Deleted != nil {
		set++
	}
	if set > 1 {
		return nil, fmt.Errorf("EventUnion: %d variants set, want one", set)
	}
	switch {
	case v.Created != nil:
		return json.Marshal(struct {
			Type string `json:"type"`
			*EventUnionCreated
		}{"CREATED", v.Created})
	case v.Deleted != nil:
		return json.Marshal(struct {
			Type string `json:"type"`
			*EventUnionDeleted
		}{"DELETED", v.Deleted})
	}
	return nil, fmt.Errorf("EventUnion: no variant set")
}

func (v *EventUnion) UnmarshalJSON(b []byte) error {
	var tag struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(b, &tag); err != nil {
		return err
	}
	if tag.Type == "" {
		return fmt.Errorf("EventUnion: missing type")
	}
	*v = EventUnion{}
	switch tag.Type {
	case "CREATED":
		v.Created = &EventUnionCreated{}
		return json.Unmarshal(b, v.Created)
	case "DELETED":
		v.Deleted = &EventUnionDeleted{}
		return json.Unmarshal(b, v.Deleted)
	}
	return nil
}

type EventUnion struct {
	Created *EventUnionCreated
	Deleted *EventUnionDeleted
}

type EventsByIDValueCreated struct {
	ID string `json:"id"`
}

func (v EventsByIDValue) MarshalJSON() ([]byte, error) {
	set := 0
	if v.Created != nil {
		set++
	}
	if set > 1 {
		return nil, fmt.Errorf("EventsByIDValue: %d variants set, want one", set)
	}
	switch {
	case v.Created != nil:
		return json.Marshal(struct {
			Type string `json:"type"`
			*EventsByIDValueCreated
		}{"CREATED", v.Created})
	}
	return nil, fmt.Errorf("EventsByIDValue: no variant set")
}

func (v *EventsByIDValue) UnmarshalJSON(b []byte) error {
	var tag struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(b, &tag); err != nil {
		return err
	}
	if tag.Type == "" {
		return fmt.Errorf("EventsByIDValue: missing type")
	}
	*v = EventsByIDValue{}
	switch tag.Type {
	case "CREATED":
		v.Created = &EventsByIDValueCreated{}
		return json.Unmarshal(b, v.Created)
	}
	return nil
}

type EventsByIDValue struct {
	Created *EventsByIDValueCreated
}

type ListResponseItemCreated struct {
	ID string `json:"id"`
}

type ListResponseItemDeleted struct {
	ID string `json:"id"`
}

func (v ListResponseItem) MarshalJSON() ([]byte, error) {
	set := 0
	if v.Created != nil {
		set++
	}
	if v.Deleted != nil {
		set++
	}
	if set > 1 {
		return nil, fmt.Errorf("ListResponseItem: %d variants set, want one", set)
	}
	switch {
	case v.Created != nil:
		return json.Marshal(struct {
			Type string `json:"type"`
			*ListResponseItemCreated
		}{"CREATED", v.Created})
	case v.Deleted != nil:
		return json.Marshal(struct {
			Type string `json:"type"`
			*ListResponseItemDeleted
		}{"DELETED", v.Deleted})
	}
	return nil, fmt.Errorf("ListResponseItem: no variant set")
}

func (v *ListResponseItem) UnmarshalJSON(b []byte) error {
	var tag struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(b, &tag); err != nil {
		return err
	}
	if tag.Type == "" {
		return fmt.Errorf("ListResponseItem: missing type")
	}
	*v = ListResponseItem{}
	switch tag.Type {
	case "CREATED":
		v.Created = &ListResponseItemCreated{}
		return json.Unmarshal(b, v.Created)
	case "DELETED":
		v.Deleted = &ListResponseItemDeleted{}
		return json.Unmarshal(b, v.Deleted)
	}
	return nil
}

type ListResponseItem struct {
	Created *ListResponseItemCreated
	Deleted *ListResponseItemDeleted
}

type PublishRequestEventCreated struct {
	ID string `json:"id"`
}

func (v PublishRequestEvent) MarshalJSON() ([]byte, error) {
	set := 0
	if v.Created != nil {
		set++
	}
	if set > 1 {
		return nil, fmt.Errorf("PublishRequestEvent: %d variants set, want one", set)
	}
	switch {
	case v.Created != nil:
		return json.Marshal(struct {
			Type string `json:"type"`
			*PublishRequestEventCreated
		}{"CREATED", v.Created})
	}
	return nil, fmt.Errorf("PublishRequestEvent: no variant set")
}

func (v *PublishRequestEvent) UnmarshalJSON(b []byte) error {
	var tag struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(b, &tag); err != nil {
		return err
	}
	if tag.Type == "" {
		return fmt.Errorf("PublishRequestEvent: missing type")
	}
	*v = PublishRequestEvent{}
	switch tag.Type {
	case "CREATED":
		v.Created = &PublishRequestEventCreated{}
		return json.Unmarshal(b, v.Created)
	}
	return nil
}

type PublishRequestEvent struct {
	Created *PublishRequestEventCreated
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
