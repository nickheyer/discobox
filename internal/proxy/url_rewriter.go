package proxy

import (
	"discobox/internal/types"
	"net/http"
	"regexp"
	"strings"
	"sync"
)


// urlRewriter implements URL rewriting functionality
type urlRewriter struct {
	mu       sync.RWMutex
	compiled map[string]*regexp.Regexp
}

// NewURLRewriter creates a new URL rewriter
func NewURLRewriter() types.URLRewriter {
	return &urlRewriter{
		compiled: make(map[string]*regexp.Regexp),
	}
}

// Rewrite modifies the request URL based on rules
func (r *urlRewriter) Rewrite(req *http.Request, rules []types.RewriteRule) error {
	for _, rule := range rules {
		switch rule.Type {
		case "regex":
			if err := r.rewriteRegex(req, rule); err != nil {
				return err
			}
		case "prefix":
			r.rewritePrefix(req, rule)
		case "strip_prefix":
			r.stripPrefix(req, rule)
		}
	}
	return nil
}

// rewriteRegex performs regex-based URL rewriting
func (r *urlRewriter) rewriteRegex(req *http.Request, rule types.RewriteRule) error {
	regex, err := r.getOrCompileRegex(rule.Pattern)
	if err != nil {
		return err
	}
	
	// Apply regex replacement to the path
	newPath := regex.ReplaceAllString(req.URL.Path, rule.Replacement)
	if newPath != req.URL.Path {
		req.URL.Path = newPath
		req.URL.RawPath = ""
		
		// Update RequestURI
		req.RequestURI = req.URL.RequestURI()
	}
	
	return nil
}

// rewritePrefix performs prefix-based URL rewriting
func (r *urlRewriter) rewritePrefix(req *http.Request, rule types.RewriteRule) {
	if strings.HasPrefix(req.URL.Path, rule.Pattern) {
		newPath := rule.Replacement + req.URL.Path[len(rule.Pattern):]
		req.URL.Path = newPath
		req.URL.RawPath = ""
		req.RequestURI = req.URL.RequestURI()
	}
}

// stripPrefix removes a prefix from the URL path
func (r *urlRewriter) stripPrefix(req *http.Request, rule types.RewriteRule) {
	req.URL.Path = strings.TrimPrefix(req.URL.Path, rule.Pattern)
	if !strings.HasPrefix(req.URL.Path, "/") {
		req.URL.Path = "/" + req.URL.Path
	}
	req.URL.RawPath = ""
	req.RequestURI = req.URL.RequestURI()
}

// getOrCompileRegex returns a compiled regex, caching it for reuse
func (r *urlRewriter) getOrCompileRegex(pattern string) (*regexp.Regexp, error) {
	r.mu.RLock()
	regex, exists := r.compiled[pattern]
	r.mu.RUnlock()
	
	if exists {
		return regex, nil
	}
	
	r.mu.Lock()
	defer r.mu.Unlock()
	
	// Double-check after acquiring write lock
	if regex, exists := r.compiled[pattern]; exists {
		return regex, nil
	}
	
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	
	r.compiled[pattern] = regex
	return regex, nil
}

// PathRewriter provides more advanced path rewriting capabilities
type PathRewriter struct {
	rules []compiledRewriteRule
}

// compiledRewriteRule represents a compiled rewrite rule
type compiledRewriteRule struct {
	Pattern     *regexp.Regexp
	Replacement string
	StripPrefix string
	AddPrefix   string
}

// NewPathRewriter creates a new path rewriter
func NewPathRewriter(rules []compiledRewriteRule) *PathRewriter {
	return &PathRewriter{rules: rules}
}

// Rewrite applies all rules to the request path
func (pr *PathRewriter) Rewrite(req *http.Request) string {
	path := req.URL.Path
	
	for _, rule := range pr.rules {
		// Strip prefix
		if rule.StripPrefix != "" && strings.HasPrefix(path, rule.StripPrefix) {
			path = path[len(rule.StripPrefix):]
			if !strings.HasPrefix(path, "/") {
				path = "/" + path
			}
		}
		
		// Apply regex replacement
		if rule.Pattern != nil {
			path = rule.Pattern.ReplaceAllString(path, rule.Replacement)
		}
		
		// Add prefix
		if rule.AddPrefix != "" {
			if !strings.HasSuffix(rule.AddPrefix, "/") && !strings.HasPrefix(path, "/") {
				path = rule.AddPrefix + "/" + path
			} else {
				path = rule.AddPrefix + path
			}
		}
	}
	
	return path
}
