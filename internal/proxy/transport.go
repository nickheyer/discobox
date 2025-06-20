package proxy

import (
	"crypto/tls"
	"discobox/internal/types"
	"net"
	"net/http"
	"time"
	"golang.org/x/net/http2"
)

// DefaultTransport returns a configured default transport
func DefaultTransport() http.RoundTripper {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		MaxConnsPerHost:       100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		DisableCompression:    false,
	}
}

// NewTransport creates a new transport with the given configuration
func NewTransport(config types.ProxyConfig) http.RoundTripper {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   config.Transport.DialTimeout,
			KeepAlive: config.Transport.KeepAlive,
		}).DialContext,
		ForceAttemptHTTP2:     config.HTTP2.Enabled,
		MaxIdleConns:          config.Transport.MaxIdleConns,
		MaxIdleConnsPerHost:   config.Transport.MaxIdleConnsPerHost,
		MaxConnsPerHost:       config.Transport.MaxConnsPerHost,
		IdleConnTimeout:       config.Transport.IdleConnTimeout,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		DisableCompression:    config.Transport.DisableCompression,
	}

	// Configure TLS
	if config.TLS.Enabled {
		tlsConfig := &tls.Config{
			MinVersion: getTLSVersion(config.TLS.MinVersion),
		}
		transport.TLSClientConfig = tlsConfig
	}

	// Configure HTTP/2
	if config.HTTP2.Enabled {
		if err := http2.ConfigureTransport(transport); err != nil {
			// Log error but continue
		}
	}

	return transport
}

// NewBackendTransport creates a transport for connecting to backend services
func NewBackendTransport(service *types.Service, config types.ProxyConfig) http.RoundTripper {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   config.Transport.DialTimeout,
			KeepAlive: config.Transport.KeepAlive,
		}).DialContext,
		ForceAttemptHTTP2:     config.HTTP2.Enabled,
		MaxIdleConns:          config.Transport.MaxIdleConns,
		MaxIdleConnsPerHost:   config.Transport.MaxIdleConnsPerHost,
		MaxConnsPerHost:       service.MaxConns,
		IdleConnTimeout:       config.Transport.IdleConnTimeout,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: service.Timeout,
		DisableCompression:    config.Transport.DisableCompression,
	}

	// Configure backend TLS
	if service.TLS != nil {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: service.TLS.InsecureSkipVerify,
			ServerName:         service.TLS.ServerName,
		}

		// Add root CAs if provided
		if len(service.TLS.RootCAs) > 0 {
			// TODO: Parse and add root CAs
		}

		// Add client certificate if provided
		if service.TLS.ClientCert != "" && service.TLS.ClientKey != "" {
			// In production, load and add client certificate
		}

		transport.TLSClientConfig = tlsConfig
	}

	return transport
}

// getTLSVersion converts string TLS version to tls constant
func getTLSVersion(version string) uint16 {
	switch version {
	case "1.0":
		return tls.VersionTLS10
	case "1.1":
		return tls.VersionTLS11
	case "1.2":
		return tls.VersionTLS12
	case "1.3":
		return tls.VersionTLS13
	default:
		return tls.VersionTLS12
	}
}

// PooledTransport wraps a transport with connection pooling
type PooledTransport struct {
	base http.RoundTripper
}

// NewPooledTransport creates a new pooled transport
func NewPooledTransport(base http.RoundTripper) *PooledTransport {
	if base == nil {
		base = DefaultTransport()
	}

	return &PooledTransport{
		base: base,
	}
}

// RoundTrip implements http.RoundTripper
func (t *PooledTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Add pooled transport logic here
	return t.base.RoundTrip(req)
}
