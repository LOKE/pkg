import (
	"github.com/LOKE/pkg/lokerpc"
)

type User struct {
	Anything     any    `json:"anything"`
	Name         string `json:"name"`
	AnythingElse any    `json:"anythingElse,omitempty"`
}

type GetUserRequest struct {
	ID string `json:"id"`
}

type TypedService interface {
	GetUser(context.Context, GetUserRequest) (*User, error)
}

type TypedRPCClient struct{}

func (c TypedRPCClient) GetUser(ctx context.Context, req GetUserRequest) (*User, error) {
	var res User
	err := c.DoRequest(ctx, "getUser", req, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}
