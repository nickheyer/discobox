package proxy

import (
	"fmt"
	"net"
	"strings"
	"time"

	"net/http"

	"discobox/internal/types"
)

// Director modifies requests before they are sent to the backend
type Director struct {
	preserveHost bool
	rewriter     types.URLRewriter
}

// NewDirector creates a new request director
func NewDirector(preserveHost bool, rewriter types.URLRewriter) *Director {
	return &Director{
		preserveHost: preserveHost,
		rewriter:     rewriter,
	}
}

// Direct modifies the request for the backend
func (d *Director) Direct(req *http.Request, backend *types.Server, route *types.Route) {
	// Set the scheme and host
	req.URL.Scheme = backend.URL.Scheme
	req.URL.Host = backend.URL.Host

	// Handle host header
	if d.preserveHost {
		req.Host = req.Header.Get("Host")
	} else {
		req.Host = backend.URL.Host
	}

	// Clear RequestURI to avoid issues
	req.RequestURI = ""

	// Add forwarding headers
	d.addForwardingHeaders(req)

	// Apply rewrite rules if configured
	if d.rewriter != nil && route != nil && len(route.RewriteRules) > 0 {
		d.rewriter.Rewrite(req, route.RewriteRules)
	}

	// Add backend-specific headers
	for k, v := range backend.Metadata {
		if strings.HasPrefix(k, "header:") {
			req.Header.Set(k[7:], v)
		}
	}

	// Remove hop-by-hop headers
	d.removeHopHeaders(req.Header)
}

// addForwardingHeaders adds X-Forwarded-* and related headers
func (d *Director) addForwardingHeaders(req *http.Request) {
	// X-Real-IP
	if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		if req.Header.Get("X-Real-IP") == "" {
			req.Header.Set("X-Real-IP", clientIP)
		}

		// X-Forwarded-For
		if prior, ok := req.Header["X-Forwarded-For"]; ok {
			clientIP = strings.Join(prior, ", ") + ", " + clientIP
		}
		req.Header.Set("X-Forwarded-For", clientIP)
	}

	// X-Forwarded-Proto
	if req.TLS != nil {
		req.Header.Set("X-Forwarded-Proto", "https")
	} else {
		req.Header.Set("X-Forwarded-Proto", "http")
	}

	// X-Forwarded-Host
	if req.Header.Get("X-Forwarded-Host") == "" {
		req.Header.Set("X-Forwarded-Host", req.Host)
	}

	// X-Forwarded-Port
	if port := d.getPort(req); port != "" {
		req.Header.Set("X-Forwarded-Port", port)
	}
}

// getPort extracts the port from the request
func (d *Director) getPort(req *http.Request) string {
	if _, port, err := net.SplitHostPort(req.Host); err == nil {
		return port
	}

	// Default ports
	if req.TLS != nil {
		return "443"
	}
	return "80"
}

// removeHopHeaders removes hop-by-hop headers that shouldn't be forwarded
func (d *Director) removeHopHeaders(header http.Header) {
	// Standard hop-by-hop headers
	hopHeaders := []string{
		"Connection",
		"Proxy-Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",
		"Trailer",
		"Transfer-Encoding",
		"Upgrade",
	}

	for _, h := range hopHeaders {
		header.Del(h)
	}

	// Remove headers mentioned in Connection header
	if c := header.Get("Connection"); c != "" {
		for _, f := range strings.Split(c, ",") {
			if f = strings.TrimSpace(f); f != "" {
				header.Del(f)
			}
		}
	}
}

// RequestModifier allows custom request modifications
type RequestModifier interface {
	ModifyRequest(req *http.Request) error
}

// RequestModifierFunc is a function adapter for RequestModifier
type RequestModifierFunc func(*http.Request) error

func (f RequestModifierFunc) ModifyRequest(req *http.Request) error {
	return f(req)
}

// ChainRequestModifiers chains multiple request modifiers
func ChainRequestModifiers(modifiers ...RequestModifier) RequestModifier {
	return RequestModifierFunc(func(req *http.Request) error {
		for _, modifier := range modifiers {
			if err := modifier.ModifyRequest(req); err != nil {
				return err
			}
		}
		return nil
	})
}

// CommonRequestModifiers returns commonly used request modifiers
func CommonRequestModifiers() []RequestModifier {
	return []RequestModifier{
		// Add request ID
		RequestModifierFunc(func(req *http.Request) error {
			if req.Header.Get("X-Request-ID") == "" {
				req.Header.Set("X-Request-ID", generateRequestID())
			}
			return nil
		}),

		// Remove sensitive headers
		RequestModifierFunc(func(req *http.Request) error {
			req.Header.Del("Cookie")
			req.Header.Del("Authorization")
			return nil
		}),
	}
}

// generateRequestID generates a unique request ID
func generateRequestID() string {
	// Simple implementation - in production use UUID
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
