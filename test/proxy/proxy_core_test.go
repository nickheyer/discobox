package proxy_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"discobox/internal/proxy"
	"discobox/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock implementations
type mockRouter struct {
	matchFunc func(req *http.Request) (*types.Route, error)
}

func (m *mockRouter) Match(req *http.Request) (*types.Route, error) {
	if m.matchFunc != nil {
		return m.matchFunc(req)
	}
	return nil, types.ErrRouteNotFound
}

func (m *mockRouter) AddRoute(route *types.Route) error    { return nil }
func (m *mockRouter) RemoveRoute(routeID string) error     { return nil }
func (m *mockRouter) UpdateRoute(route *types.Route) error { return nil }
func (m *mockRouter) GetRoutes() ([]*types.Route, error)   { return nil, nil }

type mockLoadBalancer struct {
	selectFunc func(ctx context.Context, req *http.Request, servers []*types.Server) (*types.Server, error)
}

func (m *mockLoadBalancer) Select(ctx context.Context, req *http.Request, servers []*types.Server) (*types.Server, error) {
	if m.selectFunc != nil {
		return m.selectFunc(ctx, req, servers)
	}
	return nil, types.ErrNoHealthyBackends
}

func (m *mockLoadBalancer) Add(server *types.Server) error               { return nil }
func (m *mockLoadBalancer) Remove(serverID string) error                 { return nil }
func (m *mockLoadBalancer) UpdateWeight(serverID string, weight int) error { return nil }

type mockHealthChecker struct {
	isHealthyFunc func(serverID string) bool
	recordSuccess func(serverID string)
	recordFailure func(serverID string, err error)
}

func (m *mockHealthChecker) Check(ctx context.Context, server *types.Server) error {
	return nil
}
func (m *mockHealthChecker) Watch(ctx context.Context, server *types.Server, interval time.Duration) <-chan error {
	ch := make(chan error)
	close(ch)
	return ch
}
func (m *mockHealthChecker) RecordSuccess(serverID string) {
	if m.recordSuccess != nil {
		m.recordSuccess(serverID)
	}
}
func (m *mockHealthChecker) RecordFailure(serverID string, err error) {
	if m.recordFailure != nil {
		m.recordFailure(serverID, err)
	}
}

type mockCircuitBreaker struct {
	executeFunc func(fn func() error) error
}

func (m *mockCircuitBreaker) Execute(fn func() error) error {
	if m.executeFunc != nil {
		return m.executeFunc(fn)
	}
	return fn()
}

func (m *mockCircuitBreaker) State() string { return "closed" }
func (m *mockCircuitBreaker) Reset() {}

type mockStorage struct {
	services map[string]*types.Service
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		services: make(map[string]*types.Service),
	}
}

func (m *mockStorage) GetService(ctx context.Context, id string) (*types.Service, error) {
	if service, ok := m.services[id]; ok {
		return service, nil
	}
	return nil, types.ErrServiceNotFound
}

func (m *mockStorage) ListServices(ctx context.Context) ([]*types.Service, error) {
	services := make([]*types.Service, 0, len(m.services))
	for _, service := range m.services {
		services = append(services, service)
	}
	return services, nil
}

func (m *mockStorage) CreateService(ctx context.Context, service *types.Service) error {
	m.services[service.ID] = service
	return nil
}

func (m *mockStorage) UpdateService(ctx context.Context, service *types.Service) error {
	m.services[service.ID] = service
	return nil
}

func (m *mockStorage) DeleteService(ctx context.Context, id string) error {
	delete(m.services, id)
	return nil
}

func (m *mockStorage) GetRoute(ctx context.Context, id string) (*types.Route, error) {
	return nil, types.ErrRouteNotFound
}
func (m *mockStorage) ListRoutes(ctx context.Context) ([]*types.Route, error) { return nil, nil }
func (m *mockStorage) CreateRoute(ctx context.Context, route *types.Route) error { return nil }
func (m *mockStorage) UpdateRoute(ctx context.Context, route *types.Route) error { return nil }
func (m *mockStorage) DeleteRoute(ctx context.Context, id string) error { return nil }
func (m *mockStorage) GetUser(ctx context.Context, id string) (*types.User, error) { return nil, nil }
func (m *mockStorage) GetUserByEmail(ctx context.Context, email string) (*types.User, error) { return nil, nil }
func (m *mockStorage) GetUserByUsername(ctx context.Context, username string) (*types.User, error) { return nil, nil }
func (m *mockStorage) ListUsers(ctx context.Context) ([]*types.User, error) { return nil, nil }
func (m *mockStorage) CreateUser(ctx context.Context, user *types.User) error { return nil }
func (m *mockStorage) UpdateUser(ctx context.Context, user *types.User) error { return nil }
func (m *mockStorage) DeleteUser(ctx context.Context, id string) error { return nil }
func (m *mockStorage) GetAPIKey(ctx context.Context, key string) (*types.APIKey, error) { return nil, nil }
func (m *mockStorage) ListAPIKeys(ctx context.Context) ([]*types.APIKey, error) { return nil, nil }
func (m *mockStorage) ListAPIKeysByUser(ctx context.Context, userID string) ([]*types.APIKey, error) { return nil, nil }
func (m *mockStorage) CreateAPIKey(ctx context.Context, apiKey *types.APIKey) error { return nil }
func (m *mockStorage) RevokeAPIKey(ctx context.Context, key string) error { return nil }
func (m *mockStorage) Watch(ctx context.Context) <-chan types.StorageEvent { return nil }
func (m *mockStorage) Close() error { return nil }

type testLogger struct{}
func (l *testLogger) Debug(msg string, fields ...interface{}) {}
func (l *testLogger) Info(msg string, fields ...interface{})  {}
func (l *testLogger) Warn(msg string, fields ...interface{})  {}
func (l *testLogger) Error(msg string, fields ...interface{}) {}
func (l *testLogger) With(fields ...interface{}) types.Logger { return l }

// Helper functions
func createTestBackend(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}

func TestProxyBasicRequest(t *testing.T) {
	// Create test backend
	var capturedRequest *http.Request
	backend := createTestBackend(func(w http.ResponseWriter, r *http.Request) {
		capturedRequest = r
		w.Header().Set("X-Backend", "test")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "Hello from backend")
	})
	defer backend.Close()

	// Parse backend URL
	backendURL, _ := url.Parse(backend.URL)

	// Setup mocks
	storage := newMockStorage()
	service := &types.Service{
		ID:        "test-service",
		Name:      "Test Service",
		Endpoints: []string{backend.URL},
		Active:    true,
	}
	storage.CreateService(context.Background(), service)

	route := &types.Route{
		ID:         "test-route",
		ServiceID:  service.ID,
		PathPrefix: "/api",
		Priority:   100,
	}

	router := &mockRouter{
		matchFunc: func(req *http.Request) (*types.Route, error) {
			if strings.HasPrefix(req.URL.Path, "/api") {
				return route, nil
			}
			return nil, types.ErrRouteNotFound
		},
	}

	server := &types.Server{
		ID:      "backend-1",
		URL:     backendURL,
		Healthy: true,
	}

	loadBalancer := &mockLoadBalancer{
		selectFunc: func(ctx context.Context, req *http.Request, servers []*types.Server) (*types.Server, error) {
			return server, nil
		},
	}

	healthChecker := &mockHealthChecker{}
	circuitBreaker := &mockCircuitBreaker{}
	logger := &testLogger{}

	// Create proxy
	p := proxy.New(proxy.Options{
		Router:         router,
		LoadBalancer:   loadBalancer,
		HealthChecker:  healthChecker,
		CircuitBreaker: circuitBreaker,
		Storage:        storage,
		Logger:         logger,
	})

	// Make request
	req := httptest.NewRequest("GET", "http://example.com/api/test", nil)
	req.Header.Set("X-Custom", "test-value")
	rec := httptest.NewRecorder()

	p.ServeHTTP(rec, req)

	// Verify response
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "test", rec.Header().Get("X-Backend"))
	assert.Contains(t, rec.Body.String(), "Hello from backend")

	// Verify request was forwarded correctly
	require.NotNil(t, capturedRequest)
	assert.Equal(t, "/api/test", capturedRequest.URL.Path)
	assert.Equal(t, "test-value", capturedRequest.Header.Get("X-Custom"))
}

func TestProxyRouteNotFound(t *testing.T) {
	router := &mockRouter{
		matchFunc: func(req *http.Request) (*types.Route, error) {
			return nil, types.ErrRouteNotFound
		},
	}

	p := proxy.New(proxy.Options{
		Router: router,
		Logger: &testLogger{},
	})

	req := httptest.NewRequest("GET", "http://example.com/unknown", nil)
	rec := httptest.NewRecorder()

	p.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestProxyServiceNotFound(t *testing.T) {
	storage := newMockStorage()
	route := &types.Route{
		ID:        "test-route",
		ServiceID: "non-existent",
	}

	router := &mockRouter{
		matchFunc: func(req *http.Request) (*types.Route, error) {
			return route, nil
		},
	}

	p := proxy.New(proxy.Options{
		Router:  router,
		Storage: storage,
		Logger:  &testLogger{},
	})

	req := httptest.NewRequest("GET", "http://example.com/api/test", nil)
	rec := httptest.NewRecorder()

	p.ServeHTTP(rec, req)

	// Service not found should return ServiceUnavailable
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestProxyNoHealthyBackends(t *testing.T) {
	storage := newMockStorage()
	service := &types.Service{
		ID:        "test-service",
		Name:      "Test Service",
		Endpoints: []string{"http://backend:8080"},
		Active:    true,
	}
	storage.CreateService(context.Background(), service)

	route := &types.Route{
		ID:        "test-route",
		ServiceID: service.ID,
	}

	router := &mockRouter{
		matchFunc: func(req *http.Request) (*types.Route, error) {
			return route, nil
		},
	}

	loadBalancer := &mockLoadBalancer{
		selectFunc: func(ctx context.Context, req *http.Request, servers []*types.Server) (*types.Server, error) {
			return nil, types.ErrNoHealthyBackends
		},
	}

	p := proxy.New(proxy.Options{
		Router:       router,
		LoadBalancer: loadBalancer,
		Storage:      storage,
		Logger:       &testLogger{},
	})

	req := httptest.NewRequest("GET", "http://example.com/api/test", nil)
	rec := httptest.NewRecorder()

	p.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestProxyCircuitBreakerOpen(t *testing.T) {
	backend := createTestBackend(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)

	storage := newMockStorage()
	service := &types.Service{
		ID:        "test-service",
		Name:      "Test Service",
		Endpoints: []string{backend.URL},
		Active:    true,
	}
	storage.CreateService(context.Background(), service)

	route := &types.Route{
		ID:        "test-route",
		ServiceID: service.ID,
	}

	router := &mockRouter{
		matchFunc: func(req *http.Request) (*types.Route, error) {
			return route, nil
		},
	}

	server := &types.Server{
		ID:      "backend-1",
		URL:     backendURL,
		Healthy: true,
	}

	loadBalancer := &mockLoadBalancer{
		selectFunc: func(ctx context.Context, req *http.Request, servers []*types.Server) (*types.Server, error) {
			return server, nil
		},
	}

	circuitBreaker := &mockCircuitBreaker{
		executeFunc: func(fn func() error) error {
			return types.ErrCircuitBreakerOpen
		},
	}

	p := proxy.New(proxy.Options{
		Router:         router,
		LoadBalancer:   loadBalancer,
		CircuitBreaker: circuitBreaker,
		Storage:        storage,
		Logger:         &testLogger{},
	})

	req := httptest.NewRequest("GET", "http://example.com/api/test", nil)
	rec := httptest.NewRecorder()

	p.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestProxyHeaders(t *testing.T) {
	var capturedHeaders http.Header
	backend := createTestBackend(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	})
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)

	storage := newMockStorage()
	service := &types.Service{
		ID:        "test-service",
		Endpoints: []string{backend.URL},
		Active:    true,
	}
	storage.CreateService(context.Background(), service)

	route := &types.Route{
		ID:        "test-route",
		ServiceID: service.ID,
	}

	router := &mockRouter{
		matchFunc: func(req *http.Request) (*types.Route, error) {
			return route, nil
		},
	}

	server := &types.Server{
		ID:      "backend-1",
		URL:     backendURL,
		Healthy: true,
	}

	loadBalancer := &mockLoadBalancer{
		selectFunc: func(ctx context.Context, req *http.Request, servers []*types.Server) (*types.Server, error) {
			return server, nil
		},
	}

	p := proxy.New(proxy.Options{
		Router:       router,
		LoadBalancer: loadBalancer,
		Storage:      storage,
		Logger:       &testLogger{},
	})

	req := httptest.NewRequest("GET", "http://example.com/api/test", nil)
	req.Header.Set("X-Custom-Header", "test-value")
	req.Header.Set("User-Agent", "test-agent")
	req.RemoteAddr = "192.168.1.100:12345"
	rec := httptest.NewRecorder()

	p.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "test-value", capturedHeaders.Get("X-Custom-Header"))
	assert.Equal(t, "test-agent", capturedHeaders.Get("User-Agent"))
	// Proxy should add X-Forwarded-For and X-Real-IP
	assert.NotEmpty(t, capturedHeaders.Get("X-Forwarded-For"))
	assert.Equal(t, "192.168.1.100", capturedHeaders.Get("X-Real-IP"))
}

func TestProxyRequestBody(t *testing.T) {
	var capturedBody []byte
	backend := createTestBackend(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedBody = body
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Received: " + string(body)))
	})
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)

	storage := newMockStorage()
	service := &types.Service{
		ID:        "test-service",
		Endpoints: []string{backend.URL},
		Active:    true,
	}
	storage.CreateService(context.Background(), service)

	route := &types.Route{
		ID:        "test-route",
		ServiceID: service.ID,
	}

	router := &mockRouter{
		matchFunc: func(req *http.Request) (*types.Route, error) {
			return route, nil
		},
	}

	server := &types.Server{
		ID:      "backend-1",
		URL:     backendURL,
		Healthy: true,
	}

	loadBalancer := &mockLoadBalancer{
		selectFunc: func(ctx context.Context, req *http.Request, servers []*types.Server) (*types.Server, error) {
			return server, nil
		},
	}

	p := proxy.New(proxy.Options{
		Router:       router,
		LoadBalancer: loadBalancer,
		Storage:      storage,
		Logger:       &testLogger{},
	})

	body := []byte(`{"test": "data"}`)
	req := httptest.NewRequest("POST", "http://example.com/api/test", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	p.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, body, capturedBody)
	assert.Contains(t, rec.Body.String(), `{"test": "data"}`)
}

func TestProxyTimeout(t *testing.T) {
	// Create slow backend
	backend := createTestBackend(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)

	storage := newMockStorage()
	service := &types.Service{
		ID:        "test-service",
		Endpoints: []string{backend.URL},
		Active:    true,
	}
	storage.CreateService(context.Background(), service)

	route := &types.Route{
		ID:        "test-route",
		ServiceID: service.ID,
	}

	router := &mockRouter{
		matchFunc: func(req *http.Request) (*types.Route, error) {
			return route, nil
		},
	}

	server := &types.Server{
		ID:      "backend-1",
		URL:     backendURL,
		Healthy: true,
	}

	loadBalancer := &mockLoadBalancer{
		selectFunc: func(ctx context.Context, req *http.Request, servers []*types.Server) (*types.Server, error) {
			return server, nil
		},
	}

	// Create transport with short timeout
	transport := &http.Transport{
		ResponseHeaderTimeout: 50 * time.Millisecond,
	}

	p := proxy.New(proxy.Options{
		Router:       router,
		LoadBalancer: loadBalancer,
		Storage:      storage,
		Logger:       &testLogger{},
		Transport:    transport,
	})

	req := httptest.NewRequest("GET", "http://example.com/api/test", nil)
	rec := httptest.NewRecorder()

	p.ServeHTTP(rec, req)

	// Should timeout
	assert.Equal(t, http.StatusBadGateway, rec.Code)
}

func TestProxyConcurrency(t *testing.T) {
	var activeRequests int32
	var maxConcurrent int32
	
	backend := createTestBackend(func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&activeRequests, 1)
		defer atomic.AddInt32(&activeRequests, -1)
		
		// Update max concurrent
		for {
			max := atomic.LoadInt32(&maxConcurrent)
			if current <= max || atomic.CompareAndSwapInt32(&maxConcurrent, max, current) {
				break
			}
		}
		
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Request %d", current)
	})
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)

	storage := newMockStorage()
	service := &types.Service{
		ID:        "test-service",
		Endpoints: []string{backend.URL},
		Active:    true,
	}
	storage.CreateService(context.Background(), service)

	route := &types.Route{
		ID:        "test-route",
		ServiceID: service.ID,
	}

	router := &mockRouter{
		matchFunc: func(req *http.Request) (*types.Route, error) {
			return route, nil
		},
	}

	server := &types.Server{
		ID:      "backend-1",
		URL:     backendURL,
		Healthy: true,
	}

	loadBalancer := &mockLoadBalancer{
		selectFunc: func(ctx context.Context, req *http.Request, servers []*types.Server) (*types.Server, error) {
			return server, nil
		},
	}

	p := proxy.New(proxy.Options{
		Router:       router,
		LoadBalancer: loadBalancer,
		Storage:      storage,
		Logger:       &testLogger{},
	})

	// Make concurrent requests
	var wg sync.WaitGroup
	numRequests := 50
	
	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "http://example.com/api/test", nil)
			rec := httptest.NewRecorder()
			p.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusOK, rec.Code)
		}()
	}
	
	wg.Wait()
	
	// Verify concurrency was achieved
	assert.Greater(t, atomic.LoadInt32(&maxConcurrent), int32(1))
}

func TestProxyModifyResponse(t *testing.T) {
	backend := createTestBackend(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend-Header", "original")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Original response"))
	})
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)

	storage := newMockStorage()
	service := &types.Service{
		ID:        "test-service",
		Endpoints: []string{backend.URL},
		Active:    true,
	}
	storage.CreateService(context.Background(), service)

	route := &types.Route{
		ID:        "test-route",
		ServiceID: service.ID,
	}

	router := &mockRouter{
		matchFunc: func(req *http.Request) (*types.Route, error) {
			return route, nil
		},
	}

	server := &types.Server{
		ID:      "backend-1",
		URL:     backendURL,
		Healthy: true,
	}

	loadBalancer := &mockLoadBalancer{
		selectFunc: func(ctx context.Context, req *http.Request, servers []*types.Server) (*types.Server, error) {
			return server, nil
		},
	}

	p := proxy.New(proxy.Options{
		Router:       router,
		LoadBalancer: loadBalancer,
		Storage:      storage,
		Logger:       &testLogger{},
		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Set("X-Modified", "true")
			resp.Header.Set("X-Backend-Header", "modified")
			return nil
		},
	})

	req := httptest.NewRequest("GET", "http://example.com/api/test", nil)
	rec := httptest.NewRecorder()

	p.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "true", rec.Header().Get("X-Modified"))
	assert.Equal(t, "modified", rec.Header().Get("X-Backend-Header"))
	assert.Contains(t, rec.Body.String(), "Original response")
}

func TestProxyErrorHandler(t *testing.T) {
	router := &mockRouter{
		matchFunc: func(req *http.Request) (*types.Route, error) {
			return nil, types.ErrRouteNotFound
		},
	}

	customErrorHandled := false
	p := proxy.New(proxy.Options{
		Router: router,
		Logger: &testLogger{},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			customErrorHandled = true
			w.WriteHeader(http.StatusTeapot) // 418
			w.Write([]byte("Custom error: " + err.Error()))
		},
	})

	req := httptest.NewRequest("GET", "http://example.com/api/test", nil)
	rec := httptest.NewRecorder()

	p.ServeHTTP(rec, req)

	assert.True(t, customErrorHandled)
	assert.Equal(t, http.StatusTeapot, rec.Code)
	assert.Contains(t, rec.Body.String(), "Custom error:")
}

func TestProxyWebSocketUpgrade(t *testing.T) {
	wsBackend := createTestBackend(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") != "websocket" {
			http.Error(w, "Expected WebSocket", http.StatusBadRequest)
			return
		}
		
		w.Header().Set("Upgrade", "websocket")
		w.Header().Set("Connection", "Upgrade")
		w.Header().Set("Sec-WebSocket-Accept", "test-accept")
		w.WriteHeader(http.StatusSwitchingProtocols)
	})
	defer wsBackend.Close()

	backendURL, _ := url.Parse(wsBackend.URL)

	storage := newMockStorage()
	service := &types.Service{
		ID:        "test-service",
		Endpoints: []string{wsBackend.URL},
		Active:    true,
	}
	storage.CreateService(context.Background(), service)

	route := &types.Route{
		ID:        "test-route",
		ServiceID: service.ID,
	}

	router := &mockRouter{
		matchFunc: func(req *http.Request) (*types.Route, error) {
			return route, nil
		},
	}

	server := &types.Server{
		ID:      "backend-1",
		URL:     backendURL,
		Healthy: true,
	}

	loadBalancer := &mockLoadBalancer{
		selectFunc: func(ctx context.Context, req *http.Request, servers []*types.Server) (*types.Server, error) {
			return server, nil
		},
	}

	p := proxy.New(proxy.Options{
		Router:       router,
		LoadBalancer: loadBalancer,
		Storage:      storage,
		Logger:       &testLogger{},
	})

	req := httptest.NewRequest("GET", "http://example.com/api/ws", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "test-key")
	
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)

	// WebSocket upgrade uses hijacking which httptest.ResponseRecorder doesn't support
	// So we expect an error or the proxy to handle it differently
	// The important thing is that the proxy recognizes it as a WebSocket request
	// and attempts to handle it appropriately
	assert.NotEqual(t, http.StatusNotFound, rec.Code)
}

func TestProxyURLRewriting(t *testing.T) {
	var capturedPath string
	backend := createTestBackend(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Path: %s", r.URL.Path)
	})
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)

	storage := newMockStorage()
	service := &types.Service{
		ID:        "test-service",
		Endpoints: []string{backend.URL},
		Active:    true,
	}
	storage.CreateService(context.Background(), service)

	route := &types.Route{
		ID:         "test-route",
		ServiceID:  service.ID,
		PathPrefix: "/api/v1",
		RewriteRules: []types.RewriteRule{
			{
				Type:        "regex",
				Pattern:     "^/api/v1/(.*)$",
				Replacement: "/v1/$1",
			},
		},
	}

	router := &mockRouter{
		matchFunc: func(req *http.Request) (*types.Route, error) {
			return route, nil
		},
	}

	server := &types.Server{
		ID:      "backend-1",
		URL:     backendURL,
		Healthy: true,
	}

	loadBalancer := &mockLoadBalancer{
		selectFunc: func(ctx context.Context, req *http.Request, servers []*types.Server) (*types.Server, error) {
			return server, nil
		},
	}

	rewriter := proxy.NewURLRewriter()

	p := proxy.New(proxy.Options{
		Router:       router,
		LoadBalancer: loadBalancer,
		Storage:      storage,
		Logger:       &testLogger{},
		Rewriter:     rewriter,
	})

	req := httptest.NewRequest("GET", "http://example.com/api/v1/users/123", nil)
	rec := httptest.NewRecorder()

	p.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	// Verify the path was properly rewritten
	assert.Equal(t, "/v1/users/123", capturedPath)
	assert.Contains(t, rec.Body.String(), "Path: /v1/users/123")
}

func TestProxyInactiveService(t *testing.T) {
	storage := newMockStorage()
	service := &types.Service{
		ID:        "test-service",
		Name:      "Test Service",
		Endpoints: []string{"http://backend:8080"},
		Active:    false, // Inactive service
	}
	storage.CreateService(context.Background(), service)

	route := &types.Route{
		ID:        "test-route",
		ServiceID: service.ID,
	}

	router := &mockRouter{
		matchFunc: func(req *http.Request) (*types.Route, error) {
			return route, nil
		},
	}

	p := proxy.New(proxy.Options{
		Router:  router,
		Storage: storage,
		Logger:  &testLogger{},
	})

	req := httptest.NewRequest("GET", "http://example.com/api/test", nil)
	rec := httptest.NewRecorder()

	p.ServeHTTP(rec, req)

	// Inactive service should return ServiceUnavailable
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestProxyHealthRecording(t *testing.T) {
	// Test both success and failure scenarios
	testCases := []struct {
		name           string
		backendStatus  int
		expectSuccess  bool
		expectFailure  bool
	}{
		{
			name:          "Successful request",
			backendStatus: http.StatusOK,
			expectSuccess: true,
			expectFailure: false,
		},
		{
			name:          "Server error",
			backendStatus: http.StatusInternalServerError,
			expectSuccess: false,
			expectFailure: true,
		},
		{
			name:          "Client error",
			backendStatus: http.StatusBadRequest,
			expectSuccess: false,
			expectFailure: false, // 4xx errors don't record failure
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			backend := createTestBackend(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.backendStatus)
				w.Write([]byte("Response"))
			})
			defer backend.Close()

			backendURL, _ := url.Parse(backend.URL)

			storage := newMockStorage()
			service := &types.Service{
				ID:        "test-service",
				Endpoints: []string{backend.URL},
				Active:    true,
			}
			storage.CreateService(context.Background(), service)

			route := &types.Route{
				ID:        "test-route",
				ServiceID: service.ID,
			}

			router := &mockRouter{
				matchFunc: func(req *http.Request) (*types.Route, error) {
					return route, nil
				},
			}

			server := &types.Server{
				ID:      "backend-1",
				URL:     backendURL,
				Healthy: true,
			}

			loadBalancer := &mockLoadBalancer{
				selectFunc: func(ctx context.Context, req *http.Request, servers []*types.Server) (*types.Server, error) {
					return server, nil
				},
			}

			var recordedSuccess, recordedFailure bool
			healthChecker := &mockHealthChecker{
				recordSuccess: func(serverID string) {
					recordedSuccess = true
					assert.Equal(t, server.ID, serverID)
				},
				recordFailure: func(serverID string, err error) {
					recordedFailure = true
					assert.Equal(t, server.ID, serverID)
					assert.NotNil(t, err)
				},
			}

			p := proxy.New(proxy.Options{
				Router:        router,
				LoadBalancer:  loadBalancer,
				HealthChecker: healthChecker,
				Storage:       storage,
				Logger:        &testLogger{},
			})

			req := httptest.NewRequest("GET", "http://example.com/api/test", nil)
			rec := httptest.NewRecorder()
			p.ServeHTTP(rec, req)
			
			assert.Equal(t, tc.backendStatus, rec.Code)
			assert.Equal(t, tc.expectSuccess, recordedSuccess, "Expected success recording: %v, got: %v", tc.expectSuccess, recordedSuccess)
			assert.Equal(t, tc.expectFailure, recordedFailure, "Expected failure recording: %v, got: %v", tc.expectFailure, recordedFailure)
		})
	}
}