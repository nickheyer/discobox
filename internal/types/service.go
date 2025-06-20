package types

import (
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

// TLSConfig for backend connections
type TLSConfig struct {
	Enabled            bool     `json:"enabled" yaml:"enabled"`
	InsecureSkipVerify bool     `json:"insecure_skip_verify" yaml:"insecure_skip_verify"`
	ServerName         string   `json:"server_name,omitempty" yaml:"server_name,omitempty"`
	RootCAs            []string `json:"root_cas,omitempty" yaml:"root_cas,omitempty"`
	ClientCert         string   `json:"client_cert,omitempty" yaml:"client_cert,omitempty"`
	ClientKey          string   `json:"client_key,omitempty" yaml:"client_key,omitempty"`
}

// ServiceStatus represents the health status of a service
type ServiceStatus string

const (
	ServiceStatusHealthy   ServiceStatus = "healthy"
	ServiceStatusUnhealthy ServiceStatus = "unhealthy"
	ServiceStatusUnknown   ServiceStatus = "unknown"
)

// IsHealthy returns true if the service is healthy and active
func (s *Service) IsHealthy() bool {
	return s.Active
}

// GetEndpointCount returns the number of endpoints for the service
func (s *Service) GetEndpointCount() int {
	return len(s.Endpoints)
}

// HasTLS returns true if the service has TLS configuration
func (s *Service) HasTLS() bool {
	return s.TLS != nil && s.TLS.Enabled
}
