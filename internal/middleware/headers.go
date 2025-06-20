package middleware

import (
	"strings"
	
	"net/http"

	"discobox/internal/types"
)

// SecurityHeaders adds security-related headers
func SecurityHeaders() types.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Prevent clickjacking
			w.Header().Set("X-Frame-Options", "DENY")
			
			// Prevent MIME type sniffing
			w.Header().Set("X-Content-Type-Options", "nosniff")
			
			// Enable XSS protection
			w.Header().Set("X-XSS-Protection", "1; mode=block")
			
			// HSTS
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			
			// Referrer policy
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			
			// Content Security Policy
			w.Header().Set("Content-Security-Policy", "default-src 'self'")
			
			// Permissions Policy
			w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
			
			next.ServeHTTP(w, r)
		})
	}
}

// CustomHeaders adds custom headers to responses
func CustomHeaders(headers map[string]string) types.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for key, value := range headers {
				w.Header().Set(key, value)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RemoveHeaders removes specified headers from responses
func RemoveHeaders(headers []string) types.Middleware {
	// Convert to lowercase for case-insensitive comparison
	lowercaseHeaders := make([]string, len(headers))
	for i, h := range headers {
		lowercaseHeaders[i] = strings.ToLower(h)
	}
	
	return func(next http.Handler) http.Handler {
		return &headerRemover{
			next:    next,
			headers: lowercaseHeaders,
		}
	}
}

// headerRemover intercepts WriteHeader to remove headers
type headerRemover struct {
	next    http.Handler
	headers []string
}

func (hr *headerRemover) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	wrapped := &headerRemoverWriter{
		ResponseWriter: w,
		headers:        hr.headers,
	}
	hr.next.ServeHTTP(wrapped, r)
}

type headerRemoverWriter struct {
	http.ResponseWriter
	headers     []string
	wroteHeader bool
}

func (hrw *headerRemoverWriter) WriteHeader(code int) {
	if !hrw.wroteHeader {
		// Remove specified headers
		for _, h := range hrw.headers {
			hrw.Header().Del(h)
		}
		hrw.wroteHeader = true
	}
	hrw.ResponseWriter.WriteHeader(code)
}

func (hrw *headerRemoverWriter) Write(b []byte) (int, error) {
	if !hrw.wroteHeader {
		hrw.WriteHeader(http.StatusOK)
	}
	return hrw.ResponseWriter.Write(b)
}

// RequestHeaders modifies request headers
func RequestHeaders(add map[string]string, remove []string) types.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Remove headers
			for _, h := range remove {
				r.Header.Del(h)
			}
			
			// Add headers
			for k, v := range add {
				r.Header.Set(k, v)
			}
			
			next.ServeHTTP(w, r)
		})
	}
}

// ConditionalHeaders adds headers based on conditions
func ConditionalHeaders(conditions map[string]func(*http.Request) bool, headers map[string]string) types.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for header, condition := range conditions {
				if condition(r) {
					if value, ok := headers[header]; ok {
						w.Header().Set(header, value)
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ServerHeader adds or modifies the Server header
func ServerHeader(name string) types.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Server", name)
			next.ServeHTTP(w, r)
		})
	}
}

// Headers creates middleware based on configuration
func Headers(config types.ProxyConfig) types.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Add security headers if enabled
			if config.Middleware.Headers.Security {
				SecurityHeaders()(http.HandlerFunc(func(w2 http.ResponseWriter, r2 *http.Request) {
					// Add custom headers
					for k, v := range config.Middleware.Headers.Custom {
						w2.Header().Set(k, v)
					}
					
					// Continue with next handler
					next.ServeHTTP(w2, r2)
				})).ServeHTTP(w, r)
			} else {
				// Just add custom headers
				for k, v := range config.Middleware.Headers.Custom {
					w.Header().Set(k, v)
				}
				next.ServeHTTP(w, r)
			}
		})
	}
}
