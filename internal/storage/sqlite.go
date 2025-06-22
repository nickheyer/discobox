package storage

import (
	"context"
	"database/sql"
	"discobox/internal/types"
	"encoding/json"
	"fmt"
	"os"
	fp "path/filepath"
	"sync"
	"time"
)

// sqliteStorage implements Storage interface using SQLite
type sqliteStorage struct {
	db        *sql.DB
	logger    types.Logger
	watchers  []chan types.StorageEvent
	watcherMu sync.RWMutex
	stopWatch chan struct{}
	wg        sync.WaitGroup
}

// NewSQLite creates a new SQLite storage instance
func NewSQLite(dsn string, logger types.Logger) (types.Storage, error) {
	if dsn == "" {
		dsn = "data/discobox.db"
	}

	if err := os.MkdirAll(fp.Dir(dsn), 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	s := &sqliteStorage{
		db:        db,
		logger:    logger,
		watchers:  make([]chan types.StorageEvent, 0),
		stopWatch: make(chan struct{}),
	}

	// Create tables
	if err := s.createTables(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	return s, nil
}

func (s *sqliteStorage) createTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS services (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			endpoints TEXT NOT NULL,
			health_path TEXT,
			weight INTEGER DEFAULT 1,
			max_conns INTEGER DEFAULT 0,
			timeout INTEGER DEFAULT 30000,
			metadata TEXT,
			tls_config TEXT,
			strip_prefix BOOLEAN DEFAULT FALSE,
			active BOOLEAN DEFAULT TRUE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS routes (
			id TEXT PRIMARY KEY,
			priority INTEGER DEFAULT 0,
			host TEXT,
			path_prefix TEXT,
			path_regex TEXT,
			headers TEXT,
			service_id TEXT NOT NULL,
			middlewares TEXT,
			rewrite_rules TEXT,
			metadata TEXT,
			FOREIGN KEY (service_id) REFERENCES services(id)
		)`,
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			email TEXT,
			is_admin BOOLEAN DEFAULT FALSE,
			must_change_password BOOLEAN DEFAULT FALSE,
			active BOOLEAN DEFAULT TRUE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			last_login_at TIMESTAMP,
			metadata TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS api_keys (
			key TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			name TEXT NOT NULL,
			description TEXT,
			active BOOLEAN DEFAULT TRUE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			last_used_at TIMESTAMP,
			expires_at TIMESTAMP,
			metadata TEXT,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_routes_priority ON routes(priority DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_routes_host ON routes(host)`,
		`CREATE INDEX IF NOT EXISTS idx_services_active ON services(active)`,
		`CREATE INDEX IF NOT EXISTS idx_users_username ON users(username)`,
		`CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON api_keys(user_id)`,
	}

	for _, query := range queries {
		if _, err := s.db.Exec(query); err != nil {
			return fmt.Errorf("failed to execute query: %w", err)
		}
	}

	return nil
}

// Services implementation

func (s *sqliteStorage) GetService(ctx context.Context, id string) (*types.Service, error) {
	var service types.Service
	var endpoints, metadata, tlsConfig string
	var timeout int64

	query := `SELECT id, name, endpoints, health_path, weight, max_conns, timeout, 
	          metadata, tls_config, strip_prefix, active, created_at, updated_at 
	          FROM services WHERE id = ?`

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&service.ID, &service.Name, &endpoints, &service.HealthPath,
		&service.Weight, &service.MaxConns, &timeout, &metadata, &tlsConfig,
		&service.StripPrefix, &service.Active, &service.CreatedAt, &service.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, types.ErrServiceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get service: %w", err)
	}

	// Unmarshal JSON fields
	if err := json.Unmarshal([]byte(endpoints), &service.Endpoints); err != nil {
		return nil, fmt.Errorf("failed to unmarshal endpoints: %w", err)
	}

	if metadata != "" {
		if err := json.Unmarshal([]byte(metadata), &service.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	if tlsConfig != "" {
		service.TLS = &types.TLSConfig{}
		if err := json.Unmarshal([]byte(tlsConfig), service.TLS); err != nil {
			return nil, fmt.Errorf("failed to unmarshal TLS config: %w", err)
		}
	}

	service.Timeout = time.Duration(timeout) * time.Millisecond

	return &service, nil
}

func (s *sqliteStorage) ListServices(ctx context.Context) ([]*types.Service, error) {
	query := `SELECT id, name, endpoints, health_path, weight, max_conns, timeout, 
	          metadata, tls_config, strip_prefix, active, created_at, updated_at 
	          FROM services ORDER BY name`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}
	defer rows.Close()

	var services []*types.Service
	for rows.Next() {
		var service types.Service
		var endpoints, metadata, tlsConfig string
		var timeout int64

		err := rows.Scan(
			&service.ID, &service.Name, &endpoints, &service.HealthPath,
			&service.Weight, &service.MaxConns, &timeout, &metadata, &tlsConfig,
			&service.StripPrefix, &service.Active, &service.CreatedAt, &service.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan service: %w", err)
		}

		// Unmarshal JSON fields
		if err := json.Unmarshal([]byte(endpoints), &service.Endpoints); err != nil {
			return nil, fmt.Errorf("failed to unmarshal endpoints: %w", err)
		}

		if metadata != "" {
			if err := json.Unmarshal([]byte(metadata), &service.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		if tlsConfig != "" {
			service.TLS = &types.TLSConfig{}
			if err := json.Unmarshal([]byte(tlsConfig), service.TLS); err != nil {
				return nil, fmt.Errorf("failed to unmarshal TLS config: %w", err)
			}
		}

		service.Timeout = time.Duration(timeout) * time.Millisecond
		services = append(services, &service)
	}

	return services, nil
}

func (s *sqliteStorage) CreateService(ctx context.Context, service *types.Service) error {
	if service == nil {
		return types.ErrInvalidRequest
	}

	// Marshal JSON fields
	endpoints, err := json.Marshal(service.Endpoints)
	if err != nil {
		return fmt.Errorf("failed to marshal endpoints: %w", err)
	}

	metadata, err := json.Marshal(service.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	var tlsConfig []byte
	if service.TLS != nil {
		tlsConfig, err = json.Marshal(service.TLS)
		if err != nil {
			return fmt.Errorf("failed to marshal TLS config: %w", err)
		}
	}

	query := `INSERT INTO services (id, name, endpoints, health_path, weight, max_conns, 
	          timeout, metadata, tls_config, strip_prefix, active) 
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = s.db.ExecContext(ctx, query,
		service.ID, service.Name, string(endpoints), service.HealthPath,
		service.Weight, service.MaxConns, service.Timeout.Milliseconds(),
		string(metadata), string(tlsConfig), service.StripPrefix, service.Active,
	)

	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}

	// Notify watchers
	s.notifyWatchers(types.StorageEvent{
		Type:   "created",
		Kind:   "service",
		ID:     service.ID,
		Object: service,
	})

	return nil
}

func (s *sqliteStorage) UpdateService(ctx context.Context, service *types.Service) error {
	if service == nil {
		return types.ErrInvalidRequest
	}

	// Check if service exists
	var exists bool
	err := s.db.QueryRowContext(ctx, "SELECT 1 FROM services WHERE id = ?", service.ID).Scan(&exists)
	if err == sql.ErrNoRows {
		return types.ErrServiceNotFound
	}

	// Marshal JSON fields
	endpoints, err := json.Marshal(service.Endpoints)
	if err != nil {
		return fmt.Errorf("failed to marshal endpoints: %w", err)
	}

	metadata, err := json.Marshal(service.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	var tlsConfig []byte
	if service.TLS != nil {
		tlsConfig, err = json.Marshal(service.TLS)
		if err != nil {
			return fmt.Errorf("failed to marshal TLS config: %w", err)
		}
	}

	query := `UPDATE services SET name = ?, endpoints = ?, health_path = ?, weight = ?, 
	          max_conns = ?, timeout = ?, metadata = ?, tls_config = ?, 
	          strip_prefix = ?, active = ?, updated_at = CURRENT_TIMESTAMP 
	          WHERE id = ?`

	_, err = s.db.ExecContext(ctx, query,
		service.Name, string(endpoints), service.HealthPath, service.Weight,
		service.MaxConns, service.Timeout.Milliseconds(), string(metadata),
		string(tlsConfig), service.StripPrefix, service.Active, service.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update service: %w", err)
	}

	// Notify watchers
	s.notifyWatchers(types.StorageEvent{
		Type:   "updated",
		Kind:   "service",
		ID:     service.ID,
		Object: service,
	})

	return nil
}

func (s *sqliteStorage) DeleteService(ctx context.Context, id string) error {
	// Get service before deletion for event
	service, err := s.GetService(ctx, id)
	if err != nil {
		return err
	}

	// Get all routes that reference this service for deletion events
	routesToDelete, err := s.db.QueryContext(ctx, "SELECT id FROM routes WHERE service_id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to query routes: %w", err)
	}
	defer routesToDelete.Close()

	var routeIDs []string
	for routesToDelete.Next() {
		var routeID string
		if err := routesToDelete.Scan(&routeID); err != nil {
			return fmt.Errorf("failed to scan route ID: %w", err)
		}
		routeIDs = append(routeIDs, routeID)
	}

	// Delete routes that reference this service
	if _, err := s.db.ExecContext(ctx, "DELETE FROM routes WHERE service_id = ?", id); err != nil {
		return fmt.Errorf("failed to delete routes: %w", err)
	}

	// Delete the service
	_, err = s.db.ExecContext(ctx, "DELETE FROM services WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}

	// Notify watchers about route deletions
	for _, routeID := range routeIDs {
		s.notifyWatchers(types.StorageEvent{
			Type:   "deleted",
			Kind:   "route",
			ID:     routeID,
			Object: nil, // We don't have the route object anymore
		})
	}

	// Notify watchers about service deletion
	s.notifyWatchers(types.StorageEvent{
		Type:   "deleted",
		Kind:   "service",
		ID:     id,
		Object: service,
	})

	return nil
}

// Routes implementation

func (s *sqliteStorage) GetRoute(ctx context.Context, id string) (*types.Route, error) {
	var route types.Route
	var headers, middlewares, rewriteRules, metadata string

	query := `SELECT id, priority, host, path_prefix, path_regex, headers, 
	          service_id, middlewares, rewrite_rules, metadata 
	          FROM routes WHERE id = ?`

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&route.ID, &route.Priority, &route.Host, &route.PathPrefix,
		&route.PathRegex, &headers, &route.ServiceID, &middlewares,
		&rewriteRules, &metadata,
	)

	if err == sql.ErrNoRows {
		return nil, types.ErrRouteNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get route: %w", err)
	}

	// Unmarshal JSON fields
	if headers != "" {
		if err := json.Unmarshal([]byte(headers), &route.Headers); err != nil {
			return nil, fmt.Errorf("failed to unmarshal headers: %w", err)
		}
	}

	if middlewares != "" {
		if err := json.Unmarshal([]byte(middlewares), &route.Middlewares); err != nil {
			return nil, fmt.Errorf("failed to unmarshal middlewares: %w", err)
		}
	}

	if rewriteRules != "" {
		if err := json.Unmarshal([]byte(rewriteRules), &route.RewriteRules); err != nil {
			return nil, fmt.Errorf("failed to unmarshal rewrite rules: %w", err)
		}
	}

	if metadata != "" {
		if err := json.Unmarshal([]byte(metadata), &route.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &route, nil
}

func (s *sqliteStorage) ListRoutes(ctx context.Context) ([]*types.Route, error) {
	query := `SELECT id, priority, host, path_prefix, path_regex, headers, 
	          service_id, middlewares, rewrite_rules, metadata 
	          FROM routes ORDER BY priority DESC, id`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list routes: %w", err)
	}
	defer rows.Close()

	var routes []*types.Route
	for rows.Next() {
		var route types.Route
		var headers, middlewares, rewriteRules, metadata string

		err := rows.Scan(
			&route.ID, &route.Priority, &route.Host, &route.PathPrefix,
			&route.PathRegex, &headers, &route.ServiceID, &middlewares,
			&rewriteRules, &metadata,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan route: %w", err)
		}

		// Unmarshal JSON fields
		if headers != "" {
			if err := json.Unmarshal([]byte(headers), &route.Headers); err != nil {
				return nil, fmt.Errorf("failed to unmarshal headers: %w", err)
			}
		}

		if middlewares != "" {
			if err := json.Unmarshal([]byte(middlewares), &route.Middlewares); err != nil {
				return nil, fmt.Errorf("failed to unmarshal middlewares: %w", err)
			}
		}

		if rewriteRules != "" {
			if err := json.Unmarshal([]byte(rewriteRules), &route.RewriteRules); err != nil {
				return nil, fmt.Errorf("failed to unmarshal rewrite rules: %w", err)
			}
		}

		if metadata != "" {
			if err := json.Unmarshal([]byte(metadata), &route.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		routes = append(routes, &route)
	}

	return routes, nil
}

func (s *sqliteStorage) CreateRoute(ctx context.Context, route *types.Route) error {
	if route == nil {
		return types.ErrInvalidRequest
	}

	// Verify service exists
	var exists bool
	err := s.db.QueryRowContext(ctx, "SELECT 1 FROM services WHERE id = ?", route.ServiceID).Scan(&exists)
	if err == sql.ErrNoRows {
		return fmt.Errorf("service not found for route")
	}

	// Marshal JSON fields
	headers, _ := json.Marshal(route.Headers)
	middlewares, _ := json.Marshal(route.Middlewares)
	rewriteRules, _ := json.Marshal(route.RewriteRules)
	metadata, _ := json.Marshal(route.Metadata)

	query := `INSERT INTO routes (id, priority, host, path_prefix, path_regex, 
	          headers, service_id, middlewares, rewrite_rules, metadata) 
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = s.db.ExecContext(ctx, query,
		route.ID, route.Priority, route.Host, route.PathPrefix,
		route.PathRegex, string(headers), route.ServiceID,
		string(middlewares), string(rewriteRules), string(metadata),
	)

	if err != nil {
		return fmt.Errorf("failed to create route: %w", err)
	}

	// Notify watchers
	s.notifyWatchers(types.StorageEvent{
		Type:   "created",
		Kind:   "route",
		ID:     route.ID,
		Object: route,
	})

	return nil
}

func (s *sqliteStorage) UpdateRoute(ctx context.Context, route *types.Route) error {
	if route == nil {
		return types.ErrInvalidRequest
	}

	// Check if route exists
	var exists bool
	err := s.db.QueryRowContext(ctx, "SELECT 1 FROM routes WHERE id = ?", route.ID).Scan(&exists)
	if err == sql.ErrNoRows {
		return types.ErrRouteNotFound
	}

	// Verify service exists
	err = s.db.QueryRowContext(ctx, "SELECT 1 FROM services WHERE id = ?", route.ServiceID).Scan(&exists)
	if err == sql.ErrNoRows {
		return fmt.Errorf("service not found for route")
	}

	// Marshal JSON fields
	headers, _ := json.Marshal(route.Headers)
	middlewares, _ := json.Marshal(route.Middlewares)
	rewriteRules, _ := json.Marshal(route.RewriteRules)
	metadata, _ := json.Marshal(route.Metadata)

	query := `UPDATE routes SET priority = ?, host = ?, path_prefix = ?, 
	          path_regex = ?, headers = ?, service_id = ?, middlewares = ?, 
	          rewrite_rules = ?, metadata = ? WHERE id = ?`

	_, err = s.db.ExecContext(ctx, query,
		route.Priority, route.Host, route.PathPrefix, route.PathRegex,
		string(headers), route.ServiceID, string(middlewares),
		string(rewriteRules), string(metadata), route.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update route: %w", err)
	}

	// Notify watchers
	s.notifyWatchers(types.StorageEvent{
		Type:   "updated",
		Kind:   "route",
		ID:     route.ID,
		Object: route,
	})

	return nil
}

func (s *sqliteStorage) DeleteRoute(ctx context.Context, id string) error {
	// Get route before deletion for event
	route, err := s.GetRoute(ctx, id)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, "DELETE FROM routes WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete route: %w", err)
	}

	// Notify watchers
	s.notifyWatchers(types.StorageEvent{
		Type:   "deleted",
		Kind:   "route",
		ID:     id,
		Object: route,
	})

	return nil
}

// Watch implementation

func (s *sqliteStorage) Watch(ctx context.Context) <-chan types.StorageEvent {
	s.watcherMu.Lock()
	defer s.watcherMu.Unlock()

	ch := make(chan types.StorageEvent, 100)
	s.watchers = append(s.watchers, ch)

	// Clean up when context is done
	go func() {
		<-ctx.Done()
		s.watcherMu.Lock()
		defer s.watcherMu.Unlock()

		// Remove this watcher
		for i, watcher := range s.watchers {
			if watcher == ch {
				s.watchers = append(s.watchers[:i], s.watchers[i+1:]...)
				close(ch)
				break
			}
		}
	}()

	return ch
}

// notifyWatchers sends an event to all registered watchers
func (s *sqliteStorage) notifyWatchers(event types.StorageEvent) {
	s.watcherMu.RLock()
	defer s.watcherMu.RUnlock()

	for _, watcher := range s.watchers {
		select {
		case watcher <- event:
			// Event sent successfully
		default:
			// Channel is full, log warning
			s.logger.Warn("storage event dropped", "type", event.Type, "kind", event.Kind)
		}
	}
}

// Users implementation

func (s *sqliteStorage) GetUser(ctx context.Context, id string) (*types.User, error) {
	var user types.User
	var metadata sql.NullString
	var lastLoginAt sql.NullTime

	query := `SELECT id, username, password_hash, email, is_admin, must_change_password,
	          active, created_at, updated_at, last_login_at, metadata
	          FROM users WHERE id = ?`

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.Email,
		&user.IsAdmin, &user.MustChangePassword, &user.Active,
		&user.CreatedAt, &user.UpdatedAt, &lastLoginAt, &metadata,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if lastLoginAt.Valid {
		user.LastLoginAt = &lastLoginAt.Time
	}

	if metadata.Valid && metadata.String != "" {
		if err := json.Unmarshal([]byte(metadata.String), &user.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &user, nil
}

func (s *sqliteStorage) GetUserByUsername(ctx context.Context, username string) (*types.User, error) {
	var user types.User
	var metadata sql.NullString
	var lastLoginAt sql.NullTime

	query := `SELECT id, username, password_hash, email, is_admin, must_change_password,
	          active, created_at, updated_at, last_login_at, metadata
	          FROM users WHERE username = ?`

	err := s.db.QueryRowContext(ctx, query, username).Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.Email,
		&user.IsAdmin, &user.MustChangePassword, &user.Active,
		&user.CreatedAt, &user.UpdatedAt, &lastLoginAt, &metadata,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if lastLoginAt.Valid {
		user.LastLoginAt = &lastLoginAt.Time
	}

	if metadata.Valid && metadata.String != "" {
		if err := json.Unmarshal([]byte(metadata.String), &user.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &user, nil
}

func (s *sqliteStorage) ListUsers(ctx context.Context) ([]*types.User, error) {
	query := `SELECT id, username, password_hash, email, is_admin, must_change_password,
	          active, created_at, updated_at, last_login_at, metadata
	          FROM users ORDER BY username`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer rows.Close()

	var users []*types.User
	for rows.Next() {
		var user types.User
		var metadata sql.NullString
		var lastLoginAt sql.NullTime

		err := rows.Scan(
			&user.ID, &user.Username, &user.PasswordHash, &user.Email,
			&user.IsAdmin, &user.MustChangePassword, &user.Active,
			&user.CreatedAt, &user.UpdatedAt, &lastLoginAt, &metadata,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}

		if lastLoginAt.Valid {
			user.LastLoginAt = &lastLoginAt.Time
		}

		if metadata.Valid && metadata.String != "" {
			if err := json.Unmarshal([]byte(metadata.String), &user.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		users = append(users, &user)
	}

	return users, nil
}

func (s *sqliteStorage) CreateUser(ctx context.Context, user *types.User) error {
	if user == nil {
		return types.ErrInvalidRequest
	}

	metadata, err := json.Marshal(user.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `INSERT INTO users (id, username, password_hash, email, is_admin,
	          must_change_password, active, metadata)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = s.db.ExecContext(ctx, query,
		user.ID, user.Username, user.PasswordHash, user.Email,
		user.IsAdmin, user.MustChangePassword, user.Active, string(metadata),
	)

	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

func (s *sqliteStorage) UpdateUser(ctx context.Context, user *types.User) error {
	if user == nil {
		return types.ErrInvalidRequest
	}

	metadata, err := json.Marshal(user.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `UPDATE users SET username = ?, password_hash = ?, email = ?,
	          is_admin = ?, must_change_password = ?, active = ?,
	          updated_at = CURRENT_TIMESTAMP, metadata = ?
	          WHERE id = ?`

	result, err := s.db.ExecContext(ctx, query,
		user.Username, user.PasswordHash, user.Email,
		user.IsAdmin, user.MustChangePassword, user.Active,
		string(metadata), user.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

func (s *sqliteStorage) DeleteUser(ctx context.Context, id string) error {
	// Check if user exists
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE id = ?", id).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check user existence: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("user not found")
	}

	// Delete API keys first (cascade deletion)
	_, err = s.db.ExecContext(ctx, "DELETE FROM api_keys WHERE user_id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete API keys: %w", err)
	}

	// Delete the user
	_, err = s.db.ExecContext(ctx, "DELETE FROM users WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	return nil
}

// API Keys implementation

func (s *sqliteStorage) GetAPIKey(ctx context.Context, key string) (*types.APIKey, error) {
	var apiKey types.APIKey
	var metadata sql.NullString
	var lastUsedAt, expiresAt sql.NullTime

	query := `SELECT key, user_id, name, description, active, created_at,
	          last_used_at, expires_at, metadata
	          FROM api_keys WHERE key = ?`

	err := s.db.QueryRowContext(ctx, query, key).Scan(
		&apiKey.Key, &apiKey.UserID, &apiKey.Name, &apiKey.Description,
		&apiKey.Active, &apiKey.CreatedAt, &lastUsedAt, &expiresAt, &metadata,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("API key not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get API key: %w", err)
	}

	if lastUsedAt.Valid {
		apiKey.LastUsedAt = &lastUsedAt.Time
	}

	if expiresAt.Valid {
		apiKey.ExpiresAt = &expiresAt.Time
	}

	if metadata.Valid && metadata.String != "" {
		if err := json.Unmarshal([]byte(metadata.String), &apiKey.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	// Update last used timestamp
	_, _ = s.db.ExecContext(ctx, "UPDATE api_keys SET last_used_at = CURRENT_TIMESTAMP WHERE key = ?", key)

	return &apiKey, nil
}

func (s *sqliteStorage) ListAPIKeysByUser(ctx context.Context, userID string) ([]*types.APIKey, error) {
	query := `SELECT key, user_id, name, description, active, created_at,
	          last_used_at, expires_at, metadata
	          FROM api_keys WHERE user_id = ? ORDER BY created_at DESC`

	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}
	defer rows.Close()

	var apiKeys []*types.APIKey
	for rows.Next() {
		var apiKey types.APIKey
		var metadata sql.NullString
		var lastUsedAt, expiresAt sql.NullTime

		err := rows.Scan(
			&apiKey.Key, &apiKey.UserID, &apiKey.Name, &apiKey.Description,
			&apiKey.Active, &apiKey.CreatedAt, &lastUsedAt, &expiresAt, &metadata,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan API key: %w", err)
		}

		if lastUsedAt.Valid {
			apiKey.LastUsedAt = &lastUsedAt.Time
		}

		if expiresAt.Valid {
			apiKey.ExpiresAt = &expiresAt.Time
		}

		if metadata.Valid && metadata.String != "" {
			if err := json.Unmarshal([]byte(metadata.String), &apiKey.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		apiKeys = append(apiKeys, &apiKey)
	}

	return apiKeys, nil
}

func (s *sqliteStorage) CreateAPIKey(ctx context.Context, apiKey *types.APIKey) error {
	if apiKey == nil {
		return types.ErrInvalidRequest
	}

	// Check if user exists
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE id = ?", apiKey.UserID).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check user existence: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("user not found")
	}

	metadata, err := json.Marshal(apiKey.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `INSERT INTO api_keys (key, user_id, name, description, active, expires_at, metadata)
	          VALUES (?, ?, ?, ?, ?, ?, ?)`

	var expiresAt sql.NullTime
	if apiKey.ExpiresAt != nil {
		expiresAt = sql.NullTime{Time: *apiKey.ExpiresAt, Valid: true}
	}

	_, err = s.db.ExecContext(ctx, query,
		apiKey.Key, apiKey.UserID, apiKey.Name, apiKey.Description,
		apiKey.Active, expiresAt, string(metadata),
	)

	if err != nil {
		return fmt.Errorf("failed to create API key: %w", err)
	}

	return nil
}

func (s *sqliteStorage) RevokeAPIKey(ctx context.Context, key string) error {
	result, err := s.db.ExecContext(ctx, "UPDATE api_keys SET active = FALSE WHERE key = ?", key)
	if err != nil {
		return fmt.Errorf("failed to revoke API key: %w", err)
	}
	
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	
	if rowsAffected == 0 {
		return fmt.Errorf("API key not found")
	}
	
	return nil
}

// Close closes the database connection
func (s *sqliteStorage) Close() error {
	close(s.stopWatch)
	s.wg.Wait()
	return s.db.Close()
}
