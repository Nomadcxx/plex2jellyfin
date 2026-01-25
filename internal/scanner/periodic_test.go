package scanner

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/logging"
)

func TestPeriodicScanner_IsHealthy_DefaultTrue(t *testing.T) {
	s := &PeriodicScanner{healthy: true}

	if !s.IsHealthy() {
		t.Error("expected scanner to be healthy by default")
	}
}

func TestPeriodicScanner_Status_ReturnsCorrectState(t *testing.T) {
	now := time.Now()
	s := &PeriodicScanner{
		healthy:      true,
		lastScan:     now,
		lastSuccess:  now,
		skippedTicks: 5,
		scanning:     false,
	}

	status := s.Status()

	if !status.Healthy {
		t.Error("expected healthy=true")
	}
	if status.SkippedTicks != 5 {
		t.Errorf("expected skippedTicks=5, got %d", status.SkippedTicks)
	}
	if status.Scanning {
		t.Error("expected scanning=false")
	}
}

func TestPeriodicScanner_SkipsWhenBusy(t *testing.T) {
	s := &PeriodicScanner{
		scanning: true,
		logger:   logging.Nop(),
	}

	// Call tick while already scanning
	s.tick()

	if s.skippedTicks != 1 {
		t.Errorf("expected skippedTicks=1, got %d", s.skippedTicks)
	}
}

func TestPeriodicScanner_StartStopsOnContextCancel(t *testing.T) {
	s := &PeriodicScanner{
		interval:   100 * time.Millisecond,
		watchPaths: []string{},
		logger:     logging.Nop(),
	}

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)

	var startErr error
	go func() {
		defer wg.Done()
		startErr = s.Start(ctx)
	}()

	// Let it run briefly
	time.Sleep(50 * time.Millisecond)
	cancel()
	wg.Wait()

	if startErr != nil {
		t.Errorf("expected nil error on clean shutdown, got %v", startErr)
	}
}
