package storage

import (
	"context"
	"discobox/internal/types"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// etcdStorage implements Storage interface using etcd
type etcdStorage struct {
	client    *clientv3.Client
	prefix    string
	watchers  []chan types.StorageEvent
	watcherMu sync.RWMutex
	stopWatch chan struct{}
}

// NewEtcd creates a new etcd storage instance
func NewEtcd(endpoints []string, prefix string) (types.Storage, error) {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client: %w", err)
	}

	// Test connection by attempting a simple operation with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = client.Status(ctx, endpoints[0])
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to connect to etcd: %w", err)
	}

	if prefix == "" {
		prefix = "/discobox"
	}

	s := &etcdStorage{
		client:    client,
		prefix:    prefix,
		watchers:  make([]chan types.StorageEvent, 0),
		stopWatch: make(chan struct{}),
	}

	// Start watching for changes
	go s.watchChanges()

	return s, nil
}

// Services

func (s *etcdStorage) GetService(ctx context.Context, id string) (*types.Service, error) {
	key := s.serviceKey(id)
	resp, err := s.client.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get service: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return nil, types.ErrServiceNotFound
	}

	var service types.Service
	if err := json.Unmarshal(resp.Kvs[0].Value, &service); err != nil {
		return nil, fmt.Errorf("failed to unmarshal service: %w", err)
	}

	return &service, nil
}

func (s *etcdStorage) ListServices(ctx context.Context) ([]*types.Service, error) {
	prefix := s.prefix + "/services/"
	resp, err := s.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	services := make([]*types.Service, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var service types.Service
		if err := json.Unmarshal(kv.Value, &service); err != nil {
			continue // Skip invalid entries
		}
		services = append(services, &service)
	}

	return services, nil
}

func (s *etcdStorage) CreateService(ctx context.Context, service *types.Service) error {
	key := s.serviceKey(service.ID)

	// Check if already exists
	resp, err := s.client.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to check service existence: %w", err)
	}
	if len(resp.Kvs) > 0 {
		return types.ErrAlreadyExists
	}

	// Set timestamps
	now := time.Now()
	service.CreatedAt = now
	service.UpdatedAt = now

	// Marshal service
	data, err := json.Marshal(service)
	if err != nil {
		return fmt.Errorf("failed to marshal service: %w", err)
	}

	// Create service
	if _, err := s.client.Put(ctx, key, string(data)); err != nil {
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

func (s *etcdStorage) UpdateService(ctx context.Context, service *types.Service) error {
	key := s.serviceKey(service.ID)

	// Check if exists
	resp, err := s.client.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to check service existence: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return types.ErrServiceNotFound
	}

	// Preserve created timestamp
	var existing types.Service
	if err := json.Unmarshal(resp.Kvs[0].Value, &existing); err == nil {
		service.CreatedAt = existing.CreatedAt
	}

	// Update timestamp
	service.UpdatedAt = time.Now()

	// Marshal service
	data, err := json.Marshal(service)
	if err != nil {
		return fmt.Errorf("failed to marshal service: %w", err)
	}

	// Update service
	if _, err := s.client.Put(ctx, key, string(data)); err != nil {
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

func (s *etcdStorage) DeleteService(ctx context.Context, id string) error {
	key := s.serviceKey(id)

	// Check if service exists
	resp, err := s.client.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to check service existence: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return types.ErrServiceNotFound
	}

	// Delete all routes for this service
	routes, _ := s.ListRoutes(ctx)
	for _, route := range routes {
		if route.ServiceID == id {
			s.DeleteRoute(ctx, route.ID)
		}
	}

	// Delete service
	resp2, err := s.client.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}

	if resp2.Deleted == 0 {
		return types.ErrServiceNotFound
	}

	// Notify watchers
	s.notifyWatchers(types.StorageEvent{
		Type: "deleted",
		Kind: "service",
		ID:   id,
	})

	return nil
}

// Routes

func (s *etcdStorage) GetRoute(ctx context.Context, id string) (*types.Route, error) {
	key := s.routeKey(id)
	resp, err := s.client.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get route: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return nil, types.ErrRouteNotFound
	}

	var route types.Route
	if err := json.Unmarshal(resp.Kvs[0].Value, &route); err != nil {
		return nil, fmt.Errorf("failed to unmarshal route: %w", err)
	}

	return &route, nil
}

func (s *etcdStorage) ListRoutes(ctx context.Context) ([]*types.Route, error) {
	prefix := s.prefix + "/routes/"
	resp, err := s.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list routes: %w", err)
	}

	routes := make([]*types.Route, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var route types.Route
		if err := json.Unmarshal(kv.Value, &route); err != nil {
			continue // Skip invalid entries
		}
		routes = append(routes, &route)
	}

	// Sort by priority (descending)
	for i := 0; i < len(routes); i++ {
		for j := i + 1; j < len(routes); j++ {
			if routes[i].Priority < routes[j].Priority {
				routes[i], routes[j] = routes[j], routes[i]
			}
		}
	}

	return routes, nil
}

func (s *etcdStorage) CreateRoute(ctx context.Context, route *types.Route) error {
	key := s.routeKey(route.ID)

	// Check if already exists
	resp, err := s.client.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to check route existence: %w", err)
	}
	if len(resp.Kvs) > 0 {
		return types.ErrAlreadyExists
	}

	// Validate service exists
	if _, err := s.GetService(ctx, route.ServiceID); err != nil {
		return fmt.Errorf("service %s not found", route.ServiceID)
	}

	// Marshal route
	data, err := json.Marshal(route)
	if err != nil {
		return fmt.Errorf("failed to marshal route: %w", err)
	}

	// Create route
	if _, err := s.client.Put(ctx, key, string(data)); err != nil {
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

func (s *etcdStorage) UpdateRoute(ctx context.Context, route *types.Route) error {
	key := s.routeKey(route.ID)

	// Check if exists
	resp, err := s.client.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to check route existence: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return types.ErrRouteNotFound
	}

	// Route doesn't have timestamps, just update directly

	// Marshal route
	data, err := json.Marshal(route)
	if err != nil {
		return fmt.Errorf("failed to marshal route: %w", err)
	}

	// Update route
	if _, err := s.client.Put(ctx, key, string(data)); err != nil {
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

func (s *etcdStorage) DeleteRoute(ctx context.Context, id string) error {
	key := s.routeKey(id)

	// Delete route
	resp, err := s.client.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to delete route: %w", err)
	}

	if resp.Deleted == 0 {
		return types.ErrRouteNotFound
	}

	// Notify watchers
	s.notifyWatchers(types.StorageEvent{
		Type: "deleted",
		Kind: "route",
		ID:   id,
	})

	return nil
}

// Watch for changes

func (s *etcdStorage) Watch(ctx context.Context) <-chan types.StorageEvent {
	ch := make(chan types.StorageEvent, 100)

	s.watcherMu.Lock()
	s.watchers = append(s.watchers, ch)
	s.watcherMu.Unlock()

	// Clean up on context cancellation
	go func() {
		<-ctx.Done()
		s.watcherMu.Lock()
		for i, watcher := range s.watchers {
			if watcher == ch {
				s.watchers = append(s.watchers[:i], s.watchers[i+1:]...)
				break
			}
		}
		s.watcherMu.Unlock()
		close(ch)
	}()

	return ch
}

// watchChanges watches etcd for changes
func (s *etcdStorage) watchChanges() {
	// Create a cancellable context for the watch
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Cancel the watch context when stopWatch is closed
	go func() {
		<-s.stopWatch
		cancel()
	}()

	watchChan := s.client.Watch(ctx, s.prefix, clientv3.WithPrefix())

	for {
		select {
		case <-s.stopWatch:
			return
		case resp, ok := <-watchChan:
			if !ok {
				return
			}
			if resp.Canceled {
				return
			}
			for _, event := range resp.Events {
				s.handleWatchEvent(event)
			}
		}
	}
}

// handleWatchEvent processes a watch event
func (s *etcdStorage) handleWatchEvent(event *clientv3.Event) {
	key := string(event.Kv.Key)

	// Determine event type and kind
	var eventType string
	switch event.Type {
	case clientv3.EventTypePut:
		if event.IsCreate() {
			eventType = "created"
		} else {
			eventType = "updated"
		}
	case clientv3.EventTypeDelete:
		eventType = "deleted"
	default:
		return
	}

	// Determine kind from key
	var kind string
	var id string
	if strings.Contains(key, "/services/") {
		kind = "service"
		id = strings.TrimPrefix(key, s.prefix+"/services/")
	} else if strings.Contains(key, "/routes/") {
		kind = "route"
		id = strings.TrimPrefix(key, s.prefix+"/routes/")
	} else {
		return
	}

	// Parse object if not deleted
	var object any
	if eventType != "deleted" {
		switch kind {
		case "service":
			var service types.Service
			if err := json.Unmarshal(event.Kv.Value, &service); err == nil {
				object = &service
			}
		case "route":
			var route types.Route
			if err := json.Unmarshal(event.Kv.Value, &route); err == nil {
				object = &route
			}
		}
	}

	// Notify watchers
	s.notifyWatchers(types.StorageEvent{
		Type:   eventType,
		Kind:   kind,
		ID:     id,
		Object: object,
	})
}

// notifyWatchers sends event to all watchers
func (s *etcdStorage) notifyWatchers(event types.StorageEvent) {
	s.watcherMu.RLock()
	defer s.watcherMu.RUnlock()

	for _, watcher := range s.watchers {
		select {
		case watcher <- event:
			// Event sent successfully
		default:
			// Channel is full, skip
		}
	}
}

// Close closes the etcd connection
func (s *etcdStorage) Close() error {
	close(s.stopWatch)
	return s.client.Close()
}

// Users implementation

func (s *etcdStorage) GetUser(ctx context.Context, id string) (*types.User, error) {
	key := s.userKey(id)
	resp, err := s.client.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("user not found")
	}

	var user types.User
	if err := json.Unmarshal(resp.Kvs[0].Value, &user); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user: %w", err)
	}

	return &user, nil
}

func (s *etcdStorage) GetUserByUsername(ctx context.Context, username string) (*types.User, error) {
	// List all users and find by username
	users, err := s.ListUsers(ctx)
	if err != nil {
		return nil, err
	}

	for _, user := range users {
		if user.Username == username {
			return user, nil
		}
	}

	return nil, fmt.Errorf("user not found")
}

func (s *etcdStorage) ListUsers(ctx context.Context) ([]*types.User, error) {
	prefix := s.prefix + "/users/"
	resp, err := s.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}

	users := make([]*types.User, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var user types.User
		if err := json.Unmarshal(kv.Value, &user); err != nil {
			continue // Skip invalid entries
		}
		users = append(users, &user)
	}

	return users, nil
}

func (s *etcdStorage) CreateUser(ctx context.Context, user *types.User) error {
	key := s.userKey(user.ID)

	// Check if already exists
	resp, err := s.client.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to check user existence: %w", err)
	}
	if len(resp.Kvs) > 0 {
		return fmt.Errorf("user already exists")
	}

	// Check for duplicate username
	existing, _ := s.GetUserByUsername(ctx, user.Username)
	if existing != nil {
		return fmt.Errorf("username already exists")
	}

	// Set timestamps
	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now

	// Marshal user
	data, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("failed to marshal user: %w", err)
	}

	// Create user
	if _, err := s.client.Put(ctx, key, string(data)); err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

func (s *etcdStorage) UpdateUser(ctx context.Context, user *types.User) error {
	key := s.userKey(user.ID)

	// Check if exists
	resp, err := s.client.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to check user existence: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return fmt.Errorf("user not found")
	}

	// Preserve created timestamp
	var existing types.User
	if err := json.Unmarshal(resp.Kvs[0].Value, &existing); err == nil {
		user.CreatedAt = existing.CreatedAt
	}

	// Update timestamp
	user.UpdatedAt = time.Now()

	// Marshal user
	data, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("failed to marshal user: %w", err)
	}

	// Update user
	if _, err := s.client.Put(ctx, key, string(data)); err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	return nil
}

func (s *etcdStorage) DeleteUser(ctx context.Context, id string) error {
	key := s.userKey(id)

	// Delete user
	resp, err := s.client.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	if resp.Deleted == 0 {
		return fmt.Errorf("user not found")
	}

	// Delete all API keys for this user
	apiKeys, _ := s.ListAPIKeysByUser(ctx, id)
	for _, apiKey := range apiKeys {
		s.RevokeAPIKey(ctx, apiKey.Key)
	}

	return nil
}

// API Keys implementation

func (s *etcdStorage) GetAPIKey(ctx context.Context, key string) (*types.APIKey, error) {
	keyPath := s.apiKeyKey(key)
	resp, err := s.client.Get(ctx, keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get API key: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("API key not found")
	}

	var apiKey types.APIKey
	if err := json.Unmarshal(resp.Kvs[0].Value, &apiKey); err != nil {
		return nil, fmt.Errorf("failed to unmarshal API key: %w", err)
	}

	// Update last used timestamp
	now := time.Now()
	apiKey.LastUsedAt = &now
	data, _ := json.Marshal(apiKey)
	s.client.Put(ctx, keyPath, string(data))

	return &apiKey, nil
}

func (s *etcdStorage) ListAPIKeysByUser(ctx context.Context, userID string) ([]*types.APIKey, error) {
	prefix := s.prefix + "/api_keys/"
	resp, err := s.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}

	var apiKeys []*types.APIKey
	for _, kv := range resp.Kvs {
		var apiKey types.APIKey
		if err := json.Unmarshal(kv.Value, &apiKey); err != nil {
			continue // Skip invalid entries
		}
		if apiKey.UserID == userID {
			apiKeys = append(apiKeys, &apiKey)
		}
	}

	return apiKeys, nil
}

func (s *etcdStorage) CreateAPIKey(ctx context.Context, apiKey *types.APIKey) error {
	key := s.apiKeyKey(apiKey.Key)

	// Check if already exists
	resp, err := s.client.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to check API key existence: %w", err)
	}
	if len(resp.Kvs) > 0 {
		return fmt.Errorf("API key already exists")
	}

	// Validate user exists
	if _, err := s.GetUser(ctx, apiKey.UserID); err != nil {
		return fmt.Errorf("user %s not found", apiKey.UserID)
	}

	// Set timestamp
	now := time.Now()
	apiKey.CreatedAt = now

	// Marshal API key
	data, err := json.Marshal(apiKey)
	if err != nil {
		return fmt.Errorf("failed to marshal API key: %w", err)
	}

	// Create API key
	if _, err := s.client.Put(ctx, key, string(data)); err != nil {
		return fmt.Errorf("failed to create API key: %w", err)
	}

	return nil
}

func (s *etcdStorage) RevokeAPIKey(ctx context.Context, key string) error {
	keyPath := s.apiKeyKey(key)

	// Get API key
	resp, err := s.client.Get(ctx, keyPath)
	if err != nil {
		return fmt.Errorf("failed to get API key: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return fmt.Errorf("API key not found")
	}

	var apiKey types.APIKey
	if err := json.Unmarshal(resp.Kvs[0].Value, &apiKey); err != nil {
		return fmt.Errorf("failed to unmarshal API key: %w", err)
	}

	// Mark as inactive
	apiKey.Active = false
	data, _ := json.Marshal(apiKey)
	if _, err := s.client.Put(ctx, keyPath, string(data)); err != nil {
		return fmt.Errorf("failed to revoke API key: %w", err)
	}

	return nil
}

// Helper methods

func (s *etcdStorage) serviceKey(id string) string {
	return fmt.Sprintf("%s/services/%s", s.prefix, id)
}

func (s *etcdStorage) routeKey(id string) string {
	return fmt.Sprintf("%s/routes/%s", s.prefix, id)
}

func (s *etcdStorage) userKey(id string) string {
	return fmt.Sprintf("%s/users/%s", s.prefix, id)
}

func (s *etcdStorage) apiKeyKey(key string) string {
	return fmt.Sprintf("%s/api_keys/%s", s.prefix, key)
}
