package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/caddyserver/certmagic"

	"discobox/internal/types"
)

// ACMEManager manages automatic certificate provisioning via ACME
type ACMEManager struct {
	config    *types.ProxyConfig
	logger    types.Logger
	certmagic *certmagic.Config
}

// NewACMEManager creates a new ACME manager
func NewACMEManager(config *types.ProxyConfig, logger types.Logger) (*ACMEManager, error) {
	if !config.TLS.AutoCert {
		return nil, fmt.Errorf("ACME is not enabled")
	}
	
	// Configure CertMagic
	cmConfig := certmagic.NewDefault()
	cmConfig.Storage = &certmagic.FileStorage{Path: "./certs"}
	
	// Set email for ACME account
	if config.TLS.Email != "" {
		cmConfig.DefaultServerName = config.TLS.Email
		certmagic.DefaultACME.Email = config.TLS.Email
	}
	
	// Configure ACME settings
	certmagic.DefaultACME.Agreed = true
	certmagic.DefaultACME.DisableHTTPChallenge = false
	certmagic.DefaultACME.DisableTLSALPNChallenge = false
	
	return &ACMEManager{
		config:    config,
		logger:    logger,
		certmagic: cmConfig,
	}, nil
}

// GetTLSConfig returns a TLS configuration with ACME support
func (am *ACMEManager) GetTLSConfig() (*tls.Config, error) {
	// Create base TLS config
	tlsConfig := &tls.Config{
		MinVersion: getTLSVersion(am.config.TLS.MinVersion),
		MaxVersion: tls.VersionTLS13,
		CipherSuites: getSecureCipherSuites(),
		PreferServerCipherSuites: true,
		CurvePreferences: []tls.CurveID{
			tls.X25519,
			tls.CurveP256,
		},
		NextProtos: []string{"h2", "http/1.1"},
	}
	
	// Configure CertMagic for the domains
	tlsConfig.GetCertificate = am.certmagic.GetCertificate
	
	// Manage certificates for configured domains
	if err := am.certmagic.ManageSync(context.Background(), am.config.TLS.Domains); err != nil {
		return nil, fmt.Errorf("failed to manage certificates: %w", err)
	}
	
	am.logger.Info("ACME enabled for domains", "domains", am.config.TLS.Domains)
	
	return tlsConfig, nil
}

// HandleHTTPChallenge handles ACME HTTP-01 challenges
func (am *ACMEManager) HandleHTTPChallenge(w http.ResponseWriter, r *http.Request) bool {
	// Check if this is an ACME challenge request
	if len(r.URL.Path) >= 28 && r.URL.Path[:28] == "/.well-known/acme-challenge/" {
		// CertMagic handles HTTP challenges internally
		// Return true to indicate this was an ACME challenge
		am.logger.Debug("ACME HTTP challenge detected", "path", r.URL.Path)
		return true
	}
	return false
}

// RenewCertificates triggers certificate renewal check
func (am *ACMEManager) RenewCertificates() error {
	am.logger.Info("Checking certificates for renewal")
	
	// CertMagic handles renewal automatically, but we can trigger a check
	for _, domain := range am.config.TLS.Domains {
		cert, err := am.certmagic.CacheManagedCertificate(context.Background(), domain)
		if err != nil {
			am.logger.Error("Failed to check certificate", "domain", domain, "error", err)
			continue
		}
		
		if cert.NeedsRenewal(am.certmagic) {
			am.logger.Info("Certificate needs renewal", "domain", domain)
			// CertMagic will handle renewal automatically
		}
	}
	
	return nil
}

// Cleanup cleans up ACME resources
func (am *ACMEManager) Cleanup() error {
	// CertMagic handles cleanup automatically
	return nil
}
