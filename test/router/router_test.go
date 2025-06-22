package router_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"discobox/internal/router"
	"discobox/internal/storage"
	"discobox/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testLogger is a simple logger implementation for tests
type testLogger struct{}

func (l *testLogger) Debug(msg string, fields ...interface{}) {}
func (l *testLogger) Info(msg string, fields ...interface{})  {}
func (l *testLogger) Warn(msg string, fields ...interface{})  {}
func (l *testLogger) Error(msg string, fields ...interface{}) {}
func (l *testLogger) With(fields ...interface{}) types.Logger { return l }

func TestRouterBasicRouting(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemory()
	
	// Setup all data before creating router
	service1 := &types.Service{
		ID:        "service1",
		Name:      "Service 1",
		Endpoints: []string{"http://backend1:8080"},
		Active:    true,
	}
	service2 := &types.Service{
		ID:        "service2", 
		Name:      "Service 2",
		Endpoints: []string{"http://backend2:8080"},
		Active:    true,
	}
	
	err := store.CreateService(ctx, service1)
	require.NoError(t, err)
	err = store.CreateService(ctx, service2)
	require.NoError(t, err)

	routes := []*types.Route{
		{
			ID:         "route1",
			Priority:   100,
			Host:       "api.example.com",
			PathPrefix: "/v1",
			ServiceID:  "service1",
		},
		{
			ID:         "route2",
			Priority:   90,
			Host:       "api.example.com",
			PathPrefix: "/v2",
			ServiceID:  "service2",
		},
		{
			ID:        "route3",
			Priority:  80,
			Host:      "www.example.com",
			ServiceID: "service1",
		},
		{
			ID:         "route4",
			Priority:   70,
			PathPrefix: "/admin",
			ServiceID:  "service2",
		},
	}

	for _, route := range routes {
		err = store.CreateRoute(ctx, route)
		require.NoError(t, err)
	}

	// Create router after all data is loaded
	r := router.NewRouter(store, &testLogger{})

	// Test route matching
	tests := []struct {
		name            string
		host            string
		path            string
		expectedService string
	}{
		{
			name:            "Match host and path prefix v1",
			host:            "api.example.com",
			path:            "/v1/users",
			expectedService: "service1",
		},
		{
			name:            "Match host and path prefix v2",
			host:            "api.example.com",
			path:            "/v2/products",
			expectedService: "service2",
		},
		{
			name:            "Match host only",
			host:            "www.example.com",
			path:            "/anything",
			expectedService: "service1",
		},
		{
			name:            "Match path prefix only",
			host:            "any.host.com",
			path:            "/admin/settings",
			expectedService: "service2",
		},
		{
			name:            "No match",
			host:            "unknown.com",
			path:            "/unknown",
			expectedService: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://"+tt.host+tt.path, nil)
			route, err := r.Match(req)
			
			if tt.expectedService == "" {
				assert.Error(t, err)
				assert.Nil(t, route)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, route)
				assert.Equal(t, tt.expectedService, route.ServiceID)
			}
		})
	}
}

func TestRouterWildcardHost(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemory()

	// Setup data
	service := &types.Service{
		ID:        "wildcard-service",
		Name:      "Wildcard Service",
		Endpoints: []string{"http://backend:8080"},
		Active:    true,
	}
	err := store.CreateService(ctx, service)
	require.NoError(t, err)

	route := &types.Route{
		ID:        "wildcard-route",
		Priority:  100,
		Host:      "*.example.com",
		ServiceID: "wildcard-service",
	}
	err = store.CreateRoute(ctx, route)
	require.NoError(t, err)

	// Create router
	r := router.NewRouter(store, &testLogger{})

	// Test wildcard matching
	tests := []struct {
		name    string
		host    string
		matches bool
	}{
		{"Subdomain match", "api.example.com", true},
		{"Deep subdomain match", "v1.api.example.com", true},
		{"No subdomain", "example.com", false},
		{"Different domain", "api.different.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://"+tt.host+"/test", nil)
			route, err := r.Match(req)
			
			if tt.matches {
				assert.NoError(t, err)
				require.NotNil(t, route)
				assert.Equal(t, "wildcard-service", route.ServiceID)
			} else {
				assert.Error(t, err)
				assert.Nil(t, route)
			}
		})
	}
}

func TestRouterPathRegex(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemory()

	// Setup data
	service := &types.Service{
		ID:        "regex-service",
		Name:      "Regex Service",
		Endpoints: []string{"http://backend:8080"},
		Active:    true,
	}
	err := store.CreateService(ctx, service)
	require.NoError(t, err)

	route := &types.Route{
		ID:        "regex-route",
		Priority:  100,
		PathRegex: "^/api/v[0-9]+/.*",
		ServiceID: "regex-service",
	}
	err = store.CreateRoute(ctx, route)
	require.NoError(t, err)

	// Create router
	r := router.NewRouter(store, &testLogger{})

	// Test regex matching
	tests := []struct {
		name    string
		path    string
		matches bool
	}{
		{"Version 1", "/api/v1/users", true},
		{"Version 2", "/api/v2/products", true},
		{"Version 10", "/api/v10/orders", true},
		{"No version", "/api/users", false},
		{"Wrong prefix", "/v1/users", false},
		{"Root path", "/", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://example.com"+tt.path, nil)
			route, err := r.Match(req)
			
			if tt.matches {
				assert.NoError(t, err)
				require.NotNil(t, route)
				assert.Equal(t, "regex-service", route.ServiceID)
			} else {
				assert.Error(t, err)
				assert.Nil(t, route)
			}
		})
	}
}

func TestRouterHeaderMatching(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemory()

	// Setup data
	service1 := &types.Service{
		ID:        "mobile-service",
		Name:      "Mobile Service",
		Endpoints: []string{"http://mobile-backend:8080"},
		Active:    true,
	}
	service2 := &types.Service{
		ID:        "desktop-service",
		Name:      "Desktop Service",
		Endpoints: []string{"http://desktop-backend:8080"},
		Active:    true,
	}
	
	err := store.CreateService(ctx, service1)
	require.NoError(t, err)
	err = store.CreateService(ctx, service2)
	require.NoError(t, err)

	routes := []*types.Route{
		{
			ID:       "mobile-route",
			Priority: 100,
			Headers: map[string]string{
				"X-Platform": "mobile",
			},
			ServiceID: "mobile-service",
		},
		{
			ID:       "desktop-route",
			Priority: 90,
			Headers: map[string]string{
				"X-Platform": "desktop",
			},
			ServiceID: "desktop-service",
		},
		{
			ID:       "api-v2-route",
			Priority: 80,
			Headers: map[string]string{
				"X-API-Version": "2",
			},
			ServiceID: "desktop-service",
		},
	}

	for _, route := range routes {
		err = store.CreateRoute(ctx, route)
		require.NoError(t, err)
	}

	// Create router
	r := router.NewRouter(store, &testLogger{})

	// Test header matching
	tests := []struct {
		name            string
		headers         map[string]string
		expectedService string
	}{
		{
			name: "Mobile platform",
			headers: map[string]string{
				"X-Platform": "mobile",
			},
			expectedService: "mobile-service",
		},
		{
			name: "Desktop platform",
			headers: map[string]string{
				"X-Platform": "desktop",
			},
			expectedService: "desktop-service",
		},
		{
			name: "API version 2",
			headers: map[string]string{
				"X-API-Version": "2",
			},
			expectedService: "desktop-service",
		},
		{
			name: "No matching headers",
			headers: map[string]string{
				"X-Platform": "tablet",
			},
			expectedService: "",
		},
		{
			name:            "No headers",
			headers:         map[string]string{},
			expectedService: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://example.com/test", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			
			route, err := r.Match(req)
			
			if tt.expectedService == "" {
				assert.Error(t, err)
				assert.Nil(t, route)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, route)
				assert.Equal(t, tt.expectedService, route.ServiceID)
			}
		})
	}
}

func TestRouterPriorityOrdering(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemory()

	// Setup data
	service := &types.Service{
		ID:        "test-service",
		Name:      "Test Service",
		Endpoints: []string{"http://backend:8080"},
		Active:    true,
	}
	err := store.CreateService(ctx, service)
	require.NoError(t, err)

	routes := []*types.Route{
		{
			ID:         "low-priority",
			Priority:   10,
			PathPrefix: "/",
			ServiceID:  "test-service",
			Metadata:   map[string]interface{}{"name": "catch-all"},
		},
		{
			ID:         "medium-priority",
			Priority:   50,
			PathPrefix: "/api",
			ServiceID:  "test-service",
			Metadata:   map[string]interface{}{"name": "api"},
		},
		{
			ID:         "high-priority",
			Priority:   100,
			PathPrefix: "/api/v1",
			ServiceID:  "test-service",
			Metadata:   map[string]interface{}{"name": "api-v1"},
		},
	}

	// Create routes in random order
	err = store.CreateRoute(ctx, routes[1])
	require.NoError(t, err)
	err = store.CreateRoute(ctx, routes[0])
	require.NoError(t, err)
	err = store.CreateRoute(ctx, routes[2])
	require.NoError(t, err)

	// Create router
	r := router.NewRouter(store, &testLogger{})

	// Test priority ordering
	tests := []struct {
		name         string
		path         string
		expectedName string
	}{
		{
			name:         "Most specific match",
			path:         "/api/v1/users",
			expectedName: "api-v1",
		},
		{
			name:         "Medium specific match",
			path:         "/api/v2/users",
			expectedName: "api",
		},
		{
			name:         "Catch all match",
			path:         "/health",
			expectedName: "catch-all",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://example.com"+tt.path, nil)
			route, err := r.Match(req)
			
			assert.NoError(t, err)
			require.NotNil(t, route)
			assert.Equal(t, tt.expectedName, route.Metadata["name"])
		})
	}
}

func TestRouterDynamicUpdates(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemory()

	// Initial setup
	service := &types.Service{
		ID:        "dynamic-service",
		Name:      "Dynamic Service",
		Endpoints: []string{"http://backend:8080"},
		Active:    true,
	}
	err := store.CreateService(ctx, service)
	require.NoError(t, err)

	route := &types.Route{
		ID:         "dynamic-route",
		Priority:   100,
		PathPrefix: "/test",
		ServiceID:  "dynamic-service",
	}
	err = store.CreateRoute(ctx, route)
	require.NoError(t, err)

	// Create router
	r := router.NewRouter(store, &testLogger{})

	// Give router time to fully initialize watch goroutine
	// The router now has a built-in 10ms delay before starting watch
	time.Sleep(20 * time.Millisecond)

	// Verify initial routing
	req := httptest.NewRequest("GET", "http://example.com/test/path", nil)
	matchedRoute, err := r.Match(req)
	assert.NoError(t, err)
	require.NotNil(t, matchedRoute)
	assert.Equal(t, "dynamic-route", matchedRoute.ID)

	// Update route path - fetch from storage first to get complete object
	routeToUpdate, err := store.GetRoute(ctx, "dynamic-route")
	require.NoError(t, err)
	routeToUpdate.PathPrefix = "/updated"
	err = store.UpdateRoute(ctx, routeToUpdate)
	require.NoError(t, err)

	// Give router time to process updates
	// Wait up to 1 second for the update to propagate
	updated := false
	for i := 0; i < 10; i++ {
		time.Sleep(100 * time.Millisecond)
		req := httptest.NewRequest("GET", "http://example.com/updated/path", nil)
		if route, _ := r.Match(req); route != nil {
			updated = true
			break
		}
	}
	require.True(t, updated, "Router did not update within 1 second")

	// Verify old path no longer matches
	req = httptest.NewRequest("GET", "http://example.com/test/path", nil)
	matchedRoute, err = r.Match(req)
	assert.Error(t, err)
	assert.Nil(t, matchedRoute)

	// Verify new path matches
	req = httptest.NewRequest("GET", "http://example.com/updated/path", nil)
	matchedRoute, err = r.Match(req)
	assert.NoError(t, err)
	require.NotNil(t, matchedRoute)
	assert.Equal(t, "dynamic-route", matchedRoute.ID)

	// Delete route
	err = store.DeleteRoute(ctx, "dynamic-route")
	require.NoError(t, err)

	// Give router time to process updates
	time.Sleep(200 * time.Millisecond)

	// Verify route is removed
	req = httptest.NewRequest("GET", "http://example.com/updated/path", nil)
	matchedRoute, err = r.Match(req)
	assert.Error(t, err)
	assert.Nil(t, matchedRoute)
}

func TestRouterInactiveServices(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemory()

	// Setup with inactive service
	service := &types.Service{
		ID:        "inactive-service",
		Name:      "Inactive Service",
		Endpoints: []string{"http://backend:8080"},
		Active:    false,
	}
	err := store.CreateService(ctx, service)
	require.NoError(t, err)

	route := &types.Route{
		ID:         "inactive-route",
		Priority:   100,
		PathPrefix: "/inactive",
		ServiceID:  "inactive-service",
	}
	err = store.CreateRoute(ctx, route)
	require.NoError(t, err)

	// Create router
	r := router.NewRouter(store, &testLogger{})

	// Router should still match the route (service active check is done by proxy)
	req := httptest.NewRequest("GET", "http://example.com/inactive/test", nil)
	matchedRoute, err := r.Match(req)
	assert.NoError(t, err)
	require.NotNil(t, matchedRoute)
	assert.Equal(t, "inactive-route", matchedRoute.ID)

	// Activate service
	service.Active = true
	err = store.UpdateService(ctx, service)
	require.NoError(t, err)

	// Give router time to process updates if it watches services
	time.Sleep(150 * time.Millisecond)

	// Route should still match
	matchedRoute, err = r.Match(req)
	assert.NoError(t, err)
	require.NotNil(t, matchedRoute)
	assert.Equal(t, "inactive-route", matchedRoute.ID)
}

func TestRouterConcurrentRouting(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemory()

	// Create multiple services and routes
	numServices := 10
	numRoutesPerService := 5

	for i := 0; i < numServices; i++ {
		service := &types.Service{
			ID:        fmt.Sprintf("service-%d", i),
			Name:      fmt.Sprintf("Service %d", i),
			Endpoints: []string{fmt.Sprintf("http://backend-%d:8080", i)},
			Active:    true,
		}
		err := store.CreateService(ctx, service)
		require.NoError(t, err)

		for j := 0; j < numRoutesPerService; j++ {
			route := &types.Route{
				ID:         fmt.Sprintf("route-%d-%d", i, j),
				Priority:   100 - j,
				PathPrefix: fmt.Sprintf("/service%d/path%d", i, j),
				ServiceID:  service.ID,
			}
			err = store.CreateRoute(ctx, route)
			require.NoError(t, err)
		}
	}

	// Create router
	r := router.NewRouter(store, &testLogger{})

	// Concurrent routing test
	var wg sync.WaitGroup
	numGoroutines := 100
	numRequestsPerGoroutine := 100

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			
			for reqNum := 0; reqNum < numRequestsPerGoroutine; reqNum++ {
				// Random service and path
				serviceID := goroutineID % numServices
				pathID := reqNum % numRoutesPerService
				path := fmt.Sprintf("/service%d/path%d/test", serviceID, pathID)
				
				req := httptest.NewRequest("GET", "http://example.com"+path, nil)
				route, err := r.Match(req)
				
				assert.NoError(t, err)
				assert.NotNil(t, route)
				assert.Equal(t, fmt.Sprintf("service-%d", serviceID), route.ServiceID)
			}
		}(g)
	}

	wg.Wait()
}

func TestRouterInvalidRegex(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemory()

	// Setup data
	service := &types.Service{
		ID:        "test-service",
		Name:      "Test Service",
		Endpoints: []string{"http://backend:8080"},
		Active:    true,
	}
	err := store.CreateService(ctx, service)
	require.NoError(t, err)

	route := &types.Route{
		ID:        "invalid-regex-route",
		Priority:  100,
		PathRegex: "[invalid(regex",
		ServiceID: "test-service",
	}
	err = store.CreateRoute(ctx, route)
	require.NoError(t, err)

	// Create router (it will log error about invalid regex)
	r := router.NewRouter(store, &testLogger{})

	// Route with invalid regex shouldn't match
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	matchedRoute, err := r.Match(req)
	assert.Error(t, err)
	assert.Nil(t, matchedRoute)
}

func TestRouterComplexMatching(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemory()

	// Setup data
	service := &types.Service{
		ID:        "complex-service",
		Name:      "Complex Service",
		Endpoints: []string{"http://backend:8080"},
		Active:    true,
	}
	err := store.CreateService(ctx, service)
	require.NoError(t, err)

	route := &types.Route{
		ID:         "complex-route",
		Priority:   100,
		Host:       "*.example.com",
		PathPrefix: "/api",
		PathRegex:  "^/api/v[0-9]+/.*",
		Headers: map[string]string{
			"X-API-Key": "secret",
		},
		ServiceID: "complex-service",
	}
	err = store.CreateRoute(ctx, route)
	require.NoError(t, err)

	// Create router
	r := router.NewRouter(store, &testLogger{})

	// Test various combinations
	tests := []struct {
		name    string
		host    string
		path    string
		headers map[string]string
		matches bool
	}{
		{
			name: "All criteria match",
			host: "api.example.com",
			path: "/api/v1/users",
			headers: map[string]string{
				"X-API-Key": "secret",
			},
			matches: true,
		},
		{
			name: "Wrong host",
			host: "api.different.com",
			path: "/api/v1/users",
			headers: map[string]string{
				"X-API-Key": "secret",
			},
			matches: false,
		},
		{
			name: "Path doesn't match regex",
			host: "api.example.com",
			path: "/api/users",
			headers: map[string]string{
				"X-API-Key": "secret",
			},
			matches: false,
		},
		{
			name: "Missing header",
			host: "api.example.com",
			path: "/api/v1/users",
			headers: map[string]string{},
			matches: false,
		},
		{
			name: "Wrong header value",
			host: "api.example.com",
			path: "/api/v1/users",
			headers: map[string]string{
				"X-API-Key": "wrong",
			},
			matches: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://"+tt.host+tt.path, nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			
			route, err := r.Match(req)
			
			if tt.matches {
				assert.NoError(t, err)
				require.NotNil(t, route)
				assert.Equal(t, "complex-service", route.ServiceID)
			} else {
				assert.Error(t, err)
				assert.Nil(t, route)
			}
		})
	}
}

func TestRouterPerformance(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemory()

	// Setup data
	service := &types.Service{
		ID:        "perf-service",
		Name:      "Performance Service",
		Endpoints: []string{"http://backend:8080"},
		Active:    true,
	}
	err := store.CreateService(ctx, service)
	require.NoError(t, err)

	// Create many routes
	numRoutes := 1000
	for i := 0; i < numRoutes; i++ {
		route := &types.Route{
			ID:         fmt.Sprintf("route-%d", i),
			Priority:   numRoutes - i, // Reverse priority
			PathPrefix: fmt.Sprintf("/path%d", i),
			ServiceID:  "perf-service",
		}
		err = store.CreateRoute(ctx, route)
		require.NoError(t, err)
	}

	// Create router
	r := router.NewRouter(store, &testLogger{})

	// Measure routing performance
	iterations := 10000
	start := time.Now()

	for i := 0; i < iterations; i++ {
		// Test different paths
		path := fmt.Sprintf("/path%d/test", i%numRoutes)
		req := httptest.NewRequest("GET", "http://example.com"+path, nil)
		route, err := r.Match(req)
		
		assert.NoError(t, err)
		assert.NotNil(t, route)
	}

	elapsed := time.Since(start)
	perRequest := elapsed / time.Duration(iterations)
	
	t.Logf("Routing performance: %d routes, %d iterations, %v total, %v per request",
		numRoutes, iterations, elapsed, perRequest)
	
	// Ensure reasonable performance (< 1ms per request)
	assert.Less(t, perRequest, time.Millisecond)
}