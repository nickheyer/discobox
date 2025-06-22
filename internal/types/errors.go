package types

import (
	"errors"
	"fmt"
)

// Common errors
var (
	// ErrServiceNotFound indicates the requested service does not exist
	ErrServiceNotFound = errors.New("service not found")
	
	// ErrRouteNotFound indicates the requested route does not exist
	ErrRouteNotFound = errors.New("route not found")
	
	// ErrNoHealthyBackends indicates all backends are unhealthy
	ErrNoHealthyBackends = errors.New("no healthy backends available")
	
	// ErrCircuitBreakerOpen indicates the circuit breaker is open
	ErrCircuitBreakerOpen = errors.New("circuit breaker is open")
	
	// ErrRateLimitExceeded indicates the rate limit has been exceeded
	ErrRateLimitExceeded = errors.New("rate limit exceeded")
	
	// ErrInvalidConfiguration indicates invalid configuration
	ErrInvalidConfiguration = errors.New("invalid configuration")
	
	// ErrStorageError indicates a storage operation failed
	ErrStorageError = errors.New("storage error")
	
	// ErrAlreadyExists indicates a resource already exists
	ErrAlreadyExists = errors.New("resource already exists")
	
	// ErrInvalidRequest indicates an invalid request
	ErrInvalidRequest = errors.New("invalid request")
	
	// ErrUnauthorized indicates authentication is required
	ErrUnauthorized = errors.New("unauthorized")
	
	// ErrForbidden indicates the operation is forbidden
	ErrForbidden = errors.New("forbidden")
	
	// ErrTimeout indicates an operation timed out
	ErrTimeout = errors.New("operation timed out")
	
	// ErrConnectionRefused indicates connection was refused
	ErrConnectionRefused = errors.New("connection refused")
	
	// ErrMaxConnectionsReached indicates max connections limit reached
	ErrMaxConnectionsReached = errors.New("maximum connections reached")
	
	// ErrInvalidWeight indicates an invalid weight value
	ErrInvalidWeight = errors.New("invalid weight value")
	
	// ErrServerNotFound indicates the requested server does not exist
	ErrServerNotFound = errors.New("server not found")
)

// ValidationError represents a validation error with details
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("validation error: %s - %s", e.Field, e.Message)
}

// MultiError represents multiple errors
type MultiError struct {
	Errors []error
}

func (e MultiError) Error() string {
	if len(e.Errors) == 0 {
		return "no errors"
	}
	if len(e.Errors) == 1 {
		return e.Errors[0].Error()
	}
	return fmt.Sprintf("multiple errors: %v", e.Errors)
}

// Add adds an error to the MultiError
func (e *MultiError) Add(err error) {
	if err != nil {
		e.Errors = append(e.Errors, err)
	}
}

// HasErrors returns true if there are any errors
func (e *MultiError) HasErrors() bool {
	return len(e.Errors) > 0
}

// ProxyError wraps an error with additional context
type ProxyError struct {
	Op      string // Operation that failed
	Service string // Service involved
	Err     error  // Original error
}

func (e ProxyError) Error() string {
	if e.Service != "" {
		return fmt.Sprintf("%s %s: %v", e.Op, e.Service, e.Err)
	}
	return fmt.Sprintf("%s: %v", e.Op, e.Err)
}

func (e ProxyError) Unwrap() error {
	return e.Err
}

// IsRetryable returns true if the error is retryable
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	
	// Check for specific error types
	switch {
	case errors.Is(err, ErrTimeout):
		return true
	case errors.Is(err, ErrConnectionRefused):
		return true
	case errors.Is(err, ErrNoHealthyBackends):
		return false // Don't retry if no backends are healthy
	case errors.Is(err, ErrCircuitBreakerOpen):
		return false // Don't retry if circuit breaker is open
	case errors.Is(err, ErrRateLimitExceeded):
		return false // Don't retry rate limit errors
	default:
		// Check if it's a proxy error
		var proxyErr ProxyError
		if errors.As(err, &proxyErr) {
			return IsRetryable(proxyErr.Err)
		}
		return false
	}
}
