package auth

import (
	"crypto/subtle"
	"discobox/internal/types"
	"encoding/base64"
	"net/http"
	"strings"
)

// Basic creates basic authentication middleware
func Basic(config types.ProxyConfig) types.Middleware {
	users := config.Middleware.Auth.Basic.Users
	
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get authorization header
			auth := r.Header.Get("Authorization")
			if auth == "" {
				unauthorized(w, "Basic")
				return
			}
			
			// Check if it's Basic auth
			const prefix = "Basic "
			if !strings.HasPrefix(auth, prefix) {
				unauthorized(w, "Basic")
				return
			}
			
			// Decode credentials
			encoded := auth[len(prefix):]
			decoded, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				unauthorized(w, "Basic")
				return
			}
			
			// Split username and password
			credentials := string(decoded)
			colonIdx := strings.IndexByte(credentials, ':')
			if colonIdx < 0 {
				unauthorized(w, "Basic")
				return
			}
			
			username := credentials[:colonIdx]
			password := credentials[colonIdx+1:]
			
			// Validate credentials
			expectedPassword, exists := users[username]
			if !exists {
				unauthorized(w, "Basic")
				return
			}
			
			// Use constant-time comparison to prevent timing attacks
			if subtle.ConstantTimeCompare([]byte(password), []byte(expectedPassword)) != 1 {
				unauthorized(w, "Basic")
				return
			}
			
			// Add username to request context
			r.Header.Set("X-Auth-User", username)
			
			next.ServeHTTP(w, r)
		})
	}
}

// unauthorized sends a 401 response
func unauthorized(w http.ResponseWriter, realm string) {
	w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
}

// BasicAuthValidator allows custom validation logic
type BasicAuthValidator func(username, password string) bool

// CustomBasic creates basic auth middleware with custom validation
func CustomBasic(realm string, validator BasicAuthValidator) types.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get authorization header
			auth := r.Header.Get("Authorization")
			if auth == "" {
				w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			
			// Parse Basic auth
			const prefix = "Basic "
			if !strings.HasPrefix(auth, prefix) {
				http.Error(w, "Invalid authorization header", http.StatusBadRequest)
				return
			}
			
			// Decode credentials
			encoded := auth[len(prefix):]
			decoded, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				http.Error(w, "Invalid authorization encoding", http.StatusBadRequest)
				return
			}
			
			// Extract username and password
			credentials := string(decoded)
			parts := strings.SplitN(credentials, ":", 2)
			if len(parts) != 2 {
				http.Error(w, "Invalid credentials format", http.StatusBadRequest)
				return
			}
			
			username := parts[0]
			password := parts[1]
			
			// Validate
			if !validator(username, password) {
				w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
				http.Error(w, "Invalid credentials", http.StatusUnauthorized)
				return
			}
			
			// Add username to context
			r.Header.Set("X-Auth-User", username)
			
			next.ServeHTTP(w, r)
		})
	}
}
