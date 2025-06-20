package middleware

import (
	"strconv"
	"time"
	
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
	"discobox/internal/types"

)

var (
	// Request metrics
	requestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "discobox_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)
	
	requestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "discobox_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "status"},
	)
	
	requestSize = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Name: "discobox_request_size_bytes",
			Help: "HTTP request size in bytes",
		},
		[]string{"method", "path"},
	)
	
	responseSize = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Name: "discobox_response_size_bytes",
			Help: "HTTP response size in bytes",
		},
		[]string{"method", "path", "status"},
	)
	
	// Active connections
	activeConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "discobox_active_connections",
			Help: "Number of active connections",
		},
	)
	
	// Backend metrics
	backendRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "discobox_backend_requests_total",
			Help: "Total backend requests",
		},
		[]string{"backend", "status"},
	)
	
	backendLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "discobox_backend_latency_seconds",
			Help:    "Backend response latency",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"backend"},
	)
)

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
			activeConnections.Inc()
			defer activeConnections.Dec()
			
			// Record request size
			if r.ContentLength > 0 {
				requestSize.WithLabelValues(r.Method, r.URL.Path).Observe(float64(r.ContentLength))
			}
			
			// Wrap response writer
			mrw := &metricsResponseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}
			
			// Process request
			next.ServeHTTP(mrw, r)
			
			// Record metrics
			duration := time.Since(start).Seconds()
			status := strconv.Itoa(mrw.statusCode)
			
			requestsTotal.WithLabelValues(r.Method, r.URL.Path, status).Inc()
			requestDuration.WithLabelValues(r.Method, r.URL.Path, status).Observe(duration)
			responseSize.WithLabelValues(r.Method, r.URL.Path, status).Observe(float64(mrw.bytes))
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

// PrometheusMetrics implements types.MetricsCollector using Prometheus
type PrometheusMetrics struct {
	requests         *prometheus.CounterVec
	duration         *prometheus.HistogramVec
	upstreamLatency  *prometheus.HistogramVec
	activeConns      prometheus.Gauge
}

// NewPrometheusMetrics creates a new Prometheus metrics collector
func NewPrometheusMetrics() types.MetricsCollector {
	return &PrometheusMetrics{
		requests: requestsTotal,
		duration: requestDuration,
		upstreamLatency: backendLatency,
		activeConns: activeConnections,
	}
}

// RecordRequest records request metrics
func (pm *PrometheusMetrics) RecordRequest(method, path string, statusCode int, duration time.Duration) {
	status := strconv.Itoa(statusCode)
	pm.requests.WithLabelValues(method, path, status).Inc()
	pm.duration.WithLabelValues(method, path, status).Observe(duration.Seconds())
}

// RecordUpstreamLatency records backend latency
func (pm *PrometheusMetrics) RecordUpstreamLatency(service string, duration time.Duration) {
	pm.upstreamLatency.WithLabelValues(service).Observe(duration.Seconds())
}

// RecordActiveConnections updates connection count
func (pm *PrometheusMetrics) RecordActiveConnections(count int) {
	pm.activeConns.Set(float64(count))
}

// Handler returns the metrics endpoint handler
func (pm *PrometheusMetrics) Handler() http.Handler {
	return promhttp.Handler()
}
