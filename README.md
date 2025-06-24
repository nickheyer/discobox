# Discobox

A production-grade reverse proxy and service manager built in Go, featuring nginx/Traefik-level capabilities with a focus on simplicity and performance.

## Features

### Core Capabilities
- **Multiple Routing Strategies**: Host-based and path-based routing with regex support
- **Advanced Load Balancing**: Round-robin, weighted, least connections, and IP hash algorithms
- **Health Checking**: Active and passive health monitoring with configurable thresholds
- **Circuit Breaker**: Protect backends from cascading failures
- **WebSocket & SSE Support**: Full duplex communication and server-sent events
- **HTTP/2 & HTTP/3**: Modern protocol support for optimal performance

### Security & Authentication
- **TLS Termination**: Manual certificates or automatic via Let's Encrypt
- **Multiple Auth Methods**: Basic, JWT, and OAuth2 authentication
- **Security Headers**: Automatic security header injection
- **Rate Limiting**: Per-client request throttling with configurable limits

### Performance & Reliability
- **Request Retry**: Exponential backoff for failed requests
- **Connection Pooling**: Optimized backend connection management
- **Compression**: Gzip, Brotli, and Zstandard support
- **Response Caching**: Reduce backend load (coming soon)
- **Graceful Shutdown**: Zero-downtime deployments

### Operations & Monitoring
- **Prometheus Metrics**: Comprehensive performance metrics
- **Structured Logging**: JSON/text logging with configurable levels
- **Dynamic Configuration**: Hot reload without restart
- **Admin API**: RESTful API for runtime management
- **Web UI**: Interactive dashboard for service management

## Quick Start

### Using Docker

```bash
# Clone the repository
git clone https://github.com/nickheyer/discobox.git
cd discobox

# Build and run with Docker Compose
make docker-build
make docker-run

# Access the services
# Proxy: http://localhost:8080 (for your backend services)
# Web UI & Admin API: http://localhost:8081
# Prometheus Metrics: http://localhost:8081/prometheus/metrics
```

### Building from Source

```bash
# Prerequisites: Go 1.21+
make setup      # Install development tools
make build      # Build the binary
make run        # Run with example configuration
```

## Configuration

Discobox uses YAML configuration with hot reload support:

```yaml
# Basic configuration
listen_addr: ":8080"
tls:
  enabled: true
  auto_cert: true
  domains: ["example.com"]
  email: "admin@example.com"

# Load balancing
load_balancing:
  algorithm: "least_conn"
  sticky:
    enabled: true
    cookie_name: "lb_session"

# Define services via config or API
services:
  - id: "my-app"
    name: "My Application"
    endpoints:
      - "http://app1:3000"
      - "http://app2:3000"
    health_path: "/health"
```

See [configs/discobox.yaml](configs/discobox.yaml) for a complete example.

## Architecture

Discobox follows a modular architecture with clear separation of concerns:

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│   Client    │────▶│   Discobox   │────▶│   Backend   │
└─────────────┘     │              │     └─────────────┘
                    │ ┌──────────┐ │
                    │ │ Router   │ │     ┌─────────────┐
                    │ ├──────────┤ │────▶│   Backend   │
                    │ │ LB       │ │     └─────────────┘
                    │ ├──────────┤ │
                    │ │ Circuit  │ │     ┌─────────────┐
                    │ │ Breaker  │ │────▶│   Backend   │
                    │ ├──────────┤ │     └─────────────┘
                    │ │ Middle-  │ │
                    │ │ ware     │ │
                    │ └──────────┘ │
                    └──────────────┘
```

## API Usage

### Service Management

```bash
# Add a service
curl -X POST http://localhost:8081/api/services \
  -H "Content-Type: application/json" \
  -d '{
    "id": "web-app",
    "name": "Web Application",
    "endpoints": ["http://web1:80", "http://web2:80"],
    "health_path": "/health"
  }'

# List services
curl http://localhost:8081/api/services

# Update service
curl -X PUT http://localhost:8081/api/services/web-app \
  -H "Content-Type: application/json" \
  -d '{"active": false}'
```

### Route Management

```bash
# Add a route
curl -X POST http://localhost:8081/api/routes \
  -H "Content-Type: application/json" \
  -d '{
    "id": "web-route",
    "host": "example.com",
    "service_id": "web-app",
    "middlewares": ["compression", "security-headers"]
  }'
```

## Development

### Project Structure

```
discobox/
├── cmd/discobox/          # Main application entry point
├── internal/              # Private application code
│   ├── balancer/         # Load balancing algorithms
│   ├── circuit/          # Circuit breaker implementation
│   ├── middleware/       # HTTP middleware
│   ├── proxy/           # Core proxy logic
│   ├── router/          # Request routing
│   └── storage/         # Configuration storage
├── pkg/                  # Public packages
│   ├── api/             # REST API handlers
│   └── ui/              # Web UI
└── configs/             # Configuration examples
```

### Running Tests

```bash
make test           # Run all tests
make test-unit      # Unit tests only
make test-e2e       # End-to-end tests
make bench          # Run benchmarks
make coverage       # Generate coverage report
```

### Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Run tests (`make test`)
4. Commit your changes (`git commit -m 'Add amazing feature'`)
5. Push to the branch (`git push origin feature/amazing-feature`)
6. Open a Pull Request

## Performance Tuning

### System Limits

```bash
# Increase file descriptor limits
ulimit -n 65535

# Tune kernel parameters
sysctl -w net.core.somaxconn=65535
sysctl -w net.ipv4.tcp_tw_reuse=1
```

### Configuration Optimization

```yaml
transport:
  max_idle_conns_per_host: 100  # Crucial for performance
  max_conns_per_host: 0         # Unlimited
  idle_conn_timeout: 90s
  
load_balancing:
  algorithm: "least_conn"       # Better for varying request times
  
circuit_breaker:
  failure_threshold: 5          # Adjust based on SLA
  timeout: 60s
```

## Production Deployment

### Docker

```bash
# Build production image
docker build -t discobox:latest -f deployments/docker/Dockerfile .

# Run with proper limits
docker run -d \
  --name discobox \
  --memory="2g" \
  --cpus="2.0" \
  -p 80:8080 \
  -p 443:8443 \
  -v /etc/discobox:/etc/discobox \
  -v /var/lib/discobox:/var/lib/discobox \
  discobox:latest
```

### Kubernetes

```bash
# Deploy to Kubernetes
kubectl apply -f deployments/k8s/

# Check status
kubectl get pods -l app=discobox
```

### Systemd

```bash
# Install systemd service
sudo make install

# Start service
sudo systemctl start discobox
sudo systemctl enable discobox
```

## Monitoring

### Prometheus Metrics

- `http_requests_total`: Total HTTP requests
- `http_request_duration_seconds`: Request latency histogram
- `upstream_health_status`: Backend health status
- `active_connections`: Current active connections
- `circuit_breaker_state`: Circuit breaker status

### Grafana Dashboard

Import the provided dashboard from `deployments/grafana/dashboard.json`.

## Comparison

| Feature | Discobox | nginx | Traefik | HAProxy |
|---------|----------|-------|---------|---------|
| Hot Reload | ✓ | ✗ | ✓ | ✗ |
| Auto TLS | ✓ | ✗ | ✓ | ✗ |
| Service Discovery | ✓ | ✗ | ✓ | ✗ |
| Web UI | ✓ | ✗ | ✓ | ✗ |
| API | ✓ | ✗ | ✓ | ✗ |
| WebSockets | ✓ | ✓ | ✓ | ✓ |
| HTTP/3 | ✓ | ✓ | ✗ | ✗ |
| Circuit Breaker | ✓ | ✗ | ✓ | ✗ |

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Acknowledgments

Built with best-in-class Go libraries:
- [gorilla/mux](https://github.com/gorilla/mux) - HTTP router
- [sony/gobreaker](https://github.com/sony/gobreaker) - Circuit breaker
- [caddyserver/certmagic](https://github.com/caddyserver/certmagic) - Automatic HTTPS
- [quic-go/quic-go](https://github.com/quic-go/quic-go) - HTTP/3 support

## Support

- Documentation: [docs/](docs/)
- Issues: [GitHub Issues](https://github.com/nickheyer/discobox/issues)
- Discussions: [GitHub Discussions](https://github.com/nickheyer/discobox/discussions)
