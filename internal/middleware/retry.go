package middleware

import (
	"bytes"
	"io"
	"maps"
	"time"

	"net/http"

	"discobox/internal/types"
)

// RetryConfig configures retry behavior
type RetryConfig struct {
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
	RetryIf      func(*http.Response, error) bool
}

// DefaultRetryConfig returns sensible defaults
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		Multiplier:   2.0,
		RetryIf:      defaultRetryIf,
	}
}

// defaultRetryIf determines if a request should be retried
func defaultRetryIf(resp *http.Response, err error) bool {
	if err != nil {
		return types.IsRetryable(err)
	}

	if resp == nil {
		return true
	}

	// Retry on 5xx errors and specific 4xx errors
	switch resp.StatusCode {
	case http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
		http.StatusTooManyRequests:
		return true
	default:
		return resp.StatusCode >= 500
	}
}

// Retry creates retry middleware
func Retry(config RetryConfig) types.Middleware {
	if config.RetryIf == nil {
		config.RetryIf = defaultRetryIf
	}

	return func(next http.Handler) http.Handler {
		return &retryHandler{
			next:   next,
			config: config,
		}
	}
}

// retryHandler implements retry logic
type retryHandler struct {
	next   http.Handler
	config RetryConfig
}

func (rh *retryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Buffer the request body for potential retries
	var bodyBytes []byte
	if r.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusInternalServerError)
			return
		}
		r.Body.Close()
	}

	delay := rh.config.InitialDelay

	for attempt := 0; attempt < rh.config.MaxAttempts; attempt++ {
		// Create new request body for each attempt
		if bodyBytes != nil {
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		// Capture response
		recorder := &responseRecorder{
			header: make(http.Header),
			body:   &bytes.Buffer{},
		}

		// Process request
		rh.next.ServeHTTP(recorder, r)

		// Check if we should retry
		shouldRetry := rh.config.RetryIf(&http.Response{
			StatusCode: recorder.statusCode,
			Header:     recorder.header,
		}, nil)

		if !shouldRetry || attempt == rh.config.MaxAttempts-1 {
			// Write final response
			maps.Copy(w.Header(), recorder.header)
			w.WriteHeader(recorder.statusCode)
			w.Write(recorder.body.Bytes())
			return
		}

		// Wait before retrying
		time.Sleep(delay)

		// Calculate next delay with exponential backoff
		delay = time.Duration(float64(delay) * rh.config.Multiplier)
		if delay > rh.config.MaxDelay {
			delay = rh.config.MaxDelay
		}

		// Add retry headers
		r.Header.Set("X-Retry-Attempt", string(rune(attempt+1)))
	}
}

// responseRecorder captures the response for retry logic
type responseRecorder struct {
	header     http.Header
	body       *bytes.Buffer
	statusCode int
}

func (rr *responseRecorder) Header() http.Header {
	return rr.header
}

func (rr *responseRecorder) Write(b []byte) (int, error) {
	if rr.statusCode == 0 {
		rr.statusCode = http.StatusOK
	}
	return rr.body.Write(b)
}

func (rr *responseRecorder) WriteHeader(code int) {
	rr.statusCode = code
}

// ExponentialBackoff provides exponential backoff retry logic
type ExponentialBackoff struct {
	Initial    time.Duration
	Max        time.Duration
	Multiplier float64
	current    time.Duration
}

// NewExponentialBackoff creates a new exponential backoff
func NewExponentialBackoff(initial, max time.Duration, multiplier float64) *ExponentialBackoff {
	return &ExponentialBackoff{
		Initial:    initial,
		Max:        max,
		Multiplier: multiplier,
		current:    initial,
	}
}

// Next returns the next backoff duration
func (eb *ExponentialBackoff) Next() time.Duration {
	defer func() {
		eb.current = time.Duration(float64(eb.current) * eb.Multiplier)
		if eb.current > eb.Max {
			eb.current = eb.Max
		}
	}()
	return eb.current
}

// Reset resets the backoff to initial value
func (eb *ExponentialBackoff) Reset() {
	eb.current = eb.Initial
}
