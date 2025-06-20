package types

import "time"

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
		CacheDir   string   `yaml:"cache_dir,omitempty"`
	} `yaml:"tls"`
	
	// HTTP/2 and HTTP/3
	HTTP2 struct {
		Enabled bool `yaml:"enabled"`
	} `yaml:"http2"`
	
	HTTP3 struct {
		Enabled bool   `yaml:"enabled"`
		AltSvc  string `yaml:"alt_svc,omitempty"`
		Port    string `yaml:"port,omitempty"`
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
		Enabled  bool   `yaml:"enabled"`
		RPS      int    `yaml:"rps"`
		Burst    int    `yaml:"burst"`
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
		APIKey  string `yaml:"api_key,omitempty"`
	} `yaml:"api"`
	
	// Web UI
	UI struct {
		Enabled bool   `yaml:"enabled"`
		Path    string `yaml:"path"`
	} `yaml:"ui"`
}