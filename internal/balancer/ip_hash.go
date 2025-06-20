package balancer

import (
	"context"
	"discobox/internal/types"
	"fmt"
	"hash/crc32"
	"net"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
)

// ipHash implements IP hash load balancing with consistent hashing
type ipHash struct {
	mu           sync.RWMutex
	servers      map[string]*types.Server
	ring         *consistentHash
	fallbackFunc func(context.Context, *http.Request, []*types.Server) (*types.Server, error)
}

// NewIPHash creates a new IP hash load balancer
func NewIPHash() types.LoadBalancer {
	return &ipHash{
		servers:      make(map[string]*types.Server),
		ring:         newConsistentHash(150), // 150 virtual nodes per server
		fallbackFunc: NewRoundRobin().Select, // Fallback to round-robin
	}
}

// Select returns a server based on client IP hash
func (ih *ipHash) Select(ctx context.Context, req *http.Request, servers []*types.Server) (*types.Server, error) {
	if len(servers) == 0 {
		return nil, types.ErrNoHealthyBackends
	}
	
	// Get client IP
	clientIP := getClientIP(req)
	if clientIP == "" {
		// Fallback if we can't determine client IP
		return ih.fallbackFunc(ctx, req, servers)
	}
	
	ih.mu.RLock()
	defer ih.mu.RUnlock()
	
	// Get server ID from consistent hash
	serverID := ih.ring.Get(clientIP)
	
	// Find the server in the provided list
	for _, server := range servers {
		if server.ID == serverID && server.Healthy {
			// Check max connections
			if server.MaxConns > 0 && atomic.LoadInt64(&server.ActiveConns) >= int64(server.MaxConns) {
				// Try next server in the ring
				return ih.selectNextAvailable(ctx, req, servers, clientIP)
			}
			return server, nil
		}
	}
	
	// types.Server not found or unhealthy, try next in ring
	return ih.selectNextAvailable(ctx, req, servers, clientIP)
}

// selectNextAvailable finds the next available server in the hash ring
func (ih *ipHash) selectNextAvailable(ctx context.Context, req *http.Request, servers []*types.Server, key string) (*types.Server, error) {
	serverIDs := ih.ring.GetN(key, len(servers))
	
	for _, serverID := range serverIDs {
		for _, server := range servers {
			if server.ID == serverID && server.Healthy {
				// Check max connections
				if server.MaxConns == 0 || atomic.LoadInt64(&server.ActiveConns) < int64(server.MaxConns) {
					return server, nil
				}
			}
		}
	}
	
	// No available servers in hash ring, fallback
	return ih.fallbackFunc(ctx, req, servers)
}

// Add adds a new server to the pool
func (ih *ipHash) Add(server *types.Server) error {
	if server == nil || server.ID == "" {
		return types.ErrInvalidRequest
	}
	
	ih.mu.Lock()
	defer ih.mu.Unlock()
	
	ih.servers[server.ID] = server
	ih.ring.Add(server.ID)
	
	return nil
}

// Remove removes a server from the pool
func (ih *ipHash) Remove(serverID string) error {
	ih.mu.Lock()
	defer ih.mu.Unlock()
	
	delete(ih.servers, serverID)
	ih.ring.Remove(serverID)
	
	return nil
}

// UpdateWeight updates server weight (affects virtual nodes)
func (ih *ipHash) UpdateWeight(serverID string, weight int) error {
	ih.mu.Lock()
	defer ih.mu.Unlock()
	
	if server, exists := ih.servers[serverID]; exists {
		server.Weight = weight
		// Update virtual nodes based on weight
		ih.ring.UpdateWeight(serverID, weight)
	}
	
	return nil
}

// getClientIP extracts the client IP from the request
func getClientIP(req *http.Request) string {
	// Check X-Forwarded-For header first
	xff := req.Header.Get("X-Forwarded-For")
	if xff != "" {
		// Take the first IP in the chain
		if idx := indexByte(xff, ','); idx != -1 {
			xff = xff[:idx]
		}
		if ip := net.ParseIP(trimSpace(xff)); ip != nil {
			return ip.String()
		}
	}
	
	// Check X-Real-IP header
	if xri := req.Header.Get("X-Real-IP"); xri != "" {
		if ip := net.ParseIP(xri); ip != nil {
			return ip.String()
		}
	}
	
	// Fall back to RemoteAddr
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		// RemoteAddr might be just an IP without port
		return req.RemoteAddr
	}
	
	return host
}

// consistentHash implements consistent hashing
type consistentHash struct {
	mu           sync.RWMutex
	replicas     int
	circle       map[uint32]string
	sortedHashes []uint32
	nodeWeights  map[string]int
}

// newConsistentHash creates a new consistent hash
func newConsistentHash(replicas int) *consistentHash {
	return &consistentHash{
		replicas:     replicas,
		circle:       make(map[uint32]string),
		nodeWeights:  make(map[string]int),
		sortedHashes: make([]uint32, 0),
	}
}

// Add adds a node to the hash ring
func (ch *consistentHash) Add(node string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	
	weight := ch.nodeWeights[node]
	if weight <= 0 {
		weight = 1
	}
	
	// Add virtual nodes based on weight
	virtualNodes := ch.replicas * weight
	for i := 0; i < virtualNodes; i++ {
		hash := ch.hash(fmt.Sprintf("%s:%d", node, i))
		ch.circle[hash] = node
	}
	
	ch.updateSortedHashes()
}

// Remove removes a node from the hash ring
func (ch *consistentHash) Remove(node string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	
	weight := ch.nodeWeights[node]
	if weight <= 0 {
		weight = 1
	}
	
	// Remove virtual nodes
	virtualNodes := ch.replicas * weight
	for i := 0; i < virtualNodes; i++ {
		hash := ch.hash(fmt.Sprintf("%s:%d", node, i))
		delete(ch.circle, hash)
	}
	
	delete(ch.nodeWeights, node)
	ch.updateSortedHashes()
}

// UpdateWeight updates the weight of a node
func (ch *consistentHash) UpdateWeight(node string, weight int) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	
	// Remove old virtual nodes
	oldWeight := ch.nodeWeights[node]
	if oldWeight <= 0 {
		oldWeight = 1
	}
	
	virtualNodes := ch.replicas * oldWeight
	for i := 0; i < virtualNodes; i++ {
		hash := ch.hash(fmt.Sprintf("%s:%d", node, i))
		delete(ch.circle, hash)
	}
	
	// Add new virtual nodes with new weight
	if weight <= 0 {
		weight = 1
	}
	ch.nodeWeights[node] = weight
	
	virtualNodes = ch.replicas * weight
	for i := 0; i < virtualNodes; i++ {
		hash := ch.hash(fmt.Sprintf("%s:%d", node, i))
		ch.circle[hash] = node
	}
	
	ch.updateSortedHashes()
}

// Get returns the node for a given key
func (ch *consistentHash) Get(key string) string {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	
	if len(ch.circle) == 0 {
		return ""
	}
	
	hash := ch.hash(key)
	
	// Binary search for the first node with hash >= key hash
	idx := sort.Search(len(ch.sortedHashes), func(i int) bool {
		return ch.sortedHashes[i] >= hash
	})
	
	// Wrap around if necessary
	if idx == len(ch.sortedHashes) {
		idx = 0
	}
	
	return ch.circle[ch.sortedHashes[idx]]
}

// GetN returns N nodes for a given key
func (ch *consistentHash) GetN(key string, n int) []string {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	
	if len(ch.circle) == 0 {
		return nil
	}
	
	if n > len(ch.nodeWeights) {
		n = len(ch.nodeWeights)
	}
	
	hash := ch.hash(key)
	nodes := make([]string, 0, n)
	seen := make(map[string]bool)
	
	// Find starting position
	idx := sort.Search(len(ch.sortedHashes), func(i int) bool {
		return ch.sortedHashes[i] >= hash
	})
	
	// Collect n unique nodes
	for i := 0; i < len(ch.sortedHashes) && len(nodes) < n; i++ {
		actualIdx := (idx + i) % len(ch.sortedHashes)
		node := ch.circle[ch.sortedHashes[actualIdx]]
		
		if !seen[node] {
			seen[node] = true
			nodes = append(nodes, node)
		}
	}
	
	return nodes
}

// hash generates a hash for a key
func (ch *consistentHash) hash(key string) uint32 {
	return crc32.ChecksumIEEE([]byte(key))
}

// updateSortedHashes updates the sorted list of hashes
func (ch *consistentHash) updateSortedHashes() {
	hashes := make([]uint32, 0, len(ch.circle))
	for hash := range ch.circle {
		hashes = append(hashes, hash)
	}
	sort.Slice(hashes, func(i, j int) bool {
		return hashes[i] < hashes[j]
	})
	ch.sortedHashes = hashes
}

// Helper functions

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	
	for start < end && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	
	return s[start:end]
}
