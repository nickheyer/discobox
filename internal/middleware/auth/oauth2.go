package auth

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"

	"discobox/internal/types"
)

// OAuth2 creates OAuth2 authentication middleware
func OAuth2(config types.ProxyConfig) types.Middleware {
	cfg := config.Middleware.Auth.OAuth2

	provider := &oauth2Provider{
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		redirectURL:  cfg.RedirectURL,
		provider:     cfg.Provider,
		sessions:     make(map[string]*oauth2Session),
	}

	// Configure provider endpoints
	switch cfg.Provider {
	case "google":
		provider.authURL = "https://accounts.google.com/o/oauth2/v2/auth"
		provider.tokenURL = "https://oauth2.googleapis.com/token"
		provider.userInfoURL = "https://www.googleapis.com/oauth2/v2/userinfo"
	case "github":
		provider.authURL = "https://github.com/login/oauth/authorize"
		provider.tokenURL = "https://github.com/login/oauth/access_token"
		provider.userInfoURL = "https://api.github.com/user"
	default:
		// Custom provider - URLs should be in config
	}

	return provider.Middleware
}

// oauth2Provider handles OAuth2 flow
type oauth2Provider struct {
	clientID     string
	clientSecret string
	redirectURL  string
	provider     string
	authURL      string
	tokenURL     string
	userInfoURL  string
	mu           sync.RWMutex
	sessions     map[string]*oauth2Session
}

type oauth2Session struct {
	state       string
	accessToken string
	userInfo    map[string]interface{}
	expiresAt   time.Time
}

// Middleware returns the OAuth2 middleware
func (o *oauth2Provider) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for callback
		if r.URL.Path == "/oauth2/callback" {
			o.handleCallback(w, r)
			return
		}

		// Check for existing session
		sessionID := o.getSessionID(r)
		if sessionID != "" {
			o.mu.RLock()
			session, exists := o.sessions[sessionID]
			o.mu.RUnlock()

			if exists && session.expiresAt.After(time.Now()) {
				// Valid session
				for k, v := range session.userInfo {
					if str, ok := v.(string); ok {
						r.Header.Set(fmt.Sprintf("X-OAuth2-%s", k), str)
					}
				}
				next.ServeHTTP(w, r)
				return
			}
		}

		// No valid session, redirect to OAuth2 provider
		state := o.generateState()
		o.mu.Lock()
		o.sessions[state] = &oauth2Session{
			state:     state,
			expiresAt: time.Now().Add(10 * time.Minute),
		}
		o.mu.Unlock()

		// Build authorization URL
		authURL := fmt.Sprintf("%s?client_id=%s&redirect_uri=%s&response_type=code&state=%s",
			o.authURL, o.clientID, o.redirectURL, state)

		// Add scope if needed
		switch o.provider {
		case "google":
			authURL += "&scope=openid%20email%20profile"
		case "github":
			authURL += "&scope=read:user"
		}

		http.Redirect(w, r, authURL, http.StatusFound)
	})
}

// handleCallback handles the OAuth2 callback
func (o *oauth2Provider) handleCallback(w http.ResponseWriter, r *http.Request) {
	// Verify state
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")

	if state == "" || code == "" {
		http.Error(w, "Invalid callback parameters", http.StatusBadRequest)
		return
	}

	o.mu.Lock()
	session, exists := o.sessions[state]
	if !exists || session.expiresAt.Before(time.Now()) {
		o.mu.Unlock()
		http.Error(w, "Invalid or expired state", http.StatusBadRequest)
		return
	}
	o.mu.Unlock()

	// Exchange code for token
	token, err := o.exchangeCode(code)
	if err != nil {
		http.Error(w, "Failed to exchange code", http.StatusInternalServerError)
		return
	}

	// Get user info
	userInfo, err := o.getUserInfo(token)
	if err != nil {
		http.Error(w, "Failed to get user info", http.StatusInternalServerError)
		return
	}

	// Update session
	sessionID := o.generateSessionID()
	o.mu.Lock()
	delete(o.sessions, state)
	o.sessions[sessionID] = &oauth2Session{
		accessToken: token,
		userInfo:    userInfo,
		expiresAt:   time.Now().Add(1 * time.Hour),
	}
	o.mu.Unlock()

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth2_session",
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   3600,
	})

	// Redirect to original URL or home
	http.Redirect(w, r, "/", http.StatusFound)
}

// exchangeCode exchanges authorization code for access token
func (o *oauth2Provider) exchangeCode(code string) (string, error) {
	// Build request
	data := fmt.Sprintf("grant_type=authorization_code&code=%s&client_id=%s&client_secret=%s&redirect_uri=%s",
		code, o.clientID, o.clientSecret, o.redirectURL)

	req, err := http.NewRequest("POST", o.tokenURL, strings.NewReader(data))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	// Make request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Parse response
	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}

	return tokenResp.AccessToken, nil
}

// getUserInfo fetches user information
func (o *oauth2Provider) getUserInfo(token string) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", o.userInfoURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var userInfo map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, err
	}

	return userInfo, nil
}

// generateState generates a random state parameter
func (o *oauth2Provider) generateState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

// generateSessionID generates a random session ID
func (o *oauth2Provider) generateSessionID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

// getSessionID retrieves session ID from request
func (o *oauth2Provider) getSessionID(r *http.Request) string {
	cookie, err := r.Cookie("oauth2_session")
	if err != nil {
		return ""
	}
	return cookie.Value
}

// TokenValidator validates OAuth2 tokens
type TokenValidator struct {
	introspectURL string
	clientID      string
	clientSecret  string
}

// NewTokenValidator creates a new token validator
func NewTokenValidator(introspectURL, clientID, clientSecret string) *TokenValidator {
	return &TokenValidator{
		introspectURL: introspectURL,
		clientID:      clientID,
		clientSecret:  clientSecret,
	}
}

// Validate validates an OAuth2 token
func (tv *TokenValidator) Validate(token string) (map[string]interface{}, error) {
	// Build introspection request
	data := fmt.Sprintf("token=%s&token_type_hint=access_token", token)

	req, err := http.NewRequest("POST", tv.introspectURL, strings.NewReader(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(tv.clientID, tv.clientSecret)

	// Make request
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Parse response
	var introspectResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&introspectResp); err != nil {
		return nil, err
	}

	// Check if token is active
	if active, ok := introspectResp["active"].(bool); !ok || !active {
		return nil, fmt.Errorf("token is not active")
	}

	return introspectResp, nil
}
