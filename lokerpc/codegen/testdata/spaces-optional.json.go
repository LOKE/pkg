import (
	"github.com/LOKE/pkg/lokerpc"
)

type AccountMetadata struct {
	OperatorURL      *string `json:"Operator URL"`
	Environment      string  `json:"Environment"`
	OrganizationID   string  `json:"Organization ID"`
	OrganizationName string  `json:"Organization Name"`
	LocationID       *string `json:"Location ID,omitempty"`
	LocationName     *string `json:"Location Name,omitempty"`
}

type StripePaymentsService interface {
	GetAccountMetadata(context.Context, AccountMetadata) (*AccountMetadata, error)
}

type StripePaymentsRPCClient struct{}

func (c StripePaymentsRPCClient) GetAccountMetadata(ctx context.Context, req AccountMetadata) (*AccountMetadata, error) {
	var res AccountMetadata
	err := c.DoRequest(ctx, "getAccountMetadata", req, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}
