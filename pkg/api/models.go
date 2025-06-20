package api

import (
	"time"
)

// MetricsData represents the metrics response
type MetricsData struct {
	Uptime       string                 `json:"uptime"`
	Requests     RequestMetrics         `json:"requests"`
	System       SystemMetrics          `json:"system"`
	Services     map[string]ServiceMetrics `json:"services"`
}

// RequestMetrics represents request statistics
type RequestMetrics struct {
	Total        int64   `json:"total"`
	PerSecond    float64 `json:"per_second"`
	Errors       int64   `json:"errors"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	P50LatencyMs float64 `json:"p50_latency_ms"`
	P95LatencyMs float64 `json:"p95_latency_ms"`
	P99LatencyMs float64 `json:"p99_latency_ms"`
	ErrorRate    float64 `json:"error_rate"`
}

// SystemMetrics represents system statistics
type SystemMetrics struct {
	Goroutines   int     `json:"goroutines"`
	MemoryMB     float64 `json:"memory_mb"`
	CPUPercent   float64 `json:"cpu_percent"`
	Connections  int     `json:"connections"`
}

// ServiceMetrics represents per-service statistics
type ServiceMetrics struct {
	Requests     int64   `json:"requests"`
	Errors       int64   `json:"errors"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	HealthStatus string  `json:"health_status"`
}

// ConfigUpdate represents a configuration update request
type ConfigUpdate struct {
	// Partial config updates
	LoadBalancing *struct {
		Algorithm string `json:"algorithm,omitempty"`
	} `json:"load_balancing,omitempty"`
	
	RateLimit *struct {
		Enabled bool `json:"enabled"`
		RPS     int  `json:"rps,omitempty"`
		Burst   int  `json:"burst,omitempty"`
	} `json:"rate_limit,omitempty"`
	
	CircuitBreaker *struct {
		Enabled          bool          `json:"enabled"`
		FailureThreshold int           `json:"failure_threshold,omitempty"`
		SuccessThreshold int           `json:"success_threshold,omitempty"`
		Timeout          time.Duration `json:"timeout,omitempty"`
	} `json:"circuit_breaker,omitempty"`
}
