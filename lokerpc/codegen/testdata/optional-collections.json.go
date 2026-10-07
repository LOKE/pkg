package discounts

import (
	"context"
	"time"

	"github.com/LOKE/pkg/lokerpc"
)

type Customer struct {
	CustomerID     string `json:"customerId"`
	LoyaltyPoints  int32  `json:"loyaltyPoints"`
	NullablePoints *int32 `json:"nullablePoints"`
	Uid            string `json:"uid"`
	Age            *int32 `json:"age,omitempty"`
	Coords         *struct {
		Lat float32 `json:"lat"`
		Lng float32 `json:"lng"`
	} `json:"coords,omitempty"`
	Guest        *bool            `json:"guest,omitempty"`
	Name         *string          `json:"name,omitempty"`
	NullableAge  *int32           `json:"nullableAge,omitempty"`
	NullableTags *[]string        `json:"nullableTags,omitempty"`
	Scores       map[string]int32 `json:"scores,omitzero"`
	SeenAt       *time.Time       `json:"seenAt,omitempty"`
	Tags         []string         `json:"tags,omitzero"`
	Tier         *string          `json:"tier,omitempty"`
}

func (x *Customer) GetAge() (v int32) {
	if x != nil && x.Age != nil {
		v = *x.Age
	}
	return v
}

func (x *Customer) GetCoords() (v struct {
	Lat float32 `json:"lat"`
	Lng float32 `json:"lng"`
}) {
	if x != nil && x.Coords != nil {
		v = *x.Coords
	}
	return v
}

func (x *Customer) GetGuest() (v bool) {
	if x != nil && x.Guest != nil {
		v = *x.Guest
	}
	return v
}

func (x *Customer) GetName() (v string) {
	if x != nil && x.Name != nil {
		v = *x.Name
	}
	return v
}

func (x *Customer) GetSeenAt() (v time.Time) {
	if x != nil && x.SeenAt != nil {
		v = *x.SeenAt
	}
	return v
}

func (x *Customer) GetTier() (v string) {
	if x != nil && x.Tier != nil {
		v = *x.Tier
	}
	return v
}

type DiscountsService interface {
	LockDiscount(context.Context, Customer) (*Customer, error)
}

type DiscountsRPCClient struct {
	lokerpc.Client
}

func (c DiscountsRPCClient) LockDiscount(ctx context.Context, req Customer) (*Customer, error) {
	var res Customer
	err := c.DoRequest(ctx, "lockDiscount", req, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}
