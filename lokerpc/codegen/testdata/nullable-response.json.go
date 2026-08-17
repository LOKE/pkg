package typed

import (
	"context"

	"github.com/LOKE/pkg/lokerpc"
)

type NullableGadget *struct {
	ID string `json:"id"`
}

type Widget struct {
	ID string `json:"id"`
}

type GetNicknameResponse string

type GetTagsResponse []string

type GetUserRequest struct {
	ID string `json:"id"`
}

type GetUserResponse struct {
	ID string `json:"id"`
}

type TypedService interface {
	GetGadget(context.Context, any) (NullableGadget, error)
	GetNickname(context.Context, any) (*GetNicknameResponse, error)
	GetTags(context.Context, any) (*GetTagsResponse, error)
	GetUser(context.Context, GetUserRequest) (*GetUserResponse, error)
	GetWidget(context.Context, any) (*Widget, error)
}

type TypedRPCClient struct {
	lokerpc.Client
}

func (c TypedRPCClient) GetGadget(ctx context.Context, req any) (NullableGadget, error) {
	var res NullableGadget
	err := c.DoRequest(ctx, "getGadget", req, &res)
	if err != nil {
		return nil, err
	}
	return res, nil
}
func (c TypedRPCClient) GetNickname(ctx context.Context, req any) (*GetNicknameResponse, error) {
	var res *GetNicknameResponse
	err := c.DoRequest(ctx, "getNickname", req, &res)
	if err != nil {
		return nil, err
	}
	return res, nil
}
func (c TypedRPCClient) GetTags(ctx context.Context, req any) (*GetTagsResponse, error) {
	var res *GetTagsResponse
	err := c.DoRequest(ctx, "getTags", req, &res)
	if err != nil {
		return nil, err
	}
	return res, nil
}
func (c TypedRPCClient) GetUser(ctx context.Context, req GetUserRequest) (*GetUserResponse, error) {
	var res *GetUserResponse
	err := c.DoRequest(ctx, "getUser", req, &res)
	if err != nil {
		return nil, err
	}
	return res, nil
}
func (c TypedRPCClient) GetWidget(ctx context.Context, req any) (*Widget, error) {
	var res *Widget
	err := c.DoRequest(ctx, "getWidget", req, &res)
	if err != nil {
		return nil, err
	}
	return res, nil
}
