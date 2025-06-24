package main

import (
	"context"
	"discobox/internal/balancer"
	"discobox/internal/circuit"
	"discobox/internal/config"
	"discobox/internal/middleware"
	"discobox/internal/proxy"
	"discobox/internal/router"
	"discobox/internal/storage"
	"discobox/internal/types"
	"discobox/pkg/api"
	discobox_ui "discobox/pkg/ui/discobox"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
	"go.uber.org/zap"
)

var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

func main() {
	var (
		configFile  = flag.String("config", "configs/discobox.yml", "Configuration file path")
		showVersion = flag.Bool("version", false, "Show version information")
		validate    = flag.Bool("validate", false, "Validate configuration and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("Discobox %s (commit: %s, built: %s)\n", version, commit, buildDate)
		os.Exit(0)
	}

	// Initialize logger
	zapLogger, err := initLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer zapLogger.Sync()

	// Wrap zap logger to implement types.Logger
	logger := wrapZapLogger(zapLogger)

	// Load configuration
	loader := config.NewLoader(*configFile, logger)
	cfg, err := loader.LoadConfig()
	if err != nil {
		logger.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	if *validate {
		logger.Info("Configuration is valid")
		os.Exit(0)
	}

	// Initialize components
	app, err := initializeApp(cfg, logger, loader)
	if err != nil {
		logger.Error("Failed to initialize application", "error", err)
		os.Exit(1)
	}

	// Setup signal handling
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start servers
	errChan := make(chan error, 3)

	// Main proxy server
	go func() {
		logger.Info("Starting proxy server", "addr", cfg.ListenAddr)
		if err := app.proxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("proxy server error: %w", err)
		}
	}()

	// API server
	if app.apiServer != nil {
		go func() {
			logger.Info("Starting API server", "addr", cfg.API.Addr)
			if err := app.apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errChan <- fmt.Errorf("API server error: %w", err)
			}
		}()
	}

	// Metrics server is now served on the API port, so we don't need a separate server

	// Wait for shutdown signal or error
	select {
	case sig := <-sigChan:
		logger.Info("Received shutdown signal", "signal", sig.String())
	case err := <-errChan:
		logger.Error("Server error", "error", err)
	}

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()

	logger.Info("Starting graceful shutdown")

	// Shutdown servers
	if err := app.proxyServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("Proxy server shutdown error", "error", err)
	}

	if app.apiServer != nil {
		if err := app.apiServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("API server shutdown error", "error", err)
		}
	}

	logger.Info("Shutdown completed successfully")
}

type application struct {
	proxyServer *http.Server
	apiServer   *http.Server
	storage     types.Storage
	logger      types.Logger
}

func initializeApp(cfg *types.ProxyConfig, logger types.Logger, loader *config.Loader) (*application, error) {
	// Initialize storage
	store, err := initStorage(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	// Load bootstrap data from configuration
	if err := loader.LoadBootstrapData(store); err != nil {
		logger.Error("Failed to load bootstrap data", "error", err)
		// Don't fail startup - bootstrap data is optional
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
	var breaker types.CircuitBreaker
	if cfg.CircuitBreaker.Enabled {
		breaker = circuit.NewCircuitBreaker(
			cfg.CircuitBreaker.FailureThreshold,
			cfg.CircuitBreaker.SuccessThreshold,
			cfg.CircuitBreaker.Timeout,
		)
	}

	// Initialize router
	routerImpl := router.NewRouter(store, logger)

	// Initialize URL rewriter
	rewriter := proxy.NewURLRewriter()

	// Initialize transport
	transport := proxy.NewTransport(*cfg)

	// Initialize proxy
	reverseProxy := proxy.New(proxy.Options{
		LoadBalancer:   lb,
		HealthChecker:  healthChecker,
		CircuitBreaker: breaker,
		Router:         routerImpl,
		Rewriter:       rewriter,
		Transport:      transport,
		Logger:         logger,
		Storage:        store,
	})

	// Build middleware chain
	proxyHandler := buildMiddlewareChain(cfg, reverseProxy, logger)

	// Initialize proxy server (NO UI HERE - just proxy)
	proxyServer := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      proxyHandler,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	// Initialize API server if enabled
	var apiServer *http.Server
	if cfg.API.Enabled {
		apiHandler := api.New(store, logger, cfg)

		// Create router for API server
		apiRouter := apiHandler.Router()

		// Add UI to API server if enabled
		if cfg.UI.Enabled {
			// Mount UI at root on the API server
			uiHandler := &spaHandler{
				fs: discobox_ui.GetFileSystem(),
			}

			// Create a new mux that combines API and UI
			combinedMux := http.NewServeMux()
			combinedMux.Handle("/api/", apiRouter)
			combinedMux.Handle("/health", apiRouter)
			combinedMux.Handle("/prometheus/metrics", apiRouter)
			combinedMux.Handle("/", uiHandler)

			apiServer = &http.Server{
				Addr:         cfg.API.Addr,
				Handler:      combinedMux,
				ReadTimeout:  cfg.ReadTimeout,
				WriteTimeout: cfg.WriteTimeout,
				IdleTimeout:  cfg.IdleTimeout,
			}
		} else {
			apiServer = &http.Server{
				Addr:         cfg.API.Addr,
				Handler:      apiRouter,
				ReadTimeout:  cfg.ReadTimeout,
				WriteTimeout: cfg.WriteTimeout,
				IdleTimeout:  cfg.IdleTimeout,
			}
		}
	}

	return &application{
		proxyServer: proxyServer,
		apiServer:   apiServer,
		storage:     store,
		logger:      logger,
	}, nil
}

func buildMiddlewareChain(cfg *types.ProxyConfig, handler http.Handler, logger types.Logger) http.Handler {
	chain := middleware.NewChain()

	// Security headers (outermost)
	if cfg.Middleware.Headers.Security {
		chain.Use(middleware.SecurityHeaders())
	}

	// CORS
	if cfg.Middleware.CORS.Enabled {
		chain.Use(middleware.CORS(*cfg))
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
		chain.Use(middleware.RateLimit(*cfg))
	}

	// Compression
	if cfg.Middleware.Compression.Enabled {
		chain.Use(middleware.Compression(*cfg))
	}

	// Custom headers (innermost, closest to proxy)
	if len(cfg.Middleware.Headers.Custom) > 0 {
		chain.Use(middleware.CustomHeaders(cfg.Middleware.Headers.Custom))
	}

	return chain.Then(handler)
}

func initStorage(cfg *types.ProxyConfig, logger types.Logger) (types.Storage, error) {
	switch cfg.Storage.Type {
	case "sqlite":
		return storage.NewSQLite(cfg.Storage.DSN, logger)
	case "memory":
		return storage.NewMemory(), nil
	default:
		return nil, fmt.Errorf("unknown storage type: %s", cfg.Storage.Type)
	}
}

func initLoadBalancer(cfg *types.ProxyConfig, _ types.Logger) (types.LoadBalancer, error) {
	var lb types.LoadBalancer

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
	config := zap.NewProductionConfig()

	// Customize as needed
	config.OutputPaths = []string{"stdout"}
	config.ErrorOutputPaths = []string{"stderr"}

	return config.Build()
}

// wrapZapLogger wraps zap.Logger to implement types.Logger
func wrapZapLogger(zap *zap.Logger) types.Logger {
	return &zapLoggerWrapper{zap: zap}
}

type zapLoggerWrapper struct {
	zap *zap.Logger
}

func (z *zapLoggerWrapper) Debug(msg string, fields ...interface{}) {
	z.zap.Debug(msg, z.fieldsToZap(fields)...)
}

func (z *zapLoggerWrapper) Info(msg string, fields ...interface{}) {
	z.zap.Info(msg, z.fieldsToZap(fields)...)
}

func (z *zapLoggerWrapper) Warn(msg string, fields ...interface{}) {
	z.zap.Warn(msg, z.fieldsToZap(fields)...)
}

func (z *zapLoggerWrapper) Error(msg string, fields ...interface{}) {
	z.zap.Error(msg, z.fieldsToZap(fields)...)
}

func (z *zapLoggerWrapper) With(fields ...interface{}) types.Logger {
	return &zapLoggerWrapper{zap: z.zap.With(z.fieldsToZap(fields)...)}
}

func (z *zapLoggerWrapper) fieldsToZap(fields []interface{}) []zap.Field {
	var zapFields []zap.Field
	for i := 0; i < len(fields); i += 2 {
		if i+1 < len(fields) {
			key, ok := fields[i].(string)
			if ok {
				zapFields = append(zapFields, zap.Any(key, fields[i+1]))
			}
		}
	}
	return zapFields
}

// uiProxyHandler serves UI when no proxy route matches
type uiProxyHandler struct {
	proxy  http.Handler
	ui     http.Handler
	uiPath string
}

func (h *uiProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check if this is an API request that should be proxied to the API server
	if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/health" {
		// Proxy to API server
		apiURL, _ := url.Parse("http://localhost:8081")
		proxy := httputil.NewSingleHostReverseProxy(apiURL)
		proxy.ServeHTTP(w, r)
		return
	}

	// Create a custom response writer to intercept 404s
	rw := &interceptResponseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
		headers:        make(http.Header),
		body:           make([]byte, 0, 512),
	}

	// Try the proxy first
	h.proxy.ServeHTTP(rw, r)

	// If proxy returned 404, serve UI instead
	if rw.statusCode == http.StatusNotFound {
		// Clear any headers set by proxy
		for k := range w.Header() {
			delete(w.Header(), k)
		}
		// Serve the UI
		h.ui.ServeHTTP(w, r)
		return
	}

	// Otherwise, write the intercepted response
	rw.flush()
}

type interceptResponseWriter struct {
	http.ResponseWriter
	statusCode    int
	headers       http.Header
	body          []byte
	headerWritten bool
	intercepting  bool
}

func (rw *interceptResponseWriter) Header() http.Header {
	if rw.intercepting {
		return rw.headers
	}
	return rw.ResponseWriter.Header()
}

func (rw *interceptResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	if code == http.StatusNotFound {
		// Start intercepting if we get a 404
		rw.intercepting = true
		// Copy current headers
		for k, v := range rw.ResponseWriter.Header() {
			rw.headers[k] = v
		}
		return
	}
	if !rw.headerWritten {
		rw.ResponseWriter.WriteHeader(code)
		rw.headerWritten = true
	}
}

func (rw *interceptResponseWriter) Write(b []byte) (int, error) {
	if !rw.headerWritten && rw.statusCode != http.StatusNotFound {
		rw.WriteHeader(rw.statusCode)
	}

	if rw.intercepting {
		// Buffer the 404 response body
		rw.body = append(rw.body, b...)
		return len(b), nil
	}

	return rw.ResponseWriter.Write(b)
}

func (rw *interceptResponseWriter) flush() {
	if !rw.headerWritten {
		// Copy headers
		for k, v := range rw.headers {
			rw.ResponseWriter.Header()[k] = v
		}
		rw.ResponseWriter.WriteHeader(rw.statusCode)
	}
	if len(rw.body) > 0 {
		rw.ResponseWriter.Write(rw.body)
	}
}

// spaHandler serves the SPA UI, returning index.html for non-existent paths
type spaHandler struct {
	fs http.FileSystem
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Try to open the requested file
	path := r.URL.Path
	if path == "/" {
		path = "/index.html"
	}
	
	file, err := h.fs.Open(path)
	if err != nil {
		// If file doesn't exist, serve index.html for client-side routing
		file, err = h.fs.Open("/index.html")
		if err != nil {
			http.Error(w, "index.html not found", http.StatusInternalServerError)
			return
		}
		// Set path back to "/" so http.ServeContent doesn't get confused
		r.URL.Path = "/"
	}
	defer file.Close()
	
	// Get file info
	stat, err := file.Stat()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	// Serve the file
	if stat.IsDir() {
		// If it's a directory, try index.html
		indexFile, err := h.fs.Open(path + "/index.html")
		if err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		defer indexFile.Close()
		
		indexStat, err := indexFile.Stat()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		
		if seeker, ok := indexFile.(io.ReadSeeker); ok {
			http.ServeContent(w, r, path+"/index.html", indexStat.ModTime(), seeker)
		} else {
			http.Error(w, "File not seekable", http.StatusInternalServerError)
		}
	} else {
		if seeker, ok := file.(io.ReadSeeker); ok {
			http.ServeContent(w, r, path, stat.ModTime(), seeker)
		} else {
			http.Error(w, "File not seekable", http.StatusInternalServerError)
		}
	}
}
