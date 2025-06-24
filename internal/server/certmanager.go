package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"sync"
	"time"

	"github.com/caddyserver/certmagic"

	"discobox/internal/types"
)

// CertManager manages TLS certificates with automatic renewal
type CertManager struct {
	config      *types.ProxyConfig
	logger      types.Logger
	cache       sync.Map // domain -> *tls.Certificate
	certMagic   *certmagic.Config
	staticCerts map[string]*tls.Certificate
}

// NewCertManager creates a new certificate manager
func NewCertManager(config *types.ProxyConfig, logger types.Logger) (*CertManager, error) {
	cm := &CertManager{
		config:      config,
		logger:      logger,
		staticCerts: make(map[string]*tls.Certificate),
	}

	// Initialize CertMagic if ACME is enabled
	if config.TLS.AutoCert {
		certMagicConfig := certmagic.NewDefault()

		// Configure ACME
		cacheDir := config.TLS.CacheDir
		if cacheDir == "" {
			// Default to a reasonable location
			cacheDir = "/var/cache/discobox/certs"
		}
		certMagicConfig.Storage = &certmagic.FileStorage{Path: cacheDir}

		// Set up the issuer with email
		acmeIssuer := certmagic.NewACMEIssuer(certMagicConfig, certmagic.ACMEIssuer{
			Email:  config.TLS.Email,
			Agreed: true,
		})
		certMagicConfig.Issuers = []certmagic.Issuer{acmeIssuer}

		cm.certMagic = certMagicConfig

		// Pre-manage domains if specified
		if len(config.TLS.Domains) > 0 {
			ctx := context.Background()
			err := cm.certMagic.ManageAsync(ctx, config.TLS.Domains)
			if err != nil {
				return nil, fmt.Errorf("failed to manage domains: %w", err)
			}
		}
	}

	// Load static certificates if not using ACME
	if !config.TLS.AutoCert && config.TLS.CertFile != "" {
		cert, err := tls.LoadX509KeyPair(config.TLS.CertFile, config.TLS.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load certificate: %w", err)
		}

		// Extract domains from certificate
		if len(cert.Certificate) > 0 {
			// Parse the certificate to extract domains
			x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
			if err != nil {
				return nil, fmt.Errorf("failed to parse certificate: %w", err)
			}

			// Add certificate for the common name
			if x509Cert.Subject.CommonName != "" {
				cm.staticCerts[x509Cert.Subject.CommonName] = &cert
			}

			// Add certificate for all DNS SANs
			for _, dnsName := range x509Cert.DNSNames {
				cm.staticCerts[dnsName] = &cert
			}

			// If no specific domains found, use as default certificate
			if x509Cert.Subject.CommonName == "" && len(x509Cert.DNSNames) == 0 {
				cm.staticCerts[""] = &cert
			}
		}
	}

	return cm, nil
}

// GetCertificate returns a certificate for the given ClientHelloInfo
func (cm *CertManager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	domain := hello.ServerName
	cm.logger.Debug("Certificate requested", "domain", domain)

	// Check cache first
	if cached, ok := cm.cache.Load(domain); ok {
		cert := cached.(*tls.Certificate)
		// Check if certificate is still valid
		if cm.isCertValid(cert) {
			return cert, nil
		}
		// Remove expired cert from cache
		cm.cache.Delete(domain)
	}

	// If using ACME, get certificate from CertMagic
	if cm.config.TLS.AutoCert && cm.certMagic != nil {
		tlsConfig := cm.certMagic.TLSConfig()
		cert, err := tlsConfig.GetCertificate(hello)
		if err == nil && cert != nil {
			// Cache the certificate
			cm.cache.Store(domain, cert)
			return cert, nil
		}
		cm.logger.Error("Failed to get ACME certificate", "domain", domain, "error", err)
	}

	// Check static certificates
	// First try exact match
	if cert, ok := cm.staticCerts[domain]; ok {
		return cert, nil
	}

	// Try wildcard match
	labels := splitDomain(domain)
	for i := range labels {
		wildcardDomain := "*." + joinDomain(labels[i:])
		if cert, ok := cm.staticCerts[wildcardDomain]; ok {
			// Cache for faster lookup
			cm.cache.Store(domain, cert)
			return cert, nil
		}
	}

	// Return default certificate if available
	if cert, ok := cm.staticCerts[""]; ok {
		return cert, nil
	}

	return nil, fmt.Errorf("no certificate available for domain: %s", domain)
}

// isCertValid checks if a certificate is still valid
func (cm *CertManager) isCertValid(cert *tls.Certificate) bool {
	if cert == nil || len(cert.Certificate) == 0 {
		return false
	}

	// Parse the certificate to check expiry
	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		cm.logger.Debug("Failed to parse certificate for validity check", "error", err)
		return false
	}

	now := time.Now()

	// Check if certificate is within its validity period
	if now.Before(x509Cert.NotBefore) {
		cm.logger.Debug("Certificate not yet valid", "not_before", x509Cert.NotBefore)
		return false
	}

	if now.After(x509Cert.NotAfter) {
		cm.logger.Debug("Certificate expired", "not_after", x509Cert.NotAfter)
		return false
	}

	// Also check if it's close to expiry (within 30 days)
	thirtyDaysBeforeExpiry := x509Cert.NotAfter.Add(-30 * 24 * time.Hour)
	if now.After(thirtyDaysBeforeExpiry) {
		cm.logger.Warn("Certificate expiring soon",
			"domain", x509Cert.Subject.CommonName,
			"expires", x509Cert.NotAfter,
			"days_remaining", int(x509Cert.NotAfter.Sub(now).Hours()/24))
	}

	return true
}

// splitDomain splits a domain into labels
func splitDomain(domain string) []string {
	if domain == "" {
		return nil
	}

	labels := []string{}
	start := 0
	for i := 0; i < len(domain); i++ {
		if domain[i] == '.' {
			labels = append(labels, domain[start:i])
			start = i + 1
		}
	}
	labels = append(labels, domain[start:])
	return labels
}

// joinDomain joins domain labels
func joinDomain(labels []string) string {
	result := ""
	for i, label := range labels {
		if i > 0 {
			result += "."
		}
		result += label
	}
	return result
}

// RefreshCertificates refreshes all managed certificates
func (cm *CertManager) RefreshCertificates() error {
	if !cm.config.TLS.AutoCert || cm.certMagic == nil {
		return nil
	}

	// Force renewal check for all managed certificates
	// CertMagic handles renewal automatically, but this allows manual triggering
	for _, domain := range cm.config.TLS.Domains {
		// Clear from cache to force fresh check
		cm.cache.Delete(domain)
	}

	// CertMagic will automatically renew certificates that are close to expiry
	// when GetCertificate is called next time
	cm.logger.Info("Certificate refresh triggered for all managed domains")

	return nil
}

// Close cleans up the certificate manager
func (cm *CertManager) Close() error {
	// Clear cache
	cm.cache.Range(func(key, value any) bool {
		cm.cache.Delete(key)
		return true
	})

	return nil
}
