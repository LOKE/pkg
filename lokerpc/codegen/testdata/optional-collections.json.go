package discounts

import (
	"context"
	"time"

	"github.com/LOKE/pkg/lokerpc"
)

type Customer struct {
	CustomerID    string `json:"customerId"`
	LoyaltyPoints int32  `json:"loyaltyPoints"`
	Uid           string `json:"uid"`
	Age           *int32 `json:"age,omitempty"`
	Coords        struct {
		Lat float32 `json:"lat"`
		Lng float32 `json:"lng"`
	} `json:"coords,omitempty"`
	Guest        bool             `json:"guest,omitempty"`
	Name         string           `json:"name,omitempty"`
	NullableAge  *int32           `json:"nullableAge,omitempty"`
	NullableTags *[]string        `json:"nullableTags,omitempty"`
	Scores       map[string]int32 `json:"scores,omitzero"`
	SeenAt       time.Time        `json:"seenAt,omitzero"`
	Tags         []string         `json:"tags,omitzero"`
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
