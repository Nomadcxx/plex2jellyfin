package scanner

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/activity"
	"github.com/Nomadcxx/jellywatch/internal/logging"
	"github.com/Nomadcxx/jellywatch/internal/watcher"
)

type recordingHandler struct {
	events []watcher.FileEvent
}

func (h *recordingHandler) HandleFileEvent(event watcher.FileEvent) error {
	h.events = append(h.events, event)
	return nil
}

func (h *recordingHandler) IsMediaFile(path string) bool {
	return filepath.Ext(path) == ".mkv"
}

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

func TestPeriodicScanner_ScanWatchDirectoriesForwardsFullPath(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "sample.mkv")
	if err := os.WriteFile(filePath, []byte("x"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	handler := &recordingHandler{}
	s := &PeriodicScanner{
		watchPaths: []string{tempDir},
		handler:    handler,
		logger:     logging.Nop(),
	}

	processed, errors := s.scanWatchDirectories()
	if processed != 1 || errors != 0 {
		t.Fatalf("unexpected scan results processed=%d errors=%d", processed, errors)
	}
	if len(handler.events) != 1 {
		t.Fatalf("expected one forwarded event, got %d", len(handler.events))
	}
	if handler.events[0].Path != filePath {
		t.Fatalf("expected full path %q, got %q", filePath, handler.events[0].Path)
	}
}

func TestPeriodicScanner_RetryTransferForwardsFullPath(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "retry.mkv")
	if err := os.WriteFile(filePath, []byte("x"), 0644); err != nil {
		t.Fatalf("failed to create retry file: %v", err)
	}

	handler := &recordingHandler{}
	s := &PeriodicScanner{
		handler: handler,
		logger:  logging.Nop(),
	}

	err := s.retryTransfer(activity.Entry{Source: filePath})
	if err != nil {
		t.Fatalf("unexpected retry error: %v", err)
	}
	if len(handler.events) != 1 {
		t.Fatalf("expected one retry event, got %d", len(handler.events))
	}
	if handler.events[0].Path != filePath {
		t.Fatalf("expected full retry path %q, got %q", filePath, handler.events[0].Path)
	}
}
