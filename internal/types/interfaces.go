// Package types defines the core interfaces for the discobox reverse proxy
package types

import (
	"context"
	"net/http"
	"time"
)

// LoadBalancer selects a backend server
type LoadBalancer interface {
	// Select returns the next server to use
	Select(ctx context.Context, req *http.Request, servers []*Server) (*Server, error)
	// Add adds a new server to the pool
	Add(server *Server) error
	// Remove removes a server from the pool
	Remove(serverID string) error
	// UpdateWeight updates server weight (for weighted algorithms)
	UpdateWeight(serverID string, weight int) error
}

// HealthChecker monitors backend health
type HealthChecker interface {
	// Check performs a health check on the server
	Check(ctx context.Context, server *Server) error
	// Watch continuously monitors server health
	Watch(ctx context.Context, server *Server, interval time.Duration) <-chan error
	// RecordSuccess records a successful request (for passive checks)
	RecordSuccess(serverID string)
	// RecordFailure records a failed request (for passive checks)
	RecordFailure(serverID string, err error)
}

// CircuitBreaker protects backends from cascading failures
type CircuitBreaker interface {
	// Execute runs the function with circuit breaker protection
	Execute(fn func() error) error
	// State returns the current state (closed, open, half-open)
	State() string
	// Reset manually resets the circuit breaker
	Reset()
}

// RateLimiter controls request rates
type RateLimiter interface {
	// Allow checks if a request should be allowed
	Allow(key string) bool
	// Wait blocks until the request can proceed
	Wait(ctx context.Context, key string) error
	// Limit returns the current limit for a key
	Limit(key string) int
}

// URLRewriter transforms request URLs
type URLRewriter interface {
	// Rewrite modifies the request URL based on rules
	Rewrite(req *http.Request, rules []RewriteRule) error
}

// Router manages request routing
type Router interface {
	// Match finds the best route for a request
	Match(req *http.Request) (*Route, error)
	// AddRoute adds a new route
	AddRoute(route *Route) error
	// RemoveRoute removes a route
	RemoveRoute(routeID string) error
	// UpdateRoute updates an existing route
	UpdateRoute(route *Route) error
	// GetRoutes returns all routes
	GetRoutes() ([]*Route, error)
}

// Storage persists configuration
type Storage interface {
	// Services
	GetService(ctx context.Context, id string) (*Service, error)
	ListServices(ctx context.Context) ([]*Service, error)
	CreateService(ctx context.Context, service *Service) error
	UpdateService(ctx context.Context, service *Service) error
	DeleteService(ctx context.Context, id string) error
	
	// Routes
	GetRoute(ctx context.Context, id string) (*Route, error)
	ListRoutes(ctx context.Context) ([]*Route, error)
	CreateRoute(ctx context.Context, route *Route) error
	UpdateRoute(ctx context.Context, route *Route) error
	DeleteRoute(ctx context.Context, id string) error
	
	// Users
	GetUser(ctx context.Context, id string) (*User, error)
	GetUserByUsername(ctx context.Context, username string) (*User, error)
	ListUsers(ctx context.Context) ([]*User, error)
	CreateUser(ctx context.Context, user *User) error
	UpdateUser(ctx context.Context, user *User) error
	DeleteUser(ctx context.Context, id string) error
	
	// API Keys
	GetAPIKey(ctx context.Context, key string) (*APIKey, error)
	ListAPIKeysByUser(ctx context.Context, userID string) ([]*APIKey, error)
	CreateAPIKey(ctx context.Context, apiKey *APIKey) error
	RevokeAPIKey(ctx context.Context, key string) error
	
	// Watch for changes
	Watch(ctx context.Context) <-chan StorageEvent
	
	// Close closes the storage
	Close() error
}

// Middleware wraps HTTP handlers
type Middleware func(http.Handler) http.Handler

// MiddlewareChain manages middleware execution order
type MiddlewareChain interface {
	// Use adds middleware to the chain
	Use(middleware ...Middleware)
	// Then creates the final handler
	Then(handler http.Handler) http.Handler
}

// MetricsCollector gathers performance metrics
type MetricsCollector interface {
	// RecordRequest records request metrics
	RecordRequest(method, path string, statusCode int, duration time.Duration)
	// RecordUpstreamLatency records backend latency
	RecordUpstreamLatency(service string, duration time.Duration)
	// RecordActiveConnections updates connection count
	RecordActiveConnections(count int)
	// Handler returns the metrics endpoint handler
	Handler() http.Handler
}

// Logger provides structured logging
type Logger interface {
	Debug(msg string, fields ...interface{})
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
	With(fields ...interface{}) Logger
}