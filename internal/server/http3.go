package server

import (
	"context"
	"fmt"
	"net"
	
	"crypto/tls"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"net/http"
	
	"discobox/internal/types"
)

// HTTP3Server wraps an HTTP/3 server
type HTTP3Server struct {
	config   *types.ProxyConfig
	logger   types.Logger
	server   *http3.Server
	handler  http.Handler
	quicConf *quic.Config
}

// NewHTTP3Server creates a new HTTP/3 server
func NewHTTP3Server(config *types.ProxyConfig, handler http.Handler, logger types.Logger) (*HTTP3Server, error) {
	if !config.TLS.Enabled {
		return nil, fmt.Errorf("HTTP/3 requires TLS to be enabled")
	}
	
	// Create QUIC configuration
	quicConf := &quic.Config{
		MaxIdleTimeout:        config.IdleTimeout,
		MaxIncomingStreams:    1000,
		MaxIncomingUniStreams: 1000,
		KeepAlivePeriod:       config.Transport.KeepAlive,
		DisablePathMTUDiscovery: false,
		EnableDatagrams:         true,
	}
	
	// Create TLS configuration for QUIC
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS13, // HTTP/3 requires TLS 1.3
		MaxVersion: tls.VersionTLS13,
		NextProtos: []string{"h3"},
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		},
	}
	
	// Load certificates
	if !config.TLS.AutoCert {
		cert, err := tls.LoadX509KeyPair(config.TLS.CertFile, config.TLS.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	
	// Create HTTP/3 server
	h3Server := &http3.Server{
		Handler:    handler,
		TLSConfig:  tlsConfig,
		QUICConfig: quicConf,
	}
	
	return &HTTP3Server{
		config:   config,
		logger:   logger,
		server:   h3Server,
		handler:  handler,
		quicConf: quicConf,
	}, nil
}

// Start starts the HTTP/3 server
func (h3s *HTTP3Server) Start(ctx context.Context) error {
	// Parse address
	host, port, err := net.SplitHostPort(h3s.config.ListenAddr)
	if err != nil {
		return fmt.Errorf("invalid listen address: %w", err)
	}
	
	if host == "" {
		host = "0.0.0.0"
	}
	
	// Create UDP listener for QUIC
	addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, port))
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %w", err)
	}
	
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP: %w", err)
	}
	
	h3s.logger.Info("Starting HTTP/3 server", "addr", addr.String())
	
	// Start serving
	go func() {
		if err := h3s.server.Serve(conn); err != nil {
			h3s.logger.Error("HTTP/3 server error", "error", err)
		}
	}()
	
	// Set Alt-Svc header to advertise HTTP/3
	// This should be added to responses in the HTTP/1.1 and HTTP/2 servers
	h3s.logger.Info("HTTP/3 server started", 
		"addr", addr.String(),
		"alt-svc", fmt.Sprintf(`h3="%s"`, addr.String()),
	)
	
	return nil
}

// Stop stops the HTTP/3 server
func (h3s *HTTP3Server) Stop(ctx context.Context) error {
	h3s.logger.Info("Stopping HTTP/3 server")
	
	// Close the server
	if err := h3s.server.Close(); err != nil {
		return fmt.Errorf("failed to close HTTP/3 server: %w", err)
	}
	
	h3s.logger.Info("HTTP/3 server stopped")
	return nil
}

// SetAltSvcHeader sets the Alt-Svc header to advertise HTTP/3
func SetAltSvcHeader(w http.ResponseWriter, config *types.ProxyConfig) {
	// Only set if HTTP/3 is enabled and TLS is configured
	if config.TLS.Enabled {
		// Extract port from listen address
		_, port, err := net.SplitHostPort(config.ListenAddr)
		if err != nil {
			return
		}
		
		// Set Alt-Svc header
		// ma=86400 means the alternative service is fresh for 24 hours
		w.Header().Set("Alt-Svc", fmt.Sprintf(`h3=":%s"; ma=86400`, port))
	}
}

// CreateHTTP3Transport creates an HTTP/3 client transport
func CreateHTTP3Transport(config *types.ProxyConfig) *http3.RoundTripper {
	return &http3.RoundTripper{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS13,
			MaxVersion: tls.VersionTLS13,
		},
		QUICConfig: &quic.Config{
			MaxIdleTimeout:     config.Transport.IdleConnTimeout,
			KeepAlivePeriod:    config.Transport.KeepAlive,
			HandshakeIdleTimeout: config.Transport.DialTimeout,
		},
		DisableCompression: config.Transport.DisableCompression,
	}
}

// IsHTTP3Request checks if the request is using HTTP/3
func IsHTTP3Request(r *http.Request) bool {
	return r.ProtoMajor == 3
}
