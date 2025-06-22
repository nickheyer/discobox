// Package router implements request routing for Discobox
package router

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	
	"net/http"
	
	"discobox/internal/types"
)


// router implements the Router interface
type router struct {
	storage    types.Storage
	logger     types.Logger
	mu         sync.RWMutex
	routes     []*types.Route
	compiled   map[string]*compiledRoute
	hostRouter *hostRouter // Optimization for host-based lookups
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

// compiledRoute holds pre-compiled regex patterns
type compiledRoute struct {
	route      *types.Route
	pathRegexp *regexp.Regexp
}

// NewRouter creates a new router instance
func NewRouter(storage types.Storage, logger types.Logger) types.Router {
	r := &router{
		storage:    storage,
		logger:     logger,
		routes:     make([]*types.Route, 0),
		compiled:   make(map[string]*compiledRoute),
		hostRouter: newHostRouter(),
		stopCh:     make(chan struct{}),
	}
	
	// Load initial routes
	ctx := context.Background()
	if err := r.loadRoutes(ctx); err != nil {
		logger.Error("failed to load initial routes", "error", err)
	}
	
	// Watch for route changes in a separate goroutine
	// This ensures the router is fully initialized before starting the watch
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		// Small delay to ensure storage watch is ready
		// This prevents race conditions during initialization
		select {
		case <-time.After(10 * time.Millisecond):
		case <-r.stopCh:
			return
		}
		r.watchChanges()
	}()
	
	return r
}

// Match finds the best route for a request
func (r *router) Match(req *http.Request) (*types.Route, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	// Use host router to get candidate routes
	candidates := r.hostRouter.findRoutes(req.Host)
	
	// If no candidates based on host, no match possible
	if len(candidates) == 0 {
		return nil, types.ErrRouteNotFound
	}
	
	// Routes are already sorted by priority in the candidates list
	for _, route := range candidates {
		compiledRoute := r.compiled[route.ID]
		
		// Skip routes with invalid regex (not in compiled map)
		if route.PathRegex != "" && compiledRoute == nil {
			continue
		}
		
		// Match path prefix
		if route.PathPrefix != "" && !strings.HasPrefix(req.URL.Path, route.PathPrefix) {
			continue
		}
		
		// Match path regex
		if route.PathRegex != "" && compiledRoute != nil && compiledRoute.pathRegexp != nil {
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
	
	// Clear and rebuild host router
	newHostRouter := newHostRouter()
	for _, route := range routes {
		newHostRouter.addRoute(route)
	}
	
	r.mu.Lock()
	r.routes = routes
	r.compiled = compiled
	r.hostRouter = newHostRouter
	r.mu.Unlock()
	
	r.logger.Info("loaded routes", "count", len(routes))
	return nil
}

// watchChanges watches for route changes in storage
func (r *router) watchChanges() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Cancel watch context when stopCh is closed
	go func() {
		<-r.stopCh
		cancel()
	}()
	
	events := r.storage.Watch(ctx)
	
	for {
		select {
		case <-r.stopCh:
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			if event.Kind != "route" && event.Kind != "service" {
				continue
			}
			
			r.logger.Debug("storage change detected",
				"type", event.Type,
				"kind", event.Kind,
				"id", event.ID,
			)
			
			// Reload routes on any change
			if err := r.loadRoutes(context.Background()); err != nil {
				r.logger.Error("failed to reload routes", "error", err)
			}
		}
	}
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

// Close stops the router and waits for goroutines to finish
func (r *router) Close() error {
	close(r.stopCh)
	r.wg.Wait()
	return nil
}
