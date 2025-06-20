package middleware

import (
	"strconv"
	"strings"
	
	"net/http"

	"discobox/internal/types"
)

// CORS creates CORS middleware
func CORS(config types.ProxyConfig) types.Middleware {
	cfg := config.Middleware.CORS
	
	// Prepare allowed origins map for faster lookup
	allowedOrigins := make(map[string]bool)
	allowAllOrigins := false
	for _, origin := range cfg.AllowedOrigins {
		if origin == "*" {
			allowAllOrigins = true
			break
		}
		allowedOrigins[origin] = true
	}
	
	// Prepare allowed methods
	allowedMethods := strings.Join(cfg.AllowedMethods, ", ")
	
	// Prepare allowed headers
	allowedHeaders := strings.Join(cfg.AllowedHeaders, ", ")
	
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			
			// Check if origin is allowed
			if origin != "" {
				allowed := allowAllOrigins || allowedOrigins[origin]
				
				if allowed {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					
					if cfg.AllowCredentials {
						w.Header().Set("Access-Control-Allow-Credentials", "true")
					}
					
					// Handle preflight requests
					if r.Method == "OPTIONS" {
						w.Header().Set("Access-Control-Allow-Methods", allowedMethods)
						w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
						
						if cfg.MaxAge > 0 {
							w.Header().Set("Access-Control-Max-Age", strconv.Itoa(cfg.MaxAge))
						}
						
						w.WriteHeader(http.StatusNoContent)
						return
					}
					
					// Add Vary header to indicate response varies by origin
					w.Header().Add("Vary", "Origin")
				}
			}
			
			next.ServeHTTP(w, r)
		})
	}
}

// CORSOptions provides more control over CORS configuration
type CORSOptions struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	ExposedHeaders   []string
	AllowCredentials bool
	MaxAge           int
	OriginFunc       func(origin string) bool
}

// NewCORS creates CORS middleware with custom options
func NewCORS(opts CORSOptions) types.Middleware {
	// Default methods if not specified
	if len(opts.AllowedMethods) == 0 {
		opts.AllowedMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "HEAD", "PATCH"}
	}
	
	// Default headers if not specified
	if len(opts.AllowedHeaders) == 0 {
		opts.AllowedHeaders = []string{"Accept", "Content-Type", "Content-Length", "Authorization"}
	}
	
	// Prepare strings
	allowedMethods := strings.Join(opts.AllowedMethods, ", ")
	allowedHeaders := strings.Join(opts.AllowedHeaders, ", ")
	exposedHeaders := strings.Join(opts.ExposedHeaders, ", ")
	
	// Prepare origin checker
	originAllowed := opts.OriginFunc
	if originAllowed == nil {
		// Build default origin checker
		allowAll := false
		origins := make(map[string]bool)
		for _, origin := range opts.AllowedOrigins {
			if origin == "*" {
				allowAll = true
				break
			}
			origins[origin] = true
		}
		
		originAllowed = func(origin string) bool {
			return allowAll || origins[origin]
		}
	}
	
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}
			
			if !originAllowed(origin) {
				next.ServeHTTP(w, r)
				return
			}
			
			// Set CORS headers
			w.Header().Set("Access-Control-Allow-Origin", origin)
			
			if opts.AllowCredentials {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			
			if exposedHeaders != "" {
				w.Header().Set("Access-Control-Expose-Headers", exposedHeaders)
			}
			
			// Handle preflight
			if r.Method == "OPTIONS" {
				w.Header().Set("Access-Control-Allow-Methods", allowedMethods)
				w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
				
				if opts.MaxAge > 0 {
					w.Header().Set("Access-Control-Max-Age", strconv.Itoa(opts.MaxAge))
				}
				
				// Preflight requests should not go to the next handler
				w.WriteHeader(http.StatusNoContent)
				return
			}
			
			// Add Vary header
			w.Header().Add("Vary", "Origin")
			
			next.ServeHTTP(w, r)
		})
	}
}

// PermissiveCORS creates a permissive CORS configuration for development
func PermissiveCORS() types.Middleware {
	return NewCORS(CORSOptions{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"*"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
		MaxAge:           86400, // 24 hours
	})
}
