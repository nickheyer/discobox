# Discobox API Documentation

## Overview

The Discobox Admin API provides complete control over the reverse proxy configuration and operation. All API endpoints are RESTful and return JSON responses.

**Base URL**: `http://localhost:8081/api`  
**Authentication**: Required for all endpoints (see Authentication section)  
**Content-Type**: `application/json` for all requests/responses

## Authentication

### POST /api/auth/login
Login to receive an authentication token.

**Request Body:**
```json
{
  "username": "admin",
  "password": "secretpassword"
}
```

**Response (200 OK):**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_at": "2024-01-15T10:00:00Z",
  "user": {
    "username": "admin",
    "role": "admin",
    "permissions": ["read", "write", "delete"]
  }
}
```

**Response (401 Unauthorized):**
```json
{
  "error": "Invalid credentials"
}
```

### POST /api/auth/logout
Invalidate the current token.

**Headers:**
```
Authorization: Bearer <token>
```

**Response (200 OK):**
```json
{
  "message": "Logged out successfully"
}
```

### POST /api/auth/refresh
Refresh an expiring token.

**Headers:**
```
Authorization: Bearer <token>
```

**Response (200 OK):**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_at": "2024-01-15T12:00:00Z"
}
```

## Services

### GET /api/services
List all registered services.

**Query Parameters:**
- `active` (boolean, optional): Filter by active status
- `tag` (string, optional): Filter by metadata tag
- `limit` (integer, optional): Maximum results (default: 100)
- `offset` (integer, optional): Pagination offset (default: 0)

**Response (200 OK):**
```json
{
  "services": [
    {
      "id": "web-app",
      "name": "Web Application",
      "endpoints": [
        "http://web-1:80",
        "http://web-2:80"
      ],
      "health_path": "/health",
      "weight": 1,
      "max_conns": 100,
      "timeout": "30s",
      "metadata": {
        "environment": "production",
        "team": "frontend"
      },
      "tls": null,
      "strip_prefix": true,
      "active": true,
      "created_at": "2024-01-10T08:00:00Z",
      "updated_at": "2024-01-10T08:00:00Z"
    }
  ],
  "total": 1,
  "limit": 100,
  "offset": 0
}
```

### GET /api/services/{id}
Get a specific service by ID.

**Path Parameters:**
- `id` (string, required): Service ID

**Response (200 OK):**
```json
{
  "id": "web-app",
  "name": "Web Application",
  "endpoints": [
    "http://web-1:80",
    "http://web-2:80"
  ],
  "health_path": "/health",
  "weight": 1,
  "max_conns": 100,
  "timeout": "30s",
  "metadata": {
    "environment": "production",
    "team": "frontend"
  },
  "tls": null,
  "strip_prefix": true,
  "active": true,
  "created_at": "2024-01-10T08:00:00Z",
  "updated_at": "2024-01-10T08:00:00Z"
}
```

**Response (404 Not Found):**
```json
{
  "error": "Service not found"
}
```

### POST /api/services
Create a new service.

**Request Body:**
```json
{
  "id": "api-service",
  "name": "API Service",
  "endpoints": [
    "http://api-1:3000",
    "http://api-2:3000",
    "http://api-3:3000"
  ],
  "health_path": "/api/health",
  "weight": 2,
  "max_conns": 200,
  "timeout": "10s",
  "metadata": {
    "environment": "production",
    "team": "backend",
    "version": "2.1.0"
  },
  "tls": {
    "insecure_skip_verify": false,
    "server_name": "api.internal",
    "root_cas": [
      "-----BEGIN CERTIFICATE-----\nMIIC..."
    ]
  },
  "strip_prefix": false,
  "active": true
}
```

**Response (201 Created):**
```json
{
  "id": "api-service",
  "name": "API Service",
  "endpoints": [
    "http://api-1:3000",
    "http://api-2:3000",
    "http://api-3:3000"
  ],
  "health_path": "/api/health",
  "weight": 2,
  "max_conns": 200,
  "timeout": "10s",
  "metadata": {
    "environment": "production",
    "team": "backend",
    "version": "2.1.0"
  },
  "tls": {
    "insecure_skip_verify": false,
    "server_name": "api.internal"
  },
  "strip_prefix": false,
  "active": true,
  "created_at": "2024-01-10T09:00:00Z",
  "updated_at": "2024-01-10T09:00:00Z"
}
```

**Response (400 Bad Request):**
```json
{
  "error": "Invalid service configuration",
  "details": {
    "endpoints": "At least one endpoint required",
    "id": "Must match pattern ^[a-zA-Z0-9-_]+$"
  }
}
```

**Response (409 Conflict):**
```json
{
  "error": "Service with ID 'api-service' already exists"
}
```

### PUT /api/services/{id}
Update an existing service.

**Path Parameters:**
- `id` (string, required): Service ID

**Request Body:** (all fields optional except those being updated)
```json
{
  "name": "Updated API Service",
  "endpoints": [
    "http://api-1:3000",
    "http://api-2:3000",
    "http://api-3:3000",
    "http://api-4:3000"
  ],
  "weight": 3,
  "metadata": {
    "environment": "production",
    "team": "backend",
    "version": "2.2.0"
  }
}
```

**Response (200 OK):** Returns the updated service object

### PATCH /api/services/{id}
Partially update a service (e.g., toggle active state).

**Path Parameters:**
- `id` (string, required): Service ID

**Request Body:**
```json
{
  "active": false
}
```

**Response (200 OK):** Returns the updated service object

### DELETE /api/services/{id}
Delete a service.

**Path Parameters:**
- `id` (string, required): Service ID

**Query Parameters:**
- `force` (boolean, optional): Force delete even if routes reference this service

**Response (204 No Content):** Success, no body

**Response (409 Conflict):**
```json
{
  "error": "Cannot delete service: referenced by 3 routes",
  "routes": ["web-route", "api-route", "admin-route"]
}
```

### POST /api/services/{id}/endpoints
Add endpoints to a service.

**Path Parameters:**
- `id` (string, required): Service ID

**Request Body:**
```json
{
  "endpoints": [
    "http://api-5:3000",
    "http://api-6:3000"
  ]
}
```

**Response (200 OK):** Returns the updated service object

### DELETE /api/services/{id}/endpoints
Remove endpoints from a service.

**Path Parameters:**
- `id` (string, required): Service ID

**Request Body:**
```json
{
  "endpoints": [
    "http://api-5:3000"
  ]
}
```

**Response (200 OK):** Returns the updated service object

## Routes

### GET /api/routes
List all routes.

**Query Parameters:**
- `service_id` (string, optional): Filter by service
- `host` (string, optional): Filter by host
- `priority` (integer, optional): Filter by priority
- `limit` (integer, optional): Maximum results (default: 100)
- `offset` (integer, optional): Pagination offset (default: 0)

**Response (200 OK):**
```json
{
  "routes": [
    {
      "id": "web-route",
      "priority": 100,
      "host": "example.com",
      "path_prefix": "/",
      "path_regex": null,
      "headers": {
        "X-Custom-Header": "value"
      },
      "service_id": "web-app",
      "middlewares": [
        "compression",
        "security-headers"
      ],
      "rewrite_rules": [
        {
          "type": "strip_prefix",
          "pattern": "/api/v1",
          "replacement": ""
        }
      ],
      "metadata": {
        "description": "Main website",
        "owner": "frontend-team"
      }
    }
  ],
  "total": 1,
  "limit": 100,
  "offset": 0
}
```

### GET /api/routes/{id}
Get a specific route by ID.

**Path Parameters:**
- `id` (string, required): Route ID

**Response (200 OK):** Route object (same structure as in list)

### POST /api/routes
Create a new route.

**Request Body:**
```json
{
  "id": "api-v2-route",
  "priority": 90,
  "host": "api.example.com",
  "path_prefix": "/v2/",
  "path_regex": "^/v2/(users|posts)/[0-9]+$",
  "headers": {
    "X-API-Version": "2"
  },
  "service_id": "api-service",
  "middlewares": [
    "compression",
    "cors",
    "rate-limit",
    "jwt-auth"
  ],
  "rewrite_rules": [
    {
      "type": "regex",
      "pattern": "^/v2/(.*)$",
      "replacement": "/api/$1"
    }
  ],
  "metadata": {
    "description": "API v2 endpoints",
    "deprecated": false
  }
}
```

**Response (201 Created):** Created route object

### PUT /api/routes/{id}
Update an existing route.

**Path Parameters:**
- `id` (string, required): Route ID

**Request Body:** Complete route object (all fields)

**Response (200 OK):** Updated route object

### DELETE /api/routes/{id}
Delete a route.

**Path Parameters:**
- `id` (string, required): Route ID

**Response (204 No Content):** Success, no body

### POST /api/routes/reorder
Reorder routes by priority.

**Request Body:**
```json
{
  "routes": [
    {"id": "api-route", "priority": 100},
    {"id": "web-route", "priority": 90},
    {"id": "admin-route", "priority": 80}
  ]
}
```

**Response (200 OK):**
```json
{
  "message": "Routes reordered successfully"
}
```

## Health & Metrics

### GET /api/health
Overall system health.

**Response (200 OK):**
```json
{
  "status": "healthy",
  "version": "1.0.0",
  "uptime": "72h15m30s",
  "services": {
    "total": 5,
    "healthy": 4,
    "unhealthy": 1
  },
  "routes": {
    "total": 12,
    "active": 12
  },
  "storage": {
    "type": "sqlite",
    "status": "connected"
  }
}
```

### GET /api/health/services
Health status of all services.

**Response (200 OK):**
```json
{
  "services": [
    {
      "id": "web-app",
      "name": "Web Application",
      "health": {
        "status": "healthy",
        "endpoints": [
          {
            "url": "http://web-1:80",
            "status": "healthy",
            "latency": "15ms",
            "last_check": "2024-01-10T10:00:00Z"
          },
          {
            "url": "http://web-2:80",
            "status": "healthy",
            "latency": "18ms",
            "last_check": "2024-01-10T10:00:00Z"
          }
        ],
        "consecutive_failures": 0,
        "circuit_breaker": "closed"
      }
    }
  ]
}
```

### GET /api/health/services/{id}
Health status of a specific service.

**Path Parameters:**
- `id` (string, required): Service ID

**Response (200 OK):** Service health object (same structure as in list)

### GET /api/metrics
Aggregated metrics (Prometheus format also available at :8081/metrics).

**Query Parameters:**
- `period` (string, optional): Time period (1h, 24h, 7d, 30d)
- `service_id` (string, optional): Filter by service

**Response (200 OK):**
```json
{
  "period": "1h",
  "requests": {
    "total": 1543234,
    "rate": "428.7/s",
    "by_status": {
      "2xx": 1520000,
      "3xx": 15000,
      "4xx": 8000,
      "5xx": 234
    }
  },
  "latency": {
    "p50": "25ms",
    "p90": "75ms",
    "p95": "120ms",
    "p99": "350ms"
  },
  "upstream_latency": {
    "p50": "20ms",
    "p90": "65ms",
    "p95": "100ms",
    "p99": "300ms"
  },
  "connections": {
    "active": 1250,
    "idle": 750
  },
  "bandwidth": {
    "in": "125.5 MB/s",
    "out": "89.3 MB/s"
  },
  "errors": {
    "total": 234,
    "rate": "0.065/s",
    "by_type": {
      "timeout": 100,
      "connection_refused": 80,
      "circuit_breaker_open": 54
    }
  }
}
```

## Load Balancer

### GET /api/loadbalancer/algorithms
List available load balancing algorithms.

**Response (200 OK):**
```json
{
  "algorithms": [
    {
      "name": "round_robin",
      "description": "Distributes requests evenly across all healthy endpoints"
    },
    {
      "name": "weighted",
      "description": "Distributes requests based on endpoint weights"
    },
    {
      "name": "least_conn",
      "description": "Routes to endpoint with fewest active connections"
    },
    {
      "name": "ip_hash",
      "description": "Routes based on consistent hashing of client IP"
    }
  ],
  "current": "least_conn"
}
```

### GET /api/loadbalancer/stats
Load balancer statistics.

**Response (200 OK):**
```json
{
  "services": {
    "web-app": {
      "algorithm": "round_robin",
      "endpoints": [
        {
          "url": "http://web-1:80",
          "requests": 50234,
          "active_connections": 45,
          "total_connections": 150234,
          "bytes_in": "1.2GB",
          "bytes_out": "850MB",
          "errors": 12
        }
      ]
    }
  }
}
```

### POST /api/loadbalancer/drain
Drain connections from an endpoint.

**Request Body:**
```json
{
  "service_id": "web-app",
  "endpoint": "http://web-1:80",
  "wait_time": "30s"
}
```

**Response (202 Accepted):**
```json
{
  "message": "Draining connections from endpoint",
  "endpoint": "http://web-1:80",
  "current_connections": 45,
  "estimated_completion": "2024-01-10T10:30:00Z"
}
```

## Circuit Breaker

### GET /api/circuit-breaker
Circuit breaker status for all services.

**Response (200 OK):**
```json
{
  "services": {
    "web-app": {
      "state": "closed",
      "failures": 0,
      "successes": 10000,
      "consecutive_failures": 0,
      "last_failure": null
    },
    "api-service": {
      "state": "open",
      "failures": 10,
      "successes": 5000,
      "consecutive_failures": 5,
      "last_failure": "2024-01-10T10:15:00Z",
      "opens_at": "2024-01-10T10:16:00Z"
    }
  }
}
```

### POST /api/circuit-breaker/{service_id}/reset
Manually reset a circuit breaker.

**Path Parameters:**
- `service_id` (string, required): Service ID

**Response (200 OK):**
```json
{
  "message": "Circuit breaker reset",
  "service_id": "api-service",
  "state": "closed"
}
```

## Rate Limiting

### GET /api/ratelimit/status
Current rate limit status.

**Query Parameters:**
- `key` (string, optional): Specific rate limit key (IP, user, etc.)

**Response (200 OK):**
```json
{
  "global": {
    "limit": 10000,
    "remaining": 8543,
    "reset_at": "2024-01-10T11:00:00Z"
  },
  "by_key": {
    "192.168.1.100": {
      "limit": 1000,
      "remaining": 456,
      "reset_at": "2024-01-10T10:30:00Z"
    }
  }
}
```

### PUT /api/ratelimit/rules
Update rate limit rules.

**Request Body:**
```json
{
  "global": {
    "rps": 10000,
    "burst": 20000
  },
  "per_ip": {
    "rps": 100,
    "burst": 200
  },
  "custom": [
    {
      "key": "api-key:premium",
      "rps": 1000,
      "burst": 2000
    }
  ]
}
```

**Response (200 OK):**
```json
{
  "message": "Rate limit rules updated"
}
```

### DELETE /api/ratelimit/keys/{key}
Reset rate limit for a specific key.

**Path Parameters:**
- `key` (string, required): Rate limit key

**Response (204 No Content):** Success, no body

## Middleware

### GET /api/middleware
List all available middleware.

**Response (200 OK):**
```json
{
  "middleware": [
    {
      "id": "compression",
      "name": "Compression",
      "description": "Gzip/Brotli response compression",
      "configurable": true,
      "config_schema": {
        "level": "integer (1-9)",
        "types": "array of mime types",
        "algorithms": "array of algorithms"
      }
    },
    {
      "id": "rate-limit",
      "name": "Rate Limiting",
      "description": "Request rate limiting",
      "configurable": true,
      "config_schema": {
        "rps": "integer",
        "burst": "integer",
        "by_header": "string"
      }
    }
  ]
}
```

### GET /api/middleware/{id}/config
Get middleware configuration.

**Path Parameters:**
- `id` (string, required): Middleware ID

**Response (200 OK):**
```json
{
  "id": "compression",
  "enabled": true,
  "config": {
    "level": 5,
    "types": ["text/html", "application/json"],
    "algorithms": ["br", "gzip"]
  }
}
```

### PUT /api/middleware/{id}/config
Update middleware configuration.

**Path Parameters:**
- `id` (string, required): Middleware ID

**Request Body:**
```json
{
  "enabled": true,
  "config": {
    "level": 6,
    "types": ["text/html", "application/json", "text/css"],
    "algorithms": ["br", "gzip", "zstd"]
  }
}
```

**Response (200 OK):**
```json
{
  "message": "Middleware configuration updated"
}
```

## Configuration

### GET /api/config
Get current configuration (non-sensitive values only).

**Response (200 OK):**
```json
{
  "listen_addr": ":8080",
  "tls": {
    "enabled": true,
    "auto_cert": true,
    "domains": ["example.com"]
  },
  "http2": {
    "enabled": true
  },
  "http3": {
    "enabled": false
  },
  "load_balancing": {
    "algorithm": "least_conn",
    "sticky": {
      "enabled": true,
      "cookie_name": "lb_session"
    }
  }
}
```

### PATCH /api/config
Update runtime configuration (limited subset).

**Request Body:**
```json
{
  "logging": {
    "level": "debug"
  },
  "rate_limit": {
    "enabled": false
  }
}
```

**Response (200 OK):**
```json
{
  "message": "Configuration updated",
  "restart_required": false
}
```

### POST /api/config/reload
Reload configuration from file.

**Response (200 OK):**
```json
{
  "message": "Configuration reloaded",
  "changes": [
    "Updated TLS domains",
    "Changed rate limit from 1000 to 2000 RPS"
  ]
}
```

### POST /api/config/validate
Validate a configuration without applying it.

**Request Body:** Complete configuration object

**Response (200 OK):**
```json
{
  "valid": true,
  "warnings": [
    "HTTP/3 is experimental"
  ]
}
```

**Response (400 Bad Request):**
```json
{
  "valid": false,
  "errors": [
    "Invalid TLS configuration: cert_file required when auto_cert is false"
  ]
}
```

## TLS/Certificates

### GET /api/tls/certificates
List all certificates.

**Response (200 OK):**
```json
{
  "certificates": [
    {
      "id": "example-com",
      "domains": ["example.com", "*.example.com"],
      "issuer": "Let's Encrypt",
      "subject": "CN=example.com",
      "not_before": "2024-01-01T00:00:00Z",
      "not_after": "2024-04-01T00:00:00Z",
      "auto_renew": true,
      "status": "active"
    }
  ]
}
```

### POST /api/tls/certificates
Upload a manual certificate.

**Request Body:**
```json
{
  "domains": ["custom.example.com"],
  "certificate": "-----BEGIN CERTIFICATE-----\n...",
  "private_key": "-----BEGIN PRIVATE KEY-----\n...",
  "chain": "-----BEGIN CERTIFICATE-----\n..."
}
```

**Response (201 Created):**
```json
{
  "id": "custom-example-com",
  "domains": ["custom.example.com"],
  "issuer": "Custom CA",
  "not_after": "2025-01-01T00:00:00Z"
}
```

### DELETE /api/tls/certificates/{id}
Remove a certificate.

**Path Parameters:**
- `id` (string, required): Certificate ID

**Response (204 No Content):** Success, no body

### POST /api/tls/certificates/{id}/renew
Manually trigger certificate renewal.

**Path Parameters:**
- `id` (string, required): Certificate ID

**Response (202 Accepted):**
```json
{
  "message": "Certificate renewal initiated",
  "estimated_completion": "2024-01-10T10:35:00Z"
}
```

## Logs

### GET /api/logs
Retrieve recent logs.

**Query Parameters:**
- `level` (string, optional): Minimum log level (debug, info, warn, error)
- `service_id` (string, optional): Filter by service
- `limit` (integer, optional): Maximum entries (default: 100)
- `since` (string, optional): RFC3339 timestamp
- `until` (string, optional): RFC3339 timestamp

**Response (200 OK):**
```json
{
  "logs": [
    {
      "timestamp": "2024-01-10T10:00:00Z",
      "level": "error",
      "message": "Backend connection failed",
      "fields": {
        "service_id": "api-service",
        "endpoint": "http://api-1:3000",
        "error": "connection refused",
        "attempt": 3
      }
    }
  ],
  "total": 1,
  "limit": 100
}
```

### GET /api/logs/stream
Stream logs in real-time (Server-Sent Events).

**Query Parameters:** Same as GET /api/logs

**Response (200 OK):** Server-Sent Events stream
```
event: log
data: {"timestamp":"2024-01-10T10:00:00Z","level":"info","message":"Request processed","fields":{...}}

event: log
data: {"timestamp":"2024-01-10T10:00:01Z","level":"error","message":"Backend timeout","fields":{...}}
```

## Sessions & Connections

### GET /api/sessions
List active sessions (for sticky sessions).

**Query Parameters:**
- `service_id` (string, optional): Filter by service
- `limit` (integer, optional): Maximum results

**Response (200 OK):**
```json
{
  "sessions": [
    {
      "id": "sess_abc123",
      "client_ip": "192.168.1.100",
      "service_id": "web-app",
      "endpoint": "http://web-1:80",
      "created_at": "2024-01-10T09:00:00Z",
      "last_used": "2024-01-10T10:00:00Z",
      "requests": 150
    }
  ],
  "total": 1
}
```

### DELETE /api/sessions/{id}
Force remove a session.

**Path Parameters:**
- `id` (string, required): Session ID

**Response (204 No Content):** Success, no body

### GET /api/connections
Active connection details.

**Response (200 OK):**
```json
{
  "total": 2500,
  "by_service": {
    "web-app": {
      "active": 1500,
      "idle": 500,
      "endpoints": {
        "http://web-1:80": {
          "active": 750,
          "idle": 250
        },
        "http://web-2:80": {
          "active": 750,
          "idle": 250
        }
      }
    }
  },
  "by_client": {
    "top_10": [
      {
        "ip": "192.168.1.100",
        "connections": 50,
        "bandwidth": "5.2 MB/s"
      }
    ]
  }
}
```

## Import/Export

### GET /api/export
Export complete configuration.

**Query Parameters:**
- `format` (string, optional): Export format (yaml, json, toml)
- `include_stats` (boolean, optional): Include statistics

**Response (200 OK):**
```yaml
# Complete configuration export
version: "1.0"
exported_at: "2024-01-10T10:00:00Z"
services:
  - id: web-app
    name: Web Application
    # ... full service config
routes:
  - id: web-route
    # ... full route config
```

### POST /api/import
Import configuration.

**Query Parameters:**
- `mode` (string, optional): Import mode (merge, replace)
- `dry_run` (boolean, optional): Validate without applying

**Request Body:** Configuration in YAML/JSON format

**Response (200 OK):**
```json
{
  "message": "Configuration imported successfully",
  "changes": {
    "services": {
      "added": 2,
      "updated": 1,
      "deleted": 0
    },
    "routes": {
      "added": 3,
      "updated": 2,
      "deleted": 1
    }
  }
}
```

## Maintenance

### POST /api/maintenance/mode
Enable/disable maintenance mode.

**Request Body:**
```json
{
  "enabled": true,
  "message": "Scheduled maintenance in progress",
  "allowed_ips": ["10.0.0.0/8"],
  "bypass_header": "X-Maintenance-Bypass",
  "bypass_value": "secret-token"
}
```

**Response (200 OK):**
```json
{
  "message": "Maintenance mode enabled"
}
```

### POST /api/maintenance/backup
Create configuration backup.

**Response (200 OK):**
```json
{
  "backup_id": "backup-20240110-100000",
  "size": "1.2MB",
  "location": "/var/lib/discobox/backups/backup-20240110-100000.db"
}
```

### POST /api/maintenance/restore
Restore from backup.

**Request Body:**
```json
{
  "backup_id": "backup-20240110-100000"
}
```

**Response (200 OK):**
```json
{
  "message": "Backup restored successfully",
  "restart_required": true
}
```

## Error Responses

All endpoints may return these standard error responses:

### 400 Bad Request
```json
{
  "error": "Invalid request",
  "details": {
    "field": "Description of what's wrong"
  }
}
```

### 401 Unauthorized
```json
{
  "error": "Authentication required"
}
```

### 403 Forbidden
```json
{
  "error": "Insufficient permissions",
  "required_permission": "admin"
}
```

### 404 Not Found
```json
{
  "error": "Resource not found"
}
```

### 409 Conflict
```json
{
  "error": "Resource conflict",
  "details": "Service ID already exists"
}
```

### 429 Too Many Requests
```json
{
  "error": "Rate limit exceeded",
  "retry_after": 60
}
```

### 500 Internal Server Error
```json
{
  "error": "Internal server error",
  "request_id": "req_abc123",
  "message": "Please contact support with request ID"
}
```

### 503 Service Unavailable
```json
{
  "error": "Service temporarily unavailable",
  "reason": "Database maintenance",
  "retry_after": 300
}
```

## WebSocket Endpoints

### WS /api/ws/events
Real-time event stream.

**Query Parameters:**
- `events` (string, optional): Comma-separated event types to subscribe

**Message Types:**

**Service Health Change:**
```json
{
  "type": "service.health",
  "timestamp": "2024-01-10T10:00:00Z",
  "data": {
    "service_id": "api-service",
    "endpoint": "http://api-1:3000",
    "old_status": "healthy",
    "new_status": "unhealthy",
    "reason": "health check failed"
  }
}
```

**Configuration Change:**
```json
{
  "type": "config.changed",
  "timestamp": "2024-01-10T10:00:00Z",
  "data": {
    "section": "rate_limit",
    "changes": ["rps changed from 1000 to 2000"]
  }
}
```

**Circuit Breaker State:**
```json
{
  "type": "circuit_breaker.state_change",
  "timestamp": "2024-01-10T10:00:00Z",
  "data": {
    "service_id": "api-service",
    "old_state": "closed",
    "new_state": "open",
    "reason": "consecutive failures exceeded threshold"
  }
}
```

## Response Headers

All API responses include these headers:

- `X-Request-ID`: Unique request identifier for tracing
- `X-RateLimit-Limit`: Rate limit maximum
- `X-RateLimit-Remaining`: Remaining requests
- `X-RateLimit-Reset`: Unix timestamp when limit resets
- `X-Response-Time`: Server processing time in milliseconds
