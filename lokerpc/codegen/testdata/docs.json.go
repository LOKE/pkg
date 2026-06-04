package docstest

import (
	"context"

	"github.com/LOKE/pkg/lokerpc"
)

// GetUserRequest is the request type for GetUser.
type GetUserRequest struct {
	// The unique identifier of the user.
	ID string `json:"id"`
}

// User represents a user account.
type User struct {
	ID string `json:"id"`
	// The display name of the user.
	Name string `json:"name"`
	// Email address. Optional.
	Email string `json:"email,omitempty"`
}

// A service for testing doc comments
type DocsTestService interface {
	// Get a user by ID
	GetUser(context.Context, GetUserRequest) (*User, error)
}

// A service for testing doc comments
type DocsTestRPCClient struct {
	lokerpc.Client
}

func (c DocsTestRPCClient) GetUser(ctx context.Context, req GetUserRequest) (*User, error) {
	var res User
	err := c.DoRequest(ctx, "getUser", req, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}
