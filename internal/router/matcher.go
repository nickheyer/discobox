package router

import (
	"discobox/internal/types"
	"net/http"
	"regexp"
	"strings"
)

// Matcher provides route matching functionality
type Matcher struct {
	compiledRegex map[string]*regexp.Regexp
}

// NewMatcher creates a new matcher instance
func NewMatcher() *Matcher {
	return &Matcher{
		compiledRegex: make(map[string]*regexp.Regexp),
	}
}

// MatchRoute checks if a request matches a route
func (m *Matcher) MatchRoute(req *http.Request, route *types.Route) (bool, map[string]string) {
	params := make(map[string]string)
	
	// Match host
	if route.Host != "" && !m.matchHost(req.Host, route.Host) {
		return false, nil
	}
	
	// Match path
	pathMatched, pathParams := m.matchPath(req.URL.Path, route)
	if !pathMatched {
		return false, nil
	}
	
	// Add path parameters
	for k, v := range pathParams {
		params[k] = v
	}
	
	// Match headers
	if !m.matchHeaders(req, route.Headers) {
		return false, nil
	}
	
	return true, params
}

// matchHost checks if the request host matches the route host pattern
func (m *Matcher) matchHost(reqHost, routeHost string) bool {
	// Remove port from request host
	if idx := strings.LastIndex(reqHost, ":"); idx != -1 {
		reqHost = reqHost[:idx]
	}
	
	// Exact match
	if reqHost == routeHost {
		return true
	}
	
	// Wildcard subdomain match (*.example.com)
	if strings.HasPrefix(routeHost, "*.") {
		suffix := routeHost[1:] // Remove *
		return strings.HasSuffix(reqHost, suffix)
	}
	
	// Regex match ({subdomain:[a-z]+}.example.com)
	if strings.Contains(routeHost, "{") && strings.Contains(routeHost, "}") {
		regex := m.getOrCompileRegex("host:"+routeHost, m.convertPatternToRegex(routeHost))
		if regex != nil {
			return regex.MatchString(reqHost)
		}
	}
	
	return false
}

// matchPath checks if the request path matches the route path pattern
func (m *Matcher) matchPath(reqPath string, route *types.Route) (bool, map[string]string) {
	params := make(map[string]string)
	
	// Path prefix match
	if route.PathPrefix != "" {
		if !strings.HasPrefix(reqPath, route.PathPrefix) {
			return false, nil
		}
		// If only prefix is specified, it's a match
		if route.PathRegex == "" {
			return true, params
		}
	}
	
	// Path regex match
	if route.PathRegex != "" {
		regex := m.getOrCompileRegex("path:"+route.ID, route.PathRegex)
		if regex == nil {
			return false, nil
		}
		
		matches := regex.FindStringSubmatch(reqPath)
		if matches == nil {
			return false, nil
		}
		
		// Extract named groups
		for i, name := range regex.SubexpNames() {
			if i > 0 && i < len(matches) && name != "" {
				params[name] = matches[i]
			}
		}
		
		return true, params
	}
	
	// No path constraints means match
	return route.PathPrefix == "" && route.PathRegex == "", params
}

// matchHeaders checks if request headers match route requirements
func (m *Matcher) matchHeaders(req *http.Request, routeHeaders map[string]string) bool {
	if len(routeHeaders) == 0 {
		return true
	}
	
	for key, value := range routeHeaders {
		reqValue := req.Header.Get(key)
		
		// Exact match
		if reqValue == value {
			continue
		}
		
		// Regex match
		if strings.HasPrefix(value, "~") {
			pattern := value[1:] // Remove ~ prefix
			regex := m.getOrCompileRegex("header:"+key+":"+pattern, pattern)
			if regex != nil && regex.MatchString(reqValue) {
				continue
			}
		}
		
		// No match
		return false
	}
	
	return true
}

// getOrCompileRegex returns a compiled regex, caching it for reuse
func (m *Matcher) getOrCompileRegex(key, pattern string) *regexp.Regexp {
	if regex, exists := m.compiledRegex[key]; exists {
		return regex
	}
	
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	
	m.compiledRegex[key] = regex
	return regex
}

// convertPatternToRegex converts a pattern like {id:[0-9]+} to a regex
func (m *Matcher) convertPatternToRegex(pattern string) string {
	// This is a simplified version
	// In production, you'd want more sophisticated pattern parsing
	
	// Replace {name:pattern} with (?P<name>pattern)
	result := pattern
	
	// Find all {name:pattern} occurrences
	re := regexp.MustCompile(`\{([^:}]+):([^}]+)\}`)
	result = re.ReplaceAllString(result, `(?P<$1>$2)`)
	
	// Replace {name} with (?P<name>[^/]+)
	re = regexp.MustCompile(`\{([^}]+)\}`)
	result = re.ReplaceAllString(result, `(?P<$1>[^/]+)`)
	
	// Escape dots
	result = strings.ReplaceAll(result, ".", `\.`)
	
	// Add anchors
	return "^" + result + "$"
}

// RouteScore calculates a specificity score for a route
// Higher scores indicate more specific routes
func RouteScore(route *types.Route) int {
	score := 0
	
	// Priority is the primary factor
	score += route.Priority * 1000
	
	// Host specificity
	if route.Host != "" {
		if !strings.Contains(route.Host, "*") && !strings.Contains(route.Host, "{") {
			score += 100 // Exact host match
		} else {
			score += 50 // Pattern host match
		}
	}
	
	// Path specificity
	if route.PathRegex != "" {
		score += 30 // Regex is more specific than prefix
	} else if route.PathPrefix != "" {
		score += 20 + len(route.PathPrefix) // Longer prefixes are more specific
	}
	
	// Header requirements add specificity
	score += len(route.Headers) * 10
	
	return score
}

// SortRoutesBySpecificity sorts routes from most to least specific
func SortRoutesBySpecificity(routes []*types.Route) {
	// Implement a stable sort based on route scores
	scores := make(map[string]int)
	for _, route := range routes {
		scores[route.ID] = RouteScore(route)
	}
	
	// Sort routes by score (descending)
	for i := 0; i < len(routes)-1; i++ {
		for j := 0; j < len(routes)-i-1; j++ {
			if scores[routes[j].ID] < scores[routes[j+1].ID] {
				routes[j], routes[j+1] = routes[j+1], routes[j]
			}
		}
	}
}
