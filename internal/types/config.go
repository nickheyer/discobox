package types

import "time"

// ProxyConfig represents the complete proxy configuration
type ProxyConfig struct {
	// Server configuration
	ListenAddr      string        `yaml:"listen_addr" mapstructure:"listen_addr"`
	ReadTimeout     time.Duration `yaml:"read_timeout" mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout" mapstructure:"write_timeout"`
	IdleTimeout     time.Duration `yaml:"idle_timeout" mapstructure:"idle_timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" mapstructure:"shutdown_timeout"`
	
	// TLS configuration
	TLS struct {
		Enabled    bool     `yaml:"enabled" mapstructure:"enabled"`
		CertFile   string   `yaml:"cert_file,omitempty" mapstructure:"cert_file,omitempty"`
		KeyFile    string   `yaml:"key_file,omitempty" mapstructure:"key_file,omitempty"`
		AutoCert   bool     `yaml:"auto_cert" mapstructure:"auto_cert"`
		Domains    []string `yaml:"domains,omitempty" mapstructure:"domains,omitempty"`
		Email      string   `yaml:"email,omitempty" mapstructure:"email,omitempty"`
		MinVersion string   `yaml:"min_version" mapstructure:"min_version"`
		CacheDir   string   `yaml:"cache_dir,omitempty" mapstructure:"cache_dir,omitempty"`
	} `yaml:"tls" mapstructure:"tls"`
	
	// HTTP/2 and HTTP/3
	HTTP2 struct {
		Enabled bool `yaml:"enabled" mapstructure:"enabled"`
	} `yaml:"http2" mapstructure:"http2"`
	
	HTTP3 struct {
		Enabled bool   `yaml:"enabled" mapstructure:"enabled"`
		AltSvc  string `yaml:"alt_svc,omitempty" mapstructure:"alt_svc,omitempty"`
		Port    string `yaml:"port,omitempty" mapstructure:"port,omitempty"`
	} `yaml:"http3" mapstructure:"http3"`
	
	// Transport configuration
	Transport struct {
		MaxIdleConns        int           `yaml:"max_idle_conns" mapstructure:"max_idle_conns"`
		MaxIdleConnsPerHost int           `yaml:"max_idle_conns_per_host" mapstructure:"max_idle_conns_per_host"`
		MaxConnsPerHost     int           `yaml:"max_conns_per_host" mapstructure:"max_conns_per_host"`
		IdleConnTimeout     time.Duration `yaml:"idle_conn_timeout" mapstructure:"idle_conn_timeout"`
		DialTimeout         time.Duration `yaml:"dial_timeout" mapstructure:"dial_timeout"`
		KeepAlive           time.Duration `yaml:"keep_alive" mapstructure:"keep_alive"`
		DisableCompression  bool          `yaml:"disable_compression" mapstructure:"disable_compression"`
		BufferSize          int           `yaml:"buffer_size" mapstructure:"buffer_size"`
	} `yaml:"transport" mapstructure:"transport"`
	
	// Load balancing
	LoadBalancing struct {
		Algorithm string `yaml:"algorithm" mapstructure:"algorithm"` // round_robin, weighted, least_conn, ip_hash
		Sticky    struct {
			Enabled    bool          `yaml:"enabled" mapstructure:"enabled"`
			CookieName string        `yaml:"cookie_name" mapstructure:"cookie_name"`
			TTL        time.Duration `yaml:"ttl" mapstructure:"ttl"`
		} `yaml:"sticky" mapstructure:"sticky"`
	} `yaml:"load_balancing" mapstructure:"load_balancing"`
	
	// Health checking
	HealthCheck struct {
		Interval      time.Duration `yaml:"interval" mapstructure:"interval"`
		Timeout       time.Duration `yaml:"timeout" mapstructure:"timeout"`
		FailThreshold int           `yaml:"fail_threshold" mapstructure:"fail_threshold"`
		PassThreshold int           `yaml:"pass_threshold" mapstructure:"pass_threshold"`
	} `yaml:"health_check" mapstructure:"health_check"`
	
	// Circuit breaker
	CircuitBreaker struct {
		Enabled          bool          `yaml:"enabled" mapstructure:"enabled"`
		FailureThreshold int           `yaml:"failure_threshold" mapstructure:"failure_threshold"`
		SuccessThreshold int           `yaml:"success_threshold" mapstructure:"success_threshold"`
		Timeout          time.Duration `yaml:"timeout" mapstructure:"timeout"`
	} `yaml:"circuit_breaker" mapstructure:"circuit_breaker"`
	
	// Rate limiting
	RateLimit struct {
		Enabled  bool   `yaml:"enabled" mapstructure:"enabled"`
		RPS      int    `yaml:"rps" mapstructure:"rps"`
		Burst    int    `yaml:"burst" mapstructure:"burst"`
		ByHeader string `yaml:"by_header,omitempty" mapstructure:"by_header,omitempty"`
	} `yaml:"rate_limit" mapstructure:"rate_limit"`
	
	// Middleware configuration
	Middleware struct {
		Compression struct {
			Enabled    bool     `yaml:"enabled" mapstructure:"enabled"`
			Level      int      `yaml:"level" mapstructure:"level"`
			Types      []string `yaml:"types" mapstructure:"types"`
			Algorithms []string `yaml:"algorithms" mapstructure:"algorithms"` // gzip, br, zstd
		} `yaml:"compression" mapstructure:"compression"`
		
		CORS struct {
			Enabled          bool     `yaml:"enabled" mapstructure:"enabled"`
			AllowedOrigins   []string `yaml:"allowed_origins" mapstructure:"allowed_origins"`
			AllowedMethods   []string `yaml:"allowed_methods" mapstructure:"allowed_methods"`
			AllowedHeaders   []string `yaml:"allowed_headers" mapstructure:"allowed_headers"`
			AllowCredentials bool     `yaml:"allow_credentials" mapstructure:"allow_credentials"`
			MaxAge           int      `yaml:"max_age" mapstructure:"max_age"`
		} `yaml:"cors" mapstructure:"cors"`
		
		Headers struct {
			Security bool              `yaml:"security" mapstructure:"security"`
			Custom   map[string]string `yaml:"custom,omitempty" mapstructure:"custom,omitempty"`
			Remove   []string          `yaml:"remove,omitempty" mapstructure:"remove,omitempty"`
		} `yaml:"headers" mapstructure:"headers"`
		
		Auth struct {
			Basic struct {
				Enabled bool              `yaml:"enabled" mapstructure:"enabled"`
				Users   map[string]string `yaml:"users,omitempty" mapstructure:"users,omitempty"`
			} `yaml:"basic" mapstructure:"basic"`
			
			JWT struct {
				Enabled  bool   `yaml:"enabled" mapstructure:"enabled"`
				Issuer   string `yaml:"issuer,omitempty" mapstructure:"issuer,omitempty"`
				Audience string `yaml:"audience,omitempty" mapstructure:"audience,omitempty"`
				KeyFile  string `yaml:"key_file,omitempty" mapstructure:"key_file,omitempty"`
			} `yaml:"jwt" mapstructure:"jwt"`
			
			OAuth2 struct {
				Enabled      bool   `yaml:"enabled" mapstructure:"enabled"`
				Provider     string `yaml:"provider" mapstructure:"provider"`
				ClientID     string `yaml:"client_id,omitempty" mapstructure:"client_id,omitempty"`
				ClientSecret string `yaml:"client_secret,omitempty" mapstructure:"client_secret,omitempty"`
				RedirectURL  string `yaml:"redirect_url,omitempty" mapstructure:"redirect_url,omitempty"`
			} `yaml:"oauth2" mapstructure:"oauth2"`
		} `yaml:"auth" mapstructure:"auth"`
	} `yaml:"middleware" mapstructure:"middleware"`
	
	// Logging and monitoring
	Logging struct {
		Level      string `yaml:"level" mapstructure:"level"`
		Format     string `yaml:"format" mapstructure:"format"` // json, text
		AccessLogs bool   `yaml:"access_logs" mapstructure:"access_logs"`
	} `yaml:"logging" mapstructure:"logging"`
	
	Metrics struct {
		Enabled bool   `yaml:"enabled" mapstructure:"enabled"`
		Path    string `yaml:"path" mapstructure:"path"`
	} `yaml:"metrics" mapstructure:"metrics"`
	
	// Storage backend
	Storage struct {
		Type   string `yaml:"type" mapstructure:"type"` // sqlite, memory, etcd
		DSN    string `yaml:"dsn,omitempty" mapstructure:"dsn,omitempty"`
		Prefix string `yaml:"prefix,omitempty" mapstructure:"prefix,omitempty"`
	} `yaml:"storage" mapstructure:"storage"`
	
	// Admin API
	API struct {
		Enabled bool   `yaml:"enabled" mapstructure:"enabled"`
		Addr    string `yaml:"addr" mapstructure:"addr"`
		Auth    bool   `yaml:"auth" mapstructure:"auth"`
		APIKey  string `yaml:"api_key,omitempty" mapstructure:"api_key,omitempty"`
	} `yaml:"api" mapstructure:"api"`
	
	// Web UI
	UI struct {
		Enabled bool   `yaml:"enabled" mapstructure:"enabled"`
		Path    string `yaml:"path" mapstructure:"path"`
	} `yaml:"ui" mapstructure:"ui"`
}