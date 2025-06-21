package types

import (
	"time"
)

// User represents a system user
type User struct {
	ID           string            `json:"id"`
	Username     string            `json:"username"`
	PasswordHash string            `json:"-"` // Never expose the hash
	Email        string            `json:"email,omitempty"`
	IsAdmin      bool              `json:"is_admin"`
	MustChangePassword bool        `json:"must_change_password"`
	Active       bool              `json:"active"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	LastLoginAt  *time.Time        `json:"last_login_at,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// APIKey represents an API key for authentication
type APIKey struct {
	Key         string            `json:"key"`
	UserID      string            `json:"user_id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Active      bool              `json:"active"`
	CreatedAt   time.Time         `json:"created_at"`
	LastUsedAt  *time.Time        `json:"last_used_at,omitempty"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// UserCredentials for authentication
type UserCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// PasswordChange request
type PasswordChange struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// CreateUserRequest for API
type CreateUserRequest struct {
	Username string            `json:"username"`
	Password string            `json:"password"`
	Email    string            `json:"email,omitempty"`
	IsAdmin  bool              `json:"is_admin"`
	MustChangePassword bool   `json:"must_change_password"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// CreateAPIKeyRequest for API
type CreateAPIKeyRequest struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	ExpiresIn   string            `json:"expires_in,omitempty"` // Duration string e.g. "30d", "1y"
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// AuthResponse for login
type AuthResponse struct {
	User  *User   `json:"user"`
	Token string  `json:"token,omitempty"`
	APIKey string `json:"api_key,omitempty"`
}