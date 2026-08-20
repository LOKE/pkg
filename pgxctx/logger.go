package pgxctx

import (
	"context"
	"time"

	"github.com/LOKE/pkg/requestid"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
)

// NewLogger returns a new pgx.QueryTracer that logs slow queries and query
// durations using the given logger and prometheus registerer.
// The returned tracer should be set on the pgx connection pool.
func NewLogger(logger log.Logger, reg prometheus.Registerer) (pgx.QueryTracer, error) {
	latency := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "pgx_sql_query_duration_seconds",
		Help: "Duration of sql requests",
	}, []string{"name"})

	if err := reg.Register(latency); err != nil {
		return nil, err
	}

	return &slowQueryLogger{
		logger:  logger,
		latency: latency,
	}, nil
}

type ctxKey string

const loggerKey ctxKey = "queryStart"

type logData struct {
	start time.Time
	query string
}

type slowQueryLogger struct {
	logger  log.Logger
	latency *prometheus.HistogramVec
}

func (s slowQueryLogger) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, loggerKey, logData{time.Now(), data.SQL})
}

func (s slowQueryLogger) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	data, ok := ctx.Value(loggerKey).(logData)
	if !ok {
		return
	}

	ctxdata, _ := ctx.Value(ctxDataKey).(ctxData)

	slow := 200 * time.Millisecond
	if ctxdata.slowQueryThreshold != 0 {
		slow = ctxdata.slowQueryThreshold
	}

	queryName := "<unknown>"
	if ctxdata.name != "" {
		queryName = ctxdata.name
	}

	dur := time.Since(data.start)
	if dur > slow {
		reqID, ok := requestid.FromContext(ctx)
		reqidstr := "<none>"
		if ok {
			reqidstr = reqID.String()
		}

		level.Warn(s.logger).Log(
			"msg", "slow sql query",
			"req_id", reqidstr,
			"name", queryName,
			"query", data.query,
			"time", dur,
		)
	}

	s.latency.WithLabelValues(queryName).Observe(dur.Seconds())
}

// TODO: probably want to support tracing batch queries in the future
