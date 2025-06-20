package types

import (
	"net/url"
	"time"
)

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