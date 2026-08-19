package pgxctx_test

import (
	"context"
	"os"

	"github.com/LOKE/pkg/pgxctx"
	"github.com/go-kit/log"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

func Example() {
	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout))

	cfg, err := pgxpool.ParseConfig("postgres://example.com")
	if err != nil {
		panic(err)
	}

	cfg.ConnConfig.Tracer, err = pgxctx.NewLogger(
		log.WithPrefix(logger, "service", "pg-pool"),
		prometheus.DefaultRegisterer,
	)

	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		panic(err)
	}

	prometheus.MustRegister(pgxctx.NewPrometheusCollector(pool))
}
