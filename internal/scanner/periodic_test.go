package scanner

import (
	"context"
	"encoding/json"
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
		watchPaths: []string{tempDir},
		handler:    handler,
		logger:     logging.Nop(),
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

// writeActivityEntry appends a single JSON-encoded activity entry to path.
func writeActivityEntry(t *testing.T, path string, entry activity.Entry) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		t.Fatalf("open activity file: %v", err)
	}
	defer f.Close()
	line, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal entry: %v", err)
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		t.Fatalf("write entry: %v", err)
	}
}

// TestPeriodicScanner_ProcessActivityFile_DedupPerSource verifies that
// multiple failure entries for the same source in one activity file result
// in exactly one retry attempt, not N.
func TestPeriodicScanner_ProcessActivityFile_DedupPerSource(t *testing.T) {
	tempDir := t.TempDir()
	src := filepath.Join(tempDir, "show.s01e01.mkv")
	if err := os.WriteFile(src, []byte("x"), 0644); err != nil {
		t.Fatalf("create source: %v", err)
	}

	activityPath := filepath.Join(tempDir, "activity.jsonl")
	now := time.Now()
	for i := 0; i < 5; i++ {
		writeActivityEntry(t, activityPath, activity.Entry{
			Timestamp: now.Add(-time.Duration(i) * time.Minute),
			Action:    "organize",
			Source:    src,
			Success:   false,
			Error:     "transient fs error",
		})
	}

	handler := &recordingHandler{}
	s := &PeriodicScanner{
		watchPaths: []string{tempDir},
		handler:    handler,
		logger:     logging.Nop(),
	}

	retryWindow := now.Add(-retryWindowHours * time.Hour)
	cleanupWindow := now.Add(-cleanupWindowDays * 24 * time.Hour)
	retried, _, err := s.processActivityFile(activityPath, retryWindow, cleanupWindow)
	if err != nil {
		t.Fatalf("processActivityFile: %v", err)
	}
	if retried != 1 {
		t.Fatalf("expected 1 retry (dedup by source), got %d", retried)
	}
	if len(handler.events) != 1 {
		t.Fatalf("expected 1 handler event, got %d", len(handler.events))
	}
}

// TestPeriodicScanner_ProcessActivityFile_DeterministicSkip verifies that a
// deterministic failure with unchanged source mtime and recent timestamp is
// NOT retried — this is the core fix for the log-spam amplification.
func TestPeriodicScanner_ProcessActivityFile_DeterministicSkip(t *testing.T) {
	tempDir := t.TempDir()
	src := filepath.Join(tempDir, "unparseable.mkv")
	if err := os.WriteFile(src, []byte("x"), 0644); err != nil {
		t.Fatalf("create source: %v", err)
	}
	st, _ := os.Stat(src)
	mtime := st.ModTime().Unix()

	activityPath := filepath.Join(tempDir, "activity.jsonl")
	writeActivityEntry(t, activityPath, activity.Entry{
		Timestamp:     time.Now().Add(-1 * time.Hour),
		Action:        "organize",
		Source:        src,
		Success:       false,
		Error:         "parse failed: no episode information",
		Deterministic: true,
		SourceMtime:   mtime,
	})

	handler := &recordingHandler{}
	s := &PeriodicScanner{
		watchPaths: []string{tempDir},
		handler:    handler,
		logger:     logging.Nop(),
	}

	now := time.Now()
	retried, _, err := s.processActivityFile(
		activityPath,
		now.Add(-retryWindowHours*time.Hour),
		now.Add(-cleanupWindowDays*24*time.Hour),
	)
	if err != nil {
		t.Fatalf("processActivityFile: %v", err)
	}
	if retried != 0 {
		t.Fatalf("expected 0 retries for deterministic+unchanged mtime, got %d", retried)
	}
	if len(handler.events) != 0 {
		t.Fatalf("expected 0 handler events, got %d", len(handler.events))
	}
}

// TestPeriodicScanner_ProcessActivityFile_DeterministicMtimeChange verifies
// that a deterministic failure IS retried when the source mtime changes
// (file replaced/re-downloaded).
func TestPeriodicScanner_ProcessActivityFile_DeterministicMtimeChange(t *testing.T) {
	tempDir := t.TempDir()
	src := filepath.Join(tempDir, "was-unparseable.mkv")
	if err := os.WriteFile(src, []byte("x"), 0644); err != nil {
		t.Fatalf("create source: %v", err)
	}

	activityPath := filepath.Join(tempDir, "activity.jsonl")
	writeActivityEntry(t, activityPath, activity.Entry{
		Timestamp:     time.Now().Add(-1 * time.Hour),
		Action:        "organize",
		Source:        src,
		Success:       false,
		Error:         "parse failed",
		Deterministic: true,
		SourceMtime:   1, // stale mtime, guaranteed != current
	})

	handler := &recordingHandler{}
	s := &PeriodicScanner{
		watchPaths: []string{tempDir},
		handler:    handler,
		logger:     logging.Nop(),
	}

	now := time.Now()
	retried, _, err := s.processActivityFile(
		activityPath,
		now.Add(-retryWindowHours*time.Hour),
		now.Add(-cleanupWindowDays*24*time.Hour),
	)
	if err != nil {
		t.Fatalf("processActivityFile: %v", err)
	}
	if retried != 1 {
		t.Fatalf("expected 1 retry on mtime change, got %d", retried)
	}
}
