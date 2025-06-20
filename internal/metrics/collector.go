package metrics

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	
	"github.com/prometheus/client_golang/prometheus"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// GlobalCollector is the global metrics collector instance
var GlobalCollector *Collector
var once sync.Once

// InitGlobalCollector initializes the global collector (safe to call multiple times)
func InitGlobalCollector() *Collector {
	once.Do(func() {
		GlobalCollector = NewCollector()
	})
	return GlobalCollector
}

// init ensures the global collector is initialized when the package is imported
func init() {
	InitGlobalCollector()
}

// Collector tracks various system and application metrics
type Collector struct {
	// Request counters
	totalRequests   atomic.Uint64
	totalErrors     atomic.Uint64
	activeConns     atomic.Int64
	
	// Latency tracking
	latencies       []float64
	latenciesMu     sync.RWMutex
	
	// System metrics
	cpuPercent      atomic.Value // float64
	memoryUsage     atomic.Value // float64
	
	// Prometheus metrics
	requestsTotal   *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	errorRate       prometheus.Gauge
	
	// Start time for rate calculations
	startTime       time.Time
	lastResetTime   time.Time
	
	// Update goroutine control
	stopCh          chan struct{}
	wg              sync.WaitGroup
}

// NewCollector creates a new metrics collector
func NewCollector() *Collector {
	c := &Collector{
		latencies:     make([]float64, 0, 10000),
		startTime:     time.Now(),
		lastResetTime: time.Now(),
		stopCh:        make(chan struct{}),
		
		requestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "discobox_requests_total",
				Help: "Total number of requests",
			},
			[]string{"method", "status"},
		),
		
		requestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "discobox_request_duration_seconds",
				Help:    "Request duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "status"},
		),
		
		errorRate: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "discobox_error_rate",
				Help: "Current error rate",
			},
		),
	}
	
	// Initialize CPU and memory values
	c.cpuPercent.Store(0.0)
	c.memoryUsage.Store(0.0)
	
	// Register Prometheus metrics - ignore errors if already registered
	_ = prometheus.Register(c.requestsTotal)
	_ = prometheus.Register(c.requestDuration)
	_ = prometheus.Register(c.errorRate)
	
	// Start system metrics updater
	c.startSystemMetricsUpdater()
	
	return c
}

// RecordRequest records a request with its details
func (c *Collector) RecordRequest(method string, statusCode int, duration time.Duration) {
	c.totalRequests.Add(1)
	
	status := "success"
	if statusCode >= 400 {
		c.totalErrors.Add(1)
		status = "error"
	}
	
	// Update Prometheus metrics
	c.requestsTotal.WithLabelValues(method, status).Inc()
	c.requestDuration.WithLabelValues(method, status).Observe(duration.Seconds())
	
	// Store latency for percentile calculations
	c.latenciesMu.Lock()
	c.latencies = append(c.latencies, duration.Seconds()*1000) // Convert to ms
	// Keep only last 10000 entries to prevent unbounded growth
	if len(c.latencies) > 10000 {
		c.latencies = c.latencies[len(c.latencies)-10000:]
	}
	c.latenciesMu.Unlock()
}

// IncrementActiveConnections increments active connection count
func (c *Collector) IncrementActiveConnections() {
	c.activeConns.Add(1)
}

// DecrementActiveConnections decrements active connection count
func (c *Collector) DecrementActiveConnections() {
	c.activeConns.Add(-1)
}

// GetStats returns current statistics
func (c *Collector) GetStats() Stats {
	total := c.totalRequests.Load()
	errors := c.totalErrors.Load()
	
	duration := time.Since(c.lastResetTime).Seconds()
	if duration == 0 {
		duration = 1 // Prevent division by zero
	}
	
	errorRate := 0.0
	if total > 0 {
		errorRate = float64(errors) / float64(total) * 100
	}
	
	// Update error rate gauge
	c.errorRate.Set(errorRate)
	
	return Stats{
		TotalRequests:    total,
		TotalErrors:      errors,
		RequestsPerSec:   float64(total) / duration,
		ErrorRate:        errorRate,
		ActiveConnections: c.activeConns.Load(),
		AvgLatencyMs:     c.calculateAvgLatency(),
		P50LatencyMs:     c.calculatePercentile(50),
		P95LatencyMs:     c.calculatePercentile(95),
		P99LatencyMs:     c.calculatePercentile(99),
		CPUPercent:       c.cpuPercent.Load().(float64),
		MemoryUsageMB:    c.memoryUsage.Load().(float64),
		Uptime:           time.Since(c.startTime),
	}
}

// Stats holds current metrics
type Stats struct {
	TotalRequests     uint64        `json:"total_requests"`
	TotalErrors       uint64        `json:"total_errors"`
	RequestsPerSec    float64       `json:"requests_per_second"`
	ErrorRate         float64       `json:"error_rate"`
	ActiveConnections int64         `json:"active_connections"`
	AvgLatencyMs      float64       `json:"avg_latency_ms"`
	P50LatencyMs      float64       `json:"p50_latency_ms"`
	P95LatencyMs      float64       `json:"p95_latency_ms"`
	P99LatencyMs      float64       `json:"p99_latency_ms"`
	CPUPercent        float64       `json:"cpu_percent"`
	MemoryUsageMB     float64       `json:"memory_usage_mb"`
	Uptime            time.Duration `json:"uptime"`
}

// calculateAvgLatency calculates average latency
func (c *Collector) calculateAvgLatency() float64 {
	c.latenciesMu.RLock()
	defer c.latenciesMu.RUnlock()
	
	if len(c.latencies) == 0 {
		return 0
	}
	
	sum := 0.0
	for _, l := range c.latencies {
		sum += l
	}
	return sum / float64(len(c.latencies))
}

// calculatePercentile calculates the given percentile
func (c *Collector) calculatePercentile(p int) float64 {
	c.latenciesMu.RLock()
	defer c.latenciesMu.RUnlock()
	
	if len(c.latencies) == 0 {
		return 0
	}
	
	// Simple percentile calculation (not exact but good enough)
	index := len(c.latencies) * p / 100
	if index >= len(c.latencies) {
		index = len(c.latencies) - 1
	}
	
	return c.latencies[index]
}

// startSystemMetricsUpdater starts a goroutine to update system metrics
func (c *Collector) startSystemMetricsUpdater() {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				// Update CPU usage
				if percent, err := cpu.Percent(0, false); err == nil && len(percent) > 0 {
					c.cpuPercent.Store(percent[0])
				}
				
				// Update memory usage
				if vmStat, err := mem.VirtualMemory(); err == nil {
					c.memoryUsage.Store(float64(vmStat.Used) / 1024 / 1024) // Convert to MB
				}
				
			case <-c.stopCh:
				return
			}
		}
	}()
}

// GetMemoryStats returns current memory statistics
func (c *Collector) GetMemoryStats() runtime.MemStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m
}

// Reset resets the counters (useful for testing)
func (c *Collector) Reset() {
	c.totalRequests.Store(0)
	c.totalErrors.Store(0)
	c.latenciesMu.Lock()
	c.latencies = c.latencies[:0]
	c.latenciesMu.Unlock()
	c.lastResetTime = time.Now()
}

// Stop stops the metrics collector
func (c *Collector) Stop() {
	close(c.stopCh)
	c.wg.Wait()
}