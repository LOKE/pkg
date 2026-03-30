package service1

import (
	"context"

	"github.com/LOKE/pkg/lokerpc"
)

type Service1Service interface {
	Hello1(context.Context, any) error
}

type Service1RPCClient struct {
	lokerpc.Client
}

func (c Service1RPCClient) Hello1(ctx context.Context, req any) error {
	return c.DoRequest(ctx, "hello1", req, nil)
}
