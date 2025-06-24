// Package proxy implements the core reverse proxy functionality
package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"net/http"
	"net/http/httputil"
	"net/url"
	"sync/atomic"

	"discobox/internal/types"
)

// BufferPool adapts sync.Pool to httputil.BufferPool interface
type BufferPool struct {
	pool *sync.Pool
}

func (bp *BufferPool) Get() []byte {
	return bp.pool.Get().([]byte)
}

func (bp *BufferPool) Put(b []byte) {
	bp.pool.Put(b)
}

// Proxy is the main reverse proxy implementation
type Proxy struct {
	loadBalancer   types.LoadBalancer
	healthChecker  types.HealthChecker
	circuitBreaker types.CircuitBreaker
	router         types.Router
	rewriter       types.URLRewriter
	transport      http.RoundTripper
	logger         types.Logger
	storage        types.Storage
	bufferPool     *BufferPool
	errorHandler   func(http.ResponseWriter, *http.Request, error)
	modifyResponse func(*http.Response) error
}

// Options for creating a new proxy
type Options struct {
	LoadBalancer   types.LoadBalancer
	HealthChecker  types.HealthChecker
	CircuitBreaker types.CircuitBreaker
	Router         types.Router
	Rewriter       types.URLRewriter
	Transport      http.RoundTripper
	Logger         types.Logger
	Storage        types.Storage
	ErrorHandler   func(http.ResponseWriter, *http.Request, error)
	ModifyResponse func(*http.Response) error
}

// New creates a new proxy instance
func New(opts Options) *Proxy {
	p := &Proxy{
		loadBalancer:   opts.LoadBalancer,
		healthChecker:  opts.HealthChecker,
		circuitBreaker: opts.CircuitBreaker,
		router:         opts.Router,
		rewriter:       opts.Rewriter,
		transport:      opts.Transport,
		logger:         opts.Logger,
		storage:        opts.Storage,
		errorHandler:   opts.ErrorHandler,
		modifyResponse: opts.ModifyResponse,
		bufferPool: &BufferPool{
			pool: &sync.Pool{
				New: func() any {
					return make([]byte, 32*1024) // 32KB buffers
				},
			},
		},
	}

	if p.transport == nil {
		p.transport = DefaultTransport()
	}

	if p.errorHandler == nil {
		p.errorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			p.defaultErrorHandler(w, r, err, http.StatusBadGateway)
		}
	}

	return p
}

// UpdateLoadBalancer updates the load balancer at runtime
func (p *Proxy) UpdateLoadBalancer(lb types.LoadBalancer) {
	p.loadBalancer = lb
}

// UpdateCircuitBreaker updates the circuit breaker at runtime
func (p *Proxy) UpdateCircuitBreaker(cb types.CircuitBreaker) {
	p.circuitBreaker = cb
}

// ServeHTTP handles incoming requests
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	// Find matching route
	route, err := p.router.Match(r)
	if err != nil {
		p.handleError(w, r, err, http.StatusNotFound)
		return
	}

	// Get service
	ctx := r.Context()
	service, err := p.getService(ctx, route.ServiceID)
	if err != nil {
		p.handleError(w, r, err, http.StatusServiceUnavailable)
		return
	}

	// Convert endpoints to servers
	servers := p.endpointsToServers(service)
	if len(servers) == 0 {
		p.handleError(w, r, types.ErrNoHealthyBackends, http.StatusServiceUnavailable)
		return
	}

	// Select backend server
	server, err := p.loadBalancer.Select(ctx, r, servers)
	if err != nil {
		p.handleError(w, r, err, http.StatusServiceUnavailable)
		return
	}

	// Increment active connections
	atomic.AddInt64(&server.ActiveConns, 1)
	defer atomic.AddInt64(&server.ActiveConns, -1)

	// Update last used time
	server.LastUsed = time.Now()

	// Apply URL rewriting
	if p.rewriter != nil && len(route.RewriteRules) > 0 {
		if err := p.rewriter.Rewrite(r, route.RewriteRules); err != nil {
			p.logger.Error("failed to rewrite URL",
				"error", err,
				"route_id", route.ID,
			)
		}
	}

	// Strip prefix if configured
	if service.StripPrefix && route.PathPrefix != "" {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, route.PathPrefix)
		if !strings.HasPrefix(r.URL.Path, "/") {
			r.URL.Path = "/" + r.URL.Path
		}
	}

	// Create reverse proxy for this request
	proxy := p.createReverseProxy(server, service, route)

	// Execute with circuit breaker if available
	if p.circuitBreaker != nil {
		err = p.circuitBreaker.Execute(func() error {
			proxy.ServeHTTP(w, r)
			return nil
		})
		if err != nil {
			p.handleError(w, r, err, http.StatusServiceUnavailable)
			return
		}
	} else {
		proxy.ServeHTTP(w, r)
	}

	// Log request
	duration := time.Since(startTime)
	p.logger.Debug("proxied request",
		"method", r.Method,
		"path", r.URL.Path,
		"backend", server.URL.String(),
		"duration", duration,
	)
}

// createReverseProxy creates a reverse proxy for a specific backend
func (p *Proxy) createReverseProxy(server *types.Server, service *types.Service, route *types.Route) *httputil.ReverseProxy {
	// Create error handler that records failures
	errorHandler := func(w http.ResponseWriter, r *http.Request, err error) {
		if p.healthChecker != nil {
			p.healthChecker.RecordFailure(server.ID, err)
		}
		if p.errorHandler != nil {
			p.errorHandler(w, r, err)
		} else {
			p.defaultErrorHandler(w, r, err, http.StatusBadGateway)
		}
	}

	// Create response modifier that records success
	modifyResponse := func(resp *http.Response) error {
		// Record success for 2xx and 3xx responses
		if p.healthChecker != nil && resp.StatusCode < 400 {
			p.healthChecker.RecordSuccess(server.ID)
		} else if p.healthChecker != nil && resp.StatusCode >= 500 {
			// Record failure for 5xx responses
			p.healthChecker.RecordFailure(server.ID, fmt.Errorf("backend returned %d", resp.StatusCode))
		}

		// Call the original modifier if present
		if p.modifyResponse != nil {
			return p.modifyResponse(resp)
		}
		return nil
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = server.URL.Scheme
			req.URL.Host = server.URL.Host

			// Add forwarding headers
			p.addForwardingHeaders(req)

			// Add custom headers
			for k, v := range server.Metadata {
				if strings.HasPrefix(k, "header:") {
					req.Header.Set(k[7:], v)
				}
			}
		},
		Transport:      p.transport,
		ErrorHandler:   errorHandler,
		ModifyResponse: modifyResponse,
		BufferPool:     p.bufferPool,
	}

	return proxy
}

// addForwardingHeaders adds X-Forwarded-* headers
func (p *Proxy) addForwardingHeaders(req *http.Request) {
	// X-Forwarded-For
	if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		if prior := req.Header.Get("X-Forwarded-For"); prior != "" {
			clientIP = prior + ", " + clientIP
		}
		req.Header.Set("X-Forwarded-For", clientIP)
	}

	// X-Real-IP
	if req.Header.Get("X-Real-IP") == "" {
		if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
			req.Header.Set("X-Real-IP", clientIP)
		}
	}

	// X-Forwarded-Proto
	if req.TLS != nil {
		req.Header.Set("X-Forwarded-Proto", "https")
	} else {
		req.Header.Set("X-Forwarded-Proto", "http")
	}

	// X-Forwarded-Host
	req.Header.Set("X-Forwarded-Host", req.Host)
}

// getService retrieves service from storage
func (p *Proxy) getService(ctx context.Context, serviceID string) (*types.Service, error) {
	if p.storage == nil {
		return nil, fmt.Errorf("storage not configured")
	}

	service, err := p.storage.GetService(ctx, serviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get service %s: %w", serviceID, err)
	}

	if !service.Active {
		return nil, fmt.Errorf("service %s is not active", serviceID)
	}

	return service, nil
}

// endpointsToServers converts service endpoints to server objects
func (p *Proxy) endpointsToServers(service *types.Service) []*types.Server {
	servers := make([]*types.Server, 0, len(service.Endpoints))

	for i, endpoint := range service.Endpoints {
		u, err := url.Parse(endpoint)
		if err != nil {
			p.logger.Error("invalid endpoint URL",
				"endpoint", endpoint,
				"error", err,
			)
			continue
		}

		server := &types.Server{
			ID:       fmt.Sprintf("%s-%d", service.ID, i),
			URL:      u,
			Weight:   service.Weight,
			MaxConns: service.MaxConns,
			Healthy:  true, // Should be determined by health checker
			Metadata: service.Metadata,
		}

		servers = append(servers, server)
	}

	return servers
}

// handleError sends an error response
func (p *Proxy) handleError(w http.ResponseWriter, r *http.Request, err error, statusCode int) {
	p.logger.Error("proxy error",
		"error", err,
		"method", r.Method,
		"path", r.URL.Path,
		"status", statusCode,
	)

	if p.errorHandler != nil {
		p.errorHandler(w, r, err)
		return
	}

	p.defaultErrorHandler(w, r, err, statusCode)
}

// defaultErrorHandler is the default error handler
func (p *Proxy) defaultErrorHandler(w http.ResponseWriter, r *http.Request, err error, suggestedStatus int) {
	statusCode := suggestedStatus

	// Override with specific error codes if we recognize the error
	switch {
	case errors.Is(err, types.ErrRouteNotFound):
		statusCode = http.StatusNotFound
	case errors.Is(err, types.ErrNoHealthyBackends):
		statusCode = http.StatusServiceUnavailable
	case errors.Is(err, types.ErrCircuitBreakerOpen):
		statusCode = http.StatusServiceUnavailable
	case errors.Is(err, types.ErrRateLimitExceeded):
		statusCode = http.StatusTooManyRequests
	case errors.Is(err, types.ErrTimeout):
		statusCode = http.StatusGatewayTimeout
	case errors.Is(err, types.ErrServiceNotFound):
		statusCode = http.StatusServiceUnavailable
	case strings.Contains(err.Error(), "is not active"):
		statusCode = http.StatusServiceUnavailable
	}

	http.Error(w, err.Error(), statusCode)
}

// Option functions for builder pattern
type Option func(*Options)

// WithLoadBalancer sets the load balancer
func WithLoadBalancer(lb types.LoadBalancer) Option {
	return func(o *Options) {
		o.LoadBalancer = lb
	}
}

// WithHealthChecker sets the health checker
func WithHealthChecker(hc types.HealthChecker) Option {
	return func(o *Options) {
		o.HealthChecker = hc
	}
}

// WithCircuitBreaker sets the circuit breaker
func WithCircuitBreaker(cb types.CircuitBreaker) Option {
	return func(o *Options) {
		o.CircuitBreaker = cb
	}
}

// WithRouter sets the router
func WithRouter(r types.Router) Option {
	return func(o *Options) {
		o.Router = r
	}
}

// WithRewriter sets the URL rewriter
func WithRewriter(rw types.URLRewriter) Option {
	return func(o *Options) {
		o.Rewriter = rw
	}
}

// WithTransport sets the HTTP transport
func WithTransport(t http.RoundTripper) Option {
	return func(o *Options) {
		o.Transport = t
	}
}

// WithLogger sets the logger
func WithLogger(l types.Logger) Option {
	return func(o *Options) {
		o.Logger = l
	}
}

// WithStorage sets the storage
func WithStorage(s types.Storage) Option {
	return func(o *Options) {
		o.Storage = s
	}
}

// NewWithOptions creates a proxy with option functions
func NewWithOptions(opts ...Option) *Proxy {
	options := &Options{}
	for _, opt := range opts {
		opt(options)
	}
	return New(*options)
}

// CopyBuffer copies from src to dst using a buffer from the pool
func (p *Proxy) CopyBuffer(dst io.Writer, src io.Reader) (int64, error) {
	buf := p.bufferPool.Get()
	defer p.bufferPool.Put(buf)

	return io.CopyBuffer(dst, src, buf)
}
