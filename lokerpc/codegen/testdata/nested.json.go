import (
	"github.com/LOKE/pkg/lokerpc"
)

type GetUserRequest struct {
	ID string `json:"id"`
}

type GetUserResponse struct {
	Name     string `json:"name"`
	Comments []*struct {
		Text      string    `json:"text"`
		Timestamp time.Time `json:"timestamp"`
	} `json:"comments"`
}

type NestedService interface {
	GetUser(context.Context, GetUserRequest) (*GetUserResponse, error)
}

type NestedRPCClient struct{}

func (c NestedRPCClient) GetUser(ctx context.Context, req GetUserRequest) (*GetUserResponse, error) {
	var res GetUserResponse
	err := c.DoRequest(ctx, "getUser", req, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}
