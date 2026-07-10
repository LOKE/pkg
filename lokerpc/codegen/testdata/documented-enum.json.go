package payments

import (
	"context"

	"github.com/LOKE/pkg/lokerpc"
)

// Currency is a supported payment currency.
type Currency string

const (
	CurrencyAUD Currency = "AUD"
	CurrencyNZD Currency = "NZD"
)

// Payment describes a payment.
type Payment struct {
	// Currency is used for the payment.
	Currency Currency `json:"currency"`
}

// PaymentsService manages payments.
type PaymentsService interface {
	// CreatePayment creates a payment.
	CreatePayment(context.Context, Payment) (*Payment, error)
}

// PaymentsService manages payments.
type PaymentsRPCClient struct {
	lokerpc.Client
}

// CreatePayment creates a payment.
func (c PaymentsRPCClient) CreatePayment(ctx context.Context, req Payment) (*Payment, error) {
	var res Payment
	err := c.DoRequest(ctx, "createPayment", req, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}
