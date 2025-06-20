// Package router implements request routing for Discobox
package router

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"sync"
	
	"net/http"
	
	"discobox/internal/types"
)


// router implements the Router interface
type router struct {
	storage  types.Storage
	logger   types.Logger
	mu       sync.RWMutex
	routes   []*types.Route
	compiled map[string]*compiledRoute
}

// compiledRoute holds pre-compiled regex patterns
type compiledRoute struct {
	route      *types.Route
	pathRegexp *regexp.Regexp
}

// NewRouter creates a new router instance
func NewRouter(storage types.Storage, logger types.Logger) types.Router {
	r := &router{
		storage:  storage,
		logger:   logger,
		routes:   make([]*types.Route, 0),
		compiled: make(map[string]*compiledRoute),
	}
	
	// Load initial routes
	ctx := context.Background()
	if err := r.loadRoutes(ctx); err != nil {
		logger.Error("failed to load initial routes", "error", err)
	}
	
	// Watch for route changes
	go r.watchChanges(ctx)
	
	return r
}

// Match finds the best route for a request
func (r *router) Match(req *http.Request) (*types.Route, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	// Routes are already sorted by priority
	for _, compiled := range r.routes {
		route := compiled
		compiledRoute := r.compiled[route.ID]
		
		// Match host
		if route.Host != "" && !r.matchHost(req.Host, route.Host) {
			continue
		}
		
		// Match path prefix
		if route.PathPrefix != "" && !strings.HasPrefix(req.URL.Path, route.PathPrefix) {
			continue
		}
		
		// Match path regex
		if route.PathRegex != "" && compiledRoute.pathRegexp != nil {
			if !compiledRoute.pathRegexp.MatchString(req.URL.Path) {
				continue
			}
		}
		
		// Match headers
		if !r.matchHeaders(req, route.Headers) {
			continue
		}
		
		// Found a match
		r.logger.Debug("route matched",
			"route_id", route.ID,
			"host", req.Host,
			"path", req.URL.Path,
		)
		return route, nil
	}
	
	return nil, types.ErrRouteNotFound
}

// AddRoute adds a new route
func (r *router) AddRoute(route *types.Route) error {
	if route == nil {
		return types.ErrInvalidRequest
	}
	
	// Create in storage
	ctx := context.Background()
	if err := r.storage.CreateRoute(ctx, route); err != nil {
		return err
	}
	
	// Reload routes
	return r.loadRoutes(ctx)
}

// RemoveRoute removes a route
func (r *router) RemoveRoute(routeID string) error {
	ctx := context.Background()
	if err := r.storage.DeleteRoute(ctx, routeID); err != nil {
		return err
	}
	
	// Reload routes
	return r.loadRoutes(ctx)
}

// UpdateRoute updates an existing route
func (r *router) UpdateRoute(route *types.Route) error {
	if route == nil {
		return types.ErrInvalidRequest
	}
	
	ctx := context.Background()
	if err := r.storage.UpdateRoute(ctx, route); err != nil {
		return err
	}
	
	// Reload routes
	return r.loadRoutes(ctx)
}

// GetRoutes returns all routes
func (r *router) GetRoutes() ([]*types.Route, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	// Return a copy
	routes := make([]*types.Route, len(r.routes))
	copy(routes, r.routes)
	return routes, nil
}

// loadRoutes loads routes from storage and sorts them by priority
func (r *router) loadRoutes(ctx context.Context) error {
	routes, err := r.storage.ListRoutes(ctx)
	if err != nil {
		return err
	}
	
	// Sort by priority (descending) and then by ID for stability
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Priority != routes[j].Priority {
			return routes[i].Priority > routes[j].Priority
		}
		return routes[i].ID < routes[j].ID
	})
	
	// Compile regex patterns
	compiled := make(map[string]*compiledRoute)
	for _, route := range routes {
		cr := &compiledRoute{route: route}
		
		if route.PathRegex != "" {
			regex, err := regexp.Compile(route.PathRegex)
			if err != nil {
				r.logger.Error("failed to compile route regex",
					"route_id", route.ID,
					"regex", route.PathRegex,
					"error", err,
				)
				continue
			}
			cr.pathRegexp = regex
		}
		
		compiled[route.ID] = cr
	}
	
	r.mu.Lock()
	r.routes = routes
	r.compiled = compiled
	r.mu.Unlock()
	
	r.logger.Info("loaded routes", "count", len(routes))
	return nil
}

// watchChanges watches for route changes in storage
func (r *router) watchChanges(ctx context.Context) {
	events := r.storage.Watch(ctx)
	
	for event := range events {
		if event.Kind != "route" {
			continue
		}
		
		r.logger.Debug("route change detected",
			"type", event.Type,
			"id", event.ID,
		)
		
		// Reload routes on any change
		if err := r.loadRoutes(ctx); err != nil {
			r.logger.Error("failed to reload routes", "error", err)
		}
	}
}

// matchHost checks if the request host matches the route host
func (r *router) matchHost(reqHost, routeHost string) bool {
	// Remove port from request host
	if idx := strings.LastIndex(reqHost, ":"); idx != -1 {
		reqHost = reqHost[:idx]
	}
	
	// Exact match
	if reqHost == routeHost {
		return true
	}
	
	// Wildcard match (*.example.com)
	if strings.HasPrefix(routeHost, "*.") {
		suffix := routeHost[1:] // Remove *
		return strings.HasSuffix(reqHost, suffix)
	}
	
	return false
}

// matchHeaders checks if request headers match route requirements
func (r *router) matchHeaders(req *http.Request, routeHeaders map[string]string) bool {
	if len(routeHeaders) == 0 {
		return true
	}
	
	for key, value := range routeHeaders {
		reqValue := req.Header.Get(key)
		if reqValue != value {
			return false
		}
	}
	
	return true
}
