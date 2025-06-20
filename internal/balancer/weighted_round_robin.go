package balancer

import (
	"context"
	"discobox/internal/types"
	"net/http"
	"sync"
	"sync/atomic"
)

// weightedRoundRobin implements weighted round-robin load balancing
type weightedRoundRobin struct {
	mu              sync.RWMutex
	servers         map[string]*types.Server
	weightedServers []*types.Server // Expanded list based on weights
	counter         uint64
	totalWeight     int
}

// NewWeightedRoundRobin creates a new weighted round-robin load balancer
func NewWeightedRoundRobin() types.LoadBalancer {
	return &weightedRoundRobin{
		servers:         make(map[string]*types.Server),
		weightedServers: make([]*types.Server, 0),
	}
}

// Select returns the next server based on weights
func (wrr *weightedRoundRobin) Select(ctx context.Context, req *http.Request, servers []*types.Server) (*types.Server, error) {
	if len(servers) == 0 {
		return nil, types.ErrNoHealthyBackends
	}
	
	// Build weighted list if needed
	wrr.mu.RLock()
	needsRebuild := len(wrr.weightedServers) == 0
	wrr.mu.RUnlock()
	
	if needsRebuild {
		wrr.rebuildWeightedList(servers)
	}
	
	wrr.mu.RLock()
	defer wrr.mu.RUnlock()
	
	if len(wrr.weightedServers) == 0 {
		return nil, types.ErrNoHealthyBackends
	}
	
	// Try to find a healthy server with available connections
	attempts := len(wrr.weightedServers)
	for i := 0; i < attempts; i++ {
		count := atomic.AddUint64(&wrr.counter, 1)
		index := (count - 1) % uint64(len(wrr.weightedServers))
		
		selected := wrr.weightedServers[index]
		
		// Check if server is healthy
		if !selected.Healthy {
			continue
		}
		
		// Check max connections limit
		if selected.MaxConns > 0 && atomic.LoadInt64(&selected.ActiveConns) >= int64(selected.MaxConns) {
			continue
		}
		
		return selected, nil
	}
	
	return nil, types.ErrNoHealthyBackends
}

// Add adds a new server to the pool
func (wrr *weightedRoundRobin) Add(server *types.Server) error {
	if server == nil || server.ID == "" {
		return types.ErrInvalidRequest
	}
	
	wrr.mu.Lock()
	defer wrr.mu.Unlock()
	
	wrr.servers[server.ID] = server
	wrr.weightedServers = nil // Force rebuild on next select
	
	return nil
}

// Remove removes a server from the pool
func (wrr *weightedRoundRobin) Remove(serverID string) error {
	wrr.mu.Lock()
	defer wrr.mu.Unlock()
	
	delete(wrr.servers, serverID)
	wrr.weightedServers = nil // Force rebuild on next select
	
	return nil
}

// UpdateWeight updates server weight
func (wrr *weightedRoundRobin) UpdateWeight(serverID string, weight int) error {
	wrr.mu.Lock()
	defer wrr.mu.Unlock()
	
	if server, exists := wrr.servers[serverID]; exists {
		server.Weight = weight
		wrr.weightedServers = nil // Force rebuild on next select
	}
	
	return nil
}

// rebuildWeightedList rebuilds the weighted server list
func (wrr *weightedRoundRobin) rebuildWeightedList(servers []*types.Server) {
	wrr.mu.Lock()
	defer wrr.mu.Unlock()
	
	// Clear existing list
	wrr.weightedServers = make([]*types.Server, 0)
	wrr.totalWeight = 0
	
	// Build new weighted list
	for _, server := range servers {
		if server.Healthy {
			weight := server.Weight
			if weight <= 0 {
				weight = 1 // Default weight
			}
			
			// Add server to list 'weight' times
			for i := 0; i < weight; i++ {
				wrr.weightedServers = append(wrr.weightedServers, server)
			}
			
			wrr.totalWeight += weight
		}
	}
}

// smoothWeightedRoundRobin implements smooth weighted round-robin
type smoothWeightedRoundRobin struct {
	mu      sync.RWMutex
	servers map[string]*weightedServer
}

type weightedServer struct {
	*types.Server
	currentWeight  int
	effectiveWeight int
}

// NewSmoothWeightedRoundRobin creates a smooth weighted round-robin load balancer
func NewSmoothWeightedRoundRobin() types.LoadBalancer {
	return &smoothWeightedRoundRobin{
		servers: make(map[string]*weightedServer),
	}
}

// Select implements smooth weighted round-robin algorithm
func (swrr *smoothWeightedRoundRobin) Select(ctx context.Context, req *http.Request, servers []*types.Server) (*types.Server, error) {
	if len(servers) == 0 {
		return nil, types.ErrNoHealthyBackends
	}
	
	swrr.mu.Lock()
	defer swrr.mu.Unlock()
	
	// Initialize or update weighted servers
	swrr.updateServers(servers)
	
	totalWeight := 0
	var selected *weightedServer
	
	// Find server with highest current weight
	for _, ws := range swrr.servers {
		if !ws.Healthy {
			continue
		}
		
		// Check max connections
		if ws.MaxConns > 0 && atomic.LoadInt64(&ws.ActiveConns) >= int64(ws.MaxConns) {
			continue
		}
		
		ws.currentWeight += ws.effectiveWeight
		totalWeight += ws.effectiveWeight
		
		if selected == nil || ws.currentWeight > selected.currentWeight {
			selected = ws
		}
	}
	
	if selected == nil {
		return nil, types.ErrNoHealthyBackends
	}
	
	// Update selected server's current weight
	selected.currentWeight -= totalWeight
	
	return selected.Server, nil
}

// updateServers updates the internal server map
func (swrr *smoothWeightedRoundRobin) updateServers(servers []*types.Server) {
	// Remove servers that no longer exist
	serverMap := make(map[string]*types.Server)
	for _, s := range servers {
		serverMap[s.ID] = s
	}
	
	for id := range swrr.servers {
		if _, exists := serverMap[id]; !exists {
			delete(swrr.servers, id)
		}
	}
	
	// Add or update servers
	for _, server := range servers {
		if ws, exists := swrr.servers[server.ID]; exists {
			// Update existing server
			ws.Server = server
			if server.Weight > 0 {
				ws.effectiveWeight = server.Weight
			}
		} else {
			// Add new server
			weight := server.Weight
			if weight <= 0 {
				weight = 1
			}
			swrr.servers[server.ID] = &weightedServer{
				Server:          server,
				currentWeight:   0,
				effectiveWeight: weight,
			}
		}
	}
}

// Add adds a new server to the pool
func (swrr *smoothWeightedRoundRobin) Add(server *types.Server) error {
	if server == nil || server.ID == "" {
		return types.ErrInvalidRequest
	}
	
	swrr.mu.Lock()
	defer swrr.mu.Unlock()
	
	weight := server.Weight
	if weight <= 0 {
		weight = 1
	}
	
	swrr.servers[server.ID] = &weightedServer{
		Server:          server,
		currentWeight:   0,
		effectiveWeight: weight,
	}
	
	return nil
}

// Remove removes a server from the pool
func (swrr *smoothWeightedRoundRobin) Remove(serverID string) error {
	swrr.mu.Lock()
	defer swrr.mu.Unlock()
	
	delete(swrr.servers, serverID)
	return nil
}

// UpdateWeight updates server weight
func (swrr *smoothWeightedRoundRobin) UpdateWeight(serverID string, weight int) error {
	swrr.mu.Lock()
	defer swrr.mu.Unlock()
	
	if ws, exists := swrr.servers[serverID]; exists {
		if weight <= 0 {
			weight = 1
		}
		ws.effectiveWeight = weight
		ws.Server.Weight = weight
	}
	
	return nil
}
