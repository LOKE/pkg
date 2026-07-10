package service1

import (
	"context"

	"github.com/LOKE/pkg/lokerpc"
)

// hello
type Service1Service interface {
	// hello1 method
	Hello1(context.Context, any) error
}

// hello
type Service1RPCClient struct {
	lokerpc.Client
}

// hello1 method
func (c Service1RPCClient) Hello1(ctx context.Context, req any) error {
	return c.DoRequest(ctx, "hello1", req, nil)
}
