package scanner

import (
	"testing"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/config"
)

func TestAIHelper_CircuitBreaker(t *testing.T) {
	cfg := config.AIConfig{
		Enabled:              true,
		AutoTriggerThreshold: 0.6,
		ConfidenceThreshold:  0.8,
		CircuitBreaker: config.CircuitBreakerConfig{
			FailureThreshold:     3,
			FailureWindowSeconds: 60,
			CooldownSeconds:      1,
		},
	}

	helper := NewAIHelper(cfg, nil, nil)

	// Record failures to open circuit
	for i := 0; i < 3; i++ {
		helper.RecordFailure()
	}

	if !helper.IsCircuitOpen() {
		t.Error("Circuit should be open after 3 failures")
	}

	// Wait for cooldown
	time.Sleep(1100 * time.Millisecond)

	if helper.IsCircuitOpen() {
		t.Error("Circuit should be closed after cooldown")
	}
}

func TestAIHelper_ResetFailures(t *testing.T) {
	cfg := config.AIConfig{
		CircuitBreaker: config.CircuitBreakerConfig{
			FailureThreshold:     3,
			FailureWindowSeconds: 60,
			CooldownSeconds:      5,
		},
	}

	helper := NewAIHelper(cfg, nil, nil)

	helper.RecordFailure()
	helper.RecordFailure()
	helper.ResetFailures()

	if helper.IsCircuitOpen() {
		t.Error("Circuit should be closed after reset")
	}
}
