# discobox types/interfaces

```go

// Package discobox defines the core interfaces for the production-grade reverse proxy
package discobox

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// Service represents a backend service
type Service struct {
	ID          string            `json:"id" yaml:"id"`
	Name        string            `json:"name" yaml:"name"`
	Endpoints   []string          `json:"endpoints" yaml:"endpoints"`
	HealthPath  string            `json:"health_path" yaml:"health_path"`
	Weight      int               `json:"weight" yaml:"weight"`
	MaxConns    int               `json:"max_conns" yaml:"max_conns"`
	Timeout     time.Duration     `json:"timeout" yaml:"timeout"`
	Metadata    map[string]string `json:"metadata" yaml:"metadata"`
	TLS         *TLSConfig        `json:"tls,omitempty" yaml:"tls,omitempty"`
	StripPrefix bool              `json:"strip_prefix" yaml:"strip_prefix"`
	Active      bool              `json:"active" yaml:"active"`
	CreatedAt   time.Time         `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at" yaml:"updated_at"`
}

// Route represents a routing rule
type Route struct {
	ID           string                 `json:"id" yaml:"id"`
	Priority     int                    `json:"priority" yaml:"priority"`
	Host         string                 `json:"host,omitempty" yaml:"host,omitempty"`
	PathPrefix   string                 `json:"path_prefix,omitempty" yaml:"path_prefix,omitempty"`
	PathRegex    string                 `json:"path_regex,omitempty" yaml:"path_regex,omitempty"`
	Headers      map[string]string      `json:"headers,omitempty" yaml:"headers,omitempty"`
	ServiceID    string                 `json:"service_id" yaml:"service_id"`
	Middlewares  []string               `json:"middlewares" yaml:"middlewares"`
	RewriteRules []RewriteRule          `json:"rewrite_rules,omitempty" yaml:"rewrite_rules,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// RewriteRule defines URL rewriting rules
type RewriteRule struct {
	Type        string `json:"type" yaml:"type"` // regex, prefix, strip_prefix
	Pattern     string `json:"pattern" yaml:"pattern"`
	Replacement string `json:"replacement,omitempty" yaml:"replacement,omitempty"`
}

// TLSConfig for backend connections
type TLSConfig struct {
	InsecureSkipVerify bool     `json:"insecure_skip_verify" yaml:"insecure_skip_verify"`
	ServerName         string   `json:"server_name,omitempty" yaml:"server_name,omitempty"`
	RootCAs            []string `json:"root_cas,omitempty" yaml:"root_cas,omitempty"`
	ClientCert         string   `json:"client_cert,omitempty" yaml:"client_cert,omitempty"`
	ClientKey          string   `json:"client_key,omitempty" yaml:"client_key,omitempty"`
}

// LoadBalancer selects a backend server
type LoadBalancer interface {
	// Select returns the next server to use
	Select(ctx context.Context, req *http.Request, servers []*Server) (*Server, error)
	// Add adds a new server to the pool
	Add(server *Server) error
	// Remove removes a server from the pool
	Remove(serverID string) error
	// UpdateWeight updates server weight (for weighted algorithms)
	UpdateWeight(serverID string, weight int) error
}

// Server represents a backend server instance
type Server struct {
	URL         *url.URL
	ID          string
	Weight      int
	MaxConns    int
	ActiveConns int64
	Healthy     bool
	Metadata    map[string]string
	LastUsed    time.Time
}

// HealthChecker monitors backend health
type HealthChecker interface {
	// Check performs a health check on the server
	Check(ctx context.Context, server *Server) error
	// Watch continuously monitors server health
	Watch(ctx context.Context, server *Server, interval time.Duration) <-chan error
	// RecordSuccess records a successful request (for passive checks)
	RecordSuccess(serverID string)
	// RecordFailure records a failed request (for passive checks)
	RecordFailure(serverID string, err error)
}

// CircuitBreaker protects backends from cascading failures
type CircuitBreaker interface {
	// Execute runs the function with circuit breaker protection
	Execute(fn func() error) error
	// State returns the current state (closed, open, half-open)
	State() string
	// Reset manually resets the circuit breaker
	Reset()
}

// RateLimiter controls request rates
type RateLimiter interface {
	// Allow checks if a request should be allowed
	Allow(key string) bool
	// Wait blocks until the request can proceed
	Wait(ctx context.Context, key string) error
	// Limit returns the current limit for a key
	Limit(key string) int
}

// URLRewriter transforms request URLs
type URLRewriter interface {
	// Rewrite modifies the request URL based on rules
	Rewrite(req *http.Request, rules []RewriteRule) error
}

// Router manages request routing
type Router interface {
	// Match finds the best route for a request
	Match(req *http.Request) (*Route, error)
	// AddRoute adds a new route
	AddRoute(route *Route) error
	// RemoveRoute removes a route
	RemoveRoute(routeID string) error
	// UpdateRoute updates an existing route
	UpdateRoute(route *Route) error
	// GetRoutes returns all routes
	GetRoutes() ([]*Route, error)
}

// Storage persists configuration
type Storage interface {
	// Services
	GetService(ctx context.Context, id string) (*Service, error)
	ListServices(ctx context.Context) ([]*Service, error)
	CreateService(ctx context.Context, service *Service) error
	UpdateService(ctx context.Context, service *Service) error
	DeleteService(ctx context.Context, id string) error
	
	// Routes
	GetRoute(ctx context.Context, id string) (*Route, error)
	ListRoutes(ctx context.Context) ([]*Route, error)
	CreateRoute(ctx context.Context, route *Route) error
	UpdateRoute(ctx context.Context, route *Route) error
	DeleteRoute(ctx context.Context, id string) error
	
	// Watch for changes
	Watch(ctx context.Context) <-chan StorageEvent
}

// StorageEvent represents a configuration change
type StorageEvent struct {
	Type   string // created, updated, deleted
	Kind   string // service, route
	ID     string
	Object any
}

// Middleware wraps HTTP handlers
type Middleware func(http.Handler) http.Handler

// MiddlewareChain manages middleware execution order
type MiddlewareChain interface {
	// Use adds middleware to the chain
	Use(middleware ...Middleware)
	// Then creates the final handler
	Then(handler http.Handler) http.Handler
}

// MetricsCollector gathers performance metrics
type MetricsCollector interface {
	// RecordRequest records request metrics
	RecordRequest(method, path string, statusCode int, duration time.Duration)
	// RecordUpstreamLatency records backend latency
	RecordUpstreamLatency(service string, duration time.Duration)
	// RecordActiveConnections updates connection count
	RecordActiveConnections(count int)
	// Handler returns the metrics endpoint handler
	Handler() http.Handler
}

// Logger provides structured logging
type Logger interface {
	Debug(msg string, fields ...any)
	Info(msg string, fields ...any)
	Warn(msg string, fields ...any)
	Error(msg string, fields ...any)
	With(fields ...any) Logger
}

// ProxyConfig represents the complete proxy configuration
type ProxyConfig struct {
	// Server configuration
	ListenAddr      string        `yaml:"listen_addr"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	IdleTimeout     time.Duration `yaml:"idle_timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
	
	// TLS configuration
	TLS struct {
		Enabled    bool     `yaml:"enabled"`
		CertFile   string   `yaml:"cert_file,omitempty"`
		KeyFile    string   `yaml:"key_file,omitempty"`
		AutoCert   bool     `yaml:"auto_cert"`
		Domains    []string `yaml:"domains,omitempty"`
		Email      string   `yaml:"email,omitempty"`
		MinVersion string   `yaml:"min_version"`
	} `yaml:"tls"`
	
	// HTTP/2 and HTTP/3
	HTTP2 struct {
		Enabled bool `yaml:"enabled"`
	} `yaml:"http2"`
	
	HTTP3 struct {
		Enabled bool   `yaml:"enabled"`
		AltSvc  string `yaml:"alt_svc,omitempty"`
	} `yaml:"http3"`
	
	// Transport configuration
	Transport struct {
		MaxIdleConns        int           `yaml:"max_idle_conns"`
		MaxIdleConnsPerHost int           `yaml:"max_idle_conns_per_host"`
		MaxConnsPerHost     int           `yaml:"max_conns_per_host"`
		IdleConnTimeout     time.Duration `yaml:"idle_conn_timeout"`
		DialTimeout         time.Duration `yaml:"dial_timeout"`
		KeepAlive           time.Duration `yaml:"keep_alive"`
		DisableCompression  bool          `yaml:"disable_compression"`
		BufferSize          int           `yaml:"buffer_size"`
	} `yaml:"transport"`
	
	// Load balancing
	LoadBalancing struct {
		Algorithm string `yaml:"algorithm"` // round_robin, weighted, least_conn, ip_hash
		Sticky    struct {
			Enabled    bool          `yaml:"enabled"`
			CookieName string        `yaml:"cookie_name"`
			TTL        time.Duration `yaml:"ttl"`
		} `yaml:"sticky"`
	} `yaml:"load_balancing"`
	
	// Health checking
	HealthCheck struct {
		Interval      time.Duration `yaml:"interval"`
		Timeout       time.Duration `yaml:"timeout"`
		FailThreshold int           `yaml:"fail_threshold"`
		PassThreshold int           `yaml:"pass_threshold"`
	} `yaml:"health_check"`
	
	// Circuit breaker
	CircuitBreaker struct {
		Enabled          bool          `yaml:"enabled"`
		FailureThreshold int           `yaml:"failure_threshold"`
		SuccessThreshold int           `yaml:"success_threshold"`
		Timeout          time.Duration `yaml:"timeout"`
	} `yaml:"circuit_breaker"`
	
	// Rate limiting
	RateLimit struct {
		Enabled  bool `yaml:"enabled"`
		RPS      int  `yaml:"rps"`
		Burst    int  `yaml:"burst"`
		ByHeader string `yaml:"by_header,omitempty"`
	} `yaml:"rate_limit"`
	
	// Middleware configuration
	Middleware struct {
		Compression struct {
			Enabled    bool     `yaml:"enabled"`
			Level      int      `yaml:"level"`
			Types      []string `yaml:"types"`
			Algorithms []string `yaml:"algorithms"` // gzip, br, zstd
		} `yaml:"compression"`
		
		CORS struct {
			Enabled          bool     `yaml:"enabled"`
			AllowedOrigins   []string `yaml:"allowed_origins"`
			AllowedMethods   []string `yaml:"allowed_methods"`
			AllowedHeaders   []string `yaml:"allowed_headers"`
			AllowCredentials bool     `yaml:"allow_credentials"`
			MaxAge           int      `yaml:"max_age"`
		} `yaml:"cors"`
		
		Headers struct {
			Security bool              `yaml:"security"`
			Custom   map[string]string `yaml:"custom,omitempty"`
			Remove   []string          `yaml:"remove,omitempty"`
		} `yaml:"headers"`
		
		Auth struct {
			Basic struct {
				Enabled bool              `yaml:"enabled"`
				Users   map[string]string `yaml:"users,omitempty"`
			} `yaml:"basic"`
			
			JWT struct {
				Enabled  bool   `yaml:"enabled"`
				Issuer   string `yaml:"issuer,omitempty"`
				Audience string `yaml:"audience,omitempty"`
				KeyFile  string `yaml:"key_file,omitempty"`
			} `yaml:"jwt"`
			
			OAuth2 struct {
				Enabled      bool   `yaml:"enabled"`
				Provider     string `yaml:"provider"`
				ClientID     string `yaml:"client_id,omitempty"`
				ClientSecret string `yaml:"client_secret,omitempty"`
				RedirectURL  string `yaml:"redirect_url,omitempty"`
			} `yaml:"oauth2"`
		} `yaml:"auth"`
	} `yaml:"middleware"`
	
	// Logging and monitoring
	Logging struct {
		Level      string `yaml:"level"`
		Format     string `yaml:"format"` // json, text
		AccessLogs bool   `yaml:"access_logs"`
	} `yaml:"logging"`
	
	Metrics struct {
		Enabled bool   `yaml:"enabled"`
		Path    string `yaml:"path"`
	} `yaml:"metrics"`
	
	// Storage backend
	Storage struct {
		Type   string `yaml:"type"` // sqlite, memory, etcd
		DSN    string `yaml:"dsn,omitempty"`
		Prefix string `yaml:"prefix,omitempty"`
	} `yaml:"storage"`
	
	// Admin API
	API struct {
		Enabled bool   `yaml:"enabled"`
		Addr    string `yaml:"addr"`
		Auth    bool   `yaml:"auth"`
	} `yaml:"api"`
	
	// Web UI
	UI struct {
		Enabled bool   `yaml:"enabled"`
		Path    string `yaml:"path"`
	} `yaml:"ui"`
}

```
