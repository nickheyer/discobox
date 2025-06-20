package router

import (
	"context"
	"sort"
	"strings"
	"sync"
	
	"github.com/gorilla/mux"
	"net/http"

	"discobox/internal/types"
)

// pathRouter uses gorilla/mux for advanced path-based routing
type pathRouter struct {
	mu      sync.RWMutex
	mux     *mux.Router
	routes  map[string]*types.Route
	storage types.Storage
}

// NewPathRouter creates a new path-based router using gorilla/mux
func NewPathRouter(storage types.Storage) types.Router {
	pr := &pathRouter{
		mux:     mux.NewRouter(),
		routes:  make(map[string]*types.Route),
		storage: storage,
	}

	// Configure the mux router
	pr.mux.UseEncodedPath()
	pr.mux.SkipClean(false)

	return pr
}

// Match finds the best route for a request
func (p *pathRouter) Match(req *http.Request) (*types.Route, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var matchedRoute *types.Route

	// Use gorilla/mux matching
	var match mux.RouteMatch
	if p.mux.Match(req, &match) {
		routeID := match.Route.GetName()
		if routeID != "" {
			if route, exists := p.routes[routeID]; exists {
				matchedRoute = route
			}
		}
	}

	if matchedRoute == nil {
		return nil, types.ErrRouteNotFound
	}

	return matchedRoute, nil
}

// AddRoute adds a new route
func (p *pathRouter) AddRoute(route *types.Route) error {
	if route == nil {
		return types.ErrInvalidRequest
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Add to mux router
	muxRoute := p.mux.NewRoute().Name(route.ID)

	// Configure host
	if route.Host != "" {
		muxRoute.Host(route.Host)
	}

	// Configure path
	if route.PathRegex != "" {
		muxRoute.PathPrefix(route.PathRegex)
	} else if route.PathPrefix != "" {
		muxRoute.PathPrefix(route.PathPrefix)
	}

	// Configure headers
	for key, value := range route.Headers {
		muxRoute.Headers(key, value)
	}

	// Configure priority (gorilla/mux doesn't support priority directly)
	// We'll handle this by sorting routes before adding them

	// Store route
	p.routes[route.ID] = route

	// Store in storage
	return p.storage.CreateRoute(context.Background(), route)
}

// RemoveRoute removes a route
func (p *pathRouter) RemoveRoute(routeID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Remove from internal map
	delete(p.routes, routeID)

	// Rebuild mux router (gorilla/mux doesn't support route removal)
	p.rebuildRouter()

	// Remove from storage
	return p.storage.DeleteRoute(context.Background(), routeID)
}

// UpdateRoute updates an existing route
func (p *pathRouter) UpdateRoute(route *types.Route) error {
	if route == nil {
		return types.ErrInvalidRequest
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Update internal map
	p.routes[route.ID] = route

	// Rebuild mux router
	p.rebuildRouter()

	// Update in storage
	return p.storage.UpdateRoute(context.Background(), route)
}

// GetRoutes returns all routes
func (p *pathRouter) GetRoutes() ([]*types.Route, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	routes := make([]*types.Route, 0, len(p.routes))
	for _, route := range p.routes {
		routes = append(routes, route)
	}

	// Sort by priority
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].Priority > routes[j].Priority
	})

	return routes, nil
}

// rebuildRouter rebuilds the mux router with all current routes
func (p *pathRouter) rebuildRouter() {
	// Create a new router
	newMux := mux.NewRouter()
	newMux.UseEncodedPath()
	newMux.SkipClean(false)

	// Get all routes sorted by priority
	routes := make([]*types.Route, 0, len(p.routes))
	for _, route := range p.routes {
		routes = append(routes, route)
	}
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].Priority > routes[j].Priority
	})

	// Add routes to new mux
	for _, route := range routes {
		muxRoute := newMux.NewRoute().Name(route.ID)

		if route.Host != "" {
			muxRoute.Host(route.Host)
		}

		if route.PathRegex != "" {
			muxRoute.Path(route.PathRegex)
		} else if route.PathPrefix != "" {
			muxRoute.PathPrefix(route.PathPrefix)
		}

		for key, value := range route.Headers {
			muxRoute.Headers(key, value)
		}
	}

	// Replace old router
	p.mux = newMux
}

// PathPrefixRouter is a simple prefix-based router for performance
type PathPrefixRouter struct {
	mu       sync.RWMutex
	prefixes map[string][]*types.Route // Map of prefix to routes
	routes   map[string]*types.Route   // All routes by ID
}

// NewPathPrefixRouter creates a simple prefix-based router
func NewPathPrefixRouter() *PathPrefixRouter {
	return &PathPrefixRouter{
		prefixes: make(map[string][]*types.Route),
		routes:   make(map[string]*types.Route),
	}
}

// AddRoute adds a route to the prefix router
func (p *PathPrefixRouter) AddRoute(route *types.Route) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.routes[route.ID] = route

	if route.PathPrefix != "" {
		p.prefixes[route.PathPrefix] = append(p.prefixes[route.PathPrefix], route)
	}
}

// FindRoutes finds routes matching the given path
func (p *PathPrefixRouter) FindRoutes(path string) []*types.Route {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var matches []*types.Route

	// Check each prefix
	for prefix, routes := range p.prefixes {
		if strings.HasPrefix(path, prefix) {
			matches = append(matches, routes...)
		}
	}

	// Sort by prefix length (longest first) and priority
	sort.Slice(matches, func(i, j int) bool {
		// Longer prefixes have higher priority
		if len(matches[i].PathPrefix) != len(matches[j].PathPrefix) {
			return len(matches[i].PathPrefix) > len(matches[j].PathPrefix)
		}
		// Then sort by configured priority
		return matches[i].Priority > matches[j].Priority
	})

	return matches
}

// Clear removes all routes
func (p *PathPrefixRouter) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.prefixes = make(map[string][]*types.Route)
	p.routes = make(map[string]*types.Route)
}
