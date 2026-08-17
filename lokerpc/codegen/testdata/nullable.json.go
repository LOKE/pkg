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

type SetNicknameRequest struct {
	ID string `json:"id"`
}

type SetTitleRequest string

type TypedService interface {
	GetGadget(context.Context, any) (NullableGadget, error)
	GetNickname(context.Context, any) (*GetNicknameResponse, error)
	GetTags(context.Context, any) (*GetTagsResponse, error)
	GetUser(context.Context, GetUserRequest) (*GetUserResponse, error)
	GetWidget(context.Context, any) (*Widget, error)
	SetGadget(context.Context, NullableGadget) error
	SetNickname(context.Context, *SetNicknameRequest) error
	SetTitle(context.Context, *SetTitleRequest) error
	SetWidget(context.Context, *Widget) error
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
func (c TypedRPCClient) SetGadget(ctx context.Context, req NullableGadget) error {
	return c.DoRequest(ctx, "setGadget", req, nil)
}
func (c TypedRPCClient) SetNickname(ctx context.Context, req *SetNicknameRequest) error {
	return c.DoRequest(ctx, "setNickname", req, nil)
}
func (c TypedRPCClient) SetTitle(ctx context.Context, req *SetTitleRequest) error {
	return c.DoRequest(ctx, "setTitle", req, nil)
}
func (c TypedRPCClient) SetWidget(ctx context.Context, req *Widget) error {
	return c.DoRequest(ctx, "setWidget", req, nil)
}
