package payment

import (
	"context"

	"github.com/LOKE/pkg/lokerpc"
)

type Currency string

const (
	CurrencyAUD Currency = "AUD"
	CurrencyNZD Currency = "NZD"
	CurrencyUSD Currency = "USD"
	CurrencyGBP Currency = "GBP"
	CurrencySGD Currency = "SGD"
)

type PaymentState string

const (
	PaymentStateCreated    PaymentState = "created"
	PaymentStateAuthorized PaymentState = "authorized"
	PaymentStateSubmitted  PaymentState = "submitted"
	PaymentStateSettled    PaymentState = "settled"
	PaymentStateFailed     PaymentState = "failed"
	PaymentStateCanceled   PaymentState = "canceled"
	PaymentStateRefunded   PaymentState = "refunded"
	PaymentStateCompleted  PaymentState = "completed"
)

type GetStateRequest struct {
	Amount   int32    `json:"amount"`
	Currency Currency `json:"currency"`
}

type PaymentService interface {
	GetState(context.Context, GetStateRequest) (*PaymentState, error)
}

type PaymentRPCClient struct {
	lokerpc.Client
}

func (c PaymentRPCClient) GetState(ctx context.Context, req GetStateRequest) (*PaymentState, error) {
	var res PaymentState
	err := c.DoRequest(ctx, "getState", req, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}
