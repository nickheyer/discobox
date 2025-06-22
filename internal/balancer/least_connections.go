package balancer

import (
	"context"
	"discobox/internal/types"
	"math"
	"net/http"
	"sync"
	"sync/atomic"
)

// leastConnections implements least connections load balancing
type leastConnections struct {
	mu      sync.RWMutex
	servers map[string]*types.Server
	counter uint64 // For round-robin when connections are equal
}

// NewLeastConnections creates a new least connections load balancer
func NewLeastConnections() types.LoadBalancer {
	return &leastConnections{
		servers: make(map[string]*types.Server),
	}
}

// Select returns the server with the least active connections
func (lc *leastConnections) Select(ctx context.Context, req *http.Request, servers []*types.Server) (*types.Server, error) {
	if len(servers) == 0 {
		return nil, types.ErrNoHealthyBackends
	}
	
	// First pass: find minimum connections and collect eligible servers
	minConnections := int64(math.MaxInt64)
	var eligibleServers []*types.Server
	
	for _, server := range servers {
		// Skip unhealthy servers
		if !server.Healthy {
			continue
		}
		
		activeConns := atomic.LoadInt64(&server.ActiveConns)
		
		// Check max connections limit
		if server.MaxConns > 0 && activeConns >= int64(server.MaxConns) {
			continue
		}
		
		// Track minimum connections
		if activeConns < minConnections {
			minConnections = activeConns
			eligibleServers = []*types.Server{server}
		} else if activeConns == minConnections {
			eligibleServers = append(eligibleServers, server)
		}
	}
	
	if len(eligibleServers) == 0 {
		return nil, types.ErrNoHealthyBackends
	}
	
	// If only one server has minimum connections, return it
	if len(eligibleServers) == 1 {
		return eligibleServers[0], nil
	}
	
	// Multiple servers have equal minimum connections
	// Use round-robin to distribute load fairly
	count := atomic.AddUint64(&lc.counter, 1)
	index := (count - 1) % uint64(len(eligibleServers))
	
	return eligibleServers[index], nil
}

// Add adds a new server to the pool
func (lc *leastConnections) Add(server *types.Server) error {
	if server == nil || server.ID == "" {
		return types.ErrInvalidRequest
	}
	
	lc.mu.Lock()
	defer lc.mu.Unlock()
	
	lc.servers[server.ID] = server
	return nil
}

// Remove removes a server from the pool
func (lc *leastConnections) Remove(serverID string) error {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	
	delete(lc.servers, serverID)
	return nil
}

// UpdateWeight updates server weight
func (lc *leastConnections) UpdateWeight(serverID string, weight int) error {
	if weight < 0 {
		return types.ErrInvalidWeight
	}
	
	lc.mu.Lock()
	defer lc.mu.Unlock()
	
	server, exists := lc.servers[serverID]
	if !exists {
		return types.ErrServerNotFound
	}
	
	server.Weight = weight
	
	return nil
}

// weightedLeastConnections implements weighted least connections
type weightedLeastConnections struct {
	mu      sync.RWMutex
	servers map[string]*types.Server
}

// NewWeightedLeastConnections creates a new weighted least connections load balancer
func NewWeightedLeastConnections() types.LoadBalancer {
	return &weightedLeastConnections{
		servers: make(map[string]*types.Server),
	}
}

// Select returns the server with the best connection-to-weight ratio
func (wlc *weightedLeastConnections) Select(ctx context.Context, req *http.Request, servers []*types.Server) (*types.Server, error) {
	if len(servers) == 0 {
		return nil, types.ErrNoHealthyBackends
	}
	
	var selected *types.Server
	minRatio := math.MaxFloat64
	
	for _, server := range servers {
		// Skip unhealthy servers
		if !server.Healthy {
			continue
		}
		
		activeConns := atomic.LoadInt64(&server.ActiveConns)
		
		// Check max connections limit
		if server.MaxConns > 0 && activeConns >= int64(server.MaxConns) {
			continue
		}
		
		// Calculate connection-to-weight ratio
		weight := float64(server.Weight)
		if weight <= 0 {
			weight = 1
		}
		
		ratio := float64(activeConns) / weight
		
		// Select server with lowest ratio
		if ratio < minRatio {
			minRatio = ratio
			selected = server
		}
	}
	
	if selected == nil {
		return nil, types.ErrNoHealthyBackends
	}
	
	return selected, nil
}

// Add adds a new server to the pool
func (wlc *weightedLeastConnections) Add(server *types.Server) error {
	if server == nil || server.ID == "" {
		return types.ErrInvalidRequest
	}
	
	wlc.mu.Lock()
	defer wlc.mu.Unlock()
	
	wlc.servers[server.ID] = server
	return nil
}

// Remove removes a server from the pool
func (wlc *weightedLeastConnections) Remove(serverID string) error {
	wlc.mu.Lock()
	defer wlc.mu.Unlock()
	
	delete(wlc.servers, serverID)
	return nil
}

// UpdateWeight updates server weight
func (wlc *weightedLeastConnections) UpdateWeight(serverID string, weight int) error {
	if weight < 0 {
		return types.ErrInvalidWeight
	}
	
	wlc.mu.Lock()
	defer wlc.mu.Unlock()
	
	server, exists := wlc.servers[serverID]
	if !exists {
		return types.ErrServerNotFound
	}
	
	server.Weight = weight
	
	return nil
}
