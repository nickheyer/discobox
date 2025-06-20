package middleware

import (
	"time"

	"discobox/internal/metrics"
	"discobox/internal/types"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// No longer need local metric definitions - use global metrics collector

// metricsResponseWriter captures metrics data
type metricsResponseWriter struct {
	http.ResponseWriter
	statusCode  int
	bytes       int64
	wroteHeader bool
}

func (mrw *metricsResponseWriter) WriteHeader(code int) {
	if !mrw.wroteHeader {
		mrw.statusCode = code
		mrw.wroteHeader = true
		mrw.ResponseWriter.WriteHeader(code)
	}
}

func (mrw *metricsResponseWriter) Write(b []byte) (int, error) {
	if !mrw.wroteHeader {
		mrw.WriteHeader(http.StatusOK)
	}
	n, err := mrw.ResponseWriter.Write(b)
	mrw.bytes += int64(n)
	return n, err
}

// Metrics creates metrics collection middleware
func Metrics() types.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Track active connections
			metrics.GlobalCollector.IncrementActiveConnections()
			defer metrics.GlobalCollector.DecrementActiveConnections()

			// Wrap response writer
			mrw := &metricsResponseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			// Process request
			next.ServeHTTP(mrw, r)

			// Record metrics
			duration := time.Since(start)

			// Record the request with the global collector
			metrics.GlobalCollector.RecordRequest(r.Method, mrw.statusCode, duration)
		})
	}
}

// CustomMetrics allows custom metric collection
type CustomMetrics struct {
	collector types.MetricsCollector
}

// NewCustomMetrics creates middleware with a custom metrics collector
func NewCustomMetrics(collector types.MetricsCollector) types.Middleware {
	cm := &CustomMetrics{
		collector: collector,
	}
	return cm.Middleware
}

// Middleware returns the middleware handler
func (cm *CustomMetrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer
		mrw := &metricsResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// Process request
		next.ServeHTTP(mrw, r)

		// Record metrics
		duration := time.Since(start)
		cm.collector.RecordRequest(r.Method, r.URL.Path, mrw.statusCode, duration)
	})
}

// Handler returns the Prometheus metrics endpoint handler
func MetricsHandler() http.Handler {
	return promhttp.Handler()
}
