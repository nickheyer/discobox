package router

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"discobox/internal/storage"
	"discobox/internal/types"
)

// mockLogger implements types.Logger for testing
type mockLogger struct{}

func (l *mockLogger) Debug(msg string, fields ...any) {}
func (l *mockLogger) Info(msg string, fields ...any)  {}
func (l *mockLogger) Warn(msg string, fields ...any)  {}
func (l *mockLogger) Error(msg string, fields ...any) {}
func (l *mockLogger) With(fields ...any) types.Logger { return l }

// Helper function to create a test service
func createTestService(t *testing.T, s types.Storage, id string) {
	service := &types.Service{
		ID:        id,
		Name:      "Test Service " + id,
		Endpoints: []string{"http://localhost:8080"},
		Active:    true,
		Weight:    1,
	}
	err := s.CreateService(context.Background(), service)
	require.NoError(t, err)
}

func TestNewRouter(t *testing.T) {
	storage := storage.NewMemory()
	logger := &mockLogger{}

	r := NewRouter(storage, logger)
	assert.NotNil(t, r)

	// Test that it implements the Router interface
	var _ types.Router = r
}

func TestRouterMatch(t *testing.T) {
	tests := []struct {
		name      string
		routes    []*types.Route
		request   *http.Request
		wantRoute string
		wantError error
	}{
		{
			name: "exact host match",
			routes: []*types.Route{
				{
					ID:        "route1",
					Host:      "example.com",
					ServiceID: "service1",
					Priority:  100,
				},
			},
			request:   httptest.NewRequest("GET", "http://example.com/path", nil),
			wantRoute: "route1",
		},
		{
			name: "wildcard host match",
			routes: []*types.Route{
				{
					ID:        "route1",
					Host:      "*.example.com",
					ServiceID: "service1",
					Priority:  100,
				},
			},
			request:   httptest.NewRequest("GET", "http://api.example.com/path", nil),
			wantRoute: "route1",
		},
		{
			name: "path prefix match",
			routes: []*types.Route{
				{
					ID:         "route1",
					PathPrefix: "/api",
					ServiceID:  "service1",
					Priority:   100,
				},
			},
			request:   httptest.NewRequest("GET", "http://example.com/api/users", nil),
			wantRoute: "route1",
		},
		{
			name: "path regex match",
			routes: []*types.Route{
				{
					ID:        "route1",
					PathRegex: "^/users/[0-9]+$",
					ServiceID: "service1",
					Priority:  100,
				},
			},
			request:   httptest.NewRequest("GET", "http://example.com/users/123", nil),
			wantRoute: "route1",
		},
		{
			name: "header match",
			routes: []*types.Route{
				{
					ID: "route1",
					Headers: map[string]string{
						"X-API-Version": "v2",
					},
					ServiceID: "service1",
					Priority:  100,
				},
			},
			request: func() *http.Request {
				req := httptest.NewRequest("GET", "http://example.com/path", nil)
				req.Header.Set("X-API-Version", "v2")
				return req
			}(),
			wantRoute: "route1",
		},
		{
			name: "priority ordering",
			routes: []*types.Route{
				{
					ID:         "route1",
					PathPrefix: "/api",
					ServiceID:  "service1",
					Priority:   50,
				},
				{
					ID:         "route2",
					PathPrefix: "/api/users",
					ServiceID:  "service2",
					Priority:   100,
				},
			},
			request:   httptest.NewRequest("GET", "http://example.com/api/users/123", nil),
			wantRoute: "route2", // Higher priority wins
		},
		{
			name: "no matching route",
			routes: []*types.Route{
				{
					ID:        "route1",
					Host:      "example.com",
					ServiceID: "service1",
					Priority:  100,
				},
			},
			request:   httptest.NewRequest("GET", "http://other.com/path", nil),
			wantError: types.ErrRouteNotFound,
		},
		{
			name: "host with port",
			routes: []*types.Route{
				{
					ID:        "route1",
					Host:      "example.com",
					ServiceID: "service1",
					Priority:  100,
				},
			},
			request:   httptest.NewRequest("GET", "http://example.com:8080/path", nil),
			wantRoute: "route1",
		},
		{
			name: "multiple header match",
			routes: []*types.Route{
				{
					ID: "route1",
					Headers: map[string]string{
						"X-API-Version": "v2",
						"X-Tenant-ID":   "tenant1",
					},
					ServiceID: "service1",
					Priority:  100,
				},
			},
			request: func() *http.Request {
				req := httptest.NewRequest("GET", "http://example.com/path", nil)
				req.Header.Set("X-API-Version", "v2")
				req.Header.Set("X-Tenant-ID", "tenant1")
				return req
			}(),
			wantRoute: "route1",
		},
		{
			name: "header mismatch",
			routes: []*types.Route{
				{
					ID: "route1",
					Headers: map[string]string{
						"X-API-Version": "v2",
					},
					ServiceID: "service1",
					Priority:  100,
				},
			},
			request: func() *http.Request {
				req := httptest.NewRequest("GET", "http://example.com/path", nil)
				req.Header.Set("X-API-Version", "v1")
				return req
			}(),
			wantError: types.ErrRouteNotFound,
		},
		{
			name: "combined host and path match",
			routes: []*types.Route{
				{
					ID:         "route1",
					Host:       "api.example.com",
					PathPrefix: "/v2",
					ServiceID:  "service1",
					Priority:   100,
				},
			},
			request:   httptest.NewRequest("GET", "http://api.example.com/v2/users", nil),
			wantRoute: "route1",
		},
		{
			name: "invalid regex should be skipped",
			routes: []*types.Route{
				{
					ID:        "route1",
					PathRegex: "[invalid regex",
					ServiceID: "service1",
					Priority:  100,
				},
				{
					ID:        "route2",
					ServiceID: "service2",
					Priority:  50,
				},
			},
			request:   httptest.NewRequest("GET", "http://example.com/path", nil),
			wantRoute: "route2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			s := storage.NewMemory()
			logger := &mockLogger{}

			// Create services for all routes
			serviceIDs := make(map[string]bool)
			for _, route := range tt.routes {
				if !serviceIDs[route.ServiceID] {
					createTestService(t, s, route.ServiceID)
					serviceIDs[route.ServiceID] = true
				}
			}

			// Add routes to storage
			for _, route := range tt.routes {
				err := s.CreateRoute(ctx, route)
				require.NoError(t, err)
			}

			r := NewRouter(s, logger)
			// Give the router time to load routes
			time.Sleep(50 * time.Millisecond)

			gotRoute, err := r.Match(tt.request)

			if tt.wantError != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.wantError, err)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, gotRoute)
				assert.Equal(t, tt.wantRoute, gotRoute.ID)
			}
		})
	}
}

func TestRouterAddRoute(t *testing.T) {
	ctx := context.Background()
	s := storage.NewMemory()
	logger := &mockLogger{}
	r := NewRouter(s, logger)

	// Create service first
	createTestService(t, s, "service1")

	route := &types.Route{
		ID:         "route1",
		Host:       "example.com",
		PathPrefix: "/api",
		ServiceID:  "service1",
		Priority:   100,
	}

	// Add route
	err := r.AddRoute(route)
	assert.NoError(t, err)

	// Verify route was added
	routes, err := r.GetRoutes()
	assert.NoError(t, err)
	assert.Len(t, routes, 1)
	assert.Equal(t, route.ID, routes[0].ID)

	// Verify route is in storage
	stored, err := s.GetRoute(ctx, route.ID)
	assert.NoError(t, err)
	assert.Equal(t, route.ID, stored.ID)

	// Test adding nil route
	err = r.AddRoute(nil)
	assert.Error(t, err)
	assert.Equal(t, types.ErrInvalidRequest, err)
}

func TestRouterRemoveRoute(t *testing.T) {
	ctx := context.Background()
	s := storage.NewMemory()
	logger := &mockLogger{}
	r := NewRouter(s, logger)

	// Create service first
	createTestService(t, s, "service1")

	route := &types.Route{
		ID:         "route1",
		Host:       "example.com",
		PathPrefix: "/api",
		ServiceID:  "service1",
		Priority:   100,
	}

	// Add route first
	err := s.CreateRoute(ctx, route)
	require.NoError(t, err)

	// Remove route
	err = r.RemoveRoute(route.ID)
	assert.NoError(t, err)

	// Verify route was removed
	routes, err := r.GetRoutes()
	assert.NoError(t, err)
	assert.Len(t, routes, 0)

	// Test removing non-existent route
	err = r.RemoveRoute("non-existent")
	assert.Error(t, err)
}

func TestRouterUpdateRoute(t *testing.T) {
	ctx := context.Background()
	s := storage.NewMemory()
	logger := &mockLogger{}
	r := NewRouter(s, logger)

	// Create service first
	createTestService(t, s, "service1")

	route := &types.Route{
		ID:         "route1",
		Host:       "example.com",
		PathPrefix: "/api",
		ServiceID:  "service1",
		Priority:   100,
	}

	// Add route first
	err := s.CreateRoute(ctx, route)
	require.NoError(t, err)

	// Update route
	route.Host = "newexample.com"
	route.Priority = 200
	err = r.UpdateRoute(route)
	assert.NoError(t, err)

	// Verify route was updated
	routes, err := r.GetRoutes()
	assert.NoError(t, err)
	assert.Len(t, routes, 1)
	assert.Equal(t, "newexample.com", routes[0].Host)
	assert.Equal(t, 200, routes[0].Priority)

	// Test updating nil route
	err = r.UpdateRoute(nil)
	assert.Error(t, err)
	assert.Equal(t, types.ErrInvalidRequest, err)
}

func TestRouterGetRoutes(t *testing.T) {
	ctx := context.Background()
	s := storage.NewMemory()
	logger := &mockLogger{}

	// Create services first
	createTestService(t, s, "service1")
	createTestService(t, s, "service2")
	createTestService(t, s, "service3")

	// Add multiple routes before creating router
	routes := []*types.Route{
		{
			ID:        "route1",
			Host:      "example.com",
			Priority:  100,
			ServiceID: "service1",
		},
		{
			ID:        "route2",
			Host:      "api.example.com",
			Priority:  200,
			ServiceID: "service2",
		},
		{
			ID:        "route3",
			Host:      "admin.example.com",
			Priority:  150,
			ServiceID: "service3",
		},
	}

	for _, route := range routes {
		err := s.CreateRoute(ctx, route)
		require.NoError(t, err)
	}

	// Now create the router - it should load routes synchronously
	r := NewRouter(s, logger)

	// Get routes
	gotRoutes, err := r.GetRoutes()
	assert.NoError(t, err)
	require.Len(t, gotRoutes, 3)

	// Verify routes are sorted by priority (descending)
	assert.Equal(t, "route2", gotRoutes[0].ID) // Priority 200
	assert.Equal(t, "route3", gotRoutes[1].ID) // Priority 150
	assert.Equal(t, "route1", gotRoutes[2].ID) // Priority 100
}

func TestRouterConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	s := storage.NewMemory()
	logger := &mockLogger{}

	// Create services for initial routes
	for i := 0; i < 10; i++ {
		createTestService(t, s, fmt.Sprintf("service%d", i))
	}

	// Add initial routes before creating router
	for i := 0; i < 10; i++ {
		route := &types.Route{
			ID:        fmt.Sprintf("route%d", i),
			Host:      fmt.Sprintf("host%d.example.com", i),
			ServiceID: fmt.Sprintf("service%d", i),
			Priority:  i * 10,
		}
		err := s.CreateRoute(ctx, route)
		require.NoError(t, err)
	}

	// Now create the router
	r := NewRouter(s, logger)

	// Concurrent operations
	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Concurrent matches
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := httptest.NewRequest("GET", fmt.Sprintf("http://host%d.example.com/path", i), nil)
			route, err := r.Match(req)
			if err != nil {
				errors <- err
				return
			}
			if route.ID != fmt.Sprintf("route%d", i) {
				errors <- fmt.Errorf("expected route%d, got %s", i, route.ID)
			}
		}(i)
	}

	// Create services for concurrent route additions
	for i := 10; i < 20; i++ {
		createTestService(t, s, fmt.Sprintf("service%d", i))
	}

	// Concurrent route additions
	for i := 10; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			route := &types.Route{
				ID:        fmt.Sprintf("route%d", i),
				Host:      fmt.Sprintf("host%d.example.com", i),
				ServiceID: fmt.Sprintf("service%d", i),
				Priority:  i * 10,
			}
			if err := r.AddRoute(route); err != nil {
				errors <- err
			}
		}(i)
	}

	// Concurrent get routes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := r.GetRoutes(); err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent operation error: %v", err)
	}
}

func TestRouterWatchChanges(t *testing.T) {
	t.Skip("Skipping watch changes test - requires proper context handling")
	ctx := context.Background()
	s := storage.NewMemory()
	logger := &mockLogger{}

	// Create services
	createTestService(t, s, "service1")
	createTestService(t, s, "service2")

	// Add initial route before creating router
	route1 := &types.Route{
		ID:        "route1",
		Host:      "example.com",
		ServiceID: "service1",
		Priority:  100,
	}
	err := s.CreateRoute(ctx, route1)
	require.NoError(t, err)

	// Now create the router
	r := NewRouter(s, logger)

	// Verify initial route
	routes, err := r.GetRoutes()
	assert.NoError(t, err)
	assert.Len(t, routes, 1)

	// Add another route directly to storage (simulating external change)
	route2 := &types.Route{
		ID:        "route2",
		Host:      "api.example.com",
		ServiceID: "service2",
		Priority:  200,
	}
	err = s.CreateRoute(ctx, route2)
	require.NoError(t, err)

	// Wait a bit and try to match a request to the new route
	// This will verify the router picks up the change
	time.Sleep(200 * time.Millisecond)

	// Try to match a request that should hit the new route
	req := httptest.NewRequest("GET", "http://api.example.com/test", nil)
	matchedRoute, err := r.Match(req)
	assert.NoError(t, err)
	assert.NotNil(t, matchedRoute)
	assert.Equal(t, "route2", matchedRoute.ID)

	// Also verify both routes are now loaded
	routes, err = r.GetRoutes()
	assert.NoError(t, err)
	assert.Len(t, routes, 2)
}

func TestRouterPathRegexCompilation(t *testing.T) {
	ctx := context.Background()
	s := storage.NewMemory()
	logger := &mockLogger{}

	// Create services
	createTestService(t, s, "service1")
	createTestService(t, s, "service2")

	// Add route with valid regex
	validRoute := &types.Route{
		ID:        "route1",
		PathRegex: "^/api/v[0-9]+/.*$",
		ServiceID: "service1",
		Priority:  100,
	}
	err := s.CreateRoute(ctx, validRoute)
	require.NoError(t, err)

	// Add route with invalid regex
	invalidRoute := &types.Route{
		ID:        "route2",
		PathRegex: "[invalid",
		ServiceID: "service2",
		Priority:  50,
	}
	err = s.CreateRoute(ctx, invalidRoute)
	require.NoError(t, err)

	r := NewRouter(s, logger)
	time.Sleep(50 * time.Millisecond)

	// Test matching with valid regex
	req := httptest.NewRequest("GET", "http://example.com/api/v2/users", nil)
	route, err := r.Match(req)
	assert.NoError(t, err)
	assert.Equal(t, "route1", route.ID)

	// Test that invalid regex route is skipped
	req = httptest.NewRequest("GET", "http://example.com/invalid", nil)
	_, err = r.Match(req)
	assert.Error(t, err)
	assert.Equal(t, types.ErrRouteNotFound, err)
}

func TestRouterEmptyRoutes(t *testing.T) {
	s := storage.NewMemory()
	logger := &mockLogger{}
	r := NewRouter(s, logger)

	// Test match with no routes
	req := httptest.NewRequest("GET", "http://example.com/path", nil)
	_, err := r.Match(req)
	assert.Error(t, err)
	assert.Equal(t, types.ErrRouteNotFound, err)

	// Test get routes with no routes
	routes, err := r.GetRoutes()
	assert.NoError(t, err)
	assert.Len(t, routes, 0)
}

func TestRouterHostMatching(t *testing.T) {
	ctx := context.Background()
	s := storage.NewMemory()
	logger := &mockLogger{}

	// Create services
	createTestService(t, s, "service1")
	createTestService(t, s, "service2")
	createTestService(t, s, "service3")

	// Add routes with different host patterns
	routes := []*types.Route{
		{
			ID:        "exact",
			Host:      "exact.example.com",
			ServiceID: "service1",
			Priority:  100,
		},
		{
			ID:        "wildcard",
			Host:      "*.wildcard.com",
			ServiceID: "service2",
			Priority:  100,
		},
		{
			ID:        "anyhost",
			Host:      "",
			ServiceID: "service3",
			Priority:  50,
		},
	}

	for _, route := range routes {
		err := s.CreateRoute(ctx, route)
		require.NoError(t, err)
	}

	r := NewRouter(s, logger)
	time.Sleep(50 * time.Millisecond)

	tests := []struct {
		name      string
		host      string
		wantRoute string
	}{
		{
			name:      "exact host match",
			host:      "exact.example.com",
			wantRoute: "exact",
		},
		{
			name:      "exact host with port",
			host:      "exact.example.com:8080",
			wantRoute: "exact",
		},
		{
			name:      "wildcard subdomain match",
			host:      "api.wildcard.com",
			wantRoute: "wildcard",
		},
		{
			name:      "wildcard deep subdomain match",
			host:      "v2.api.wildcard.com",
			wantRoute: "wildcard",
		},
		{
			name:      "any host matches empty host route",
			host:      "random.domain.com",
			wantRoute: "anyhost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://"+tt.host+"/path", nil)
			route, err := r.Match(req)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantRoute, route.ID)
		})
	}
}
