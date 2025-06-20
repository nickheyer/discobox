// Package config provides configuration management for Discobox
package config

import (
	"github.com/spf13/viper"
)

// setDefaults sets default configuration values
func setDefaults() {
	// Server defaults
	viper.SetDefault("listen_addr", ":8080")
	viper.SetDefault("read_timeout", "30s")
	viper.SetDefault("write_timeout", "30s")
	viper.SetDefault("idle_timeout", "120s")
	viper.SetDefault("shutdown_timeout", "30s")

	// TLS defaults
	viper.SetDefault("tls.enabled", false)
	viper.SetDefault("tls.min_version", "1.2")

	// HTTP/2 defaults
	viper.SetDefault("http2.enabled", true)

	// Transport defaults
	viper.SetDefault("transport.max_idle_conns", 100)
	viper.SetDefault("transport.max_idle_conns_per_host", 10)
	viper.SetDefault("transport.max_conns_per_host", 100)
	viper.SetDefault("transport.idle_conn_timeout", "90s")
	viper.SetDefault("transport.dial_timeout", "30s")
	viper.SetDefault("transport.keep_alive", "30s")
	viper.SetDefault("transport.buffer_size", 32768)

	// Load balancing defaults
	viper.SetDefault("load_balancing.algorithm", "round_robin")
	viper.SetDefault("load_balancing.sticky.enabled", false)
	viper.SetDefault("load_balancing.sticky.cookie_name", "lb_session")
	viper.SetDefault("load_balancing.sticky.ttl", "30m")

	// Health check defaults
	viper.SetDefault("health_check.interval", "10s")
	viper.SetDefault("health_check.timeout", "5s")
	viper.SetDefault("health_check.fail_threshold", 3)
	viper.SetDefault("health_check.pass_threshold", 2)

	// Circuit breaker defaults
	viper.SetDefault("circuit_breaker.enabled", true)
	viper.SetDefault("circuit_breaker.failure_threshold", 5)
	viper.SetDefault("circuit_breaker.success_threshold", 2)
	viper.SetDefault("circuit_breaker.timeout", "60s")

	// Rate limiting defaults
	viper.SetDefault("rate_limit.enabled", false)
	viper.SetDefault("rate_limit.rps", 100)
	viper.SetDefault("rate_limit.burst", 200)

	// Middleware defaults
	viper.SetDefault("middleware.compression.enabled", true)
	viper.SetDefault("middleware.compression.level", 5)
	viper.SetDefault("middleware.headers.security", true)

	// Logging defaults
	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.format", "json")
	viper.SetDefault("logging.access_logs", true)

	// Metrics defaults
	viper.SetDefault("metrics.enabled", true)
	viper.SetDefault("metrics.path", "/metrics")

	// Storage defaults
	viper.SetDefault("storage.type", "sqlite")
	viper.SetDefault("storage.dsn", "discobox.db")

	// API defaults
	viper.SetDefault("api.enabled", true)
	viper.SetDefault("api.addr", ":8081")
	viper.SetDefault("api.auth", false)
}
