import (
	"github.com/LOKE/pkg/lokerpc"
)

type Service1Service interface {
	Hello1(context.Context, any) (any, error)
}

type Service1RPCClient struct{}

func (c Service1RPCClient) Hello1(ctx context.Context, req any) (any, error) {
	var res any
	err := c.DoRequest(ctx, "hello1", req, &res)
	if err != nil {
		return nil, err
	}
	return res, nil
}
