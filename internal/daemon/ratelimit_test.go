package daemon

import (
	"testing"
	"time"
)

func TestAIRateLimiter_AllowWithinLimits(t *testing.T) {
	rl := NewAIRateLimiter(10, 50)
	for i := 0; i < 10; i++ {
		if !rl.Allow() {
			t.Fatalf("Allow() returned false on call %d, expected true", i+1)
		}
		rl.Record()
	}
}

func TestAIRateLimiter_HourlyCapExhausted(t *testing.T) {
	rl := NewAIRateLimiter(3, 50)
	for i := 0; i < 3; i++ {
		rl.Allow()
		rl.Record()
	}
	if rl.Allow() {
		t.Fatal("Allow() returned true after hourly cap exhausted")
	}
}

func TestAIRateLimiter_DailyCapExhausted(t *testing.T) {
	rl := NewAIRateLimiter(100, 5)
	for i := 0; i < 5; i++ {
		rl.Allow()
		rl.Record()
	}
	if rl.Allow() {
		t.Fatal("Allow() returned true after daily cap exhausted")
	}
}

func TestAIRateLimiter_HourlyResetsAfterWindow(t *testing.T) {
	rl := NewAIRateLimiter(2, 50)
	rl.Allow()
	rl.Record()
	rl.Allow()
	rl.Record()

	if rl.Allow() {
		t.Fatal("should be exhausted before reset")
	}

	// Simulate hour passing
	rl.mu.Lock()
	rl.hourlyReset = time.Now().Add(-1 * time.Second)
	rl.mu.Unlock()

	if !rl.Allow() {
		t.Fatal("Allow() should return true after hourly window reset")
	}
}

func TestAIRateLimiter_DailyResetsAfterWindow(t *testing.T) {
	rl := NewAIRateLimiter(100, 2)
	rl.Allow()
	rl.Record()
	rl.Allow()
	rl.Record()

	if rl.Allow() {
		t.Fatal("should be exhausted before reset")
	}

	// Simulate day passing
	rl.mu.Lock()
	rl.dailyReset = time.Now().Add(-1 * time.Second)
	rl.mu.Unlock()

	if !rl.Allow() {
		t.Fatal("Allow() should return true after daily window reset")
	}
}

func TestAIRateLimiter_Stats(t *testing.T) {
	rl := NewAIRateLimiter(10, 50)
	rl.Allow()
	rl.Record()
	rl.Allow()
	rl.Record()

	h, d := rl.Stats()
	if h != 2 {
		t.Errorf("hourlyUsed = %d, want 2", h)
	}
	if d != 2 {
		t.Errorf("dailyUsed = %d, want 2", d)
	}
}
