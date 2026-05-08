package middleware

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsOpts ... (TODO: fill in documentation)
type MetricsOpts struct {
	Name            string
	Description     string
	ResponseBuckets []float64
}

// Metrics ... (TODO: fill in documentation)
type Metrics struct {
	registry *prometheus.Registry
	counter  *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

// NewServiceMetrics ... (TODO: fill in documentation)
func NewServiceMetrics(opts MetricsOpts) *Metrics {
	registry := prometheus.NewRegistry()

	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: fmt.Sprintf("%s_requests_total", opts.Name),
			Help: opts.Description,
		},
		[]string{"endpoint", "code", "method"},
	)

	// duration is partitioned by the HTTP method and handler. It uses custom
	// buckets based on the expected request duration.
	duration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    fmt.Sprintf("%s_request_duration_seconds", opts.Name),
			Help:    "A histogram of latencies for requests.",
			Buckets: opts.ResponseBuckets,
		},
		[]string{"endpoint", "method"},
	)
	registry.MustRegister(counter)
	registry.MustRegister(duration)

	return &Metrics{
		registry: registry,
		counter:  counter,
		duration: duration,
	}
}

// Endpoint ... (TODO: fill in documentation)
func (m *Metrics) Endpoint(endpoint string, h http.HandlerFunc) http.HandlerFunc {
	return promhttp.InstrumentHandlerDuration(m.duration.MustCurryWith(prometheus.Labels{"endpoint": endpoint}),
		promhttp.InstrumentHandlerCounter(m.counter.MustCurryWith(prometheus.Labels{"endpoint": endpoint}), h),
	)
}

// Handler ... (TODO: fill in documentation)
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}
