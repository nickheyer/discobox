package config

import (
	"fmt"
	"net"
	"strings"
	
	"discobox/internal/types"
)

// Validate validates a ProxyConfig
func Validate(cfg *types.ProxyConfig) error {
	// Validate listen address
	if cfg.ListenAddr == "" {
		return fmt.Errorf("listen_addr is required")
	}
	
	if _, _, err := net.SplitHostPort(cfg.ListenAddr); err != nil {
		// Try adding default port
		if _, _, err := net.SplitHostPort(cfg.ListenAddr + ":80"); err != nil {
			return fmt.Errorf("invalid listen_addr: %w", err)
		}
	}
	
	// Validate timeouts
	if cfg.ReadTimeout <= 0 {
		return fmt.Errorf("read_timeout must be positive")
	}
	
	if cfg.WriteTimeout <= 0 {
		return fmt.Errorf("write_timeout must be positive")
	}
	
	// Validate load balancing
	validAlgorithms := map[string]bool{
		"round_robin": true,
		"weighted":    true,
		"least_conn":  true,
		"ip_hash":     true,
	}
	
	if !validAlgorithms[cfg.LoadBalancing.Algorithm] {
		return fmt.Errorf("invalid load balancing algorithm: %s", cfg.LoadBalancing.Algorithm)
	}
	
	// Validate health check
	if cfg.HealthCheck.Interval <= 0 {
		return fmt.Errorf("health_check.interval must be positive")
	}
	
	if cfg.HealthCheck.Timeout <= 0 {
		return fmt.Errorf("health_check.timeout must be positive")
	}
	
	if cfg.HealthCheck.Timeout >= cfg.HealthCheck.Interval {
		return fmt.Errorf("health_check.timeout must be less than interval")
	}
	
	if cfg.HealthCheck.FailThreshold <= 0 {
		return fmt.Errorf("health_check.fail_threshold must be positive")
	}
	
	if cfg.HealthCheck.PassThreshold <= 0 {
		return fmt.Errorf("health_check.pass_threshold must be positive")
	}
	
	// Validate circuit breaker
	if cfg.CircuitBreaker.Enabled {
		if cfg.CircuitBreaker.FailureThreshold <= 0 {
			return fmt.Errorf("circuit_breaker.failure_threshold must be positive")
		}
		
		if cfg.CircuitBreaker.SuccessThreshold <= 0 {
			return fmt.Errorf("circuit_breaker.success_threshold must be positive")
		}
		
		if cfg.CircuitBreaker.Timeout <= 0 {
			return fmt.Errorf("circuit_breaker.timeout must be positive")
		}
	}
	
	// Validate rate limiting
	if cfg.RateLimit.Enabled {
		if cfg.RateLimit.RPS <= 0 {
			return fmt.Errorf("rate_limit.rps must be positive")
		}
		
		if cfg.RateLimit.Burst < cfg.RateLimit.RPS {
			return fmt.Errorf("rate_limit.burst must be >= rps")
		}
	}
	
	// Validate TLS
	if cfg.TLS.Enabled {
		if !cfg.TLS.AutoCert && (cfg.TLS.CertFile == "" || cfg.TLS.KeyFile == "") {
			return fmt.Errorf("tls.cert_file and tls.key_file are required when auto_cert is disabled")
		}
		
		if cfg.TLS.AutoCert && len(cfg.TLS.Domains) == 0 {
			return fmt.Errorf("tls.domains are required when auto_cert is enabled")
		}
		
		validVersions := map[string]bool{
			"1.0": true,
			"1.1": true,
			"1.2": true,
			"1.3": true,
		}
		
		if !validVersions[cfg.TLS.MinVersion] {
			return fmt.Errorf("invalid tls.min_version: %s", cfg.TLS.MinVersion)
		}
	}
	
	// Validate storage
	validStorageTypes := map[string]bool{
		"sqlite": true,
		"memory": true,
		"etcd":   true,
	}
	
	if !validStorageTypes[cfg.Storage.Type] {
		return fmt.Errorf("invalid storage.type: %s", cfg.Storage.Type)
	}
	
	// Validate API
	if cfg.API.Enabled {
		if cfg.API.Addr == "" {
			return fmt.Errorf("api.addr is required when API is enabled")
		}
		
		if _, _, err := net.SplitHostPort(cfg.API.Addr); err != nil {
			// Try adding default port
			if _, _, err := net.SplitHostPort(cfg.API.Addr + ":80"); err != nil {
				return fmt.Errorf("invalid api.addr: %w", err)
			}
		}
	}
	
	// Validate logging
	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
		"fatal": true,
	}
	
	if !validLogLevels[strings.ToLower(cfg.Logging.Level)] {
		return fmt.Errorf("invalid logging.level: %s", cfg.Logging.Level)
	}
	
	validLogFormats := map[string]bool{
		"json": true,
		"text": true,
	}
	
	if !validLogFormats[strings.ToLower(cfg.Logging.Format)] {
		return fmt.Errorf("invalid logging.format: %s", cfg.Logging.Format)
	}
	
	return nil
}
