package balancer

import (
	"context"
	"discobox/internal/types"
	"net/http"
	"sync"
	"sync/atomic"
)

// roundRobin implements round-robin load balancing
type roundRobin struct {
	counter uint64
	mu      sync.RWMutex
	servers map[string]*types.Server
}

// NewRoundRobin creates a new round-robin load balancer
func NewRoundRobin() types.LoadBalancer {
	return &roundRobin{
		servers: make(map[string]*types.Server),
	}
}

// Select returns the next server in round-robin fashion
func (rr *roundRobin) Select(ctx context.Context, req *http.Request, servers []*types.Server) (*types.Server, error) {
	if len(servers) == 0 {
		return nil, types.ErrNoHealthyBackends
	}
	
	// Filter healthy servers
	healthyServers := make([]*types.Server, 0, len(servers))
	for _, server := range servers {
		if server.Healthy {
			healthyServers = append(healthyServers, server)
		}
	}
	
	if len(healthyServers) == 0 {
		return nil, types.ErrNoHealthyBackends
	}
	
	// Round-robin selection
	count := atomic.AddUint64(&rr.counter, 1)
	index := (count - 1) % uint64(len(healthyServers))
	
	selected := healthyServers[index]
	
	// Check max connections limit
	if selected.MaxConns > 0 && atomic.LoadInt64(&selected.ActiveConns) >= int64(selected.MaxConns) {
		// Try next server
		for i := 1; i < len(healthyServers); i++ {
			nextIndex := (index + uint64(i)) % uint64(len(healthyServers))
			next := healthyServers[nextIndex]
			if next.MaxConns == 0 || atomic.LoadInt64(&next.ActiveConns) < int64(next.MaxConns) {
				return next, nil
			}
		}
		return nil, types.ErrMaxConnectionsReached
	}
	
	return selected, nil
}

// Add adds a new server to the pool
func (rr *roundRobin) Add(server *types.Server) error {
	if server == nil || server.ID == "" {
		return types.ErrInvalidRequest
	}
	
	rr.mu.Lock()
	defer rr.mu.Unlock()
	
	rr.servers[server.ID] = server
	return nil
}

// Remove removes a server from the pool
func (rr *roundRobin) Remove(serverID string) error {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	
	delete(rr.servers, serverID)
	return nil
}

// UpdateWeight updates server weight (no-op for round-robin)
func (rr *roundRobin) UpdateWeight(serverID string, weight int) error {
	// Round-robin doesn't use weights
	return nil
}
