package server

import (
	"fmt"
	
	"crypto/tls"
	
	"discobox/internal/types"
)

// TLSManager manages TLS certificates and configuration
type TLSManager struct {
	config      *types.ProxyConfig
	logger      types.Logger
	certManager *CertManager
}

// NewTLSManager creates a new TLS manager
func NewTLSManager(config *types.ProxyConfig, logger types.Logger) (*TLSManager, error) {
	certManager, err := NewCertManager(config, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate manager: %w", err)
	}
	
	return &TLSManager{
		config:      config,
		logger:      logger,
		certManager: certManager,
	}, nil
}

// GetCertificate returns a certificate for the given ClientHelloInfo
func (tm *TLSManager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	return tm.certManager.GetCertificate(hello)
}

// CreateTLSConfig creates a TLS configuration
func (tm *TLSManager) CreateTLSConfig() (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: getTLSVersion(tm.config.TLS.MinVersion),
		MaxVersion: tls.VersionTLS13,
		
		// Modern cipher suites only
		CipherSuites: getSecureCipherSuites(),
		
		// Prefer server cipher suites for better security
		PreferServerCipherSuites: true,
		
		// Use secure curves
		CurvePreferences: []tls.CurveID{
			tls.X25519,
			tls.CurveP256,
			tls.CurveP384,
		},
		
		// Certificate selection
		GetCertificate: tm.GetCertificate,
		
		// Enable session tickets for performance
		SessionTicketsDisabled: false,
		
		// Set reasonable session cache
		ClientSessionCache: tls.NewLRUClientSessionCache(1000),
	}
	
	// Load static certificates if not using ACME
	if !tm.config.TLS.AutoCert && tm.config.TLS.CertFile != "" {
		cert, err := tls.LoadX509KeyPair(tm.config.TLS.CertFile, tm.config.TLS.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	
	// Configure NextProtos for ALPN
	tlsConfig.NextProtos = []string{"h2", "http/1.1"}
	
	return tlsConfig, nil
}

// getSecureCipherSuites returns a list of secure cipher suites
func getSecureCipherSuites() []uint16 {
	return []uint16{
		// TLS 1.3 cipher suites (automatically selected)
		
		// TLS 1.2 cipher suites
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		
		// Fallback cipher suites (still secure but less preferred)
		tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
	}
}

// ValidateTLSConfig validates the TLS configuration
func ValidateTLSConfig(config *types.ProxyConfig) error {
	if !config.TLS.Enabled {
		return nil
	}
	
	// Validate minimum TLS version
	minVersion := getTLSVersion(config.TLS.MinVersion)
	if minVersion < tls.VersionTLS12 {
		return fmt.Errorf("minimum TLS version should be 1.2 or higher for security")
	}
	
	// Validate certificate configuration
	if !config.TLS.AutoCert {
		if config.TLS.CertFile == "" || config.TLS.KeyFile == "" {
			return fmt.Errorf("cert_file and key_file are required when auto_cert is disabled")
		}
	} else {
		if len(config.TLS.Domains) == 0 {
			return fmt.Errorf("at least one domain is required when auto_cert is enabled")
		}
		
		if config.TLS.Email == "" {
			return fmt.Errorf("email is required for ACME certificate requests")
		}
	}
	
	return nil
}
