# Periodic Scanner Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement periodic directory scanning in jellywatchd to catch files missed by the real-time watcher.

**Architecture:** Dedicated `PeriodicScanner` service in `internal/scanner/` that runs alongside the existing watcher. Ticker-based scanning with activity log reconciliation (retry recent failures, clean old ones).

**Tech Stack:** Go stdlib (time.Ticker, sync.Mutex, filepath.Walk), existing logging/activity packages

---

## Task 1: Create Scanner Package Structure

**Files:**
- Create: `internal/scanner/periodic.go`
- Create: `internal/scanner/types.go`

**Step 1: Create the types file with config and status structs**

```go
// internal/scanner/types.go
package scanner

import (
	"time"

	"github.com/Nomadcxx/jellywatch/internal/daemon"
	"github.com/Nomadcxx/jellywatch/internal/logging"
)

// ScannerConfig holds configuration for the periodic scanner
type ScannerConfig struct {
	Interval    time.Duration
	WatchPaths  []string
	Handler     *daemon.MediaHandler
	Logger      *logging.Logger
	ActivityDir string
}

// ScannerStatus holds the current state for health reporting
type ScannerStatus struct {
	Healthy      bool      `json:"healthy"`
	LastScan     time.Time `json:"last_scan,omitempty"`
	LastSuccess  time.Time `json:"last_success,omitempty"`
	LastError    string    `json:"last_error,omitempty"`
	SkippedTicks int64     `json:"skipped_ticks"`
	Scanning     bool      `json:"scanning"`
}
```

**Step 2: Create the scanner struct skeleton**

```go
// internal/scanner/periodic.go
package scanner

import (
	"context"
	"sync"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/daemon"
	"github.com/Nomadcxx/jellywatch/internal/logging"
)

// PeriodicScanner runs periodic directory scans to catch missed files
type PeriodicScanner struct {
	// Configuration
	interval    time.Duration
	watchPaths  []string
	handler     *daemon.MediaHandler
	logger      *logging.Logger
	activityDir string

	// State tracking
	mu           sync.Mutex
	scanning     bool
	lastScan     time.Time
	lastSuccess  time.Time
	lastError    error
	skippedTicks int64

	// Health tracking
	healthy bool
}

// NewPeriodicScanner creates a new scanner with the given config
func NewPeriodicScanner(cfg ScannerConfig) *PeriodicScanner {
	return &PeriodicScanner{
		interval:    cfg.Interval,
		watchPaths:  cfg.WatchPaths,
		handler:     cfg.Handler,
		logger:      cfg.Logger,
		activityDir: cfg.ActivityDir,
		healthy:     true,
	}
}
```

**Step 3: Verify files compile**

Run: `go build ./internal/scanner/...`
Expected: Build succeeds with no errors

**Step 4: Commit**

```bash
git add internal/scanner/types.go internal/scanner/periodic.go
git commit -m "feat(scanner): add periodic scanner package structure"
```

---

## Task 2: Implement Core Scanner Methods

**Files:**
- Modify: `internal/scanner/periodic.go`

**Step 1: Add the IsHealthy and Status methods**

Add to `internal/scanner/periodic.go`:

```go
// IsHealthy returns whether the scanner is in a healthy state
func (s *PeriodicScanner) IsHealthy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.healthy
}

// Status returns the current scanner status for health reporting
func (s *PeriodicScanner) Status() ScannerStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	status := ScannerStatus{
		Healthy:      s.healthy,
		LastScan:     s.lastScan,
		LastSuccess:  s.lastSuccess,
		SkippedTicks: s.skippedTicks,
		Scanning:     s.scanning,
	}

	if s.lastError != nil {
		status.LastError = s.lastError.Error()
	}

	return status
}
```

**Step 2: Add the Start method with ticker loop**

Add to `internal/scanner/periodic.go`:

```go
// Start begins the periodic scanning loop. Blocks until context is cancelled.
func (s *PeriodicScanner) Start(ctx context.Context) error {
	s.logger.Info("scanner", "Periodic scanner starting",
		logging.F("interval", s.interval.String()),
		logging.F("watch_paths", len(s.watchPaths)))

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("scanner", "Periodic scanner stopped")
			return nil
		case <-ticker.C:
			s.tick()
		}
	}
}

func (s *PeriodicScanner) tick() {
	s.mu.Lock()
	if s.scanning {
		s.skippedTicks++
		s.mu.Unlock()
		s.logger.Warn("scanner", "Periodic scan skipped - previous scan still running",
			logging.F("skipped_ticks", s.skippedTicks))
		return
	}
	s.scanning = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.scanning = false
		s.mu.Unlock()
	}()

	if err := s.runScan(); err != nil {
		s.mu.Lock()
		s.lastError = err
		s.healthy = false
		s.mu.Unlock()
		s.logger.Error("scanner", "Periodic scan failed", err)
	} else {
		s.mu.Lock()
		s.lastSuccess = time.Now()
		s.lastError = nil
		s.healthy = true
		s.mu.Unlock()
	}

	s.mu.Lock()
	s.lastScan = time.Now()
	s.mu.Unlock()
}
```

**Step 3: Add stub runScan method**

Add to `internal/scanner/periodic.go`:

```go
func (s *PeriodicScanner) runScan() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("scan panic: %v", r)
			s.logger.Error("scanner", "Panic during periodic scan", err)
		}
	}()

	start := time.Now()
	s.logger.Info("scanner", "Periodic scan starting")

	// Phase 1: Scan watch directories
	processed, errors := s.scanWatchDirectories()

	// Phase 2: Reconcile activity logs
	retried, cleaned, reconcileErr := s.reconcileActivity()
	if reconcileErr != nil {
		s.logger.Warn("scanner", "Activity reconciliation had errors",
			logging.F("error", reconcileErr.Error()))
	}

	elapsed := time.Since(start)
	s.logger.Info("scanner", "Periodic scan complete",
		logging.F("duration_ms", elapsed.Milliseconds()),
		logging.F("processed", processed),
		logging.F("errors", errors),
		logging.F("retried", retried),
		logging.F("cleaned", cleaned))

	return nil
}
```

**Step 4: Add required import**

Update imports in `internal/scanner/periodic.go`:

```go
import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/daemon"
	"github.com/Nomadcxx/jellywatch/internal/logging"
)
```

**Step 5: Verify compilation**

Run: `go build ./internal/scanner/...`
Expected: Build fails - scanWatchDirectories and reconcileActivity not defined yet (expected)

**Step 6: Commit**

```bash
git add internal/scanner/periodic.go
git commit -m "feat(scanner): add Start loop and health status methods"
```

---

## Task 3: Implement Watch Directory Scanning

**Files:**
- Modify: `internal/scanner/periodic.go`

**Step 1: Add the scanWatchDirectories method**

Add to `internal/scanner/periodic.go`:

```go
import (
	"os"
	"path/filepath"

	"github.com/Nomadcxx/jellywatch/internal/watcher"
)
```

```go
func (s *PeriodicScanner) scanWatchDirectories() (processed int, errors int) {
	for _, watchPath := range s.watchPaths {
		s.logger.Info("scanner", "Scanning directory", logging.F("path", watchPath))

		err := filepath.Walk(watchPath, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				s.logger.Warn("scanner", "Directory inaccessible during scan",
					logging.F("path", path),
					logging.F("error", walkErr.Error()))
				return nil // Continue scanning other directories
			}

			if info.IsDir() {
				return nil
			}

			if !s.handler.IsMediaFile(path) {
				return nil
			}

			event := watcher.FileEvent{
				Type: watcher.EventCreate,
				Path: path,
			}

			if err := s.handler.HandleFileEvent(event); err != nil {
				s.logger.Warn("scanner", "Failed to process file during scan",
					logging.F("path", path),
					logging.F("error", err.Error()))
				errors++
			} else {
				processed++
			}

			return nil
		})

		if err != nil {
			s.logger.Warn("scanner", "Error walking directory",
				logging.F("path", watchPath),
				logging.F("error", err.Error()))
		}
	}

	s.logger.Info("scanner", "Watch directory scan complete",
		logging.F("processed", processed),
		logging.F("errors", errors))

	return processed, errors
}
```

**Step 2: Add stub reconcileActivity**

Add temporary stub to allow compilation:

```go
func (s *PeriodicScanner) reconcileActivity() (retried int, cleaned int, err error) {
	// TODO: Implement in next task
	return 0, 0, nil
}
```

**Step 3: Verify compilation**

Run: `go build ./internal/scanner/...`
Expected: Build succeeds

**Step 4: Commit**

```bash
git add internal/scanner/periodic.go
git commit -m "feat(scanner): implement watch directory scanning"
```

---

## Task 4: Implement Activity Log Reconciliation

**Files:**
- Create: `internal/scanner/reconcile.go`

**Step 1: Create reconcile.go with helper types**

```go
// internal/scanner/reconcile.go
package scanner

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/activity"
	"github.com/Nomadcxx/jellywatch/internal/logging"
	"github.com/Nomadcxx/jellywatch/internal/watcher"
)

const (
	retryWindowHours   = 24
	cleanupWindowDays  = 7
)
```

**Step 2: Implement reconcileActivity in reconcile.go**

```go
func (s *PeriodicScanner) reconcileActivity() (retried int, cleaned int, err error) {
	if s.activityDir == "" {
		return 0, 0, nil
	}

	now := time.Now()
	retryWindow := now.Add(-retryWindowHours * time.Hour)
	cleanupWindow := now.Add(-cleanupWindowDays * 24 * time.Hour)

	entries, err := os.ReadDir(s.activityDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}

	var toClean []string

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasPrefix(name, "activity-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}

		filePath := filepath.Join(s.activityDir, name)
		fileRetried, fileToClean, fileErr := s.processActivityFile(filePath, retryWindow, cleanupWindow)
		if fileErr != nil {
			s.logger.Warn("scanner", "Error processing activity file",
				logging.F("file", name),
				logging.F("error", fileErr.Error()))
			continue
		}

		retried += fileRetried
		if fileToClean {
			toClean = append(toClean, filePath)
		}
	}

	// Clean files that are entirely old failures
	for _, path := range toClean {
		if err := s.cleanActivityFile(path, cleanupWindow); err != nil {
			s.logger.Warn("scanner", "Error cleaning activity file",
				logging.F("path", path),
				logging.F("error", err.Error()))
		} else {
			cleaned++
		}
	}

	s.logger.Info("scanner", "Activity reconciliation complete",
		logging.F("retried", retried),
		logging.F("cleaned", cleaned))

	return retried, cleaned, nil
}
```

**Step 3: Add processActivityFile method**

```go
func (s *PeriodicScanner) processActivityFile(path string, retryWindow, cleanupWindow time.Time) (retried int, shouldClean bool, err error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	hasRecentOrSuccess := false
	hasOldFailures := false

	for scanner.Scan() {
		var entry activity.Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}

		if entry.Success {
			hasRecentOrSuccess = true
			continue
		}

		// Failed entry
		if entry.Timestamp.After(retryWindow) {
			// Recent failure - retry it
			hasRecentOrSuccess = true
			if entry.Source != "" {
				if retryErr := s.retryTransfer(entry); retryErr == nil {
					retried++
				}
			}
		} else if entry.Timestamp.Before(cleanupWindow) {
			// Old failure - mark for cleanup
			hasOldFailures = true
		} else {
			// Between retry and cleanup window - keep but don't retry
			hasRecentOrSuccess = true
		}
	}

	// Only clean if file has old failures and nothing recent/successful
	shouldClean = hasOldFailures && !hasRecentOrSuccess

	return retried, shouldClean, scanner.Err()
}
```

**Step 4: Add retryTransfer and cleanActivityFile methods**

```go
func (s *PeriodicScanner) retryTransfer(entry activity.Entry) error {
	// Check if source file still exists
	if _, err := os.Stat(entry.Source); os.IsNotExist(err) {
		return err
	}

	s.logger.Info("scanner", "Retrying failed transfer",
		logging.F("source", entry.Source))

	event := watcher.FileEvent{
		Type: watcher.EventCreate,
		Path: entry.Source,
	}

	return s.handler.HandleFileEvent(event)
}

func (s *PeriodicScanner) cleanActivityFile(path string, cleanupWindow time.Time) error {
	// Read all entries, keep only successes and recent failures
	file, err := os.Open(path)
	if err != nil {
		return err
	}

	var keepEntries []activity.Entry
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		var entry activity.Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}

		// Keep successful entries and failures newer than cleanup window
		if entry.Success || entry.Timestamp.After(cleanupWindow) {
			keepEntries = append(keepEntries, entry)
		}
	}
	file.Close()

	if err := scanner.Err(); err != nil {
		return err
	}

	// If nothing to keep, remove the file
	if len(keepEntries) == 0 {
		return os.Remove(path)
	}

	// Rewrite file with kept entries
	tmpPath := path + ".tmp"
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	for _, entry := range keepEntries {
		line, _ := json.Marshal(entry)
		tmpFile.Write(append(line, '\n'))
	}
	tmpFile.Close()

	return os.Rename(tmpPath, path)
}
```

**Step 5: Remove stub from periodic.go**

Remove the stub `reconcileActivity` from `internal/scanner/periodic.go` (the real implementation is now in reconcile.go).

**Step 6: Verify compilation**

Run: `go build ./internal/scanner/...`
Expected: Build succeeds

**Step 7: Commit**

```bash
git add internal/scanner/reconcile.go internal/scanner/periodic.go
git commit -m "feat(scanner): implement activity log reconciliation"
```

---

## Task 5: Write Unit Tests

**Files:**
- Create: `internal/scanner/periodic_test.go`

**Step 1: Create test file with basic tests**

```go
// internal/scanner/periodic_test.go
package scanner

import (
	"context"
	"sync"
	"testing"
	"time"
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
		logger:   noopLogger(),
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
		logger:     noopLogger(),
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

// noopLogger returns a logger that discards all output
func noopLogger() *logging.Logger {
	// Use the Nop logger from the logging package
	return logging.Nop()
}
```

**Step 2: Add import for logging**

```go
import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/logging"
)
```

**Step 3: Run tests to verify they pass**

Run: `go test ./internal/scanner/... -v`
Expected: All tests pass

**Step 4: Commit**

```bash
git add internal/scanner/periodic_test.go
git commit -m "test(scanner): add unit tests for periodic scanner"
```

---

## Task 6: Integrate Scanner into Health Server

**Files:**
- Modify: `internal/daemon/server.go`

**Step 1: Add scanner field to Server struct**

Update `internal/daemon/server.go`:

```go
import (
	"github.com/Nomadcxx/jellywatch/internal/scanner"
)

type Server struct {
	httpServer *http.Server
	handler    *MediaHandler
	scanner    *scanner.PeriodicScanner // Add this field
	startTime  time.Time
	mu         sync.RWMutex
	healthy    bool
	logger     *logging.Logger
}
```

**Step 2: Update NewServer signature**

```go
func NewServer(handler *MediaHandler, periodicScanner *scanner.PeriodicScanner, addr string, logger *logging.Logger) *Server {
	if logger == nil {
		logger = logging.Nop()
	}
	s := &Server{
		handler:   handler,
		scanner:   periodicScanner,
		startTime: time.Now(),
		healthy:   true,
		logger:    logger,
	}
	// ... rest unchanged
}
```

**Step 3: Update HealthResponse to include scanner status**

```go
type HealthResponse struct {
	Status        string                `json:"status"`
	Uptime        string                `json:"uptime"`
	Timestamp     time.Time             `json:"timestamp"`
	ScannerStatus *scanner.ScannerStatus `json:"scanner,omitempty"`
}
```

**Step 4: Update handleHealth to include scanner status**

```go
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	healthy := s.healthy
	s.mu.RUnlock()

	// Check scanner health too
	scannerHealthy := true
	var scannerStatus *scanner.ScannerStatus
	if s.scanner != nil {
		scannerHealthy = s.scanner.IsHealthy()
		status := s.scanner.Status()
		scannerStatus = &status
	}

	overallHealthy := healthy && scannerHealthy

	response := HealthResponse{
		Uptime:        time.Since(s.startTime).Round(time.Second).String(),
		Timestamp:     time.Now(),
		ScannerStatus: scannerStatus,
	}

	if overallHealthy {
		response.Status = "healthy"
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	} else if healthy && !scannerHealthy {
		response.Status = "degraded"
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK) // Degraded but still serving
	} else {
		response.Status = "unhealthy"
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(response)
}
```

**Step 5: Verify compilation**

Run: `go build ./internal/daemon/...`
Expected: Build succeeds

**Step 6: Commit**

```bash
git add internal/daemon/server.go
git commit -m "feat(daemon): integrate scanner status into health endpoint"
```

---

## Task 7: Integrate Scanner into jellywatchd Main

**Files:**
- Modify: `cmd/jellywatchd/main.go`

**Step 1: Add scanner import**

Add to imports in `cmd/jellywatchd/main.go`:

```go
import (
	// ... existing imports
	"github.com/Nomadcxx/jellywatch/internal/scanner"
)
```

**Step 2: Parse scan interval and create scanner**

Add after `handler := daemon.NewMediaHandler(...)` (around line 162):

```go
	// Parse scan frequency
	scanInterval, err := time.ParseDuration(cfg.Daemon.ScanFrequency)
	if err != nil {
		logger.Warn("daemon", "Invalid scan_frequency, using default",
			logging.F("configured", cfg.Daemon.ScanFrequency),
			logging.F("default", "5m"))
		scanInterval = 5 * time.Minute
	}

	// Create periodic scanner
	periodicScanner := scanner.NewPeriodicScanner(scanner.ScannerConfig{
		Interval:    scanInterval,
		WatchPaths:  watchPaths,
		Handler:     handler,
		Logger:      logger,
		ActivityDir: filepath.Join(configDir, "activity"),
	})
```

**Step 3: Update NewServer call to include scanner**

Change:
```go
healthServer := daemon.NewServer(handler, healthAddr, logger)
```

To:
```go
healthServer := daemon.NewServer(handler, periodicScanner, healthAddr, logger)
```

**Step 4: Add scanner to goroutine pool**

Add after the existing goroutines (around line 210):

```go
	go func() {
		errChan <- periodicScanner.Start(ctx)
	}()
```

Update errChan buffer size from 2 to 3:
```go
errChan := make(chan error, 3)
```

**Step 5: Verify compilation**

Run: `go build ./cmd/jellywatchd/...`
Expected: Build succeeds

**Step 6: Commit**

```bash
git add cmd/jellywatchd/main.go
git commit -m "feat(daemon): integrate periodic scanner into jellywatchd"
```

---

## Task 8: Update Installer Text

**Files:**
- Modify: `cmd/installer/types.go`
- Modify: `cmd/installer/screens.go`

**Step 1: Fix scan frequency option text**

In `cmd/installer/types.go`, change line ~251:

```go
// Before
var scanFrequencyOptions = []string{
	"Every 5 minutes",
	"Every 10 minutes",
	"Every 30 minutes",
	"Hourly",
	"Daily (3:00 AM)",
}

// After
var scanFrequencyOptions = []string{
	"Every 5 minutes",
	"Every 10 minutes",
	"Every 30 minutes",
	"Hourly",
	"Every 24 hours",
}
```

**Step 2: Update daemon screen description**

In `cmd/installer/screens.go`, around line 381, change:

```go
// Before
b.WriteString("\n" + lipgloss.NewStyle().Foreground(FgMuted).Render(
	"The daemon monitors watch folders and organizes new media"))

// After
b.WriteString("\n" + lipgloss.NewStyle().Foreground(FgMuted).Render(
	"The daemon monitors watch folders and runs periodic scans to catch missed files"))
```

**Step 3: Verify compilation**

Run: `go build ./cmd/installer/...`
Expected: Build succeeds

**Step 4: Commit**

```bash
git add cmd/installer/types.go cmd/installer/screens.go
git commit -m "fix(installer): update scan frequency text for accuracy"
```

---

## Task 9: Build and Manual Test

**Files:** None (manual testing)

**Step 1: Build all binaries**

Run: `go build ./...`
Expected: All packages build successfully

**Step 2: Run unit tests**

Run: `go test ./... -v`
Expected: All tests pass

**Step 3: Start daemon with short interval for testing**

Create test config or modify existing to use short interval:
```bash
# In ~/.config/jellywatch/config.toml, temporarily set:
# scan_frequency = "30s"
```

Run: `./build/jellywatchd`

**Step 4: Verify logs show periodic scans**

Expected log output:
```
[INFO] [scanner] Periodic scanner starting | interval=30s | watch_paths=2
[INFO] [scanner] Periodic scan starting
[INFO] [scanner] Scanning directory | path=/mnt/NVME3/Sabnzbd/complete/movies
[INFO] [scanner] Scanning directory | path=/mnt/NVME3/Sabnzbd/complete/tv
[INFO] [scanner] Watch directory scan complete | processed=0 | errors=0
[INFO] [scanner] Activity reconciliation complete | retried=0 | cleaned=0
[INFO] [scanner] Periodic scan complete | duration_ms=XX
```

**Step 5: Check health endpoint includes scanner**

Run: `curl localhost:8686/health | jq`

Expected:
```json
{
  "status": "healthy",
  "uptime": "1m30s",
  "timestamp": "...",
  "scanner": {
    "healthy": true,
    "last_scan": "...",
    "last_success": "...",
    "skipped_ticks": 0,
    "scanning": false
  }
}
```

**Step 6: Final commit**

```bash
git add -A
git commit -m "feat: complete periodic scanner implementation"
```

---

## Summary

| Task | Description | Files |
|------|-------------|-------|
| 1 | Create scanner package structure | `internal/scanner/types.go`, `periodic.go` |
| 2 | Implement core scanner methods | `internal/scanner/periodic.go` |
| 3 | Implement watch directory scanning | `internal/scanner/periodic.go` |
| 4 | Implement activity reconciliation | `internal/scanner/reconcile.go` |
| 5 | Write unit tests | `internal/scanner/periodic_test.go` |
| 6 | Integrate into health server | `internal/daemon/server.go` |
| 7 | Integrate into jellywatchd main | `cmd/jellywatchd/main.go` |
| 8 | Update installer text | `cmd/installer/types.go`, `screens.go` |
| 9 | Build and manual test | N/A |
