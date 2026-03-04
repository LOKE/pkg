package typed

import (
	"context"

	"github.com/LOKE/pkg/lokerpc"
)

type GetUserRequest struct {
	Name string `json:"name"`
}

type GetUserResponse struct {
	Name string `json:"name"`
}

type GetUserRequest_ struct {
	ID string `json:"id"`
}

type GetUserResponse_ struct {
	ID string `json:"id"`
}

type TypedService interface {
	GetUser(context.Context, GetUserRequest_) (*GetUserResponse_, error)
}

type TypedRPCClient struct {
	lokerpc.Client
}

func (c TypedRPCClient) GetUser(ctx context.Context, req GetUserRequest_) (*GetUserResponse_, error) {
	var res GetUserResponse_
	err := c.DoRequest(ctx, "getUser", req, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}
