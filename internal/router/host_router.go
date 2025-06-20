package router

import (
	"strings"
	"sync"
	
	"net/http"

	"discobox/internal/types"
)

// hostRouter is an optimized router for host-based routing
type hostRouter struct {
	mu         sync.RWMutex
	exactHosts map[string][]*types.Route      // Exact host matches
	wildcards  map[string][]*types.Route      // Wildcard domains (*.example.com)
	allRoutes  []*types.Route                 // Routes without host constraints
}

// newHostRouter creates a new host-based router
func newHostRouter() *hostRouter {
	return &hostRouter{
		exactHosts: make(map[string][]*types.Route),
		wildcards:  make(map[string][]*types.Route),
		allRoutes:  make([]*types.Route, 0),
	}
}

// addRoute adds a route to the host router
func (h *hostRouter) addRoute(route *types.Route) {
	h.mu.Lock()
	defer h.mu.Unlock()
	
	if route.Host == "" {
		// Route matches all hosts
		h.allRoutes = append(h.allRoutes, route)
		return
	}
	
	if strings.HasPrefix(route.Host, "*.") {
		// Wildcard host
		domain := route.Host[1:] // Remove * prefix
		h.wildcards[domain] = append(h.wildcards[domain], route)
	} else {
		// Exact host
		h.exactHosts[route.Host] = append(h.exactHosts[route.Host], route)
	}
}

// findRoutes returns all routes that could match the given host
func (h *hostRouter) findRoutes(host string) []*types.Route {
	h.mu.RLock()
	defer h.mu.RUnlock()
	
	// Remove port if present
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	
	var routes []*types.Route
	
	// Check exact match first
	if exactRoutes, ok := h.exactHosts[host]; ok {
		routes = append(routes, exactRoutes...)
	}
	
	// Check wildcard matches
	for domain, wildcardRoutes := range h.wildcards {
		if strings.HasSuffix(host, domain) {
			routes = append(routes, wildcardRoutes...)
		}
	}
	
	// Add routes without host constraints
	routes = append(routes, h.allRoutes...)
	
	return routes
}

// clear removes all routes
func (h *hostRouter) clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	
	h.exactHosts = make(map[string][]*types.Route)
	h.wildcards = make(map[string][]*types.Route)
	h.allRoutes = make([]*types.Route, 0)
}

// HostBasedRouter wraps the main router with host-based optimization
type HostBasedRouter struct {
	types.Router
	hostRouter *hostRouter
	mu         sync.RWMutex
}

// NewHostBasedRouter creates a router optimized for host-based routing
func NewHostBasedRouter(baseRouter types.Router) *HostBasedRouter {
	return &HostBasedRouter{
		Router:     baseRouter,
		hostRouter: newHostRouter(),
	}
}

// Match finds the best route for a request using host-based optimization
func (r *HostBasedRouter) Match(req *http.Request) (*types.Route, error) {
	// Get potential routes based on host
	candidates := r.hostRouter.findRoutes(req.Host)
	
	if len(candidates) == 0 {
		return nil, types.ErrRouteNotFound
	}
	
	// If only one candidate, check if it matches other criteria
	if len(candidates) == 1 {
		route := candidates[0]
		if r.matchesPath(req, route) && r.matchesHeaders(req, route) {
			return route, nil
		}
		return nil, types.ErrRouteNotFound
	}
	
	// Multiple candidates - fall back to base router
	return r.Router.Match(req)
}

// matchesPath checks if request path matches route requirements
func (r *HostBasedRouter) matchesPath(req *http.Request, route *types.Route) bool {
	// Check path prefix
	if route.PathPrefix != "" && !strings.HasPrefix(req.URL.Path, route.PathPrefix) {
		return false
	}
	
	// Path regex matching would require compilation, so we skip it here
	// and let the base router handle it
	
	return true
}

// matchesHeaders checks if request headers match route requirements
func (r *HostBasedRouter) matchesHeaders(req *http.Request, route *types.Route) bool {
	if len(route.Headers) == 0 {
		return true
	}
	
	for key, value := range route.Headers {
		if req.Header.Get(key) != value {
			return false
		}
	}
	
	return true
}
