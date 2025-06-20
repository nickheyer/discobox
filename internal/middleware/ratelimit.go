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

// rateLimiter implements rate limiting middleware
type rateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	rps      int
	burst    int
	byHeader string
	keyFunc  func(*http.Request) string
}

// RateLimit creates rate limiting middleware
func RateLimit(config types.ProxyConfig) types.Middleware {
	rl := &rateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rps:      config.RateLimit.RPS,
		burst:    config.RateLimit.Burst,
		byHeader: config.RateLimit.ByHeader,
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
	limiter, exists := rl.limiters[key]
	rl.mu.RUnlock()
	
	if exists {
		return limiter
	}
	
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	// Double-check
	if limiter, exists := rl.limiters[key]; exists {
		return limiter
	}
	
	limiter = rate.NewLimiter(rate.Limit(rl.rps), rl.burst)
	rl.limiters[key] = limiter
	
	return limiter
}

// cleanup periodically removes unused limiters
func (rl *rateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		rl.mu.Lock()
		// In production, track last used time and remove old entries
		// For now, clear if map gets too large
		if len(rl.limiters) > 10000 {
			rl.limiters = make(map[string]*rate.Limiter)
		}
		rl.mu.Unlock()
	}
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
	tokens    int
	lastCheck time.Time
	mu        sync.Mutex
}

// NewTokenBucketRateLimiter creates a token bucket rate limiter
func NewTokenBucketRateLimiter(capacity, refillRate int, interval time.Duration) types.RateLimiter {
	return &TokenBucketRateLimiter{
		buckets:  make(map[string]*tokenBucket),
		capacity: capacity,
		refill:   refillRate,
		interval: interval,
	}
}

// Allow checks if a request should be allowed
func (t *TokenBucketRateLimiter) Allow(key string) bool {
	bucket := t.getBucket(key)
	
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	
	// Refill tokens
	now := time.Now()
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
	
	bucket = &tokenBucket{
		tokens:    t.capacity,
		lastCheck: time.Now(),
	}
	t.buckets[key] = bucket
	
	return bucket
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
