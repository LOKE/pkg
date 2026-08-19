package pgxctx

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

func NewPrometheusCollector(pool *pgxpool.Pool) prometheus.Collector {
	return statsCollector{pool: pool}
}

type statsCollector struct {
	pool *pgxpool.Pool
}

func (s statsCollector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(s, ch)
}

func (s statsCollector) Collect(ch chan<- prometheus.Metric) {
	stats := s.pool.Stat()

	// go_sql_connections
	connDesc := prometheus.NewDesc("pgx_connections", "Number of connections constructing", []string{"state"}, nil)
	ch <- prometheus.MustNewConstMetric(
		connDesc,
		prometheus.GaugeValue,
		float64(stats.ConstructingConns()),
		"constructing",
	)
	ch <- prometheus.MustNewConstMetric(
		connDesc,
		prometheus.GaugeValue,
		float64(stats.AcquiredConns()),
		"acquired",
	)
	ch <- prometheus.MustNewConstMetric(
		connDesc,
		prometheus.GaugeValue,
		float64(stats.IdleConns()),
		"idle",
	)

	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc("pgx_connections_acquired_total", "the cumulative count of successful acquires from the pool", nil, nil),
		prometheus.CounterValue,
		float64(stats.AcquireCount()),
	)
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc("pgx_connections_created_total", "the cumulative count of new connections opened", nil, nil),
		prometheus.CounterValue,
		float64(stats.NewConnsCount()),
	)
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc("pgx_connection_acquire_seconds_total", "the total duration of all successful acquires from the pool", nil, nil),
		prometheus.CounterValue,
		stats.AcquireDuration().Seconds(),
	)
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc("pgx_empty_pool_acquires_total", "the cumulative count of successful acquires from the pool that waited for a resource to be released or constructed because the pool was empty", nil, nil),
		prometheus.CounterValue,
		float64(stats.EmptyAcquireCount()),
	)
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc("pgx_max_connections", "the maximum size of the pool", nil, nil),
		prometheus.GaugeValue,
		float64(stats.MaxConns()),
	)
}
