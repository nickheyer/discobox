discobox/
├── cmd/
│   └── discobox/
│       └── main.go                   # Entry point
├── internal/
│   ├── balancer/                     # Load balancing algorithms
│   │   ├── balancer.go               # Interface definition
│   │   ├── round_robin.go            # Round-robin implementation
│   │   ├── weighted_round_robin.go   # Weighted round-robin
│   │   ├── least_connections.go      # Least connections
│   │   ├── ip_hash.go                # IP hash using consistent hashing
│   │   └── sticky_session.go         # Cookie-based session affinity
│   ├── circuit/                      # Circuit breaker implementation
│   │   ├── breaker.go                # Circuit breaker with sony/gobreaker
│   │   └── health_checker.go         # Active & passive health checks
│   ├── config/                       # Configuration management
│   │   ├── config.go                 # Config structures
│   │   ├── loader.go                 # Config loading with viper
│   │   ├── watcher.go                # Hot reload with fsnotify
│   │   └── validator.go              # Config validation
│   ├── middleware/                   # HTTP middleware
│   │   ├── auth/                     # Authentication middleware
│   │   │   ├── basic.go              # Basic auth
│   │   │   ├── jwt.go                # JWT validation
│   │   │   └── oauth2.go             # OAuth2 proxy
│   │   ├── chain.go                  # Middleware chaining
│   │   ├── compression.go            # Gzip/Brotli compression
│   │   ├── cors.go                   # CORS handling
│   │   ├── headers.go                # Security headers & forwarding
│   │   ├── logging.go                # Access logging
│   │   ├── metrics.go                # Prometheus metrics
│   │   ├── ratelimit.go              # Rate limiting
│   │   └── retry.go                  # Retry with backoff
│   ├── proxy/                        # Core proxy functionality
│   │   ├── director.go               # Request director/rewriter
│   │   ├── proxy.go                  # Main proxy implementation
│   │   ├── transport.go              # Custom transport with H2/H3
│   │   ├── url_rewriter.go           # URL rewriting rules
│   │   └── websocket.go              # WebSocket/SSE support
│   ├── router/                       # Routing implementation
│   │   ├── router.go                 # Main router interface
│   │   ├── host_router.go            # Host-based routing
│   │   ├── path_router.go            # Path-based routing with gorilla/mux
│   │   └── matcher.go                # Route matching logic
│   ├── server/                       # HTTP/HTTPS server
│   │   ├── server.go                 # Main server with graceful shutdown
│   │   ├── tls.go                    # TLS configuration
│   │   ├── acme.go                   # ACME/Let's Encrypt with certmagic
│   │   ├── http2.go                  # HTTP/2 configuration
│   │   └── http3.go                  # HTTP/3 with quic-go
│   ├── storage/                      # Service configuration storage
│   │   ├── storage.go                # Storage interface
│   │   ├── sqlite.go                 # SQLite implementation with GORM
│   │   ├── memory.go                 # In-memory for testing
│   │   └── etcd.go                   # etcd support (future)
│   └── types/                        # Common types
│       ├── service.go                # Service definition
│       ├── route.go                  # Route definition
│       └── errors.go                 # Custom errors
        ... OTHER COMMON TYPES ...
├── pkg/                              # Public packages
│   ├── api/                          # REST API
│   │   ├── handler.go                # API handlers
│   │   ├── middleware.go             # API-specific middleware
│   │   └── models.go                 # API request/response models
│   └── ui/                           # Web UI
│       └── handler.go                # UI file serving
├── configs/                          # Configuration files
│   ├── discobox.yaml                 # Main config example
│   └── routes.yaml                   # Routes config example
├── deployments/                      # Deployment configurations
│   ├── docker/
│   │   ├── Dockerfile
│   │   └── docker-compose.yml
│   ├── k8s/                          # Kubernetes manifests
│   │   ├── deployment.yaml
│   │   ├── service.yaml
│   │   └── configmap.yaml
│   └── systemd/
│       └── discobox.service
├── docs/                             # Documentation
├── scripts/                          # Build and utility scripts
├── static/                           # Static files for UI
├── test/                             # Integration tests
├── go.mod
├── go.sum
├── Makefile
├── README.md
├── LICENSE
└── .gitignore

Key Design Decisions:
1. Modular architecture with clear separation of concerns
2. Interface-based design for extensibility
3. All features from the implementation guide included
4. Structured for both development and production use
5. Test infrastructure included from the start
