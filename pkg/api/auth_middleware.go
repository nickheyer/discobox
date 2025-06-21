package api

import (
	"context"
	"net/http"
	"strings"
	"time"
	
	"discobox/internal/types"
)

// storageAuthMiddleware provides database-backed authentication
func storageAuthMiddleware(next http.Handler, storage types.Storage, logger types.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get API key from header
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			// Also check query parameter as fallback
			apiKey = r.URL.Query().Get("api_key")
		}
		
		if apiKey == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		
		// Validate API key
		key, err := storage.GetAPIKey(ctx, apiKey)
		if err != nil {
			logger.Debug("Invalid API key", "key", apiKey[:8]+"...", "error", err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		
		// Check if key is active
		if !key.Active {
			http.Error(w, "API key is revoked", http.StatusUnauthorized)
			return
		}
		
		// Check if key is expired
		if key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now()) {
			http.Error(w, "API key is expired", http.StatusUnauthorized)
			return
		}
		
		// Get user info
		user, err := storage.GetUser(ctx, key.UserID)
		if err != nil {
			logger.Error("Failed to get user for API key", "user_id", key.UserID, "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		
		// Check if user is active
		if !user.Active {
			http.Error(w, "User account is disabled", http.StatusUnauthorized)
			return
		}
		
		// Add user info to request context
		r.Header.Set("X-User-ID", user.ID)
		r.Header.Set("X-User-Name", user.Username)
		if user.IsAdmin {
			r.Header.Set("X-User-Admin", "true")
		}
		
		next.ServeHTTP(w, r)
	})
}

// requireAdminMiddleware ensures the user is an admin
func requireAdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isAdmin := r.Header.Get("X-User-Admin") == "true"
		if !isAdmin {
			http.Error(w, "Forbidden - admin access required", http.StatusForbidden)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

// publicEndpoints is a list of endpoints that don't require authentication
var publicEndpoints = map[string]bool{
	"/health":          true,
	"/api/v1/auth/login": true,
}

// isPublicEndpoint checks if an endpoint is public
func isPublicEndpoint(path string) bool {
	// Exact match
	if publicEndpoints[path] {
		return true
	}
	
	// Check if it's an OPTIONS request (for CORS)
	if strings.HasSuffix(path, "/") {
		path = path[:len(path)-1]
	}
	
	return publicEndpoints[path]
}