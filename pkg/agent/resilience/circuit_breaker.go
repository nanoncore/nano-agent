// Package resilience provides fault tolerance primitives for the nano-agent.
package resilience

import (
	"sync"
	"time"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	// StateClosed is the normal state where requests are allowed through.
	StateClosed CircuitState = iota
	// StateOpen is the failure state where requests are rejected immediately.
	StateOpen
	// StateHalfOpen is the testing state where limited requests are allowed.
	StateHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig configures a circuit breaker.
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of consecutive failures before opening.
	FailureThreshold int
	// SuccessThreshold is the number of consecutive successes needed to close from half-open.
	SuccessThreshold int
	// Timeout is how long to stay in open state before transitioning to half-open.
	Timeout time.Duration
}

// DefaultCircuitBreakerConfig returns the default circuit breaker configuration.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          30 * time.Second,
	}
}

// CircuitBreaker implements the circuit breaker pattern.
type CircuitBreaker struct {
	mu sync.RWMutex

	config       CircuitBreakerConfig
	state        CircuitState
	failureCount int
	successCount int
	lastFailure  time.Time
	openedAt     time.Time
}

// NewCircuitBreaker creates a new circuit breaker with the given configuration.
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		config: config,
		state:  StateClosed,
	}
}

// Allow returns true if a request should be allowed through.
// It also handles state transitions from Open to HalfOpen after timeout.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return true

	case StateOpen:
		// Check if timeout has elapsed
		if time.Since(cb.openedAt) >= cb.config.Timeout {
			cb.state = StateHalfOpen
			cb.failureCount = 0
			cb.successCount = 0
			return true
		}
		return false

	case StateHalfOpen:
		// Allow limited requests in half-open state
		return true

	default:
		return false
	}
}

// RecordSuccess records a successful request.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		// Reset failure count on success
		cb.failureCount = 0

	case StateHalfOpen:
		cb.successCount++
		// Close circuit if we've had enough successes
		if cb.successCount >= cb.config.SuccessThreshold {
			cb.state = StateClosed
			cb.failureCount = 0
			cb.successCount = 0
		}
	}
}

// RecordFailure records a failed request.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.lastFailure = time.Now()

	switch cb.state {
	case StateClosed:
		cb.failureCount++
		// Open circuit if we've had too many failures
		if cb.failureCount >= cb.config.FailureThreshold {
			cb.state = StateOpen
			cb.openedAt = time.Now()
		}

	case StateHalfOpen:
		// Return to open state immediately on failure
		cb.state = StateOpen
		cb.openedAt = time.Now()
		cb.successCount = 0
	}
}

// State returns the current state of the circuit breaker.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Stats returns current statistics for the circuit breaker.
func (cb *CircuitBreaker) Stats() CircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitBreakerStats{
		State:        cb.state,
		FailureCount: cb.failureCount,
		SuccessCount: cb.successCount,
		LastFailure:  cb.lastFailure,
		OpenedAt:     cb.openedAt,
	}
}

// Reset resets the circuit breaker to the closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = StateClosed
	cb.failureCount = 0
	cb.successCount = 0
	cb.lastFailure = time.Time{}
	cb.openedAt = time.Time{}
}

// CircuitBreakerStats contains statistics about a circuit breaker.
type CircuitBreakerStats struct {
	State        CircuitState
	FailureCount int
	SuccessCount int
	LastFailure  time.Time
	OpenedAt     time.Time
}
