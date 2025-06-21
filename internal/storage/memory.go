package storage

import (
	"context"
	"errors"
	"sync"
	"time"

	"discobox/internal/types"
)

// memoryStorage implements Storage interface using in-memory maps
type memoryStorage struct {
	mu        sync.RWMutex
	services  map[string]*types.Service
	routes    map[string]*types.Route
	users     map[string]*types.User
	usernames map[string]string // username -> userID mapping
	apiKeys   map[string]*types.APIKey
	watchers  []chan types.StorageEvent
	watcherMu sync.RWMutex
}

// NewMemory creates a new in-memory storage instance
func NewMemory() types.Storage {
	return &memoryStorage{
		services:  make(map[string]*types.Service),
		routes:    make(map[string]*types.Route),
		users:     make(map[string]*types.User),
		usernames: make(map[string]string),
		apiKeys:   make(map[string]*types.APIKey),
		watchers:  make([]chan types.StorageEvent, 0),
	}
}

// Services implementation

func (m *memoryStorage) GetService(ctx context.Context, id string) (*types.Service, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	service, exists := m.services[id]
	if !exists {
		return nil, types.ErrServiceNotFound
	}
	
	// Return a copy to prevent external modifications
	serviceCopy := *service
	return &serviceCopy, nil
}

func (m *memoryStorage) ListServices(ctx context.Context) ([]*types.Service, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	services := make([]*types.Service, 0, len(m.services))
	for _, service := range m.services {
		// Create a copy
		serviceCopy := *service
		services = append(services, &serviceCopy)
	}
	
	return services, nil
}

func (m *memoryStorage) CreateService(ctx context.Context, service *types.Service) error {
	if service == nil {
		return types.ErrInvalidRequest
	}
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.services[service.ID]; exists {
		return types.ErrAlreadyExists
	}
	
	// Set timestamps
	now := time.Now()
	service.CreatedAt = now
	service.UpdatedAt = now
	
	// Create a copy to store
	serviceCopy := *service
	m.services[service.ID] = &serviceCopy
	
	// Notify watchers
	m.notifyWatchers(types.StorageEvent{
		Type:   "created",
		Kind:   "service",
		ID:     service.ID,
		Object: &serviceCopy,
	})
	
	return nil
}

func (m *memoryStorage) UpdateService(ctx context.Context, service *types.Service) error {
	if service == nil {
		return types.ErrInvalidRequest
	}
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	existing, exists := m.services[service.ID]
	if !exists {
		return types.ErrServiceNotFound
	}
	
	// Update timestamp
	service.UpdatedAt = time.Now()
	// Preserve creation timestamp
	service.CreatedAt = existing.CreatedAt
	
	// Create a copy to store
	serviceCopy := *service
	m.services[service.ID] = &serviceCopy
	
	// Notify watchers
	m.notifyWatchers(types.StorageEvent{
		Type:   "updated",
		Kind:   "service",
		ID:     service.ID,
		Object: &serviceCopy,
	})
	
	return nil
}

func (m *memoryStorage) DeleteService(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	service, exists := m.services[id]
	if !exists {
		return types.ErrServiceNotFound
	}
	
	delete(m.services, id)
	
	// Notify watchers
	m.notifyWatchers(types.StorageEvent{
		Type:   "deleted",
		Kind:   "service",
		ID:     id,
		Object: service,
	})
	
	return nil
}

// Routes implementation

func (m *memoryStorage) GetRoute(ctx context.Context, id string) (*types.Route, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	route, exists := m.routes[id]
	if !exists {
		return nil, types.ErrRouteNotFound
	}
	
	// Return a copy
	routeCopy := *route
	return &routeCopy, nil
}

func (m *memoryStorage) ListRoutes(ctx context.Context) ([]*types.Route, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	routes := make([]*types.Route, 0, len(m.routes))
	for _, route := range m.routes {
		// Create a copy
		routeCopy := *route
		routes = append(routes, &routeCopy)
	}
	
	return routes, nil
}

func (m *memoryStorage) CreateRoute(ctx context.Context, route *types.Route) error {
	if route == nil {
		return types.ErrInvalidRequest
	}
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.routes[route.ID]; exists {
		return types.ErrAlreadyExists
	}
	
	// Validate that the service exists
	if _, exists := m.services[route.ServiceID]; !exists {
		return errors.New("service not found for route")
	}
	
	// Create a copy to store
	routeCopy := *route
	m.routes[route.ID] = &routeCopy
	
	// Notify watchers
	m.notifyWatchers(types.StorageEvent{
		Type:   "created",
		Kind:   "route",
		ID:     route.ID,
		Object: &routeCopy,
	})
	
	return nil
}

func (m *memoryStorage) UpdateRoute(ctx context.Context, route *types.Route) error {
	if route == nil {
		return types.ErrInvalidRequest
	}
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.routes[route.ID]; !exists {
		return types.ErrRouteNotFound
	}
	
	// Validate that the service exists
	if _, exists := m.services[route.ServiceID]; !exists {
		return errors.New("service not found for route")
	}
	
	// Create a copy to store
	routeCopy := *route
	m.routes[route.ID] = &routeCopy
	
	// Notify watchers
	m.notifyWatchers(types.StorageEvent{
		Type:   "updated",
		Kind:   "route",
		ID:     route.ID,
		Object: &routeCopy,
	})
	
	return nil
}

func (m *memoryStorage) DeleteRoute(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	route, exists := m.routes[id]
	if !exists {
		return types.ErrRouteNotFound
	}
	
	delete(m.routes, id)
	
	// Notify watchers
	m.notifyWatchers(types.StorageEvent{
		Type:   "deleted",
		Kind:   "route",
		ID:     id,
		Object: route,
	})
	
	return nil
}

// Watch implementation

func (m *memoryStorage) Watch(ctx context.Context) <-chan types.StorageEvent {
	m.watcherMu.Lock()
	defer m.watcherMu.Unlock()
	
	ch := make(chan types.StorageEvent, 100) // Buffered channel
	m.watchers = append(m.watchers, ch)
	
	// Clean up when context is done
	go func() {
		<-ctx.Done()
		m.watcherMu.Lock()
		defer m.watcherMu.Unlock()
		
		// Remove this watcher
		for i, watcher := range m.watchers {
			if watcher == ch {
				m.watchers = append(m.watchers[:i], m.watchers[i+1:]...)
				close(ch)
				break
			}
		}
	}()
	
	return ch
}

// notifyWatchers sends an event to all registered watchers
func (m *memoryStorage) notifyWatchers(event types.StorageEvent) {
	m.watcherMu.RLock()
	defer m.watcherMu.RUnlock()
	
	for _, watcher := range m.watchers {
		select {
		case watcher <- event:
			// Event sent successfully
		default:
			// Channel is full, drop the event
			// In a production system, we might want to log this
		}
	}
}

// Users implementation

func (m *memoryStorage) GetUser(ctx context.Context, id string) (*types.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	user, exists := m.users[id]
	if !exists {
		return nil, errors.New("user not found")
	}
	
	// Return a copy
	userCopy := *user
	return &userCopy, nil
}

func (m *memoryStorage) GetUserByUsername(ctx context.Context, username string) (*types.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	userID, exists := m.usernames[username]
	if !exists {
		return nil, errors.New("user not found")
	}
	
	user, exists := m.users[userID]
	if !exists {
		return nil, errors.New("user not found")
	}
	
	// Return a copy
	userCopy := *user
	return &userCopy, nil
}

func (m *memoryStorage) ListUsers(ctx context.Context) ([]*types.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	users := make([]*types.User, 0, len(m.users))
	for _, user := range m.users {
		// Create a copy
		userCopy := *user
		users = append(users, &userCopy)
	}
	
	return users, nil
}

func (m *memoryStorage) CreateUser(ctx context.Context, user *types.User) error {
	if user == nil {
		return types.ErrInvalidRequest
	}
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.users[user.ID]; exists {
		return errors.New("user already exists")
	}
	
	if _, exists := m.usernames[user.Username]; exists {
		return errors.New("username already taken")
	}
	
	// Set timestamps
	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now
	
	// Create a copy to store
	userCopy := *user
	m.users[user.ID] = &userCopy
	m.usernames[user.Username] = user.ID
	
	return nil
}

func (m *memoryStorage) UpdateUser(ctx context.Context, user *types.User) error {
	if user == nil {
		return types.ErrInvalidRequest
	}
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	existing, exists := m.users[user.ID]
	if !exists {
		return errors.New("user not found")
	}
	
	// If username changed, update mapping
	if existing.Username != user.Username {
		// Check if new username is taken
		if _, exists := m.usernames[user.Username]; exists {
			return errors.New("username already taken")
		}
		delete(m.usernames, existing.Username)
		m.usernames[user.Username] = user.ID
	}
	
	// Update timestamp
	user.UpdatedAt = time.Now()
	// Preserve creation timestamp
	user.CreatedAt = existing.CreatedAt
	
	// Create a copy to store
	userCopy := *user
	m.users[user.ID] = &userCopy
	
	return nil
}

func (m *memoryStorage) DeleteUser(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	user, exists := m.users[id]
	if !exists {
		return errors.New("user not found")
	}
	
	delete(m.users, id)
	delete(m.usernames, user.Username)
	
	// Delete all API keys for this user
	for key, apiKey := range m.apiKeys {
		if apiKey.UserID == id {
			delete(m.apiKeys, key)
		}
	}
	
	return nil
}

// API Keys implementation

func (m *memoryStorage) GetAPIKey(ctx context.Context, key string) (*types.APIKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	apiKey, exists := m.apiKeys[key]
	if !exists {
		return nil, errors.New("API key not found")
	}
	
	// Update last used time
	now := time.Now()
	apiKey.LastUsedAt = &now
	
	// Return a copy
	apiKeyCopy := *apiKey
	return &apiKeyCopy, nil
}

func (m *memoryStorage) ListAPIKeysByUser(ctx context.Context, userID string) ([]*types.APIKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var apiKeys []*types.APIKey
	for _, apiKey := range m.apiKeys {
		if apiKey.UserID == userID {
			// Create a copy
			apiKeyCopy := *apiKey
			apiKeys = append(apiKeys, &apiKeyCopy)
		}
	}
	
	return apiKeys, nil
}

func (m *memoryStorage) CreateAPIKey(ctx context.Context, apiKey *types.APIKey) error {
	if apiKey == nil {
		return types.ErrInvalidRequest
	}
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.apiKeys[apiKey.Key]; exists {
		return errors.New("API key already exists")
	}
	
	// Verify user exists
	if _, exists := m.users[apiKey.UserID]; !exists {
		return errors.New("user not found")
	}
	
	// Set timestamp
	apiKey.CreatedAt = time.Now()
	
	// Create a copy to store
	apiKeyCopy := *apiKey
	m.apiKeys[apiKey.Key] = &apiKeyCopy
	
	return nil
}

func (m *memoryStorage) RevokeAPIKey(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	apiKey, exists := m.apiKeys[key]
	if !exists {
		return errors.New("API key not found")
	}
	
	apiKey.Active = false
	
	return nil
}

// Close closes the storage
func (m *memoryStorage) Close() error {
	m.watcherMu.Lock()
	defer m.watcherMu.Unlock()
	
	// Close all watcher channels
	for _, watcher := range m.watchers {
		close(watcher)
	}
	m.watchers = nil
	
	return nil
}
