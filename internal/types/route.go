package types

import (
	"net/http"
	"strings"
)

// Route represents a routing rule
type Route struct {
	ID           string            `json:"id" yaml:"id"`
	Priority     int               `json:"priority" yaml:"priority"`
	Host         string            `json:"host,omitempty" yaml:"host,omitempty"`
	PathPrefix   string            `json:"path_prefix,omitempty" yaml:"path_prefix,omitempty"`
	PathRegex    string            `json:"path_regex,omitempty" yaml:"path_regex,omitempty"`
	Headers      map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	ServiceID    string            `json:"service_id" yaml:"service_id"`
	Middlewares  []string          `json:"middlewares" yaml:"middlewares"`
	RewriteRules []RewriteRule     `json:"rewrite_rules,omitempty" yaml:"rewrite_rules,omitempty"`
	Metadata     map[string]any    `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// RewriteRule defines URL rewriting rules
type RewriteRule struct {
	Type        string `json:"type" yaml:"type"` // regex, prefix, strip_prefix
	Pattern     string `json:"pattern" yaml:"pattern"`
	Replacement string `json:"replacement,omitempty" yaml:"replacement,omitempty"`
}

// MatchesHost returns true if the route matches the given host
func (r *Route) MatchesHost(host string) bool {
	if r.Host == "" {
		return true // No host constraint means match all hosts
	}

	// Remove port from host if present
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}

	// Exact match or wildcard match
	if r.Host == host {
		return true
	}

	// Support wildcard domains like *.example.com
	if strings.HasPrefix(r.Host, "*.") {
		suffix := r.Host[1:] // Remove the * to get .example.com
		return strings.HasSuffix(host, suffix)
	}

	return false
}

// MatchesPath returns true if the route matches the given path
func (r *Route) MatchesPath(path string) bool {
	// If both prefix and regex are empty, match all paths
	if r.PathPrefix == "" && r.PathRegex == "" {
		return true
	}

	// Check prefix match
	if r.PathPrefix != "" {
		if !strings.HasPrefix(path, r.PathPrefix) {
			return false
		}
	}

	// Regex matching is done by the router with compiled regex
	return true
}

// MatchesHeaders returns true if the route matches the given headers
func (r *Route) MatchesHeaders(headers http.Header) bool {
	if len(r.Headers) == 0 {
		return true
	}

	for key, value := range r.Headers {
		headerValue := headers.Get(key)
		if headerValue != value {
			return false
		}
	}

	return true
}

// HasMiddleware returns true if the route has the specified middleware
func (r *Route) HasMiddleware(name string) bool {
	for _, mw := range r.Middlewares {
		if mw == name {
			return true
		}
	}
	return false
}

// GetRewriteRule returns the rewrite rule of the specified type
func (r *Route) GetRewriteRule(ruleType string) *RewriteRule {
	for _, rule := range r.RewriteRules {
		if rule.Type == ruleType {
			return &rule
		}
	}
	return nil
}
