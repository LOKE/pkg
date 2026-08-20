package pgxctx_test

import (
	"context"
	"time"

	"github.com/LOKE/pkg/pgxctx"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ExampleWithContext() {
	var pool *pgxpool.Pool
	ctx := context.Background()

	pool.QueryRow(pgxctx.WithContext(ctx, "OrderRepo.GetOrder",
		pgxctx.WithSlowQueryThreshold(100*time.Millisecond),
	), `
		SELECT * FROM orders WHERE id = $1
	`, 1)
}
