package proxy

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"crypto/tls"
	"net/http"
	"net/http/httputil"

	"discobox/internal/types"
)

// WebSocketProxy handles WebSocket connections
type WebSocketProxy struct {
	logger    types.Logger
	tlsConfig *tls.Config
	dialer    *net.Dialer
}

// NewWebSocketProxy creates a new WebSocket proxy
func NewWebSocketProxy(logger types.Logger) *WebSocketProxy {
	return &WebSocketProxy{
		logger: logger,
		dialer: &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		},
	}
}

// ServeHTTP handles WebSocket upgrade requests
func (wp *WebSocketProxy) ServeHTTP(w http.ResponseWriter, r *http.Request, backend *types.Server) {
	if !wp.isWebSocketRequest(r) {
		http.Error(w, "Not a WebSocket request", http.StatusBadRequest)
		return
	}

	// Hijack the connection
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "WebSocket hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, clientBuf, err := hijacker.Hijack()
	if err != nil {
		wp.logger.Error("failed to hijack connection", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	// Connect to backend
	backendConn, err := wp.connectToBackend(backend, r)
	if err != nil {
		wp.logger.Error("failed to connect to backend", "error", err, "backend", backend.URL.String())
		wp.writeError(clientBuf, http.StatusBadGateway)
		return
	}
	defer backendConn.Close()

	// Forward the original request
	if err := wp.forwardRequest(backendConn, r); err != nil {
		wp.logger.Error("failed to forward request", "error", err)
		wp.writeError(clientBuf, http.StatusBadGateway)
		return
	}

	// Forward the response
	if err := wp.forwardResponse(clientConn, clientBuf, backendConn); err != nil {
		wp.logger.Error("failed to forward response", "error", err)
		return
	}

	// Bidirectional copy
	errCh := make(chan error, 2)

	go func() {
		_, err := io.Copy(backendConn, clientConn)
		errCh <- err
	}()

	go func() {
		_, err := io.Copy(clientConn, backendConn)
		errCh <- err
	}()

	// Wait for either direction to close
	<-errCh
}

// isWebSocketRequest checks if the request is a WebSocket upgrade
func (wp *WebSocketProxy) isWebSocketRequest(r *http.Request) bool {
	return strings.ToLower(r.Header.Get("Upgrade")) == "websocket" &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

// connectToBackend establishes a connection to the backend server
func (wp *WebSocketProxy) connectToBackend(backend *types.Server, r *http.Request) (net.Conn, error) {
	host := backend.URL.Host
	if backend.URL.Port() == "" {
		if backend.URL.Scheme == "https" || backend.URL.Scheme == "wss" {
			host += ":443"
		} else {
			host += ":80"
		}
	}

	var conn net.Conn
	var err error

	if backend.URL.Scheme == "https" || backend.URL.Scheme == "wss" {
		conn, err = tls.DialWithDialer(wp.dialer, "tcp", host, wp.tlsConfig)
	} else {
		conn, err = wp.dialer.Dial("tcp", host)
	}

	return conn, err
}

// forwardRequest forwards the WebSocket upgrade request to the backend
func (wp *WebSocketProxy) forwardRequest(backendConn net.Conn, r *http.Request) error {
	// Modify the request for the backend
	outReq := r.Clone(r.Context())
	outReq.URL.Scheme = ""
	outReq.URL.Host = ""
	outReq.RequestURI = ""

	// Remove hop-by-hop headers
	removeHopHeaders(outReq.Header)

	// Dump the request
	dump, err := httputil.DumpRequestOut(outReq, true)
	if err != nil {
		return err
	}

	_, err = backendConn.Write(dump)
	return err
}

// forwardResponse forwards the WebSocket upgrade response to the client
func (wp *WebSocketProxy) forwardResponse(clientConn net.Conn, clientBuf *bufio.ReadWriter, backendConn net.Conn) error {
	backendBuf := bufio.NewReader(backendConn)

	// Read response line
	line, err := backendBuf.ReadString('\n')
	if err != nil {
		return err
	}

	if _, err := clientBuf.WriteString(line); err != nil {
		return err
	}

	// Read and forward headers
	for {
		line, err := backendBuf.ReadString('\n')
		if err != nil {
			return err
		}

		if _, err := clientBuf.WriteString(line); err != nil {
			return err
		}

		// Empty line marks end of headers
		if line == "\r\n" || line == "\n" {
			break
		}
	}

	// Flush the response
	return clientBuf.Flush()
}

// writeError writes an HTTP error response
func (wp *WebSocketProxy) writeError(w *bufio.ReadWriter, code int) {
	resp := &http.Response{
		StatusCode: code,
		Status:     http.StatusText(code),
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
	}

	resp.Header.Set("Content-Type", "text/plain")
	resp.Header.Set("Connection", "close")

	fmt.Fprintf(w, "HTTP/1.1 %d %s\r\n", code, http.StatusText(code))
	resp.Header.Write(w)
	fmt.Fprintf(w, "\r\n%s\n", http.StatusText(code))
	w.Flush()
}

// removeHopHeaders removes hop-by-hop headers
func removeHopHeaders(h http.Header) {
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

	for _, header := range hopHeaders {
		h.Del(header)
	}

	// Remove connection-specific headers
	for _, header := range h["Connection"] {
		h.Del(header)
	}
}

// SSEProxy handles Server-Sent Events connections
type SSEProxy struct {
	logger    types.Logger
	transport http.RoundTripper
}

// NewSSEProxy creates a new SSE proxy
func NewSSEProxy(logger types.Logger, transport http.RoundTripper) *SSEProxy {
	if transport == nil {
		transport = DefaultTransport()
	}

	return &SSEProxy{
		logger:    logger,
		transport: transport,
	}
}

// ServeHTTP handles SSE requests
func (sp *SSEProxy) ServeHTTP(w http.ResponseWriter, r *http.Request, backend *types.Server) {
	// Check if this is an SSE request
	if !sp.isSSERequest(r) {
		http.Error(w, "Not an SSE request", http.StatusBadRequest)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable Nginx buffering

	// Create flusher
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Create backend request
	outReq := r.Clone(r.Context())
	outReq.URL.Scheme = backend.URL.Scheme
	outReq.URL.Host = backend.URL.Host
	outReq.Close = false

	// Forward to backend
	resp, err := sp.transport.RoundTrip(outReq)
	if err != nil {
		sp.logger.Error("failed to connect to backend", "error", err)
		http.Error(w, "Backend connection failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy status code
	w.WriteHeader(resp.StatusCode)

	// Stream the response
	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				sp.logger.Error("error reading from backend", "error", err)
			}
			break
		}

		if _, err := w.Write(line); err != nil {
			sp.logger.Error("error writing to client", "error", err)
			break
		}

		flusher.Flush()
	}
}

// isSSERequest checks if the request is for Server-Sent Events
func (sp *SSEProxy) isSSERequest(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "text/event-stream")
}
