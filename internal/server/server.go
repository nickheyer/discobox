// Package server implements the HTTP/HTTPS server for Discobox
package server

import (
	"context"
	"fmt"
	"net"
	"sync"
	
	"crypto/tls"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"net/http"
	
	"discobox/internal/types"
)

// Server represents the main HTTP/HTTPS server
type Server struct {
	config     *types.ProxyConfig
	handler    http.Handler
	logger     types.Logger
	httpServer *http.Server
	listeners  []net.Listener
	mu         sync.RWMutex
	running    bool
}

// New creates a new server instance
func New(config *types.ProxyConfig, handler http.Handler, logger types.Logger) *Server {
	return &Server{
		config:    config,
		handler:   handler,
		logger:    logger,
		listeners: make([]net.Listener, 0),
	}
}

// Start starts the server
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.running {
		return fmt.Errorf("server already running")
	}
	
	// Create base HTTP server
	s.httpServer = &http.Server{
		Addr:         s.config.ListenAddr,
		Handler:      s.handler,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
		IdleTimeout:  s.config.IdleTimeout,
		ErrorLog:     nil, // Use our logger instead
	}
	
	// Configure TLS if enabled
	if s.config.TLS.Enabled {
		tlsConfig, err := s.createTLSConfig()
		if err != nil {
			return fmt.Errorf("failed to create TLS config: %w", err)
		}
		s.httpServer.TLSConfig = tlsConfig
	}
	
	// Configure HTTP/2
	if s.config.HTTP2.Enabled {
		if err := s.configureHTTP2(); err != nil {
			return fmt.Errorf("failed to configure HTTP/2: %w", err)
		}
	}
	
	// Start listening
	listener, err := net.Listen("tcp", s.config.ListenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.config.ListenAddr, err)
	}
	s.listeners = append(s.listeners, listener)
	
	// Start server
	s.running = true
	go s.serve(listener)
	
	s.logger.Info("Server started", 
		"addr", s.config.ListenAddr,
		"tls", s.config.TLS.Enabled,
		"http2", s.config.HTTP2.Enabled,
	)
	
	return nil
}

// Stop gracefully stops the server
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if !s.running {
		return nil
	}
	
	s.logger.Info("Stopping server")
	
	// Shutdown HTTP server
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown server: %w", err)
	}
	
	// Close all listeners
	for _, listener := range s.listeners {
		if err := listener.Close(); err != nil {
			s.logger.Error("Failed to close listener", "error", err)
		}
	}
	
	s.running = false
	s.logger.Info("Server stopped")
	
	return nil
}

// serve serves requests on the given listener
func (s *Server) serve(listener net.Listener) {
	var err error
	
	if s.config.TLS.Enabled {
		err = s.httpServer.ServeTLS(listener, "", "")
	} else {
		// Support H2C (HTTP/2 without TLS) if enabled
		if s.config.HTTP2.Enabled {
			h2s := &http2.Server{}
			s.httpServer.Handler = h2c.NewHandler(s.handler, h2s)
		}
		err = s.httpServer.Serve(listener)
	}
	
	if err != nil && err != http.ErrServerClosed {
		s.logger.Error("Server error", "error", err)
	}
}

// createTLSConfig creates the TLS configuration
func (s *Server) createTLSConfig() (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: getTLSVersion(s.config.TLS.MinVersion),
		MaxVersion: tls.VersionTLS13,
		CipherSuites: []uint16{
			// TLS 1.3 cipher suites
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_CHACHA20_POLY1305_SHA256,
			
			// TLS 1.2 cipher suites
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		},
		PreferServerCipherSuites: true,
		CurvePreferences: []tls.CurveID{
			tls.X25519,
			tls.CurveP256,
		},
	}
	
	// Load certificates
	if !s.config.TLS.AutoCert {
		cert, err := tls.LoadX509KeyPair(s.config.TLS.CertFile, s.config.TLS.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	
	return tlsConfig, nil
}

// configureHTTP2 configures HTTP/2 support
func (s *Server) configureHTTP2() error {
	h2Server := &http2.Server{
		MaxHandlers:                  0, // Unlimited
		MaxConcurrentStreams:         250,
		MaxReadFrameSize:             1 << 20, // 1MB
		PermitProhibitedCipherSuites: false,
		IdleTimeout:                  s.config.IdleTimeout,
	}
	
	if err := http2.ConfigureServer(s.httpServer, h2Server); err != nil {
		return fmt.Errorf("failed to configure HTTP/2: %w", err)
	}
	
	return nil
}

// getTLSVersion converts string TLS version to uint16
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

// UpdateConfig updates the server configuration
// This doesn't affect the running server, only new connections
func (s *Server) UpdateConfig(config *types.ProxyConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = config
}

// IsRunning returns whether the server is running
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// GetListenAddr returns the actual listen address
func (s *Server) GetListenAddr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if len(s.listeners) > 0 {
		return s.listeners[0].Addr().String()
	}
	
	return s.config.ListenAddr
}
