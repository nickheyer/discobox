package router

import (
	"strings"
	"sync"

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
	
	// Note: Routes are already sorted by priority when they were added
	// The router maintains the priority order when loading from storage
	return routes
}


