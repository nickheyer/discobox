// Package circuit implements circuit breaker and health checking
package circuit

import (
	"discobox/internal/types"
	"sync"
	"time"

	"github.com/sony/gobreaker"
)


// circuitBreaker implements the CircuitBreaker interface using sony/gobreaker
type circuitBreaker struct {
	breaker *gobreaker.CircuitBreaker
	mu      sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(failureThreshold, successThreshold int, timeout time.Duration) types.CircuitBreaker {
	settings := gobreaker.Settings{
		Name:        "backend",
		MaxRequests: uint32(successThreshold),
		Interval:    timeout,
		Timeout:     timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= uint32(failureThreshold) && failureRatio >= 0.6
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			// Log state changes if needed
		},
	}
	
	return &circuitBreaker{
		breaker: gobreaker.NewCircuitBreaker(settings),
	}
}

// Execute runs the function with circuit breaker protection
func (cb *circuitBreaker) Execute(fn func() error) error {
	_, err := cb.breaker.Execute(func() (interface{}, error) {
		return nil, fn()
	})
	
	if err == gobreaker.ErrOpenState {
		return types.ErrCircuitBreakerOpen
	}
	
	return err
}

// State returns the current state
func (cb *circuitBreaker) State() string {
	state := cb.breaker.State()
	switch state {
	case gobreaker.StateClosed:
		return "closed"
	case gobreaker.StateOpen:
		return "open"
	case gobreaker.StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Reset manually resets the circuit breaker
func (cb *circuitBreaker) Reset() {
	// gobreaker doesn't have a direct reset method, so we simulate it
	// by executing a successful operation
	cb.breaker.Execute(func() (interface{}, error) {
		return nil, nil
	})
}

// MultiCircuitBreaker manages multiple circuit breakers for different services
type MultiCircuitBreaker struct {
	mu       sync.RWMutex
	breakers map[string]types.CircuitBreaker
	settings CircuitBreakerSettings
}

// CircuitBreakerSettings contains configuration for circuit breakers
type CircuitBreakerSettings struct {
	FailureThreshold int
	SuccessThreshold int
	Timeout          time.Duration
}

// NewMultiCircuitBreaker creates a circuit breaker manager
func NewMultiCircuitBreaker(settings CircuitBreakerSettings) *MultiCircuitBreaker {
	return &MultiCircuitBreaker{
		breakers: make(map[string]types.CircuitBreaker),
		settings: settings,
	}
}

// GetBreaker returns a circuit breaker for the given service
func (m *MultiCircuitBreaker) GetBreaker(serviceID string) types.CircuitBreaker {
	m.mu.RLock()
	breaker, exists := m.breakers[serviceID]
	m.mu.RUnlock()
	
	if exists {
		return breaker
	}
	
	// Create new breaker
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Double-check after acquiring write lock
	if breaker, exists := m.breakers[serviceID]; exists {
		return breaker
	}
	
	breaker = NewCircuitBreaker(
		m.settings.FailureThreshold,
		m.settings.SuccessThreshold,
		m.settings.Timeout,
	)
	
	m.breakers[serviceID] = breaker
	return breaker
}

// RemoveBreaker removes a circuit breaker for a service
func (m *MultiCircuitBreaker) RemoveBreaker(serviceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.breakers, serviceID)
}

// GetAllStates returns the states of all circuit breakers
func (m *MultiCircuitBreaker) GetAllStates() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	states := make(map[string]string)
	for id, breaker := range m.breakers {
		states[id] = breaker.State()
	}
	
	return states
}

// ResetAll resets all circuit breakers
func (m *MultiCircuitBreaker) ResetAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	for _, breaker := range m.breakers {
		breaker.Reset()
	}
}
