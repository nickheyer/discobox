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
	watchers  []chan types.StorageEvent
	watcherMu sync.RWMutex
}

// NewMemory creates a new in-memory storage instance
func NewMemory() types.Storage {
	return &memoryStorage{
		services: make(map[string]*types.Service),
		routes:   make(map[string]*types.Route),
		watchers: make([]chan types.StorageEvent, 0),
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
