package optionalrefs

import (
	"context"
	"time"

	"github.com/LOKE/pkg/lokerpc"
)

type Count int32

type CountAlias Count

type CreatedAt time.Time

type Filter struct {
	Count       *Count      `json:"count,omitempty"`
	CountAlias  *CountAlias `json:"countAlias,omitempty"`
	CreatedAt   CreatedAt   `json:"createdAt,omitzero"`
	InlineCount *int32      `json:"inlineCount,omitempty"`
	MaybeCount  MaybeCount  `json:"maybeCount,omitempty"`
	Ratio       Ratio       `json:"ratio,omitempty"`
	Tags        Tags        `json:"tags,omitzero"`
}

type MaybeCount *int32

type Ratio float64

type Tags []string

type SearchResponse []Count

type OptionalRefsService interface {
	Search(context.Context, Filter) (*SearchResponse, error)
}

type OptionalRefsRPCClient struct {
	lokerpc.Client
}

func (c OptionalRefsRPCClient) Search(ctx context.Context, req Filter) (*SearchResponse, error) {
	var res SearchResponse
	err := c.DoRequest(ctx, "search", req, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}
