package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	
	"discobox/internal/config"
	"discobox/internal/types"
)

// User management endpoints

// handleListUsers handles GET /api/v1/users
func (h *Handler) handleListUsers(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	
	users, err := h.storage.ListUsers(ctx)
	if err != nil {
		h.logger.Error("Failed to list users", "error", err)
		respondError(w, http.StatusInternalServerError, "Failed to list users")
		return
	}
	
	respondJSON(w, http.StatusOK, users)
}

// handleCreateUser handles POST /api/v1/users
func (h *Handler) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req types.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	
	// Validate request
	if req.Username == "" || req.Password == "" {
		respondError(w, http.StatusBadRequest, "Username and password are required")
		return
	}
	
	// Hash password
	hashedPassword, err := config.HashPassword(req.Password)
	if err != nil {
		h.logger.Error("Failed to hash password", "error", err)
		respondError(w, http.StatusInternalServerError, "Failed to create user")
		return
	}
	
	// Create user
	user := &types.User{
		ID:                 fmt.Sprintf("user_%s", uuid.New().String()),
		Username:           req.Username,
		PasswordHash:       hashedPassword,
		Email:              req.Email,
		IsAdmin:            req.IsAdmin,
		MustChangePassword: req.MustChangePassword,
		Active:             true,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
		Metadata:           req.Metadata,
	}
	
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	
	if err := h.storage.CreateUser(ctx, user); err != nil {
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "already taken") {
			respondError(w, http.StatusConflict, "Username already exists")
			return
		}
		h.logger.Error("Failed to create user", "error", err)
		respondError(w, http.StatusInternalServerError, "Failed to create user")
		return
	}
	
	// Don't return password hash
	user.PasswordHash = ""
	
	respondJSON(w, http.StatusCreated, user)
}

// handleGetUser handles GET /api/v1/users/{id}
func (h *Handler) handleGetUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userID := vars["id"]
	
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	
	user, err := h.storage.GetUser(ctx, userID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondError(w, http.StatusNotFound, "User not found")
			return
		}
		h.logger.Error("Failed to get user", "error", err)
		respondError(w, http.StatusInternalServerError, "Failed to get user")
		return
	}
	
	// Don't return password hash
	user.PasswordHash = ""
	
	respondJSON(w, http.StatusOK, user)
}

// handleUpdateUser handles PUT /api/v1/users/{id}
func (h *Handler) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userID := vars["id"]
	
	var user types.User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	
	// Ensure ID matches
	user.ID = userID
	
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	
	// Get existing user to preserve password
	existing, err := h.storage.GetUser(ctx, userID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondError(w, http.StatusNotFound, "User not found")
			return
		}
		h.logger.Error("Failed to get user", "error", err)
		respondError(w, http.StatusInternalServerError, "Failed to update user")
		return
	}
	
	// Preserve password hash
	user.PasswordHash = existing.PasswordHash
	
	if err := h.storage.UpdateUser(ctx, &user); err != nil {
		if strings.Contains(err.Error(), "already taken") {
			respondError(w, http.StatusConflict, "Username already taken")
			return
		}
		h.logger.Error("Failed to update user", "error", err)
		respondError(w, http.StatusInternalServerError, "Failed to update user")
		return
	}
	
	// Don't return password hash
	user.PasswordHash = ""
	
	respondJSON(w, http.StatusOK, user)
}

// handleDeleteUser handles DELETE /api/v1/users/{id}
func (h *Handler) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userID := vars["id"]
	
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	
	if err := h.storage.DeleteUser(ctx, userID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondError(w, http.StatusNotFound, "User not found")
			return
		}
		h.logger.Error("Failed to delete user", "error", err)
		respondError(w, http.StatusInternalServerError, "Failed to delete user")
		return
	}
	
	respondJSON(w, http.StatusNoContent, nil)
}

// handleChangePassword handles POST /api/v1/users/{id}/password
func (h *Handler) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userID := vars["id"]
	
	var req types.PasswordChange
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	
	if req.NewPassword == "" {
		respondError(w, http.StatusBadRequest, "New password is required")
		return
	}
	
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	
	// Get existing user
	user, err := h.storage.GetUser(ctx, userID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondError(w, http.StatusNotFound, "User not found")
			return
		}
		h.logger.Error("Failed to get user", "error", err)
		respondError(w, http.StatusInternalServerError, "Failed to change password")
		return
	}
	
	// Verify old password if provided (not required for admin users changing other users)
	if req.OldPassword != "" {
		if !config.ComparePasswords(user.PasswordHash, req.OldPassword) {
			respondError(w, http.StatusUnauthorized, "Invalid old password")
			return
		}
	}
	
	// Hash new password
	hashedPassword, err := config.HashPassword(req.NewPassword)
	if err != nil {
		h.logger.Error("Failed to hash password", "error", err)
		respondError(w, http.StatusInternalServerError, "Failed to change password")
		return
	}
	
	// Update user
	user.PasswordHash = hashedPassword
	user.MustChangePassword = false
	user.UpdatedAt = time.Now()
	
	if err := h.storage.UpdateUser(ctx, user); err != nil {
		h.logger.Error("Failed to update user", "error", err)
		respondError(w, http.StatusInternalServerError, "Failed to change password")
		return
	}
	
	respondJSON(w, http.StatusOK, map[string]string{
		"message": "Password changed successfully",
	})
}

// handleListUserAPIKeys handles GET /api/v1/users/{id}/api-keys
func (h *Handler) handleListUserAPIKeys(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userID := vars["id"]
	
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	
	// Verify user exists
	if _, err := h.storage.GetUser(ctx, userID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondError(w, http.StatusNotFound, "User not found")
			return
		}
		h.logger.Error("Failed to get user", "error", err)
		respondError(w, http.StatusInternalServerError, "Failed to list API keys")
		return
	}
	
	apiKeys, err := h.storage.ListAPIKeysByUser(ctx, userID)
	if err != nil {
		h.logger.Error("Failed to list API keys", "error", err)
		respondError(w, http.StatusInternalServerError, "Failed to list API keys")
		return
	}
	
	respondJSON(w, http.StatusOK, apiKeys)
}

// handleCreateAPIKey handles POST /api/v1/users/{id}/api-keys
func (h *Handler) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userID := vars["id"]
	
	var req types.CreateAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	
	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "API key name is required")
		return
	}
	
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	
	// Verify user exists
	if _, err := h.storage.GetUser(ctx, userID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondError(w, http.StatusNotFound, "User not found")
			return
		}
		h.logger.Error("Failed to get user", "error", err)
		respondError(w, http.StatusInternalServerError, "Failed to create API key")
		return
	}
	
	// Generate API key
	apiKey := &types.APIKey{
		Key:         config.GenerateAPIKey(),
		UserID:      userID,
		Name:        req.Name,
		Description: req.Description,
		Active:      true,
		CreatedAt:   time.Now(),
		Metadata:    req.Metadata,
	}
	
	// Parse expiration if provided
	if req.ExpiresIn != "" {
		duration, err := time.ParseDuration(req.ExpiresIn)
		if err != nil {
			respondError(w, http.StatusBadRequest, "Invalid expiration duration")
			return
		}
		expiresAt := time.Now().Add(duration)
		apiKey.ExpiresAt = &expiresAt
	}
	
	if err := h.storage.CreateAPIKey(ctx, apiKey); err != nil {
		h.logger.Error("Failed to create API key", "error", err)
		respondError(w, http.StatusInternalServerError, "Failed to create API key")
		return
	}
	
	respondJSON(w, http.StatusCreated, apiKey)
}

// handleRevokeAPIKey handles DELETE /api/v1/api-keys/{key}
func (h *Handler) handleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	key := vars["key"]
	
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	
	if err := h.storage.RevokeAPIKey(ctx, key); err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondError(w, http.StatusNotFound, "API key not found")
			return
		}
		h.logger.Error("Failed to revoke API key", "error", err)
		respondError(w, http.StatusInternalServerError, "Failed to revoke API key")
		return
	}
	
	respondJSON(w, http.StatusNoContent, nil)
}

// Authentication endpoints

// handleLogin handles POST /api/v1/auth/login
func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var creds types.UserCredentials
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	
	if creds.Username == "" || creds.Password == "" {
		respondError(w, http.StatusBadRequest, "Username and password are required")
		return
	}
	
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	
	// Get user by username
	user, err := h.storage.GetUserByUsername(ctx, creds.Username)
	if err != nil {
		// Don't reveal whether user exists
		respondError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}
	
	// Check if user is active
	if !user.Active {
		respondError(w, http.StatusUnauthorized, "Account is disabled")
		return
	}
	
	// Verify password
	if !config.ComparePasswords(user.PasswordHash, creds.Password) {
		respondError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}
	
	// Update last login time
	now := time.Now()
	user.LastLoginAt = &now
	h.storage.UpdateUser(ctx, user)
	
	// Generate API key for session
	sessionKey := &types.APIKey{
		Key:         config.GenerateAPIKey(),
		UserID:      user.ID,
		Name:        fmt.Sprintf("Session for %s", user.Username),
		Description: "Auto-generated session key",
		Active:      true,
		CreatedAt:   time.Now(),
		ExpiresAt:   &time.Time{}, // Set expiration for 24 hours
		Metadata: map[string]string{
			"type": "session",
		},
	}
	
	// Set expiration to 24 hours
	expires := time.Now().Add(24 * time.Hour)
	sessionKey.ExpiresAt = &expires
	
	if err := h.storage.CreateAPIKey(ctx, sessionKey); err != nil {
		h.logger.Error("Failed to create session key", "error", err)
		respondError(w, http.StatusInternalServerError, "Failed to create session")
		return
	}
	
	// Don't return password hash
	user.PasswordHash = ""
	
	response := types.AuthResponse{
		User:   user,
		APIKey: sessionKey.Key,
	}
	
	respondJSON(w, http.StatusOK, response)
}

// handleWhoAmI handles GET /api/v1/auth/whoami
func (h *Handler) handleWhoAmI(w http.ResponseWriter, r *http.Request) {
	// Get API key from header
	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		respondError(w, http.StatusUnauthorized, "No API key provided")
		return
	}
	
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	
	// Get API key info
	key, err := h.storage.GetAPIKey(ctx, apiKey)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "Invalid API key")
		return
	}
	
	// Check if key is active
	if !key.Active {
		respondError(w, http.StatusUnauthorized, "API key is revoked")
		return
	}
	
	// Check if key is expired
	if key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now()) {
		respondError(w, http.StatusUnauthorized, "API key is expired")
		return
	}
	
	// Get user
	user, err := h.storage.GetUser(ctx, key.UserID)
	if err != nil {
		h.logger.Error("Failed to get user for API key", "error", err)
		respondError(w, http.StatusInternalServerError, "Failed to get user info")
		return
	}
	
	// Don't return password hash
	user.PasswordHash = ""
	
	respondJSON(w, http.StatusOK, user)
}