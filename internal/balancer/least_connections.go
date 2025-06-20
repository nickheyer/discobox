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
	
	var selected *types.Server
	minConnections := int64(math.MaxInt64)
	
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
		
		// Select server with least connections
		if activeConns < minConnections {
			minConnections = activeConns
			selected = server
		} else if activeConns == minConnections && selected != nil {
			// If connections are equal, prefer server with higher weight
			if server.Weight > selected.Weight {
				selected = server
			}
		}
	}
	
	if selected == nil {
		return nil, types.ErrNoHealthyBackends
	}
	
	return selected, nil
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
	lc.mu.Lock()
	defer lc.mu.Unlock()
	
	if server, exists := lc.servers[serverID]; exists {
		server.Weight = weight
	}
	
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
	wlc.mu.Lock()
	defer wlc.mu.Unlock()
	
	if server, exists := wlc.servers[serverID]; exists {
		server.Weight = weight
	}
	
	return nil
}
