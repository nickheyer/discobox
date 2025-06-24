# discobox main entrypoint concept

```go

package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"discobox/internal/balancer"
	"discobox/internal/circuit"
	"discobox/internal/config"
	"discobox/internal/middleware"
	"discobox/internal/middleware/auth"
	"discobox/internal/proxy"
	"discobox/internal/router"
	"discobox/internal/server"
	"discobox/internal/storage"
	"discobox/pkg/api"
	"discobox/pkg/ui"

	"github.com/caddyserver/certmagic"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

func main() {
	var (
		configFile  = flag.String("config", "discobox.yaml", "Configuration file path")
		showVersion = flag.Bool("version", false, "Show version information")
		validate    = flag.Bool("validate", false, "Validate configuration and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("Discobox %s (commit: %s, built: %s)\n", version, commit, buildDate)
		os.Exit(0)
	}

	// Initialize logger
	logger, err := initLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// Load configuration
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	if *validate {
		logger.Info("Configuration is valid")
		os.Exit(0)
	}

	// Initialize components
	app, err := initializeApp(cfg, logger)
	if err != nil {
		logger.Fatal("Failed to initialize application", zap.Error(err))
	}

	// Start configuration watcher
	go app.configWatcher.Watch(context.Background())

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start servers
	errChan := make(chan error, 3)

	// Main proxy server
	go func() {
		logger.Info("Starting proxy server", zap.String("addr", cfg.ListenAddr))
		if err := app.proxyServer.Start(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("proxy server error: %w", err)
		}
	}()

	// Admin API server
	if cfg.API.Enabled {
		go func() {
			logger.Info("Starting admin API", zap.String("addr", cfg.API.Addr))
			if err := app.apiServer.Start(); err != nil && err != http.ErrServerClosed {
				errChan <- fmt.Errorf("API server error: %w", err)
			}
		}()
	}

	// Metrics server
	if cfg.Metrics.Enabled {
		go func() {
			metricsAddr := ":8081" // Default metrics port
			logger.Info("Starting metrics server", zap.String("addr", metricsAddr))
			http.Handle(cfg.Metrics.Path, promhttp.Handler())
			if err := http.ListenAndServe(metricsAddr, nil); err != nil {
				errChan <- fmt.Errorf("metrics server error: %w", err)
			}
		}()
	}

	// Wait for shutdown signal or error
	select {
	case sig := <-sigChan:
		logger.Info("Received shutdown signal", zap.String("signal", sig.String()))
	case err := <-errChan:
		logger.Error("Server error", zap.Error(err))
	}

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()

	logger.Info("Starting graceful shutdown")
	
	// Shutdown servers
	var shutdownErr error
	if err := app.proxyServer.Shutdown(shutdownCtx); err != nil {
		shutdownErr = fmt.Errorf("proxy server shutdown error: %w", err)
	}
	
	if cfg.API.Enabled {
		if err := app.apiServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("API server shutdown error", zap.Error(err))
		}
	}

	// Close storage
	if err := app.storage.Close(); err != nil {
		logger.Error("Storage shutdown error", zap.Error(err))
	}

	if shutdownErr != nil {
		logger.Error("Shutdown completed with errors", zap.Error(shutdownErr))
		os.Exit(1)
	}

	logger.Info("Shutdown completed successfully")
}

type application struct {
	proxyServer   *server.ProxyServer
	apiServer     *api.Server
	configWatcher *config.Watcher
	storage       storage.Storage
	logger        *zap.Logger
}

func initializeApp(cfg *config.ProxyConfig, logger *zap.Logger) (*application, error) {
	// Initialize storage
	store, err := initStorage(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	// Initialize load balancer
	lb, err := initLoadBalancer(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize load balancer: %w", err)
	}

	// Initialize health checker
	healthChecker := circuit.NewHealthChecker(
		cfg.HealthCheck.Interval,
		cfg.HealthCheck.Timeout,
		cfg.HealthCheck.FailThreshold,
		cfg.HealthCheck.PassThreshold,
		logger,
	)

	// Initialize circuit breaker
	var breaker circuit.CircuitBreaker
	if cfg.CircuitBreaker.Enabled {
		breaker = circuit.NewCircuitBreaker(
			cfg.CircuitBreaker.FailureThreshold,
			cfg.CircuitBreaker.SuccessThreshold,
			cfg.CircuitBreaker.Timeout,
		)
	}

	// Initialize router
	router := router.NewRouter(store, logger)

	// Initialize URL rewriter
	rewriter := proxy.NewURLRewriter()

	// Initialize transport
	transport := proxy.NewTransport(cfg.Transport)

	// Initialize proxy
	reverseProxy := proxy.New(
		proxy.WithLoadBalancer(lb),
		proxy.WithHealthChecker(healthChecker),
		proxy.WithCircuitBreaker(breaker),
		proxy.WithRouter(router),
		proxy.WithRewriter(rewriter),
		proxy.WithTransport(transport),
		proxy.WithLogger(logger),
	)

	// Build middleware chain
	handler := buildMiddlewareChain(cfg, reverseProxy, logger)

	// Initialize servers
	proxyServer, err := server.NewProxyServer(cfg, handler, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize proxy server: %w", err)
	}

	// Initialize API server
	var apiServer *api.Server
	if cfg.API.Enabled {
		apiServer = api.NewServer(cfg, store, logger)
	}

	// Initialize config watcher
	configWatcher := config.NewWatcher(cfg.ConfigFile, func(newCfg *config.ProxyConfig) {
		logger.Info("Configuration reloaded")
		// Apply new configuration
		// This would update routes, services, etc.
	})

	return &application{
		proxyServer:   proxyServer,
		apiServer:     apiServer,
		configWatcher: configWatcher,
		storage:       store,
		logger:        logger,
	}, nil
}

func buildMiddlewareChain(cfg *config.ProxyConfig, handler http.Handler, logger *zap.Logger) http.Handler {
	chain := middleware.NewChain()

	// Security headers (outermost)
	if cfg.Middleware.Headers.Security {
		chain.Use(middleware.SecurityHeaders())
	}

	// CORS
	if cfg.Middleware.CORS.Enabled {
		chain.Use(middleware.CORS(cfg.Middleware.CORS))
	}

	// Access logging
	if cfg.Logging.AccessLogs {
		chain.Use(middleware.AccessLogging(logger))
	}

	// Metrics
	if cfg.Metrics.Enabled {
		chain.Use(middleware.Metrics())
	}

	// Rate limiting
	if cfg.RateLimit.Enabled {
		chain.Use(middleware.RateLimit(cfg.RateLimit))
	}

	// Compression
	if cfg.Middleware.Compression.Enabled {
		chain.Use(middleware.Compression(cfg.Middleware.Compression))
	}

	// Authentication (must be after rate limiting)
	if cfg.Middleware.Auth.Basic.Enabled {
		chain.Use(auth.Basic(cfg.Middleware.Auth.Basic))
	}
	if cfg.Middleware.Auth.JWT.Enabled {
		chain.Use(auth.JWT(cfg.Middleware.Auth.JWT))
	}
	if cfg.Middleware.Auth.OAuth2.Enabled {
		chain.Use(auth.OAuth2(cfg.Middleware.Auth.OAuth2))
	}

	// Custom headers (innermost, closest to proxy)
	if len(cfg.Middleware.Headers.Custom) > 0 {
		chain.Use(middleware.CustomHeaders(cfg.Middleware.Headers.Custom))
	}

	return chain.Then(handler)
}

func initStorage(cfg *config.ProxyConfig, logger *zap.Logger) (storage.Storage, error) {
	switch cfg.Storage.Type {
	case "sqlite":
		return storage.NewSQLite(cfg.Storage.DSN, logger)
	case "memory":
		return storage.NewMemory(), nil
	case "etcd":
		return nil, fmt.Errorf("etcd storage not yet implemented")
	default:
		return nil, fmt.Errorf("unknown storage type: %s", cfg.Storage.Type)
	}
}

func initLoadBalancer(cfg *config.ProxyConfig, logger *zap.Logger) (balancer.LoadBalancer, error) {
	var lb balancer.LoadBalancer
	
	switch cfg.LoadBalancing.Algorithm {
	case "round_robin":
		lb = balancer.NewRoundRobin()
	case "weighted":
		lb = balancer.NewWeightedRoundRobin()
	case "least_conn":
		lb = balancer.NewLeastConnections()
	case "ip_hash":
		lb = balancer.NewIPHash()
	default:
		return nil, fmt.Errorf("unknown load balancing algorithm: %s", cfg.LoadBalancing.Algorithm)
	}

	// Wrap with sticky sessions if enabled
	if cfg.LoadBalancing.Sticky.Enabled {
		lb = balancer.NewStickySession(
			lb,
			cfg.LoadBalancing.Sticky.CookieName,
			cfg.LoadBalancing.Sticky.TTL,
		)
	}

	return lb, nil
}

func initLogger() (*zap.Logger, error) {
	// TODO: If implementing this logging use structured JSON logging
	// In development, use console output
	config := zap.NewProductionConfig()
	
	// Customize as needed
	config.OutputPaths = []string{"stdout"}
	config.ErrorOutputPaths = []string{"stderr"}
	
	return config.Build()
}

// initTLS initializes TLS configuration with automatic certificates if enabled
func initTLS(cfg *config.ProxyConfig, logger *zap.Logger) error {
	if !cfg.TLS.Enabled {
		return nil
	}

	if cfg.TLS.AutoCert {
		// Use CertMagic for automatic certificates
		certmagic.DefaultACME.Email = cfg.TLS.Email
		certmagic.DefaultACME.Agreed = true
		
		logger.Info("Automatic TLS enabled", zap.Strings("domains", cfg.TLS.Domains))
		
		// CertMagic will handle everything
		return nil
	}

	// Manual certificate configuration
	if cfg.TLS.CertFile == "" || cfg.TLS.KeyFile == "" {
		return fmt.Errorf("TLS enabled but cert_file or key_file not specified")
	}

	return nil
}

```