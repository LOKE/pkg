package stripepayments

import (
	"context"

	"github.com/LOKE/pkg/lokerpc"
)

type AccountMetadata struct {
	Environment      string  `json:"Environment"`
	OperatorURL      *string `json:"Operator URL"`
	OrganizationID   string  `json:"Organization ID"`
	OrganizationName string  `json:"Organization Name"`
	LocationID       *string `json:"Location ID,omitempty"`
	LocationName     *string `json:"Location Name,omitempty"`
}

// Test service for AccountMetadata shape
type StripePaymentsService interface {
	// Fetch account metadata
	GetAccountMetadata(context.Context, AccountMetadata) (*AccountMetadata, error)
}

// Test service for AccountMetadata shape
type StripePaymentsRPCClient struct {
	lokerpc.Client
}

func (c StripePaymentsRPCClient) GetAccountMetadata(ctx context.Context, req AccountMetadata) (*AccountMetadata, error) {
	var res AccountMetadata
	err := c.DoRequest(ctx, "getAccountMetadata", req, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}
