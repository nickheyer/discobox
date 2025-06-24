package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// Logger interface for API logging
type Logger interface {
	Debug(msg string, fields ...any)
	Info(msg string, fields ...any)
	Warn(msg string, fields ...any)
	Error(msg string, fields ...any)
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	Enabled bool
	Type    string // "basic", "bearer", "api-key"
	// Basic auth
	Username string
	Password string
	// Bearer/API key
	Token      string
	HeaderName string // Default: "Authorization" or "X-API-Key"
}

// WithMiddleware applies common middleware to the API
func WithMiddleware(handler http.Handler, logger Logger, authConfig *AuthConfig) http.Handler {
	// Add CORS headers
	handler = corsMiddleware(handler)

	// Add JSON content type
	handler = jsonMiddleware(handler)

	// Add request logging
	handler = loggingMiddleware(handler, logger)

	// Add authentication if needed
	if authConfig != nil && authConfig.Enabled {
		handler = authMiddleware(handler, authConfig)
	}

	return handler
}

// corsMiddleware adds CORS headers
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// jsonMiddleware sets JSON content type
func jsonMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs API requests
func loggingMiddleware(next http.Handler, logger Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Info("API request",
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
		)
		next.ServeHTTP(w, r)
	})
}

// authMiddleware provides authentication for API endpoints
func authMiddleware(next http.Handler, config *AuthConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch config.Type {
		case "basic":
			if !checkBasicAuth(r, config.Username, config.Password) {
				w.Header().Set("WWW-Authenticate", `Basic realm="Discobox API"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

		case "bearer":
			if !checkBearerToken(r, config.Token) {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

		case "api-key":
			headerName := config.HeaderName
			if headerName == "" {
				headerName = "X-API-Key"
			}
			// If no static token configured, authentication will be handled per-request
			if config.Token != "" && !checkAPIKey(r, config.Token, headerName) {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

		default:
			// Unknown auth type, deny access
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// checkBasicAuth verifies basic authentication
func checkBasicAuth(r *http.Request, username, password string) bool {
	user, pass, ok := r.BasicAuth()
	if !ok {
		return false
	}

	// Use constant time comparison to prevent timing attacks
	userMatch := subtle.ConstantTimeCompare([]byte(user), []byte(username))
	passMatch := subtle.ConstantTimeCompare([]byte(pass), []byte(password))

	return userMatch == 1 && passMatch == 1
}

// checkBearerToken verifies bearer token authentication
func checkBearerToken(r *http.Request, expectedToken string) bool {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return false
	}

	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return false
	}

	token := auth[len(prefix):]
	return subtle.ConstantTimeCompare([]byte(token), []byte(expectedToken)) == 1
}

// checkAPIKey verifies API key authentication
func checkAPIKey(r *http.Request, expectedKey string, headerName string) bool {
	key := r.Header.Get(headerName)
	if key == "" {
		// Also check query parameter as fallback
		key = r.URL.Query().Get("api_key")
	}

	return subtle.ConstantTimeCompare([]byte(key), []byte(expectedKey)) == 1
}
