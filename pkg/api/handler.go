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
	
	"discobox/internal/types"
)

// Handler provides the REST API implementation
type Handler struct {
	storage types.Storage
	logger  types.Logger
	config  *types.ProxyConfig
}

// New creates a new API handler instance
func New(storage types.Storage, logger types.Logger, config *types.ProxyConfig) *Handler {
	return &Handler{
		storage: storage,
		logger:  logger,
		config:  config,
	}
}

// Router returns the HTTP handler for the API
func (h *Handler) Router() http.Handler {
	router := mux.NewRouter()
	
	// Health check
	router.HandleFunc("/health", h.handleHealth).Methods("GET")
	
	// Services
	router.HandleFunc("/api/v1/services", h.handleListServices).Methods("GET")
	router.HandleFunc("/api/v1/services", h.handleCreateService).Methods("POST")
	router.HandleFunc("/api/v1/services/{id}", h.handleGetService).Methods("GET")
	router.HandleFunc("/api/v1/services/{id}", h.handleUpdateService).Methods("PUT")
	router.HandleFunc("/api/v1/services/{id}", h.handleDeleteService).Methods("DELETE")
	
	// Routes
	router.HandleFunc("/api/v1/routes", h.handleListRoutes).Methods("GET")
	router.HandleFunc("/api/v1/routes", h.handleCreateRoute).Methods("POST")
	router.HandleFunc("/api/v1/routes/{id}", h.handleGetRoute).Methods("GET")
	router.HandleFunc("/api/v1/routes/{id}", h.handleUpdateRoute).Methods("PUT")
	router.HandleFunc("/api/v1/routes/{id}", h.handleDeleteRoute).Methods("DELETE")
	
	// Metrics
	router.HandleFunc("/api/v1/metrics", h.handleMetrics).Methods("GET")
	
	// Admin
	router.HandleFunc("/api/v1/admin/reload", h.handleReload).Methods("POST")
	router.HandleFunc("/api/v1/admin/config", h.handleGetConfig).Methods("GET")
	router.HandleFunc("/api/v1/admin/config", h.handleUpdateConfig).Methods("PUT")
	
	// Apply middleware
	// Create auth config from proxy config
	authConfig := &AuthConfig{
		Enabled: h.config.API.Auth,
		Type:    "api-key",
		Token:   h.config.API.APIKey,
		HeaderName: "X-API-Key",
	}
	
	return WithMiddleware(router, h.logger, authConfig)
}

// Health endpoint handlers

// handleHealth handles GET /health
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"version":   "1.0.0", // TODO: Get from build info
		"runtime": map[string]interface{}{
			"goroutines": runtime.NumGoroutine(),
			"gomaxprocs": runtime.GOMAXPROCS(0),
			"version":    runtime.Version(),
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
	
	respondJSON(w, http.StatusOK, services)
}

// handleCreateService handles POST /api/v1/services
func (h *Handler) handleCreateService(w http.ResponseWriter, r *http.Request) {
	var service types.Service
	if err := json.NewDecoder(r.Body).Decode(&service); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	
	// Validate service
	if service.Name == "" {
		respondError(w, http.StatusBadRequest, "Service name is required")
		return
	}
	
	if len(service.Endpoints) == 0 {
		respondError(w, http.StatusBadRequest, "At least one endpoint is required")
		return
	}
	
	// Generate ID if not provided
	if service.ID == "" {
		service.ID = uuid.New().String()
	}
	
	// Set defaults
	if service.HealthPath == "" {
		service.HealthPath = "/"
	}
	
	if service.Weight == 0 {
		service.Weight = 1
	}
	
	if service.Timeout == 0 {
		service.Timeout = 30 * time.Second
	}
	
	// Set timestamps
	service.CreatedAt = time.Now()
	service.UpdatedAt = time.Now()
	service.Active = true
	
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	
	if err := h.storage.CreateService(ctx, &service); err != nil {
		h.logger.Error("failed to create service", "error", err)
		respondError(w, http.StatusInternalServerError, "Failed to create service")
		return
	}
	
	respondJSON(w, http.StatusCreated, service)
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
	
	respondJSON(w, http.StatusOK, service)
}

// handleUpdateService handles PUT /api/v1/services/{id}
func (h *Handler) handleUpdateService(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	
	var service types.Service
	if err := json.NewDecoder(r.Body).Decode(&service); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	
	// Ensure ID matches URL
	service.ID = id
	
	// Validate service
	if service.Name == "" {
		respondError(w, http.StatusBadRequest, "Service name is required")
		return
	}
	
	if len(service.Endpoints) == 0 {
		respondError(w, http.StatusBadRequest, "At least one endpoint is required")
		return
	}
	
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	
	// Check if service exists
	if _, err := h.storage.GetService(ctx, id); err != nil {
		respondError(w, http.StatusNotFound, "Service not found")
		return
	}
	
	// Update timestamp
	service.UpdatedAt = time.Now()
	
	if err := h.storage.UpdateService(ctx, &service); err != nil {
		h.logger.Error("failed to update service", "error", err, "id", id)
		respondError(w, http.StatusInternalServerError, "Failed to update service")
		return
	}
	
	respondJSON(w, http.StatusOK, service)
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
	
	respondJSON(w, http.StatusOK, routes)
}

// handleCreateRoute handles POST /api/v1/routes
func (h *Handler) handleCreateRoute(w http.ResponseWriter, r *http.Request) {
	var route types.Route
	if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
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
	
	respondJSON(w, http.StatusCreated, route)
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
	
	respondJSON(w, http.StatusOK, route)
}

// handleUpdateRoute handles PUT /api/v1/routes/{id}
func (h *Handler) handleUpdateRoute(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	
	var route types.Route
	if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	
	// Ensure ID matches URL
	route.ID = id
	
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
	
	respondJSON(w, http.StatusOK, route)
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

// handleMetrics handles GET /api/v1/metrics
func (h *Handler) handleMetrics(w http.ResponseWriter, r *http.Request) {
	// Get memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	
	// Calculate uptime
	uptime := time.Since(startTime)
	
	// Build metrics response
	// In a real implementation, these would come from actual metric collectors
	metrics := MetricsData{
		Uptime: uptime.String(),
		Requests: RequestMetrics{
			Total:        10000,  // TODO: Get from metrics collector
			PerSecond:    50.5,   // TODO: Calculate from actual data
			Errors:       25,     // TODO: Get from metrics collector
			AvgLatencyMs: 125.3,  // TODO: Calculate from actual data
		},
		System: SystemMetrics{
			Goroutines:  runtime.NumGoroutine(),
			MemoryMB:    float64(memStats.Alloc) / 1024 / 1024,
			CPUPercent:  0.0,  // TODO: Implement CPU tracking
			Connections: 100,  // TODO: Get from connection tracker
		},
		Services: make(map[string]ServiceMetrics),
	}
	
	// Add per-service metrics
	// In a real implementation, this would aggregate from actual service metrics
	metrics.Services["example-service"] = ServiceMetrics{
		Requests:     5000,
		Errors:       10,
		AvgLatencyMs: 100.5,
		HealthStatus: "healthy",
	}
	
	respondJSON(w, http.StatusOK, metrics)
}

// Admin endpoint handlers

// handleReload handles POST /api/v1/admin/reload
func (h *Handler) handleReload(w http.ResponseWriter, r *http.Request) {
	// In a real implementation, this would trigger a configuration reload
	// For now, we'll simulate the operation
	
	h.logger.Info("Configuration reload requested")
	
	// Simulate reload process
	response := map[string]interface{}{
		"status":    "success",
		"message":   "Configuration reloaded successfully",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	
	respondJSON(w, http.StatusOK, response)
}

// handleGetConfig handles GET /api/v1/admin/config
func (h *Handler) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	// In a real implementation, this would return the current configuration
	// For now, return a sample config
	
	config := types.ProxyConfig{
		ListenAddr:      ":8080",
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    30 * time.Second,
		IdleTimeout:     120 * time.Second,
		ShutdownTimeout: 30 * time.Second,
		LoadBalancing: struct {
			Algorithm string `yaml:"algorithm"`
			Sticky    struct {
				Enabled    bool          `yaml:"enabled"`
				CookieName string        `yaml:"cookie_name"`
				TTL        time.Duration `yaml:"ttl"`
			} `yaml:"sticky"`
		}{
			Algorithm: "round_robin",
		},
		RateLimit: struct {
			Enabled  bool   `yaml:"enabled"`
			RPS      int    `yaml:"rps"`
			Burst    int    `yaml:"burst"`
			ByHeader string `yaml:"by_header,omitempty"`
		}{
			Enabled: true,
			RPS:     100,
			Burst:   200,
		},
		CircuitBreaker: struct {
			Enabled          bool          `yaml:"enabled"`
			FailureThreshold int           `yaml:"failure_threshold"`
			SuccessThreshold int           `yaml:"success_threshold"`
			Timeout          time.Duration `yaml:"timeout"`
		}{
			Enabled:          true,
			FailureThreshold: 5,
			SuccessThreshold: 2,
			Timeout:          60 * time.Second,
		},
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
	
	// In a real implementation, this would validate and apply the configuration changes
	// For now, we'll simulate the operation
	
	h.logger.Info("Configuration update requested", "update", update)
	
	// Validate the update
	if update.RateLimit != nil {
		if update.RateLimit.RPS < 0 || update.RateLimit.Burst < 0 {
			respondError(w, http.StatusBadRequest, "Invalid rate limit values")
			return
		}
	}
	
	if update.CircuitBreaker != nil {
		if update.CircuitBreaker.FailureThreshold < 0 || update.CircuitBreaker.SuccessThreshold < 0 {
			respondError(w, http.StatusBadRequest, "Invalid circuit breaker values")
			return
		}
	}
	
	// Simulate applying the update
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

// respondError writes an error response
func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{
		"error": message,
	})
}

var startTime = time.Now()
