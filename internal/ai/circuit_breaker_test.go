package ai

import (
	"testing"
	"time"
)

func TestCircuitBreaker_InitialState(t *testing.T) {
	cb := NewCircuitBreaker(5, 2*time.Minute, 30*time.Second)

	if cb.State() != CircuitClosed {
		t.Errorf("expected initial state CircuitClosed, got %v", cb.State())
	}
	if !cb.Allow() {
		t.Error("expected Allow() to return true when circuit is closed")
	}
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(3, 1*time.Minute, 100*time.Millisecond)

	cb.RecordFailure("error 1")
	cb.RecordFailure("error 2")
	cb.RecordFailure("error 3")

	if cb.State() != CircuitOpen {
		t.Errorf("expected CircuitOpen after 3 failures, got %v", cb.State())
	}
	if cb.Allow() {
		t.Error("expected Allow() to return false when circuit is open")
	}
}

func TestCircuitBreaker_RecoverAfterCooldown(t *testing.T) {
	cb := NewCircuitBreaker(2, 1*time.Minute, 50*time.Millisecond)

	cb.RecordFailure("error 1")
	cb.RecordFailure("error 2")

	if cb.State() != CircuitOpen {
		t.Fatalf("expected CircuitOpen, got %v", cb.State())
	}

	time.Sleep(60 * time.Millisecond)

	if !cb.Allow() {
		t.Error("expected Allow() to return true after cooldown")
	}
	if cb.State() != CircuitHalfOpen {
		t.Errorf("expected CircuitHalfOpen, got %v", cb.State())
	}

	cb.RecordSuccess()

	if cb.State() != CircuitClosed {
		t.Errorf("expected CircuitClosed after success, got %v", cb.State())
	}
}
