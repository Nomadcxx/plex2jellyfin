package scanner

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/activity"
	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin"
	"github.com/Nomadcxx/plex2jellyfin/internal/logging"
	"github.com/Nomadcxx/plex2jellyfin/internal/radarr"
	"github.com/Nomadcxx/plex2jellyfin/internal/watcher"
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

func TestPeriodicScannerArrDriftIsUnhealthyBeforeFirstTick(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/config/downloadClient":
			_ = json.NewEncoder(w).Encode(radarr.DownloadClientConfig{
				ID: 1, EnableCompletedDownloadHandling: false,
			})
		case "/api/v3/config/naming":
			_ = json.NewEncoder(w).Encode(radarr.NamingConfig{ID: 1, RenameMovies: true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	s := NewPeriodicScanner(ScannerConfig{
		Interval:     time.Hour,
		Handler:      &recordingHandler{},
		Logger:       logging.Nop(),
		RadarrClient: radarr.NewClient(radarr.Config{URL: server.URL, APIKey: "key"}),
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if s.IsHealthy() {
		t.Fatal("Arr drift was not reflected before the first periodic tick")
	}
}

func TestPeriodicScannerArrDriftRemainsUnhealthyAfterScan(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/config/downloadClient":
			_ = json.NewEncoder(w).Encode(radarr.DownloadClientConfig{
				ID: 1, EnableCompletedDownloadHandling: true,
			})
		case "/api/v3/config/naming":
			_ = json.NewEncoder(w).Encode(radarr.NamingConfig{ID: 1, RenameMovies: false})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	s := NewPeriodicScanner(ScannerConfig{
		Handler:      &recordingHandler{},
		Logger:       logging.Nop(),
		RadarrClient: radarr.NewClient(radarr.Config{URL: server.URL, APIKey: "key"}),
	})
	s.tick()
	if s.IsHealthy() {
		t.Fatal("successful file scan overwrote incompatible Arr health")
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

func TestPeriodicScannerTickMarksIncompleteWatchScanUnhealthy(t *testing.T) {
	root := t.TempDir()
	goodRoot := filepath.Join(root, "good")
	if err := os.Mkdir(goodRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	goodFile := filepath.Join(goodRoot, "movie.mkv")
	if err := os.WriteFile(goodFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := &recordingHandler{}
	s := NewPeriodicScanner(ScannerConfig{
		WatchPaths: []string{filepath.Join(root, "missing"), goodRoot},
		Handler:    handler,
		Logger:     logging.Nop(),
	})
	events, cancel := s.Subscribe()
	defer cancel()
	s.tick()

	status := s.Status()
	if status.Healthy || !status.LastSuccess.IsZero() || status.LastError == "" {
		t.Fatalf("incomplete scan status = %+v, want unhealthy with no successful scan and an error", status)
	}
	if len(handler.events) != 1 || handler.events[0].Path != goodFile {
		t.Fatalf("scanner stopped after one root failed; events = %+v", handler.events)
	}
	var scanFailed bool
	for len(events) > 0 {
		event := <-events
		if event.Type == "scan_failed" && event.Error != "" {
			scanFailed = true
		}
	}
	if !scanFailed {
		t.Fatal("incomplete scan did not publish a scan_failed event")
	}
}

func TestPeriodicScannerTickMarksActivityReconcileFailureUnhealthy(t *testing.T) {
	activityPath := filepath.Join(t.TempDir(), "activity")
	if err := os.WriteFile(activityPath, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := NewPeriodicScanner(ScannerConfig{
		Handler:     &recordingHandler{},
		Logger:      logging.Nop(),
		ActivityDir: activityPath,
	})
	s.tick()

	status := s.Status()
	if status.Healthy || !status.LastSuccess.IsZero() || !strings.Contains(status.LastError, "activity") {
		t.Fatalf("activity reconciliation failure status = %+v", status)
	}
}

func TestPeriodicScannerTickMarksFileProcessingFailureUnhealthy(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "failed.mkv")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := NewPeriodicScanner(ScannerConfig{
		WatchPaths: []string{root},
		Handler:    failingHandler{err: errors.New("handler failed")},
		Logger:     logging.Nop(),
	})
	s.tick()

	status := s.Status()
	if status.Healthy || !status.LastSuccess.IsZero() || !strings.Contains(status.LastError, "watch") {
		t.Fatalf("file processing failure status = %+v", status)
	}
}

type failingHandler struct {
	err error
}

func (h failingHandler) HandleFileEvent(watcher.FileEvent) error { return h.err }
func (failingHandler) IsMediaFile(path string) bool              { return filepath.Ext(path) == ".mkv" }

func TestPeriodicScannerReplaceWatchPaths(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "old")
	newPath := filepath.Join(root, "new")
	if err := os.MkdirAll(oldPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newPath, 0o755); err != nil {
		t.Fatal(err)
	}
	oldFile := filepath.Join(oldPath, "old.mkv")
	newFile := filepath.Join(newPath, "new.mkv")
	if err := os.WriteFile(oldFile, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newFile, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := &recordingHandler{}
	s := NewPeriodicScanner(ScannerConfig{
		WatchPaths: []string{oldPath},
		Handler:    handler,
		Logger:     logging.Nop(),
	})
	s.ReplaceWatchPaths([]string{newPath})
	processed, scanErrors := s.scanWatchDirectories()
	if processed != 1 || scanErrors != 0 {
		t.Fatalf("scan result = (%d, %d), want (1, 0)", processed, scanErrors)
	}
	if len(handler.events) != 1 || handler.events[0].Path != newFile {
		t.Fatalf("events = %+v, want only %q", handler.events, newFile)
	}
	paths := s.WatchPaths()
	paths[0] = "/mutated"
	if s.WatchPaths()[0] != newPath {
		t.Fatal("WatchPaths returned aliased storage")
	}
}

func TestPeriodicScannerReplaceWatchPathsConcurrentWithScan(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "old")
	newPath := filepath.Join(root, "new")
	if err := os.MkdirAll(oldPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newPath, 0o755); err != nil {
		t.Fatal(err)
	}
	s := NewPeriodicScanner(ScannerConfig{
		WatchPaths: []string{oldPath},
		Handler:    &recordingHandler{},
		Logger:     logging.Nop(),
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 500; i++ {
			s.scanWatchDirectories()
		}
	}()
	for i := 0; i < 500; i++ {
		if i%2 == 0 {
			s.ReplaceWatchPaths([]string{newPath})
		} else {
			s.ReplaceWatchPaths([]string{oldPath})
		}
	}
	<-done
}

func TestPeriodicScannerReplaceWatchPathsConcurrentWithRetry(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "old")
	newPath := filepath.Join(root, "new")
	for _, path := range []string{oldPath, newPath} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	filePath := filepath.Join(oldPath, "retry.mkv")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := NewPeriodicScanner(ScannerConfig{
		WatchPaths: []string{oldPath, newPath},
		Handler:    &recordingHandler{},
		Logger:     logging.Nop(),
	})
	entry := activity.Entry{Source: filePath}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			_ = s.retryTransfer(entry)
		}
	}()
	for i := 0; i < 2000; i++ {
		if i%2 == 0 {
			s.ReplaceWatchPaths([]string{oldPath, newPath})
		} else {
			s.ReplaceWatchPaths([]string{newPath, oldPath})
		}
	}
	wg.Wait()
}

func TestPeriodicScanner_RunScanIgnoresTypedNilOrphanChecker(t *testing.T) {
	var checker *jellyfin.Client
	s := NewPeriodicScanner(ScannerConfig{
		Handler:     &recordingHandler{},
		Logger:      logging.Nop(),
		OrphanCheck: checker,
	})

	if err := s.runScan(); err != nil {
		t.Fatalf("expected typed nil orphan checker to be disabled, got %v", err)
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

func TestPeriodicScannerRetryRejectsSiblingWatchRoot(t *testing.T) {
	root := t.TempDir()
	watchRoot := filepath.Join(root, "movies")
	sibling := filepath.Join(root, "movies-old")
	if err := os.MkdirAll(watchRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(sibling, "retry.mkv")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := &recordingHandler{}
	s := NewPeriodicScanner(ScannerConfig{
		WatchPaths: []string{watchRoot},
		Handler:    handler,
		Logger:     logging.Nop(),
	})
	err := s.retryTransfer(activity.Entry{Source: filePath})
	if err == nil || !strings.Contains(err.Error(), "outside watch roots") {
		t.Fatalf("retry error = %v, want outside watch roots", err)
	}
	if len(handler.events) != 0 {
		t.Fatalf("retry events = %+v, want none", handler.events)
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

func TestPeriodicScanner_ReconcileRetriesOnlyLatestOutcomeAcrossFiles(t *testing.T) {
	root := t.TempDir()
	activityDir := filepath.Join(root, "activity")
	watchDir := filepath.Join(root, "watch")
	if err := os.MkdirAll(activityDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(watchDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(watchDir, "show.s01e01.mkv")
	if err := os.WriteFile(source, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	write := func(name string, at time.Time, success bool) {
		t.Helper()
		writeActivityEntry(t, filepath.Join(activityDir, name), activity.Entry{
			Timestamp: at,
			Action:    "organize",
			Source:    source,
			Success:   success,
			Error:     "transfer failed",
		})
	}
	write("activity-2026-09-10.jsonl", now.Add(-3*time.Minute), false)
	write("activity-2026-09-11.jsonl", now.Add(-2*time.Minute), true)
	write("activity-2026-09-12.jsonl", now.Add(-time.Minute), false)

	handler := &recordingHandler{}
	s := &PeriodicScanner{
		watchPaths:  []string{watchDir},
		handler:     handler,
		logger:      logging.Nop(),
		activityDir: activityDir,
	}

	retried, _, err := s.reconcileActivity()
	if err != nil {
		t.Fatalf("reconcileActivity: %v", err)
	}
	if retried != 1 || len(handler.events) != 1 {
		t.Fatalf("retried=%d events=%d, want one retry for the latest failure", retried, len(handler.events))
	}
}

func TestPeriodicScanner_ReconcileHonorsHandlerRetryBackoff(t *testing.T) {
	root := t.TempDir()
	activityDir := filepath.Join(root, "activity")
	watchDir := filepath.Join(root, "watch")
	if err := os.MkdirAll(activityDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(watchDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(watchDir, "show.s01e01.mkv")
	if err := os.WriteFile(source, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeActivityEntry(t, filepath.Join(activityDir, "activity-2026-09-12.jsonl"), activity.Entry{
		Timestamp: time.Now().Add(-time.Minute),
		Action:    "organize",
		Source:    source,
		Success:   false,
		Error:     "insufficient space: target is full",
	})

	handler := &recordingHandler{}
	s := &PeriodicScanner{
		watchPaths:      []string{watchDir},
		handler:         handler,
		logger:          logging.Nop(),
		activityDir:     activityDir,
		retrySuppressed: func(path string) bool { return path == source },
	}

	retried, _, err := s.reconcileActivity()
	if err != nil {
		t.Fatalf("reconcileActivity: %v", err)
	}
	if retried != 0 || len(handler.events) != 0 {
		t.Fatalf("retried=%d events=%d, want no retry while handler backoff is active", retried, len(handler.events))
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

func TestPeriodicScanner_ProcessActivityFile_LegacyDeterministicErrorSkip(t *testing.T) {
	tempDir := t.TempDir()
	src := filepath.Join(tempDir, "Supergirl.S03.1080p.BluRay.x264-YELLOWBiRD.mkv")
	if err := os.WriteFile(src, []byte("x"), 0644); err != nil {
		t.Fatalf("create source: %v", err)
	}

	activityPath := filepath.Join(tempDir, "activity.jsonl")
	writeActivityEntry(t, activityPath, activity.Entry{
		Timestamp: time.Now().Add(-1 * time.Hour),
		Action:    "organize",
		Source:    src,
		Success:   false,
		Error:     "season_pack_unresolved: /downloads/tv/Supergirl.S03.1080p.BluRay.x264-YELLOWBiRD",
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
		t.Fatalf("expected 0 retries for legacy deterministic error, got %d", retried)
	}
	if len(handler.events) != 0 {
		t.Fatalf("expected 0 handler events, got %d", len(handler.events))
	}
}

func TestPeriodicScanner_ProcessActivityFile_LegacyQualityEvidenceSkip(t *testing.T) {
	tempDir := t.TempDir()
	src := filepath.Join(tempDir, "incoming.mkv")
	if err := os.WriteFile(src, []byte("x"), 0644); err != nil {
		t.Fatalf("create source: %v", err)
	}

	activityPath := filepath.Join(tempDir, "activity.jsonl")
	writeActivityEntry(t, activityPath, activity.Entry{
		Timestamp: time.Now().Add(-1 * time.Hour),
		Action:    "organize",
		Source:    src,
		Success:   false,
		Error:     "incoming file quality evidence is unavailable",
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
	if retried != 0 || len(handler.events) != 0 {
		t.Fatalf("retried=%d events=%d, want no retry for legacy quality-evidence failure", retried, len(handler.events))
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
