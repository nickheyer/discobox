package balancer

import (
	"context"
	"crypto/rand"
	"discobox/internal/types"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

// stickySession wraps a load balancer with session affinity
type stickySession struct {
	base       types.LoadBalancer
	cookieName string
	ttl        time.Duration
	mu         sync.RWMutex
	sessions   map[string]*sessionEntry
	ticker     *time.Ticker
	stopCh     chan struct{}
}

type sessionEntry struct {
	serverID  string
	expiresAt time.Time
}

// NewStickySession creates a new sticky session load balancer
func NewStickySession(base types.LoadBalancer, cookieName string, ttl time.Duration) types.LoadBalancer {
	if cookieName == "" {
		cookieName = "lb_session"
	}
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	
	ss := &stickySession{
		base:       base,
		cookieName: cookieName,
		ttl:        ttl,
		sessions:   make(map[string]*sessionEntry),
		ticker:     time.NewTicker(5 * time.Minute), // Cleanup interval
		stopCh:     make(chan struct{}),
	}
	
	// Start cleanup goroutine
	go ss.cleanupLoop()
	
	return ss
}

// Select returns a server based on session affinity
func (ss *stickySession) Select(ctx context.Context, req *http.Request, servers []*types.Server) (*types.Server, error) {
	// Check for existing session
	cookie, err := req.Cookie(ss.cookieName)
	if err == nil && cookie.Value != "" {
		ss.mu.RLock()
		session, exists := ss.sessions[cookie.Value]
		ss.mu.RUnlock()
		
		if exists && session.expiresAt.After(time.Now()) {
			// Find the server in the list
			for _, server := range servers {
				if server.ID == session.serverID && server.Healthy {
					// Extend session
					ss.mu.Lock()
					session.expiresAt = time.Now().Add(ss.ttl)
					ss.mu.Unlock()
					
					return server, nil
				}
			}
		}
	}
	
	// No valid session, select new server
	server, err := ss.base.Select(ctx, req, servers)
	if err != nil {
		return nil, err
	}
	
	// Create new session
	sessionID := generateSessionID()
	ss.mu.Lock()
	ss.sessions[sessionID] = &sessionEntry{
		serverID:  server.ID,
		expiresAt: time.Now().Add(ss.ttl),
	}
	ss.mu.Unlock()
	
	// Set cookie on the response writer if available
	if rw, ok := ctx.Value("responseWriter").(http.ResponseWriter); ok {
		http.SetCookie(rw, &http.Cookie{
			Name:     ss.cookieName,
			Value:    sessionID,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   int(ss.ttl.Seconds()),
		})
	}
	
	return server, nil
}

// Add adds a new server to the pool
func (ss *stickySession) Add(server *types.Server) error {
	return ss.base.Add(server)
}

// Remove removes a server from the pool
func (ss *stickySession) Remove(serverID string) error {
	// Remove sessions for this server
	ss.mu.Lock()
	for sessionID, session := range ss.sessions {
		if session.serverID == serverID {
			delete(ss.sessions, sessionID)
		}
	}
	ss.mu.Unlock()
	
	return ss.base.Remove(serverID)
}

// UpdateWeight updates server weight
func (ss *stickySession) UpdateWeight(serverID string, weight int) error {
	return ss.base.UpdateWeight(serverID, weight)
}

// cleanupLoop periodically removes expired sessions
func (ss *stickySession) cleanupLoop() {
	for {
		select {
		case <-ss.ticker.C:
			ss.cleanup()
		case <-ss.stopCh:
			ss.ticker.Stop()
			return
		}
	}
}

// cleanup removes expired sessions
func (ss *stickySession) cleanup() {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	
	now := time.Now()
	for sessionID, session := range ss.sessions {
		if session.expiresAt.Before(now) {
			delete(ss.sessions, sessionID)
		}
	}
}

// Stop stops the cleanup goroutine
func (ss *stickySession) Stop() {
	close(ss.stopCh)
}

// generateSessionID generates a random session ID
func generateSessionID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based ID
		return hex.EncodeToString([]byte(time.Now().String()))
	}
	return hex.EncodeToString(bytes)
}

// IPStickySession implements IP-based session affinity
type IPStickySession struct {
	base     types.LoadBalancer
	ttl      time.Duration
	mu       sync.RWMutex
	sessions map[string]*sessionEntry
	ticker   *time.Ticker
	stopCh   chan struct{}
}

// NewIPStickySession creates a new IP-based sticky session load balancer
func NewIPStickySession(base types.LoadBalancer, ttl time.Duration) types.LoadBalancer {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	
	iss := &IPStickySession{
		base:     base,
		ttl:      ttl,
		sessions: make(map[string]*sessionEntry),
		ticker:   time.NewTicker(5 * time.Minute),
		stopCh:   make(chan struct{}),
	}
	
	go iss.cleanupLoop()
	
	return iss
}

// Select returns a server based on client IP affinity
func (iss *IPStickySession) Select(ctx context.Context, req *http.Request, servers []*types.Server) (*types.Server, error) {
	clientIP := getClientIP(req)
	if clientIP == "" {
		// Can't determine IP, fall back to base balancer
		return iss.base.Select(ctx, req, servers)
	}
	
	// Check for existing session
	iss.mu.RLock()
	session, exists := iss.sessions[clientIP]
	iss.mu.RUnlock()
	
	if exists && session.expiresAt.After(time.Now()) {
		// Find the server
		for _, server := range servers {
			if server.ID == session.serverID && server.Healthy {
				// Extend session
				iss.mu.Lock()
				session.expiresAt = time.Now().Add(iss.ttl)
				iss.mu.Unlock()
				
				return server, nil
			}
		}
	}
	
	// No valid session, select new server
	server, err := iss.base.Select(ctx, req, servers)
	if err != nil {
		return nil, err
	}
	
	// Create new session
	iss.mu.Lock()
	iss.sessions[clientIP] = &sessionEntry{
		serverID:  server.ID,
		expiresAt: time.Now().Add(iss.ttl),
	}
	iss.mu.Unlock()
	
	return server, nil
}

// Add adds a new server to the pool
func (iss *IPStickySession) Add(server *types.Server) error {
	return iss.base.Add(server)
}

// Remove removes a server from the pool
func (iss *IPStickySession) Remove(serverID string) error {
	// Remove sessions for this server
	iss.mu.Lock()
	for ip, session := range iss.sessions {
		if session.serverID == serverID {
			delete(iss.sessions, ip)
		}
	}
	iss.mu.Unlock()
	
	return iss.base.Remove(serverID)
}

// UpdateWeight updates server weight
func (iss *IPStickySession) UpdateWeight(serverID string, weight int) error {
	return iss.base.UpdateWeight(serverID, weight)
}

// cleanupLoop periodically removes expired sessions
func (iss *IPStickySession) cleanupLoop() {
	for {
		select {
		case <-iss.ticker.C:
			iss.cleanup()
		case <-iss.stopCh:
			iss.ticker.Stop()
			return
		}
	}
}

// cleanup removes expired sessions
func (iss *IPStickySession) cleanup() {
	iss.mu.Lock()
	defer iss.mu.Unlock()
	
	now := time.Now()
	for ip, session := range iss.sessions {
		if session.expiresAt.Before(now) {
			delete(iss.sessions, ip)
		}
	}
}

// Stop stops the cleanup goroutine
func (iss *IPStickySession) Stop() {
	close(iss.stopCh)
}
