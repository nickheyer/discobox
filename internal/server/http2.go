package server

import (
	"crypto/tls"
	"discobox/internal/types"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// HTTP2Config provides HTTP/2 configuration
type HTTP2Config struct {
	Enabled              bool
	MaxHandlers          int
	MaxConcurrentStreams uint32
	MaxReadFrameSize     uint32
	IdleTimeout          int
}

// ConfigureHTTP2Server configures HTTP/2 for the given server
func ConfigureHTTP2Server(srv *http.Server, config *types.ProxyConfig) error {
	if !config.HTTP2.Enabled {
		return nil
	}
	
	// Create HTTP/2 server configuration
	h2Server := &http2.Server{
		MaxHandlers:                  0,    // Unlimited
		MaxConcurrentStreams:         250,  // Default is 250
		MaxReadFrameSize:             1 << 20, // 1MB
		PermitProhibitedCipherSuites: false,
		IdleTimeout:                  config.IdleTimeout,
	}
	
	// Configure the server for HTTP/2
	if err := http2.ConfigureServer(srv, h2Server); err != nil {
		return err
	}
	
	// If TLS is not enabled, we need to use h2c (HTTP/2 cleartext)
	if !config.TLS.Enabled && srv.Handler != nil {
		srv.Handler = h2c.NewHandler(srv.Handler, h2Server)
	}
	
	return nil
}

// CreateHTTP2Transport creates an HTTP/2 enabled transport
func CreateHTTP2Transport(config *types.ProxyConfig) *http2.Transport {
	return &http2.Transport{
		// Allow HTTP/2 over cleartext TCP
		AllowHTTP: true,
		
		// Reuse connections
		DisableCompression: config.Transport.DisableCompression,
		
		// TLS configuration
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false, // Always verify in production
			MinVersion:         tls.VersionTLS12,
			MaxVersion:         tls.VersionTLS13,
		},
		
		// Connection pooling is handled by the underlying transport
		
		// Timeouts
		ReadIdleTimeout:  config.ReadTimeout,
		WriteByteTimeout: config.WriteTimeout,
		PingTimeout:      config.Transport.DialTimeout,
	}
}

// PushableResource represents a resource that can be pushed via HTTP/2
type PushableResource struct {
	Path    string
	Headers http.Header
}

// PushRules defines rules for HTTP/2 server push
type PushRules struct {
	// Map of request paths to resources that should be pushed
	Rules map[string][]PushableResource
	mu    sync.RWMutex
}

// NewPushRules creates a new push rules instance
func NewPushRules() *PushRules {
	return &PushRules{
		Rules: make(map[string][]PushableResource),
	}
}

// AddRule adds a push rule for a given path
func (pr *PushRules) AddRule(requestPath string, resources []PushableResource) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	pr.Rules[requestPath] = resources
}

// GetResources returns resources to push for a given path
func (pr *PushRules) GetResources(requestPath string) []PushableResource {
	pr.mu.RLock()
	defer pr.mu.RUnlock()
	
	// Exact match
	if resources, ok := pr.Rules[requestPath]; ok {
		return resources
	}
	
	// Check for pattern matches (e.g., *.html)
	for pattern, resources := range pr.Rules {
		if matched, _ := filepath.Match(pattern, requestPath); matched {
			return resources
		}
	}
	
	return nil
}

// DefaultPushRules creates default push rules for common scenarios
func DefaultPushRules() *PushRules {
	rules := NewPushRules()
	
	// Push CSS and JS for HTML pages
	rules.AddRule("*.html", []PushableResource{
		{Path: "/css/style.css", Headers: http.Header{"Accept": []string{"text/css"}}},
		{Path: "/js/app.js", Headers: http.Header{"Accept": []string{"application/javascript"}}},
	})
	
	// Push CSS for index page
	rules.AddRule("/", []PushableResource{
		{Path: "/css/style.css", Headers: http.Header{"Accept": []string{"text/css"}}},
		{Path: "/js/app.js", Headers: http.Header{"Accept": []string{"application/javascript"}}},
		{Path: "/img/logo.png", Headers: http.Header{"Accept": []string{"image/*"}}},
	})
	
	return rules
}

// EnableHTTP2Push enables HTTP/2 server push with intelligent resource pushing
func EnableHTTP2Push(w http.ResponseWriter, r *http.Request, config *types.ProxyConfig, rules *PushRules) {
	if !config.HTTP2.Enabled {
		return
	}
	
	// Check if this is an HTTP/2 connection
	pusher, ok := w.(http.Pusher)
	if !ok {
		return
	}
	
	// Don't push if client sent X-Push-Disabled header
	if r.Header.Get("X-Push-Disabled") != "" {
		return
	}
	
	// Get resources to push based on request path
	resources := rules.GetResources(r.URL.Path)
	if len(resources) == 0 {
		// Auto-detect resources based on content type
		resources = autoDetectPushResources(r)
	}
	
	// Push each resource
	for _, resource := range resources {
		// Don't push if resource was already requested
		if wasPreviouslyRequested(r, resource.Path) {
			continue
		}
		
		// Build push options
		opts := &http.PushOptions{
			Method: "GET",
			Header: resource.Headers,
		}
		
		// Attempt to push the resource
		if err := pusher.Push(resource.Path, opts); err != nil {
			// Log error but don't fail the request
			// Common errors include client refusing push or connection closed
			continue
		}
	}
}

// autoDetectPushResources intelligently detects resources to push
func autoDetectPushResources(r *http.Request) []PushableResource {
	var resources []PushableResource
	
	// Only auto-push for HTML requests
	if !strings.Contains(r.Header.Get("Accept"), "text/html") {
		return resources
	}
	
	// Common resources to push for HTML pages
	basePath := path.Dir(r.URL.Path)
	if basePath == "." {
		basePath = ""
	}
	
	// Try common resource paths
	commonResources := []struct {
		file   string
		accept string
	}{
		{"style.css", "text/css"},
		{"styles.css", "text/css"},
		{"main.css", "text/css"},
		{"app.js", "application/javascript"},
		{"main.js", "application/javascript"},
		{"script.js", "application/javascript"},
	}
	
	for _, res := range commonResources {
		resourcePath := path.Join(basePath, res.file)
		if !strings.HasPrefix(resourcePath, "/") {
			resourcePath = "/" + resourcePath
		}
		
		resources = append(resources, PushableResource{
			Path:    resourcePath,
			Headers: http.Header{"Accept": []string{res.accept}},
		})
	}
	
	return resources
}

// wasPreviouslyRequested checks if a resource was already requested by the client
func wasPreviouslyRequested(r *http.Request, resourcePath string) bool {
	// Check if client sent If-None-Match or If-Modified-Since headers
	// indicating they already have the resource cached
	referer := r.Header.Get("Referer")
	if referer != "" && strings.Contains(referer, resourcePath) {
		return true
	}
	
	// Check for cache headers that indicate client has the resource
	if r.Header.Get("If-None-Match") != "" || r.Header.Get("If-Modified-Since") != "" {
		return true
	}
	
	return false
}

// IsHTTP2Request checks if the request is using HTTP/2
func IsHTTP2Request(r *http.Request) bool {
	return r.ProtoMajor == 2
}

// HTTP2Stats holds HTTP/2 statistics for monitoring
type HTTP2Stats struct {
	ActiveStreams   int32
	TotalStreams    int64
	BytesReceived   int64
	BytesSent       int64
	PushesAttempted int64
	PushesSucceeded int64
	PushesFailed    int64
}

// globalHTTP2Stats holds global HTTP/2 statistics
var globalHTTP2Stats = &HTTP2Stats{}

// HTTP2StatsCollector collects HTTP/2 statistics
type HTTP2StatsCollector struct {
	stats *HTTP2Stats
}

// NewHTTP2StatsCollector creates a new stats collector
func NewHTTP2StatsCollector() *HTTP2StatsCollector {
	return &HTTP2StatsCollector{
		stats: globalHTTP2Stats,
	}
}

// IncrementActiveStreams increments active stream count
func (c *HTTP2StatsCollector) IncrementActiveStreams() {
	atomic.AddInt32(&c.stats.ActiveStreams, 1)
}

// DecrementActiveStreams decrements active stream count
func (c *HTTP2StatsCollector) DecrementActiveStreams() {
	atomic.AddInt32(&c.stats.ActiveStreams, -1)
}

// RecordStream records a new stream
func (c *HTTP2StatsCollector) RecordStream() {
	atomic.AddInt64(&c.stats.TotalStreams, 1)
}

// RecordBytesReceived records bytes received
func (c *HTTP2StatsCollector) RecordBytesReceived(bytes int64) {
	atomic.AddInt64(&c.stats.BytesReceived, bytes)
}

// RecordBytesSent records bytes sent
func (c *HTTP2StatsCollector) RecordBytesSent(bytes int64) {
	atomic.AddInt64(&c.stats.BytesSent, bytes)
}

// RecordPushAttempt records a push attempt
func (c *HTTP2StatsCollector) RecordPushAttempt(success bool) {
	atomic.AddInt64(&c.stats.PushesAttempted, 1)
	if success {
		atomic.AddInt64(&c.stats.PushesSucceeded, 1)
	} else {
		atomic.AddInt64(&c.stats.PushesFailed, 1)
	}
}

// CollectHTTP2Stats returns current HTTP/2 statistics
func CollectHTTP2Stats() *HTTP2Stats {
	return &HTTP2Stats{
		ActiveStreams:   atomic.LoadInt32(&globalHTTP2Stats.ActiveStreams),
		TotalStreams:    atomic.LoadInt64(&globalHTTP2Stats.TotalStreams),
		BytesReceived:   atomic.LoadInt64(&globalHTTP2Stats.BytesReceived),
		BytesSent:       atomic.LoadInt64(&globalHTTP2Stats.BytesSent),
		PushesAttempted: atomic.LoadInt64(&globalHTTP2Stats.PushesAttempted),
		PushesSucceeded: atomic.LoadInt64(&globalHTTP2Stats.PushesSucceeded),
		PushesFailed:    atomic.LoadInt64(&globalHTTP2Stats.PushesFailed),
	}
}

// ResetHTTP2Stats resets all statistics
func ResetHTTP2Stats() {
	atomic.StoreInt32(&globalHTTP2Stats.ActiveStreams, 0)
	atomic.StoreInt64(&globalHTTP2Stats.TotalStreams, 0)
	atomic.StoreInt64(&globalHTTP2Stats.BytesReceived, 0)
	atomic.StoreInt64(&globalHTTP2Stats.BytesSent, 0)
	atomic.StoreInt64(&globalHTTP2Stats.PushesAttempted, 0)
	atomic.StoreInt64(&globalHTTP2Stats.PushesSucceeded, 0)
	atomic.StoreInt64(&globalHTTP2Stats.PushesFailed, 0)
}

// HTTP2StatsMiddleware creates middleware that tracks HTTP/2 statistics
func HTTP2StatsMiddleware(collector *HTTP2StatsCollector) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only track HTTP/2 requests
			if !IsHTTP2Request(r) {
				next.ServeHTTP(w, r)
				return
			}
			
			// Track active streams
			collector.IncrementActiveStreams()
			defer collector.DecrementActiveStreams()
			
			// Record new stream
			collector.RecordStream()
			
			// Track bytes received from request
			if r.ContentLength > 0 {
				collector.RecordBytesReceived(r.ContentLength)
			}
			
			// Wrap response writer to track bytes sent
			wrapped := &statsResponseWriter{
				ResponseWriter: w,
				collector:      collector,
			}
			
			next.ServeHTTP(wrapped, r)
		})
	}
}

// statsResponseWriter wraps http.ResponseWriter to track bytes sent
type statsResponseWriter struct {
	http.ResponseWriter
	collector    *HTTP2StatsCollector
	bytesWritten int64
}

func (w *statsResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytesWritten += int64(n)
	w.collector.RecordBytesSent(int64(n))
	return n, err
}
