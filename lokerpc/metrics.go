package lokerpc

import "time"

// NewRequestObserver records invocations in the existing RPC metric series.
func NewRequestObserver(handler string) func(failureType string, duration time.Duration) {
	requests := count.WithLabelValues(handler)
	durationMetric := latency.WithLabelValues(handler)
	return func(failureType string, duration time.Duration) {
		requests.Inc()
		durationMetric.Observe(duration.Seconds())
		if failureType != "" {
			failures.WithLabelValues(handler, failureType).Inc()
		}
	}
}
