package auth

import (
	"fmt"
	"os"
	"strings"

	"crypto/rsa"
	"net/http"

	"github.com/golang-jwt/jwt/v5"

	"discobox/internal/types"
)

// JWT creates JWT authentication middleware
func JWT(config types.ProxyConfig) types.Middleware {
	cfg := config.Middleware.Auth.JWT

	// Load key
	var key any

	if cfg.KeyFile != "" {
		keyData, err := os.ReadFile(cfg.KeyFile)
		if err != nil {
			// Return middleware that always fails
			return func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, "JWT configuration error", http.StatusInternalServerError)
				})
			}
		}

		// Try to parse as RSA public key first
		key, err = jwt.ParseRSAPublicKeyFromPEM(keyData)
		if err != nil {
			// Fall back to symmetric key
			key = keyData
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get token from header
			tokenString := extractToken(r)
			if tokenString == "" {
				http.Error(w, "Missing authorization token", http.StatusUnauthorized)
				return
			}

			// Parse and validate token
			token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
				// Validate signing method
				switch token.Method.(type) {
				case *jwt.SigningMethodRSA, *jwt.SigningMethodRSAPSS:
					if _, ok := key.(*rsa.PublicKey); !ok {
						return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
					}
				case *jwt.SigningMethodHMAC:
					if _, ok := key.([]byte); !ok {
						return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
					}
				default:
					return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
				}

				return key, nil
			})

			if err != nil || !token.Valid {
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}

			// Validate claims
			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				http.Error(w, "Invalid token claims", http.StatusUnauthorized)
				return
			}

			// Check issuer
			if cfg.Issuer != "" {
				if iss, ok := claims["iss"].(string); !ok || iss != cfg.Issuer {
					http.Error(w, "Invalid token issuer", http.StatusUnauthorized)
					return
				}
			}

			// Check audience
			if cfg.Audience != "" {
				if aud, ok := claims["aud"].(string); !ok || aud != cfg.Audience {
					http.Error(w, "Invalid token audience", http.StatusUnauthorized)
					return
				}
			}

			// Add claims to request headers
			if sub, ok := claims["sub"].(string); ok {
				r.Header.Set("X-Auth-Subject", sub)
			}

			// Add other useful claims
			for key, value := range claims {
				if str, ok := value.(string); ok {
					r.Header.Set(fmt.Sprintf("X-JWT-%s", key), str)
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// extractToken extracts JWT token from request
func extractToken(r *http.Request) string {
	// Check Authorization header
	auth := r.Header.Get("Authorization")
	if auth != "" {
		// Bearer token
		const prefix = "Bearer "
		if strings.HasPrefix(auth, prefix) {
			return auth[len(prefix):]
		}
	}

	// Check query parameter
	if token := r.URL.Query().Get("token"); token != "" {
		return token
	}

	// Check cookie
	if cookie, err := r.Cookie("token"); err == nil {
		return cookie.Value
	}

	return ""
}

// JWTValidator provides custom JWT validation
type JWTValidator struct {
	keyFunc    jwt.Keyfunc
	validators []ClaimsValidator
}

// ClaimsValidator validates JWT claims
type ClaimsValidator func(jwt.MapClaims) error

// NewJWTValidator creates a new JWT validator
func NewJWTValidator(keyFunc jwt.Keyfunc, validators ...ClaimsValidator) *JWTValidator {
	return &JWTValidator{
		keyFunc:    keyFunc,
		validators: validators,
	}
}

// Middleware returns JWT validation middleware
func (jv *JWTValidator) Middleware() types.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString := extractToken(r)
			if tokenString == "" {
				http.Error(w, "Missing authorization token", http.StatusUnauthorized)
				return
			}

			// Parse token
			token, err := jwt.Parse(tokenString, jv.keyFunc)
			if err != nil {
				http.Error(w, fmt.Sprintf("Invalid token: %v", err), http.StatusUnauthorized)
				return
			}

			if !token.Valid {
				http.Error(w, "Token is not valid", http.StatusUnauthorized)
				return
			}

			// Get claims
			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				http.Error(w, "Invalid token claims", http.StatusUnauthorized)
				return
			}

			// Run validators
			for _, validator := range jv.validators {
				if err := validator(claims); err != nil {
					http.Error(w, err.Error(), http.StatusUnauthorized)
					return
				}
			}

			// Add claims to headers
			for key, value := range claims {
				if str, ok := value.(string); ok {
					r.Header.Set(fmt.Sprintf("X-JWT-%s", key), str)
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Common claim validators

// RequireClaim ensures a claim exists
func RequireClaim(claim string) ClaimsValidator {
	return func(claims jwt.MapClaims) error {
		if _, ok := claims[claim]; !ok {
			return fmt.Errorf("missing required claim: %s", claim)
		}
		return nil
	}
}

// ValidateIssuer validates the issuer claim
func ValidateIssuer(expectedIssuer string) ClaimsValidator {
	return func(claims jwt.MapClaims) error {
		if iss, ok := claims["iss"].(string); !ok || iss != expectedIssuer {
			return fmt.Errorf("invalid issuer")
		}
		return nil
	}
}

// ValidateAudience validates the audience claim
func ValidateAudience(expectedAudience string) ClaimsValidator {
	return func(claims jwt.MapClaims) error {
		if aud, ok := claims["aud"].(string); !ok || aud != expectedAudience {
			return fmt.Errorf("invalid audience")
		}
		return nil
	}
}
