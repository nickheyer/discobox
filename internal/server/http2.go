package server

import (
	"crypto/tls"
	"discobox/internal/types"
	"net/http"
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

// EnableHTTP2Push enables HTTP/2 server push
func EnableHTTP2Push(w http.ResponseWriter, config *types.ProxyConfig) {
	if !config.HTTP2.Enabled {
		return
	}
	
	// Check if this is an HTTP/2 connection
	if pusher, ok := w.(http.Pusher); ok {
		// In a real implementation, you would:
		// 1. Analyze the request to determine what resources to push
		// 2. Push those resources proactively
		// For now, this is just a placeholder
		_ = pusher
	}
}

// IsHTTP2Request checks if the request is using HTTP/2
func IsHTTP2Request(r *http.Request) bool {
	return r.ProtoMajor == 2
}

// GetHTTP2Stats returns HTTP/2 statistics for monitoring
type HTTP2Stats struct {
	ActiveStreams   int
	TotalStreams    int64
	BytesReceived   int64
	BytesSent       int64
}

// CollectHTTP2Stats collects HTTP/2 statistics
func CollectHTTP2Stats() *HTTP2Stats {
	// In a real implementation, this would collect actual stats
	// from the HTTP/2 server
	return &HTTP2Stats{
		ActiveStreams:   0,
		TotalStreams:    0,
		BytesReceived:   0,
		BytesSent:       0,
	}
}
