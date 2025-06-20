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

	// Delete service
	resp, err := s.client.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}

	if resp.Deleted == 0 {
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
	watchChan := s.client.Watch(context.Background(), s.prefix, clientv3.WithPrefix())

	for {
		select {
		case <-s.stopWatch:
			return
		case resp := <-watchChan:
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
	var object interface{}
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

// Helper methods

func (s *etcdStorage) serviceKey(id string) string {
	return fmt.Sprintf("%s/services/%s", s.prefix, id)
}

func (s *etcdStorage) routeKey(id string) string {
	return fmt.Sprintf("%s/routes/%s", s.prefix, id)
}
