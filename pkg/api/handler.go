// Package api implements the REST API for Discobox
package api

import (
	"context"
	"fmt"
	"regexp"
	"runtime"
	"time"
	
	"encoding/json"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"net/http"
	
	"discobox/internal/metrics"
	"discobox/internal/middleware"
	"discobox/internal/types"
	"discobox/internal/version"
)

// Handler provides the REST API implementation
type Handler struct {
	storage      types.Storage
	logger       types.Logger
	config       *types.ProxyConfig
	configLoader ConfigLoader
	onReload     func(*types.ProxyConfig) error
}

// ConfigLoader defines the interface for loading configuration
type ConfigLoader interface {
	LoadConfig() (*types.ProxyConfig, error)
}

// New creates a new API handler instance
func New(storage types.Storage, logger types.Logger, config *types.ProxyConfig) *Handler {
	return &Handler{
		storage: storage,
		logger:  logger,
		config:  config,
	}
}

// SetConfigLoader sets the configuration loader
func (h *Handler) SetConfigLoader(loader ConfigLoader) {
	h.configLoader = loader
}

// SetReloadCallback sets the reload callback function
func (h *Handler) SetReloadCallback(callback func(*types.ProxyConfig) error) {
	h.onReload = callback
}

// Router returns the HTTP handler for the API
func (h *Handler) Router() http.Handler {
	mainRouter := mux.NewRouter()
	
	// Public endpoints (no auth required)
	publicRouter := mainRouter.PathPrefix("/").Subrouter()
	publicRouter.HandleFunc("/health", h.handleHealth).Methods("GET")
	publicRouter.HandleFunc("/api/v1/auth/login", h.handleLogin).Methods("POST", "OPTIONS")
	
	// Prometheus metrics endpoint (no auth, no JSON middleware)
	if h.config.Metrics.Enabled {
		mainRouter.Handle(h.config.Metrics.Path, middleware.MetricsHandler()).Methods("GET")
	}
	
	// Apply only CORS and logging middleware to public routes
	publicRouter.Use(func(next http.Handler) http.Handler {
		return corsMiddleware(jsonMiddleware(loggingMiddleware(next, h.logger)))
	})
	
	// Protected API endpoints
	apiRouter := mainRouter.PathPrefix("/api/v1").Subrouter()
	
	// Services
	apiRouter.HandleFunc("/services", h.handleListServices).Methods("GET", "OPTIONS")
	apiRouter.HandleFunc("/services", h.handleCreateService).Methods("POST", "OPTIONS")
	apiRouter.HandleFunc("/services/{id}", h.handleGetService).Methods("GET", "OPTIONS")
	apiRouter.HandleFunc("/services/{id}", h.handleUpdateService).Methods("PUT", "OPTIONS")
	apiRouter.HandleFunc("/services/{id}", h.handleDeleteService).Methods("DELETE", "OPTIONS")
	
	// Routes
	apiRouter.HandleFunc("/routes", h.handleListRoutes).Methods("GET", "OPTIONS")
	apiRouter.HandleFunc("/routes", h.handleCreateRoute).Methods("POST", "OPTIONS")
	apiRouter.HandleFunc("/routes/{id}", h.handleGetRoute).Methods("GET", "OPTIONS")
	apiRouter.HandleFunc("/routes/{id}", h.handleUpdateRoute).Methods("PUT", "OPTIONS")
	apiRouter.HandleFunc("/routes/{id}", h.handleDeleteRoute).Methods("DELETE", "OPTIONS")
	
	// Metrics (JSON format for UI)
	apiRouter.HandleFunc("/stats", h.handleMetrics).Methods("GET", "OPTIONS")
	
	// Users
	apiRouter.HandleFunc("/users", h.handleListUsers).Methods("GET", "OPTIONS")
	apiRouter.HandleFunc("/users", h.handleCreateUser).Methods("POST", "OPTIONS")
	apiRouter.HandleFunc("/users/{id}", h.handleGetUser).Methods("GET", "OPTIONS")
	apiRouter.HandleFunc("/users/{id}", h.handleUpdateUser).Methods("PUT", "OPTIONS")
	apiRouter.HandleFunc("/users/{id}", h.handleDeleteUser).Methods("DELETE", "OPTIONS")
	apiRouter.HandleFunc("/users/{id}/password", h.handleChangePassword).Methods("POST", "OPTIONS")
	apiRouter.HandleFunc("/users/{id}/api-keys", h.handleListUserAPIKeys).Methods("GET", "OPTIONS")
	apiRouter.HandleFunc("/users/{id}/api-keys", h.handleCreateAPIKey).Methods("POST", "OPTIONS")
	
	// API Keys
	apiRouter.HandleFunc("/api-keys/{key}", h.handleRevokeAPIKey).Methods("DELETE", "OPTIONS")
	
	// Auth (whoami is protected, login is public)
	apiRouter.HandleFunc("/auth/whoami", h.handleWhoAmI).Methods("GET", "OPTIONS")
	
	// Admin endpoints - create a subrouter for admin-only endpoints
	adminRouter := apiRouter.PathPrefix("/admin").Subrouter()
	adminRouter.HandleFunc("/reload", h.handleReload).Methods("POST", "OPTIONS")
	adminRouter.HandleFunc("/config", h.handleGetConfig).Methods("GET", "OPTIONS")
	adminRouter.HandleFunc("/config", h.handleUpdateConfig).Methods("PUT", "OPTIONS")
	
	// Apply common middleware to API routes first
	apiRouter.Use(func(next http.Handler) http.Handler {
		return loggingMiddleware(next, h.logger)
	})
	apiRouter.Use(func(next http.Handler) http.Handler {
		return corsMiddleware(next)
	})
	apiRouter.Use(func(next http.Handler) http.Handler {
		return jsonMiddleware(next)
	})
	
	// Apply auth middleware to API routes last
	if h.config.API.Auth {
		// Use storage-based authentication
		apiRouter.Use(func(next http.Handler) http.Handler {
			return storageAuthMiddleware(next, h.storage, h.logger)
		})
		
		// If static API key is configured, also allow that
		if h.config.API.APIKey != "" {
			authConfig := &AuthConfig{
				Enabled: true,
				Type:    "api-key",
				Token:   h.config.API.APIKey,
				HeaderName: "X-API-Key",
			}
			apiRouter.Use(func(next http.Handler) http.Handler {
				return authMiddleware(next, authConfig)
			})
		}
		
		// Apply admin middleware to admin routes after auth
		adminRouter.Use(requireAdminMiddleware)
	}
	
	return mainRouter
}

// Health endpoint handlers

// handleHealth handles GET /health
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	versionInfo := version.GetInfo()
	memStats := metrics.GlobalCollector.GetMemoryStats()
	
	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"version":   versionInfo.Version,
		"build": map[string]interface{}{
			"git_commit": versionInfo.GitCommit,
			"build_time": versionInfo.BuildTime,
			"go_version": versionInfo.GoVersion,
			"platform":   versionInfo.Platform,
		},
		"runtime": map[string]interface{}{
			"goroutines":    runtime.NumGoroutine(),
			"gomaxprocs":    runtime.GOMAXPROCS(0),
			"version":       runtime.Version(),
			"uptime":        versionInfo.Uptime,
			"memory_mb":     memStats.Alloc / 1024 / 1024,
			"gc_count":      memStats.NumGC,
		},
	}
	
	respondJSON(w, http.StatusOK, health)
}

// Service endpoint handlers

// handleListServices handles GET /api/v1/services
func (h *Handler) handleListServices(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	
	services, err := h.storage.ListServices(ctx)
	if err != nil {
		h.logger.Error("failed to list services", "error", err)
		respondError(w, http.StatusInternalServerError, "Failed to list services")
		return
	}
	
	respondJSON(w, http.StatusOK, servicesToResponse(services))
}

// handleCreateService handles POST /api/v1/services
func (h *Handler) handleCreateService(w http.ResponseWriter, r *http.Request) {
	var req ServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	
	// Validate request
	if err := validateServiceRequest(&req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	
	// Generate ID if not provided
	if req.ID == "" {
		req.ID = uuid.New().String()
	}
	
	// Parse service request
	service, err := parseServiceRequest(&req, nil)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	
	if err := h.storage.CreateService(ctx, service); err != nil {
		h.logger.Error("failed to create service", "error", err)
		respondError(w, http.StatusInternalServerError, "Failed to create service")
		return
	}
	
	response := serviceToResponse(service)
	respondJSON(w, http.StatusCreated, response)
}

// handleGetService handles GET /api/v1/services/{id}
func (h *Handler) handleGetService(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	
	service, err := h.storage.GetService(ctx, id)
	if err != nil {
		h.logger.Error("failed to get service", "error", err, "id", id)
		respondError(w, http.StatusNotFound, "Service not found")
		return
	}
	
	response := serviceToResponse(service)
	respondJSON(w, http.StatusOK, response)
}

// handleUpdateService handles PUT /api/v1/services/{id}
func (h *Handler) handleUpdateService(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	
	var req ServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	
	// Validate request
	if err := validateServiceRequest(&req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	
	// Get existing service
	existingService, err := h.storage.GetService(ctx, id)
	if err != nil {
		respondError(w, http.StatusNotFound, "Service not found")
		return
	}
	
	// Set ID from URL to ensure it's not changed
	req.ID = id
	
	// Parse service request with existing service for timestamp preservation
	service, err := parseServiceRequest(&req, existingService)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	
	if err := h.storage.UpdateService(ctx, service); err != nil {
		h.logger.Error("failed to update service", "error", err, "id", id)
		respondError(w, http.StatusInternalServerError, "Failed to update service")
		return
	}
	
	response := serviceToResponse(service)
	respondJSON(w, http.StatusOK, response)
}

// handleDeleteService handles DELETE /api/v1/services/{id}
func (h *Handler) handleDeleteService(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	
	// Check if service exists
	if _, err := h.storage.GetService(ctx, id); err != nil {
		respondError(w, http.StatusNotFound, "Service not found")
		return
	}
	
	// Check if service is referenced by any routes
	routes, err := h.storage.ListRoutes(ctx)
	if err != nil {
		h.logger.Error("failed to list routes", "error", err)
		respondError(w, http.StatusInternalServerError, "Failed to check service dependencies")
		return
	}
	
	for _, route := range routes {
		if route.ServiceID == id {
			respondError(w, http.StatusConflict, "Service is referenced by routes")
			return
		}
	}
	
	if err := h.storage.DeleteService(ctx, id); err != nil {
		h.logger.Error("failed to delete service", "error", err, "id", id)
		respondError(w, http.StatusInternalServerError, "Failed to delete service")
		return
	}
	
	w.WriteHeader(http.StatusNoContent)
}

// Route endpoint handlers

// handleListRoutes handles GET /api/v1/routes
func (h *Handler) handleListRoutes(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	
	routes, err := h.storage.ListRoutes(ctx)
	if err != nil {
		h.logger.Error("failed to list routes", "error", err)
		respondError(w, http.StatusInternalServerError, "Failed to list routes")
		return
	}
	
	respondJSON(w, http.StatusOK, routesToResponse(routes))
}

// handleCreateRoute handles POST /api/v1/routes
func (h *Handler) handleCreateRoute(w http.ResponseWriter, r *http.Request) {
	var req RouteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	
	// Convert request to route
	route := types.Route{
		ID:          req.ID,
		Priority:    req.Priority,
		Host:        req.Host,
		PathPrefix:  req.PathPrefix,
		PathRegex:   req.PathRegex,
		Headers:     req.Headers,
		ServiceID:   req.ServiceID,
		Middlewares: req.Middlewares,
	}
	
	// Convert metadata
	if req.Metadata != nil {
		route.Metadata = make(map[string]interface{})
		for k, v := range req.Metadata {
			route.Metadata[k] = v
		}
	}
	
	// Convert rewrite rules
	if len(req.RewriteRules) > 0 {
		route.RewriteRules = make([]types.RewriteRule, len(req.RewriteRules))
		for i, rule := range req.RewriteRules {
			route.RewriteRules[i] = types.RewriteRule{
				Type:        rule.Type,
				Pattern:     rule.Pattern,
				Replacement: rule.Replacement,
			}
		}
	}
	
	// Validate route
	if err := validateRoute(&route); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	
	// Generate ID if not provided
	if route.ID == "" {
		route.ID = uuid.New().String()
	}
	
	// Set defaults
	if route.Priority == 0 {
		route.Priority = 1000
	}
	
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	
	// Verify service exists
	if _, err := h.storage.GetService(ctx, route.ServiceID); err != nil {
		respondError(w, http.StatusBadRequest, "Service not found")
		return
	}
	
	if err := h.storage.CreateRoute(ctx, &route); err != nil {
		h.logger.Error("failed to create route", "error", err)
		respondError(w, http.StatusInternalServerError, "Failed to create route")
		return
	}
	
	response := routeToResponse(&route)
	respondJSON(w, http.StatusCreated, response)
}

// handleGetRoute handles GET /api/v1/routes/{id}
func (h *Handler) handleGetRoute(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	
	route, err := h.storage.GetRoute(ctx, id)
	if err != nil {
		h.logger.Error("failed to get route", "error", err, "id", id)
		respondError(w, http.StatusNotFound, "Route not found")
		return
	}
	
	response := routeToResponse(route)
	respondJSON(w, http.StatusOK, response)
}

// handleUpdateRoute handles PUT /api/v1/routes/{id}
func (h *Handler) handleUpdateRoute(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	
	var req RouteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	
	// Convert request to route
	route := types.Route{
		ID:          id, // Use ID from URL
		Priority:    req.Priority,
		Host:        req.Host,
		PathPrefix:  req.PathPrefix,
		PathRegex:   req.PathRegex,
		Headers:     req.Headers,
		ServiceID:   req.ServiceID,
		Middlewares: req.Middlewares,
	}
	
	// Convert metadata
	if req.Metadata != nil {
		route.Metadata = make(map[string]interface{})
		for k, v := range req.Metadata {
			route.Metadata[k] = v
		}
	}
	
	// Convert rewrite rules
	if len(req.RewriteRules) > 0 {
		route.RewriteRules = make([]types.RewriteRule, len(req.RewriteRules))
		for i, rule := range req.RewriteRules {
			route.RewriteRules[i] = types.RewriteRule{
				Type:        rule.Type,
				Pattern:     rule.Pattern,
				Replacement: rule.Replacement,
			}
		}
	}
	
	// Validate route
	if err := validateRoute(&route); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	
	// Check if route exists
	if _, err := h.storage.GetRoute(ctx, id); err != nil {
		respondError(w, http.StatusNotFound, "Route not found")
		return
	}
	
	// Verify service exists
	if _, err := h.storage.GetService(ctx, route.ServiceID); err != nil {
		respondError(w, http.StatusBadRequest, "Service not found")
		return
	}
	
	if err := h.storage.UpdateRoute(ctx, &route); err != nil {
		h.logger.Error("failed to update route", "error", err, "id", id)
		respondError(w, http.StatusInternalServerError, "Failed to update route")
		return
	}
	
	response := routeToResponse(&route)
	respondJSON(w, http.StatusOK, response)
}

// handleDeleteRoute handles DELETE /api/v1/routes/{id}
func (h *Handler) handleDeleteRoute(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	
	// Check if route exists
	if _, err := h.storage.GetRoute(ctx, id); err != nil {
		respondError(w, http.StatusNotFound, "Route not found")
		return
	}
	
	if err := h.storage.DeleteRoute(ctx, id); err != nil {
		h.logger.Error("failed to delete route", "error", err, "id", id)
		respondError(w, http.StatusInternalServerError, "Failed to delete route")
		return
	}
	
	w.WriteHeader(http.StatusNoContent)
}

// Metrics endpoint handlers

// handleMetrics handles GET /api/v1/stats
func (h *Handler) handleMetrics(w http.ResponseWriter, r *http.Request) {
	// Get real metrics from the global collector
	stats := metrics.GlobalCollector.GetStats()
	
	// Build metrics response with real data
	metricsData := MetricsData{
		Uptime: formatDuration(stats.Uptime),
		Requests: RequestMetrics{
			Total:        int64(stats.TotalRequests),
			PerSecond:    stats.RequestsPerSec,
			Errors:       int64(stats.TotalErrors),
			AvgLatencyMs: stats.AvgLatencyMs,
			P50LatencyMs: stats.P50LatencyMs,
			P95LatencyMs: stats.P95LatencyMs,
			P99LatencyMs: stats.P99LatencyMs,
			ErrorRate:    stats.ErrorRate,
		},
		System: SystemMetrics{
			Goroutines:  runtime.NumGoroutine(),
			MemoryMB:    stats.MemoryUsageMB,
			CPUPercent:  stats.CPUPercent,
			Connections: int(stats.ActiveConnections),
		},
		Services: make(map[string]ServiceMetrics),
	}
	
	// Get all services and their health status
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	
	services, err := h.storage.ListServices(ctx)
	if err == nil {
		for _, service := range services {
			// Get health status for each service
			// Check if service is healthy based on whether it's active
			health := "unknown"
			if service.Active {
				// Consider service healthy if it was updated recently
				// This assumes health checks are updating the service status
				if time.Since(service.UpdatedAt) < 1*time.Minute {
					health = "healthy"
				} else {
					health = "degraded"
				}
			} else {
				health = "unhealthy"
			}
			
			metricsData.Services[service.ID] = ServiceMetrics{
				Requests:     int64(stats.TotalRequests / uint64(len(services))), // Distribute evenly for now
				Errors:       int64(stats.TotalErrors / uint64(len(services))),
				AvgLatencyMs: stats.AvgLatencyMs,
				HealthStatus: health,
			}
		}
	}
	
	respondJSON(w, http.StatusOK, metricsData)
}

// Admin endpoint handlers

// handleReload handles POST /api/v1/admin/reload
func (h *Handler) handleReload(w http.ResponseWriter, r *http.Request) {
	h.logger.Info("Configuration reload requested")
	
	// Check if config loader is available
	if h.configLoader == nil {
		respondError(w, http.StatusServiceUnavailable, "Configuration loader not available")
		return
	}
	
	// Load new configuration
	newConfig, err := h.configLoader.LoadConfig()
	if err != nil {
		h.logger.Error("Failed to load configuration", "error", err)
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to load configuration: %v", err))
		return
	}
	
	// Validate the new configuration
	if err := validateConfig(newConfig); err != nil {
		h.logger.Error("Invalid configuration", "error", err)
		respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid configuration: %v", err))
		return
	}
	
	// Apply the new configuration if callback is set
	if h.onReload != nil {
		if err := h.onReload(newConfig); err != nil {
			h.logger.Error("Failed to apply configuration", "error", err)
			respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to apply configuration: %v", err))
			return
		}
	}
	
	// Update the handler's config reference
	h.config = newConfig
	
	h.logger.Info("Configuration reloaded successfully")
	
	response := map[string]interface{}{
		"status":    "success",
		"message":   "Configuration reloaded successfully",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"summary": map[string]interface{}{
			"listen_addr":     newConfig.ListenAddr,
			"tls_enabled":     newConfig.TLS.Enabled,
			"rate_limit":      newConfig.RateLimit.Enabled,
			"circuit_breaker": newConfig.CircuitBreaker.Enabled,
		},
	}
	
	respondJSON(w, http.StatusOK, response)
}

// handleGetConfig handles GET /api/v1/admin/config
func (h *Handler) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	// Return the current configuration
	// Note: This returns the full configuration including sensitive data
	// In production, you might want to filter out sensitive fields
	
	// Create a sanitized copy of the config
	config := *h.config
	
	// Remove sensitive data
	if config.TLS.Enabled {
		config.TLS.CertFile = "<redacted>"
		config.TLS.KeyFile = "<redacted>"
	}
	
	// Remove sensitive auth data
	if config.Middleware.Auth.Basic.Users != nil {
		// Just show user count, not actual credentials
		userCount := len(config.Middleware.Auth.Basic.Users)
		config.Middleware.Auth.Basic.Users = map[string]string{"<redacted>": fmt.Sprintf("%d users", userCount)}
	}
	if config.Middleware.Auth.JWT.KeyFile != "" {
		config.Middleware.Auth.JWT.KeyFile = "<redacted>"
	}
	if config.Middleware.Auth.OAuth2.ClientSecret != "" {
		config.Middleware.Auth.OAuth2.ClientSecret = "<redacted>"
	}
	
	respondJSON(w, http.StatusOK, config)
}

// handleUpdateConfig handles PUT /api/v1/admin/config
func (h *Handler) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	var update ConfigUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	
	h.logger.Info("Configuration update requested", "update", update)
	
	// Apply updates to current config
	newConfig := *h.config
	
	// Update load balancing if provided
	if update.LoadBalancing != nil {
		if update.LoadBalancing.Algorithm != "" {
			newConfig.LoadBalancing.Algorithm = update.LoadBalancing.Algorithm
		}
	}
	
	// Update rate limiting if provided
	if update.RateLimit != nil {
		if update.RateLimit.RPS < 0 || update.RateLimit.Burst < 0 {
			respondError(w, http.StatusBadRequest, "Invalid rate limit values")
			return
		}
		newConfig.RateLimit.Enabled = update.RateLimit.Enabled
		if update.RateLimit.RPS > 0 {
			newConfig.RateLimit.RPS = update.RateLimit.RPS
		}
		if update.RateLimit.Burst > 0 {
			newConfig.RateLimit.Burst = update.RateLimit.Burst
		}
	}
	
	// Update circuit breaker if provided
	if update.CircuitBreaker != nil {
		if update.CircuitBreaker.FailureThreshold < 0 || update.CircuitBreaker.SuccessThreshold < 0 {
			respondError(w, http.StatusBadRequest, "Invalid circuit breaker values")
			return
		}
		newConfig.CircuitBreaker.Enabled = update.CircuitBreaker.Enabled
		if update.CircuitBreaker.FailureThreshold > 0 {
			newConfig.CircuitBreaker.FailureThreshold = update.CircuitBreaker.FailureThreshold
		}
		if update.CircuitBreaker.SuccessThreshold > 0 {
			newConfig.CircuitBreaker.SuccessThreshold = update.CircuitBreaker.SuccessThreshold
		}
		if update.CircuitBreaker.Timeout > 0 {
			newConfig.CircuitBreaker.Timeout = update.CircuitBreaker.Timeout
		}
	}
	
	// Apply the new configuration if callback is set
	if h.onReload != nil {
		if err := h.onReload(&newConfig); err != nil {
			h.logger.Error("Failed to apply configuration update", "error", err)
			respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to apply configuration: %v", err))
			return
		}
	}
	
	// Update the handler's config reference
	h.config = &newConfig
	
	response := map[string]interface{}{
		"status":    "success",
		"message":   "Configuration updated successfully",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"applied":   update,
	}
	
	respondJSON(w, http.StatusOK, response)
}

// Helper functions

// validateRoute validates a route configuration
func validateRoute(route *types.Route) error {
	if route.ServiceID == "" {
		return fmt.Errorf("Service ID is required")
	}
	
	// Must have at least one matching criterion
	if route.Host == "" && route.PathPrefix == "" && route.PathRegex == "" &&
		len(route.Headers) == 0 {
		return fmt.Errorf("At least one matching criterion is required")
	}
	
	// Validate regex if provided
	if route.PathRegex != "" {
		if _, err := regexp.Compile(route.PathRegex); err != nil {
			return fmt.Errorf("Invalid path regex: %v", err)
		}
	}
	
	return nil
}

// respondJSON writes a JSON response
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.WriteHeader(status)
	if data != nil {
		if err := json.NewEncoder(w).Encode(data); err != nil {
			// Log error but don't modify response since headers are already sent
			return
		}
	}
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}

// respondError writes an error response
func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, ErrorResponse{
		Error: message,
	})
}

// respondErrorWithCode writes an error response with error code
func respondErrorWithCode(w http.ResponseWriter, status int, message, code string) {
	respondJSON(w, status, ErrorResponse{
		Error: message,
		Code:  code,
	})
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm %ds", days, hours, minutes, seconds)
	} else if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// validateConfig validates a proxy configuration
func validateConfig(config *types.ProxyConfig) error {
	if config.ListenAddr == "" {
		return fmt.Errorf("listen_addr is required")
	}
	
	// Validate timeouts
	if config.ReadTimeout <= 0 {
		return fmt.Errorf("read_timeout must be positive")
	}
	if config.WriteTimeout <= 0 {
		return fmt.Errorf("write_timeout must be positive")
	}
	
	// Validate load balancing algorithm
	validAlgorithms := map[string]bool{
		"round_robin": true,
		"weighted":    true,
		"least_conn":  true,
		"ip_hash":     true,
	}
	if !validAlgorithms[config.LoadBalancing.Algorithm] {
		return fmt.Errorf("invalid load balancing algorithm: %s", config.LoadBalancing.Algorithm)
	}
	
	// Validate TLS configuration if enabled
	if config.TLS.Enabled {
		if !config.TLS.AutoCert && (config.TLS.CertFile == "" || config.TLS.KeyFile == "") {
			return fmt.Errorf("cert_file and key_file are required when TLS is enabled and auto_cert is false")
		}
	}
	
	// Validate rate limiting
	if config.RateLimit.Enabled {
		if config.RateLimit.RPS <= 0 {
			return fmt.Errorf("rate limit RPS must be positive")
		}
		if config.RateLimit.Burst < config.RateLimit.RPS {
			return fmt.Errorf("rate limit burst must be >= RPS")
		}
	}
	
	// Validate circuit breaker
	if config.CircuitBreaker.Enabled {
		if config.CircuitBreaker.FailureThreshold <= 0 {
			return fmt.Errorf("circuit breaker failure threshold must be positive")
		}
		if config.CircuitBreaker.SuccessThreshold <= 0 {
			return fmt.Errorf("circuit breaker success threshold must be positive")
		}
	}
	
	return nil
}

var startTime = time.Now()

// serviceToResponse converts a types.Service to a ServiceResponse
func serviceToResponse(s *types.Service) ServiceResponse {
	return ServiceResponse{
		ID:          s.ID,
		Name:        s.Name,
		Endpoints:   s.Endpoints,
		HealthPath:  s.HealthPath,
		Weight:      s.Weight,
		MaxConns:    s.MaxConns,
		Timeout:     s.Timeout.String(),
		Metadata:    s.Metadata,
		StripPrefix: s.StripPrefix,
		Active:      s.Active,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
	}
}

// servicesToResponse converts a slice of types.Service to ServiceResponse
func servicesToResponse(services []*types.Service) []ServiceResponse {
	responses := make([]ServiceResponse, len(services))
	for i, s := range services {
		responses[i] = serviceToResponse(s)
	}
	return responses
}

// validateServiceRequest validates a service request
func validateServiceRequest(req *ServiceRequest) error {
	if req.Name == "" {
		return fmt.Errorf("service name is required")
	}
	
	if len(req.Endpoints) == 0 {
		return fmt.Errorf("at least one endpoint is required")
	}
	
	// Validate endpoints format
	for _, endpoint := range req.Endpoints {
		if endpoint == "" {
			return fmt.Errorf("endpoint cannot be empty")
		}
	}
	
	// Validate timeout format if provided
	if req.Timeout != "" {
		if _, err := time.ParseDuration(req.Timeout); err != nil {
			return fmt.Errorf("invalid timeout format: %v", err)
		}
	}
	
	if req.Weight < 0 {
		return fmt.Errorf("weight must be non-negative")
	}
	
	if req.MaxConns < 0 {
		return fmt.Errorf("max connections must be non-negative")
	}
	
	return nil
}

// parseServiceRequest converts a ServiceRequest to types.Service
func parseServiceRequest(req *ServiceRequest, existingService *types.Service) (*types.Service, error) {
	// Parse timeout
	var timeout time.Duration
	if req.Timeout != "" {
		var err error
		timeout, err = time.ParseDuration(req.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout format: %v", err)
		}
	} else {
		timeout = 30 * time.Second
	}
	
	service := &types.Service{
		ID:          req.ID,
		Name:        req.Name,
		Endpoints:   req.Endpoints,
		HealthPath:  req.HealthPath,
		Weight:      req.Weight,
		MaxConns:    req.MaxConns,
		Timeout:     timeout,
		Metadata:    req.Metadata,
		StripPrefix: req.StripPrefix,
		Active:      req.Active,
	}
	
	// Preserve timestamps from existing service if updating
	if existingService != nil {
		service.CreatedAt = existingService.CreatedAt
		service.UpdatedAt = time.Now()
	} else {
		service.CreatedAt = time.Now()
		service.UpdatedAt = time.Now()
	}
	
	// Set defaults
	if service.HealthPath == "" {
		service.HealthPath = "/"
	}
	
	if service.Weight == 0 {
		service.Weight = 1
	}
	
	return service, nil
}

// routeToResponse converts a types.Route to a RouteResponse
func routeToResponse(r *types.Route) RouteResponse {
	response := RouteResponse{
		ID:          r.ID,
		Priority:    r.Priority,
		Host:        r.Host,
		PathPrefix:  r.PathPrefix,
		PathRegex:   r.PathRegex,
		Headers:     r.Headers,
		ServiceID:   r.ServiceID,
		Middlewares: r.Middlewares,
		Metadata:    r.Metadata,
	}
	
	// Convert rewrite rules
	if len(r.RewriteRules) > 0 {
		response.RewriteRules = make([]struct {
			Type        string `json:"type"`
			Pattern     string `json:"pattern"`
			Replacement string `json:"replacement,omitempty"`
		}, len(r.RewriteRules))
		for i, rule := range r.RewriteRules {
			response.RewriteRules[i] = struct {
				Type        string `json:"type"`
				Pattern     string `json:"pattern"`
				Replacement string `json:"replacement,omitempty"`
			}{
				Type:        rule.Type,
				Pattern:     rule.Pattern,
				Replacement: rule.Replacement,
			}
		}
	}
	
	return response
}

// routesToResponse converts a slice of types.Route to RouteResponse
func routesToResponse(routes []*types.Route) []RouteResponse {
	responses := make([]RouteResponse, len(routes))
	for i, r := range routes {
		responses[i] = routeToResponse(r)
	}
	return responses
}
