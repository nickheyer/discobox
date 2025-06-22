package middleware

import (
	"context"
	"discobox/internal/types"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// limiterEntry wraps a rate limiter with last access time
type limiterEntry struct {
	limiter    *rate.Limiter
	lastAccess time.Time
	mu         sync.Mutex
}

// rateLimiter implements rate limiting middleware
type rateLimiter struct {
	limiters map[string]*limiterEntry
	mu       sync.RWMutex
	rps      int
	burst    int
	byHeader string
	keyFunc  func(*http.Request) string
	ttl      time.Duration // Time-to-live for idle limiters
	stopCh   chan struct{}
}

// RateLimit creates rate limiting middleware
func RateLimit(config types.ProxyConfig) types.Middleware {
	rl := &rateLimiter{
		limiters: make(map[string]*limiterEntry),
		rps:      config.RateLimit.RPS,
		burst:    config.RateLimit.Burst,
		byHeader: config.RateLimit.ByHeader,
		ttl:      5 * time.Minute, // Default TTL for idle limiters
		stopCh:   make(chan struct{}),
	}
	
	// Set key function based on configuration
	if rl.byHeader != "" {
		rl.keyFunc = func(r *http.Request) string {
			return r.Header.Get(rl.byHeader)
		}
	} else {
		rl.keyFunc = getClientIP
	}
	
	// Start cleanup goroutine
	go rl.cleanup()
	
	return rl.Middleware
}

// Middleware returns the middleware handler
func (rl *rateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := rl.keyFunc(r)
		if key == "" {
			// No key, allow request
			next.ServeHTTP(w, r)
			return
		}
		
		limiter := rl.getLimiter(key)
		if !limiter.Allow() {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

// getLimiter returns a limiter for the given key
func (rl *rateLimiter) getLimiter(key string) *rate.Limiter {
	rl.mu.RLock()
	entry, exists := rl.limiters[key]
	rl.mu.RUnlock()
	
	if exists {
		// Update last access time
		entry.mu.Lock()
		entry.lastAccess = time.Now()
		entry.mu.Unlock()
		return entry.limiter
	}
	
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	// Double-check
	if entry, exists := rl.limiters[key]; exists {
		entry.mu.Lock()
		entry.lastAccess = time.Now()
		entry.mu.Unlock()
		return entry.limiter
	}
	
	// Create new limiter entry
	entry = &limiterEntry{
		limiter:    rate.NewLimiter(rate.Limit(rl.rps), rl.burst),
		lastAccess: time.Now(),
	}
	rl.limiters[key] = entry
	
	return entry.limiter
}

// cleanup periodically removes unused limiters
func (rl *rateLimiter) cleanup() {
	ticker := time.NewTicker(1 * time.Minute) // Check every minute
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			rl.cleanupStale()
		case <-rl.stopCh:
			return
		}
	}
}

// cleanupStale removes limiters that haven't been accessed recently
func (rl *rateLimiter) cleanupStale() {
	now := time.Now()
	expiredKeys := make([]string, 0)
	
	// First pass: identify expired entries
	rl.mu.RLock()
	for key, entry := range rl.limiters {
		entry.mu.Lock()
		if now.Sub(entry.lastAccess) > rl.ttl {
			expiredKeys = append(expiredKeys, key)
		}
		entry.mu.Unlock()
	}
	rl.mu.RUnlock()
	
	// Second pass: remove expired entries
	if len(expiredKeys) > 0 {
		rl.mu.Lock()
		for _, key := range expiredKeys {
			// Double-check the entry is still expired
			if entry, exists := rl.limiters[key]; exists {
				entry.mu.Lock()
				if now.Sub(entry.lastAccess) > rl.ttl {
					delete(rl.limiters, key)
				}
				entry.mu.Unlock()
			}
		}
		rl.mu.Unlock()
	}
}

// Stop stops the cleanup goroutine
func (rl *rateLimiter) Stop() {
	close(rl.stopCh)
}

// getClientIP extracts client IP from request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	
	// Check X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		if net.ParseIP(xri) != nil {
			return xri
		}
	}
	
	// Use RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	
	return host
}

// CustomRateLimiter allows custom rate limiting implementations
type CustomRateLimiter struct {
	limiter types.RateLimiter
	keyFunc func(*http.Request) string
}

// NewCustomRateLimiter creates middleware with a custom rate limiter
func NewCustomRateLimiter(limiter types.RateLimiter, keyFunc func(*http.Request) string) types.Middleware {
	rl := &CustomRateLimiter{
		limiter: limiter,
		keyFunc: keyFunc,
	}
	
	if rl.keyFunc == nil {
		rl.keyFunc = getClientIP
	}
	
	return rl.Middleware
}

// Middleware returns the middleware handler
func (rl *CustomRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := rl.keyFunc(r)
		if key == "" {
			next.ServeHTTP(w, r)
			return
		}
		
		if !rl.limiter.Allow(key) {
			// Try to wait if possible
			ctx, cancel := context.WithTimeout(r.Context(), 100*time.Millisecond)
			defer cancel()
			
			if err := rl.limiter.Wait(ctx, key); err != nil {
				w.Header().Set("X-RateLimit-Limit", string(rune(rl.limiter.Limit(key))))
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("Retry-After", "1")
				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}
		}
		
		next.ServeHTTP(w, r)
	})
}

// TokenBucketRateLimiter implements a simple token bucket rate limiter
type TokenBucketRateLimiter struct {
	mu       sync.RWMutex
	buckets  map[string]*tokenBucket
	capacity int
	refill   int
	interval time.Duration
}

type tokenBucket struct {
	tokens     int
	lastCheck  time.Time
	lastAccess time.Time
	mu         sync.Mutex
}

// NewTokenBucketRateLimiter creates a token bucket rate limiter
func NewTokenBucketRateLimiter(capacity, refillRate int, interval time.Duration) types.RateLimiter {
	tb := &TokenBucketRateLimiter{
		buckets:  make(map[string]*tokenBucket),
		capacity: capacity,
		refill:   refillRate,
		interval: interval,
	}
	
	// Start cleanup goroutine for token buckets
	go tb.cleanup()
	
	return tb
}

// Allow checks if a request should be allowed
func (t *TokenBucketRateLimiter) Allow(key string) bool {
	bucket := t.getBucket(key)
	
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	
	// Update last access time
	now := time.Now()
	bucket.lastAccess = now
	
	// Refill tokens
	elapsed := now.Sub(bucket.lastCheck)
	tokensToAdd := int(elapsed / t.interval) * t.refill
	
	if tokensToAdd > 0 {
		bucket.tokens = min(bucket.tokens+tokensToAdd, t.capacity)
		bucket.lastCheck = now
	}
	
	// Check if we have tokens
	if bucket.tokens > 0 {
		bucket.tokens--
		return true
	}
	
	return false
}

// Wait blocks until the request can proceed
func (t *TokenBucketRateLimiter) Wait(ctx context.Context, key string) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if t.Allow(key) {
				return nil
			}
		}
	}
}

// Limit returns the current limit for a key
func (t *TokenBucketRateLimiter) Limit(key string) int {
	return t.capacity
}

// getBucket returns or creates a bucket for the key
func (t *TokenBucketRateLimiter) getBucket(key string) *tokenBucket {
	t.mu.RLock()
	bucket, exists := t.buckets[key]
	t.mu.RUnlock()
	
	if exists {
		return bucket
	}
	
	t.mu.Lock()
	defer t.mu.Unlock()
	
	// Double-check
	if bucket, exists := t.buckets[key]; exists {
		return bucket
	}
	
	now := time.Now()
	bucket = &tokenBucket{
		tokens:     t.capacity,
		lastCheck:  now,
		lastAccess: now,
	}
	t.buckets[key] = bucket
	
	return bucket
}

// cleanup periodically removes unused token buckets
func (t *TokenBucketRateLimiter) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	
	ttl := 5 * time.Minute // Remove buckets idle for 5 minutes
	
	for range ticker.C {
		now := time.Now()
		expiredKeys := make([]string, 0)
		
		// First pass: identify expired buckets
		t.mu.RLock()
		for key, bucket := range t.buckets {
			bucket.mu.Lock()
			if now.Sub(bucket.lastAccess) > ttl {
				expiredKeys = append(expiredKeys, key)
			}
			bucket.mu.Unlock()
		}
		t.mu.RUnlock()
		
		// Second pass: remove expired buckets
		if len(expiredKeys) > 0 {
			t.mu.Lock()
			for _, key := range expiredKeys {
				// Double-check the bucket is still expired
				if bucket, exists := t.buckets[key]; exists {
					bucket.mu.Lock()
					if now.Sub(bucket.lastAccess) > ttl {
						delete(t.buckets, key)
					}
					bucket.mu.Unlock()
				}
			}
			t.mu.Unlock()
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
