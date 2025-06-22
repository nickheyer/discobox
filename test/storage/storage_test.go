package storage_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"discobox/internal/storage"
	"discobox/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clientv3 "go.etcd.io/etcd/client/v3"
	_ "github.com/mattn/go-sqlite3"
)

// testLogger is a simple logger implementation for tests
type testLogger struct{}

func (l *testLogger) Debug(msg string, fields ...interface{}) {}
func (l *testLogger) Info(msg string, fields ...interface{})  {}
func (l *testLogger) Warn(msg string, fields ...interface{})  {}
func (l *testLogger) Error(msg string, fields ...interface{}) {}
func (l *testLogger) With(fields ...interface{}) types.Logger { return l }

func setupMemoryStorage(t *testing.T) types.Storage {
	s := storage.NewMemory()
	return s
}

func setupSQLiteStorage(t *testing.T) types.Storage {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	logger := &testLogger{}
	s, err := storage.NewSQLite(dbPath, logger)
	require.NoError(t, err)
	return s
}

func setupEtcdStorage(t *testing.T) types.Storage {
	// Skip if etcd is not available
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: 1 * time.Second,
	})
	if err != nil {
		t.Skip("etcd not available: " + err.Error())
		return nil
	}
	
	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_, err = client.Status(ctx, "localhost:2379")
	client.Close()
	if err != nil {
		t.Skip("etcd not available: " + err.Error())
		return nil
	}

	s, err := storage.NewEtcd([]string{"localhost:2379"}, "/test-"+fmt.Sprintf("%d", time.Now().UnixNano()))
	if err != nil {
		t.Skip("etcd not available: " + err.Error())
		return nil
	}
	return s
}

func testStorageImplementations(t *testing.T, name string, setupFunc func(*testing.T) types.Storage) {
	t.Run(name, func(t *testing.T) {
		t.Run("ServiceOperations", func(t *testing.T) { testServiceOperations(t, setupFunc) })
		t.Run("RouteOperations", func(t *testing.T) { testRouteOperations(t, setupFunc) })
		t.Run("UserOperations", func(t *testing.T) { testUserOperations(t, setupFunc) })
		t.Run("APIKeyOperations", func(t *testing.T) { testAPIKeyOperations(t, setupFunc) })
		t.Run("WatchOperations", func(t *testing.T) { testWatchOperations(t, setupFunc) })
		t.Run("ConcurrentOperations", func(t *testing.T) { testConcurrentOperations(t, setupFunc) })
	})
}

func TestStorageImplementations(t *testing.T) {
	testStorageImplementations(t, "Memory", setupMemoryStorage)
	testStorageImplementations(t, "SQLite", setupSQLiteStorage)
	testStorageImplementations(t, "Etcd", setupEtcdStorage)
}

func testServiceOperations(t *testing.T, setupFunc func(*testing.T) types.Storage) {
	s := setupFunc(t)
	if s == nil {
		return // Storage setup was skipped
	}
	defer s.Close()

	ctx := context.Background()

	// Test CreateService
	service1 := &types.Service{
		ID:          "service1",
		Name:        "Test Service 1",
		Endpoints:   []string{"http://localhost:8080", "http://localhost:8081"},
		HealthPath:  "/health",
		Weight:      100,
		MaxConns:    1000,
		Timeout:     30,
		StripPrefix: false,
		Active:      true,
		Metadata: map[string]string{
			"env": "test",
		},
	}

	err := s.CreateService(ctx, service1)
	assert.NoError(t, err)

	// Test CreateService with duplicate ID
	err = s.CreateService(ctx, service1)
	assert.Error(t, err)

	// Test GetService
	retrieved, err := s.GetService(ctx, "service1")
	assert.NoError(t, err)
	assert.Equal(t, service1.ID, retrieved.ID)
	assert.Equal(t, service1.Name, retrieved.Name)
	assert.Equal(t, service1.Endpoints, retrieved.Endpoints)
	assert.Equal(t, service1.HealthPath, retrieved.HealthPath)
	assert.Equal(t, service1.Weight, retrieved.Weight)
	assert.NotNil(t, retrieved.CreatedAt)
	assert.NotNil(t, retrieved.UpdatedAt)

	// Test GetService with non-existent ID
	_, err = s.GetService(ctx, "non-existent")
	assert.Error(t, err)

	// Test ListServices
	service2 := &types.Service{
		ID:        "service2",
		Name:      "Test Service 2",
		Endpoints: []string{"http://localhost:9080"},
		Active:    true,
	}
	err = s.CreateService(ctx, service2)
	assert.NoError(t, err)

	services, err := s.ListServices(ctx)
	assert.NoError(t, err)
	assert.Len(t, services, 2)

	// Test UpdateService
	service1.Name = "Updated Service 1"
	service1.Endpoints = append(service1.Endpoints, "http://localhost:8082")
	service1.Weight = 200
	err = s.UpdateService(ctx, service1)
	assert.NoError(t, err)

	updated, err := s.GetService(ctx, "service1")
	assert.NoError(t, err)
	assert.Equal(t, "Updated Service 1", updated.Name)
	assert.Len(t, updated.Endpoints, 3)
	assert.Equal(t, 200, updated.Weight)
	// Allow for timestamp precision differences (SQLite may have lower precision)
	assert.True(t, updated.UpdatedAt.After(retrieved.UpdatedAt) || updated.UpdatedAt.Equal(retrieved.UpdatedAt))

	// Test UpdateService with non-existent ID
	nonExistent := &types.Service{ID: "non-existent", Name: "Ghost"}
	err = s.UpdateService(ctx, nonExistent)
	assert.Error(t, err)

	// Test DeleteService
	err = s.DeleteService(ctx, "service2")
	assert.NoError(t, err)

	services, err = s.ListServices(ctx)
	assert.NoError(t, err)
	assert.Len(t, services, 1)

	// Test DeleteService with non-existent ID
	err = s.DeleteService(ctx, "non-existent")
	assert.Error(t, err)

	// Test service data isolation (mutations don't affect stored data)
	svc, err := s.GetService(ctx, "service1")
	assert.NoError(t, err)
	originalEndpoints := len(svc.Endpoints)
	svc.Endpoints = append(svc.Endpoints, "http://localhost:9999")

	svc2, err := s.GetService(ctx, "service1")
	assert.NoError(t, err)
	assert.Len(t, svc2.Endpoints, originalEndpoints)
}

func testRouteOperations(t *testing.T, setupFunc func(*testing.T) types.Storage) {
	s := setupFunc(t)
	if s == nil {
		return // Storage setup was skipped
	}
	defer s.Close()

	ctx := context.Background()

	// Create a service first (routes reference services)
	service := &types.Service{
		ID:        "service1",
		Name:      "Test Service",
		Endpoints: []string{"http://localhost:8080"},
		Active:    true,
	}
	err := s.CreateService(ctx, service)
	require.NoError(t, err)

	// Test CreateRoute
	route1 := &types.Route{
		ID:         "route1",
		Priority:   100,
		Host:       "example.com",
		PathPrefix: "/api",
		ServiceID:  "service1",
		Middlewares: []string{"auth", "ratelimit"},
		Headers: map[string]string{
			"X-API-Version": "v1",
		},
	}

	err = s.CreateRoute(ctx, route1)
	assert.NoError(t, err)

	// Test CreateRoute with duplicate ID
	err = s.CreateRoute(ctx, route1)
	assert.Error(t, err)

	// Test CreateRoute with non-existent service
	invalidRoute := &types.Route{
		ID:        "invalid",
		ServiceID: "non-existent",
		Host:      "test.com",
	}
	err = s.CreateRoute(ctx, invalidRoute)
	assert.Error(t, err)

	// Test GetRoute
	retrieved, err := s.GetRoute(ctx, "route1")
	assert.NoError(t, err)
	assert.Equal(t, route1.ID, retrieved.ID)
	assert.Equal(t, route1.Priority, retrieved.Priority)
	assert.Equal(t, route1.Host, retrieved.Host)
	assert.Equal(t, route1.PathPrefix, retrieved.PathPrefix)
	assert.Equal(t, route1.ServiceID, retrieved.ServiceID)
	assert.Equal(t, route1.Middlewares, retrieved.Middlewares)

	// Test GetRoute with non-existent ID
	_, err = s.GetRoute(ctx, "non-existent")
	assert.Error(t, err)

	// Test ListRoutes
	route2 := &types.Route{
		ID:        "route2",
		Priority:  50,
		Host:      "api.example.com",
		ServiceID: "service1",
	}
	err = s.CreateRoute(ctx, route2)
	assert.NoError(t, err)

	routes, err := s.ListRoutes(ctx)
	assert.NoError(t, err)
	assert.Len(t, routes, 2)

	// Verify routes are sorted by priority (descending)
	assert.Equal(t, 100, routes[0].Priority)
	assert.Equal(t, 50, routes[1].Priority)

	// Test UpdateRoute
	route1.Priority = 150
	route1.PathPrefix = "/v2"
	err = s.UpdateRoute(ctx, route1)
	assert.NoError(t, err)

	updated, err := s.GetRoute(ctx, "route1")
	assert.NoError(t, err)
	assert.Equal(t, 150, updated.Priority)
	assert.Equal(t, "/v2", updated.PathPrefix)

	// Test UpdateRoute with non-existent ID
	nonExistent := &types.Route{ID: "non-existent", ServiceID: "service1"}
	err = s.UpdateRoute(ctx, nonExistent)
	assert.Error(t, err)

	// Test DeleteRoute
	err = s.DeleteRoute(ctx, "route2")
	assert.NoError(t, err)

	routes, err = s.ListRoutes(ctx)
	assert.NoError(t, err)
	assert.Len(t, routes, 1)

	// Test DeleteRoute with non-existent ID
	err = s.DeleteRoute(ctx, "non-existent")
	assert.Error(t, err)

	// Test route deletion when service is deleted
	err = s.DeleteService(ctx, "service1")
	assert.NoError(t, err)

	routes, err = s.ListRoutes(ctx)
	assert.NoError(t, err)
	assert.Len(t, routes, 0)
}

func testUserOperations(t *testing.T, setupFunc func(*testing.T) types.Storage) {
	s := setupFunc(t)
	if s == nil {
		return // Storage setup was skipped
	}
	defer s.Close()

	ctx := context.Background()

	// Test CreateUser
	user1 := &types.User{
		ID:           "user1",
		Username:     "testuser1",
		PasswordHash: "$2a$10$YMF3qRxtVk2u7qCYVayhOe7dN0jp0VQ5k5uF0x5xJxUJB1Y0xJzDm", // bcrypt hash
		Email:        "test1@example.com",
		IsAdmin:      false,
		Active:       true,
	}

	err := s.CreateUser(ctx, user1)
	assert.NoError(t, err)

	// Test CreateUser with duplicate ID
	err = s.CreateUser(ctx, user1)
	assert.Error(t, err)

	// Test CreateUser with duplicate username
	duplicateUsername := &types.User{
		ID:           "user2",
		Username:     "testuser1", // Same username
		PasswordHash: "$2a$10$YMF3qRxtVk2u7qCYVayhOe7dN0jp0VQ5k5uF0x5xJxUJB1Y0xJzDm",
		Email:        "test2@example.com",
	}
	err = s.CreateUser(ctx, duplicateUsername)
	assert.Error(t, err)

	// Test GetUser
	retrieved, err := s.GetUser(ctx, "user1")
	assert.NoError(t, err)
	assert.Equal(t, user1.ID, retrieved.ID)
	assert.Equal(t, user1.Username, retrieved.Username)
	assert.Equal(t, user1.Email, retrieved.Email)
	assert.NotNil(t, retrieved.CreatedAt)
	assert.NotNil(t, retrieved.UpdatedAt)

	// Test GetUser with non-existent ID
	_, err = s.GetUser(ctx, "non-existent")
	assert.Error(t, err)

	// Test GetUserByUsername
	byUsername, err := s.GetUserByUsername(ctx, "testuser1")
	assert.NoError(t, err)
	assert.Equal(t, user1.ID, byUsername.ID)

	// Test GetUserByUsername with non-existent username
	_, err = s.GetUserByUsername(ctx, "non-existent")
	assert.Error(t, err)

	// Test ListUsers
	user2 := &types.User{
		ID:           "user2",
		Username:     "testuser2",
		PasswordHash: "$2a$10$YMF3qRxtVk2u7qCYVayhOe7dN0jp0VQ5k5uF0x5xJxUJB1Y0xJzDm",
		Email:        "test2@example.com",
		IsAdmin:      true,
		Active:       true,
	}
	err = s.CreateUser(ctx, user2)
	assert.NoError(t, err)

	users, err := s.ListUsers(ctx)
	assert.NoError(t, err)
	assert.Len(t, users, 2)

	// Test UpdateUser
	user1.Email = "updated@example.com"
	user1.IsAdmin = true
	err = s.UpdateUser(ctx, user1)
	assert.NoError(t, err)

	updated, err := s.GetUser(ctx, "user1")
	assert.NoError(t, err)
	assert.Equal(t, "updated@example.com", updated.Email)
	assert.True(t, updated.IsAdmin)
	// Allow for timestamp precision differences (SQLite may have lower precision)
	assert.True(t, updated.UpdatedAt.After(retrieved.UpdatedAt) || updated.UpdatedAt.Equal(retrieved.UpdatedAt))

	// Test UpdateUser with non-existent ID
	nonExistent := &types.User{ID: "non-existent", Username: "ghost"}
	err = s.UpdateUser(ctx, nonExistent)
	assert.Error(t, err)

	// Test DeleteUser
	err = s.DeleteUser(ctx, "user2")
	assert.NoError(t, err)

	users, err = s.ListUsers(ctx)
	assert.NoError(t, err)
	assert.Len(t, users, 1)

	// Test DeleteUser with non-existent ID
	err = s.DeleteUser(ctx, "non-existent")
	assert.Error(t, err)
}

func testAPIKeyOperations(t *testing.T, setupFunc func(*testing.T) types.Storage) {
	s := setupFunc(t)
	if s == nil {
		return // Storage setup was skipped
	}
	defer s.Close()

	ctx := context.Background()

	// Create a user first (API keys reference users)
	user := &types.User{
		ID:           "user1",
		Username:     "testuser",
		PasswordHash: "$2a$10$YMF3qRxtVk2u7qCYVayhOe7dN0jp0VQ5k5uF0x5xJxUJB1Y0xJzDm",
		Email:        "test@example.com",
		Active:       true,
	}
	err := s.CreateUser(ctx, user)
	require.NoError(t, err)

	// Test CreateAPIKey
	expiresAt := time.Now().Add(24 * time.Hour)
	apiKey1 := &types.APIKey{
		Key:         "test-api-key-1",
		UserID:      "user1",
		Name:        "key1",
		Description: "Test API Key 1",
		ExpiresAt:   &expiresAt,
		Active:      true,
	}

	err = s.CreateAPIKey(ctx, apiKey1)
	assert.NoError(t, err)

	// Test CreateAPIKey with duplicate ID
	err = s.CreateAPIKey(ctx, apiKey1)
	assert.Error(t, err)

	// Test CreateAPIKey with non-existent user
	invalidKey := &types.APIKey{
		Key:    "invalid-key",
		UserID: "non-existent",
		Name:   "invalid",
	}
	err = s.CreateAPIKey(ctx, invalidKey)
	assert.Error(t, err)

	// Test GetAPIKey
	retrieved, err := s.GetAPIKey(ctx, "test-api-key-1")
	assert.NoError(t, err)
	assert.Equal(t, apiKey1.Name, retrieved.Name)
	assert.Equal(t, apiKey1.UserID, retrieved.UserID)
	assert.Equal(t, apiKey1.Key, retrieved.Key)
	assert.Equal(t, apiKey1.Description, retrieved.Description)
	assert.NotZero(t, retrieved.CreatedAt)

	// Test GetAPIKey with non-existent key
	_, err = s.GetAPIKey(ctx, "non-existent")
	assert.Error(t, err)

	// Test ListAPIKeysByUser
	apiKey2 := &types.APIKey{
		Key:         "test-api-key-2",
		UserID:      "user1",
		Name:        "key2",
		Description: "Test API Key 2",
		Active:      true,
	}
	err = s.CreateAPIKey(ctx, apiKey2)
	assert.NoError(t, err)

	keys, err := s.ListAPIKeysByUser(ctx, "user1")
	assert.NoError(t, err)
	assert.Len(t, keys, 2)

	// Test ListAPIKeysByUser with non-existent user
	keys, err = s.ListAPIKeysByUser(ctx, "non-existent")
	assert.NoError(t, err)
	assert.Len(t, keys, 0)

	// Test RevokeAPIKey
	err = s.RevokeAPIKey(ctx, "test-api-key-2")
	assert.NoError(t, err)

	revoked, err := s.GetAPIKey(ctx, "test-api-key-2")
	assert.NoError(t, err)
	assert.False(t, revoked.Active)

	// Test RevokeAPIKey with non-existent key
	err = s.RevokeAPIKey(ctx, "non-existent")
	assert.Error(t, err)

	// Test API key deletion when user is deleted
	err = s.DeleteUser(ctx, "user1")
	assert.NoError(t, err)

	_, err = s.GetAPIKey(ctx, "test-api-key-1")
	assert.Error(t, err) // Key should be deleted with user
}

func testWatchOperations(t *testing.T, setupFunc func(*testing.T) types.Storage) {
	s := setupFunc(t)
	if s == nil {
		return // Storage setup was skipped
	}
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start watching
	events := s.Watch(ctx)

	// Give the watch goroutine time to start
	time.Sleep(100 * time.Millisecond)

	// Create a service and expect an event
	service := &types.Service{
		ID:        "watch-test",
		Name:      "Watch Test Service",
		Endpoints: []string{"http://localhost:8080"},
		Active:    true,
	}
	err := s.CreateService(ctx, service)
	require.NoError(t, err)

	// Wait for event with timeout
	select {
	case event := <-events:
		assert.NotNil(t, event)
		assert.Equal(t, "created", event.Type)
		assert.Equal(t, "service", event.Kind)
		assert.Equal(t, "watch-test", event.ID)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for create event")
	}

	// Update the service
	service.Name = "Updated Watch Test"
	err = s.UpdateService(ctx, service)
	require.NoError(t, err)

	// Wait for update event
	select {
	case event := <-events:
		assert.NotNil(t, event)
		assert.Equal(t, "updated", event.Type)
		assert.Equal(t, "service", event.Kind)
		assert.Equal(t, "watch-test", event.ID)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for update event")
	}

	// Delete the service
	err = s.DeleteService(ctx, "watch-test")
	require.NoError(t, err)

	// Wait for delete event
	select {
	case event := <-events:
		assert.NotNil(t, event)
		assert.Equal(t, "deleted", event.Type)
		assert.Equal(t, "service", event.Kind)
		assert.Equal(t, "watch-test", event.ID)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for delete event")
	}

	// Test context cancellation
	cancel()
	
	// Channel should be closed
	select {
	case _, ok := <-events:
		assert.False(t, ok, "Channel should be closed after context cancellation")
	case <-time.After(1 * time.Second):
		t.Fatal("Channel not closed after context cancellation")
	}
}

func testConcurrentOperations(t *testing.T, setupFunc func(*testing.T) types.Storage) {
	s := setupFunc(t)
	if s == nil {
		return // Storage setup was skipped
	}
	defer s.Close()

	ctx := context.Background()

	// Test concurrent service creation
	var wg sync.WaitGroup
	numGoroutines := 10
	numServicesPerGoroutine := 5

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < numServicesPerGoroutine; j++ {
				service := &types.Service{
					ID:        fmt.Sprintf("service-%d-%d", goroutineID, j),
					Name:      fmt.Sprintf("Service %d-%d", goroutineID, j),
					Endpoints: []string{fmt.Sprintf("http://backend-%d-%d:8080", goroutineID, j)},
					Active:    true,
				}
				err := s.CreateService(ctx, service)
				assert.NoError(t, err)
			}
		}(i)
	}

	wg.Wait()

	// Verify all services were created
	services, err := s.ListServices(ctx)
	assert.NoError(t, err)
	assert.Len(t, services, numGoroutines*numServicesPerGoroutine)

	// Test concurrent reads and updates
	wg = sync.WaitGroup{}
	for i := 0; i < numGoroutines; i++ {
		wg.Add(2)
		
		// Reader goroutine
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < numServicesPerGoroutine; j++ {
				serviceID := fmt.Sprintf("service-%d-%d", goroutineID, j)
				service, err := s.GetService(ctx, serviceID)
				assert.NoError(t, err)
				assert.NotNil(t, service)
			}
		}(i)

		// Updater goroutine
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < numServicesPerGoroutine; j++ {
				serviceID := fmt.Sprintf("service-%d-%d", goroutineID, j)
				service, err := s.GetService(ctx, serviceID)
				if err == nil {
					service.Name = fmt.Sprintf("Updated Service %d-%d", goroutineID, j)
					err = s.UpdateService(ctx, service)
					assert.NoError(t, err)
				}
			}
		}(i)
	}

	wg.Wait()

	// Test concurrent operations across different entity types
	wg = sync.WaitGroup{}
	wg.Add(4)

	// Create users concurrently
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			user := &types.User{
				ID:           fmt.Sprintf("user-%d", i),
				Username:     fmt.Sprintf("user%d", i),
				PasswordHash: "$2a$10$YMF3qRxtVk2u7qCYVayhOe7dN0jp0VQ5k5uF0x5xJxUJB1Y0xJzDm",
				Email:        fmt.Sprintf("user%d@example.com", i),
				Active:       true,
			}
			s.CreateUser(ctx, user)
		}
	}()

	// Create routes concurrently
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			route := &types.Route{
				ID:        fmt.Sprintf("route-%d", i),
				Priority:  i * 10,
				Host:      fmt.Sprintf("host%d.com", i),
				ServiceID: fmt.Sprintf("service-0-%d", i%numServicesPerGoroutine),
			}
			s.CreateRoute(ctx, route)
		}
	}()

	// List operations concurrently
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			s.ListServices(ctx)
			s.ListRoutes(ctx)
			s.ListUsers(ctx)
		}
	}()

	// Watch operations concurrently
	go func() {
		defer wg.Done()
		watchCtx, watchCancel := context.WithTimeout(ctx, 2*time.Second)
		defer watchCancel()
		events := s.Watch(watchCtx)
		for range events {
			// Just consume events
		}
	}()

	wg.Wait()

	// Final verification
	services, _ = s.ListServices(ctx)
	assert.GreaterOrEqual(t, len(services), numGoroutines*numServicesPerGoroutine)

	users, _ := s.ListUsers(ctx)
	assert.GreaterOrEqual(t, len(users), 5) // At least some users created

	routes, _ := s.ListRoutes(ctx)
	assert.GreaterOrEqual(t, len(routes), 5) // At least some routes created
}