package ai

import (
	"sync"
	"time"
)

type CircuitState int

const (
	CircuitClosed   CircuitState = iota // Normal operation
	CircuitOpen                         // Rejecting requests
	CircuitHalfOpen                     // Testing recovery
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

type CircuitBreaker struct {
	mu sync.RWMutex

	state     CircuitState
	failures  []time.Time // timestamps of recent failures
	openedAt  time.Time
	lastError string

	// Configuration
	failureThreshold int
	failureWindow    time.Duration
	cooldownPeriod   time.Duration
}

func NewCircuitBreaker(threshold int, window, cooldown time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:            CircuitClosed,
		failures:         make([]time.Time, 0, threshold),
		failureThreshold: threshold,
		failureWindow:    window,
		cooldownPeriod:   cooldown,
	}
}

func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		if time.Since(cb.openedAt) >= cb.cooldownPeriod {
			cb.state = CircuitHalfOpen
			return true
		}
		return false
	case CircuitHalfOpen:
		return true
	default:
		return false
	}
}

func (cb *CircuitBreaker) RecordFailure(errMsg string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	cb.lastError = errMsg

	cutoff := now.Add(-cb.failureWindow)
	validFailures := make([]time.Time, 0, len(cb.failures))
	for _, t := range cb.failures {
		if t.After(cutoff) {
			validFailures = append(validFailures, t)
		}
	}

	validFailures = append(validFailures, now)
	cb.failures = validFailures

	if len(cb.failures) >= cb.failureThreshold {
		cb.state = CircuitOpen
		cb.openedAt = now
	}
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = CircuitClosed
	cb.failures = cb.failures[:0]
	cb.lastError = ""
}

func (cb *CircuitBreaker) LastError() string {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.lastError
}

func (cb *CircuitBreaker) CooldownRemaining() time.Duration {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if cb.state != CircuitOpen {
		return 0
	}

	remaining := cb.cooldownPeriod - time.Since(cb.openedAt)
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (cb *CircuitBreaker) FailureCount() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return len(cb.failures)
}
