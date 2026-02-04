# Periodic Scanner Design

**Date:** 2026-01-25
**Status:** Approved
**Author:** Claude + User

## Problem Statement

The `jellywatchd` daemon currently only processes files via:
1. One-time initial scan on startup
2. Real-time file system events (fsnotify/inotify)

The `scan_frequency` config option exists but is never used. Users expect periodic scans to catch files missed by the watcher (buffer overflow, network mount issues, daemon restarts).

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Scan behavior | Re-scan + Activity reconciliation | Catches missed files AND cleans up failed transfers |
| Overlap handling | Skip tick if busy | Prevents resource contention and duplicate processing |
| Error handling | Log and continue + Health degradation | Keeps daemon running while surfacing issues to monitoring |
| Reconciliation | Retry recent + Clean old failures | 24h retry window, 7-day cleanup window |
| Architecture | Dedicated Scanner Service | Clean separation, testable, follows existing patterns |
| Daily option | Simple interval ("Every 24 hours") | Avoids cron-style scheduling complexity |

## Architecture

### Component Structure

```
internal/scanner/
├── periodic.go        # Main PeriodicScanner implementation
├── reconcile.go       # Activity log reconciliation logic
└── periodic_test.go   # Unit tests
```

### PeriodicScanner Struct

```go
type PeriodicScanner struct {
    // Configuration
    interval     time.Duration
    watchPaths   []string
    handler      *daemon.MediaHandler
    logger       *logging.Logger
    activityDir  string

    // State tracking
    mu           sync.Mutex
    scanning     bool
    lastScan     time.Time
    lastSuccess  time.Time
    lastError    error
    skippedTicks int64

    // Health tracking
    healthy      bool
    degradedAt   time.Time
}

type ScannerConfig struct {
    Interval      time.Duration
    WatchPaths    []string
    Handler       *daemon.MediaHandler
    Logger        *logging.Logger
    ActivityDir   string  // ~/.config/jellywatch/activity
}
```

### Core Methods

| Method | Purpose |
|--------|---------|
| `Start(ctx context.Context) error` | Main loop - runs ticker, handles cancellation |
| `Stop()` | Graceful shutdown |
| `runScan() error` | Execute one full periodic scan cycle |
| `reconcileActivity() error` | Check activity logs, retry failures, cleanup old |
| `IsHealthy() bool` | For health endpoint integration |
| `Status() ScannerStatus` | Return metrics snapshot |

## Scan Cycle

### Phase 1: Watch Directory Scan

```go
func (s *PeriodicScanner) scanWatchDirectories() (processed int, errors int) {
    for _, watchPath := range s.watchPaths {
        filepath.Walk(watchPath, func(path string, info os.FileInfo) error {
            if !info.IsDir() && s.handler.IsMediaFile(path) {
                event := watcher.FileEvent{Type: watcher.EventCreate, Path: path}
                if err := s.handler.HandleFileEvent(event); err != nil {
                    errors++
                } else {
                    processed++
                }
            }
            return nil
        })
    }
    return processed, errors
}
```

The handler's existing debouncing and duplicate detection prevents re-processing already-transferred files.

### Phase 2: Activity Log Reconciliation

```go
func (s *PeriodicScanner) reconcileActivity() error {
    now := time.Now()
    retryWindow := now.Add(-24 * time.Hour)
    cleanupWindow := now.Add(-7 * 24 * time.Hour)

    entries, _ := s.loadActivityEntries()

    for _, entry := range entries {
        if !entry.Success && entry.Timestamp.After(retryWindow) {
            // Retry recent failures (< 24 hours)
            s.retryTransfer(entry)
        } else if !entry.Success && entry.Timestamp.Before(cleanupWindow) {
            // Clean old failures (> 7 days)
            s.markForCleanup(entry)
        }
    }

    s.pruneCleanedEntries()
    return nil
}
```

## Health Integration

### Health States

- **Healthy**: Recent scan succeeded
- **Degraded**: Recent scan failed, but still attempting

### Health Endpoint Updates

```go
type HealthStatus struct {
    Healthy       bool           `json:"healthy"`
    HandlerStats  StatsSnapshot  `json:"handler_stats"`
    ScannerStatus ScannerStatus  `json:"scanner_status"`
}

type ScannerStatus struct {
    Healthy      bool      `json:"healthy"`
    LastScan     time.Time `json:"last_scan"`
    LastSuccess  time.Time `json:"last_success"`
    LastError    string    `json:"last_error,omitempty"`
    SkippedTicks int64     `json:"skipped_ticks"`
    Scanning     bool      `json:"scanning"`
}
```

## Error Handling

| Error Type | Action | Health Impact |
|------------|--------|---------------|
| Single file fails | Log, continue scanning others | None |
| Watch dir inaccessible | Log warning, skip that dir | None |
| All dirs fail | Log error | Degraded |
| Activity log read fails | Log error, skip reconciliation | None |
| Panic during scan | Recover, log, mark degraded | Degraded |

### Panic Recovery

```go
func (s *PeriodicScanner) runScan() (err error) {
    defer func() {
        if r := recover(); r != nil {
            err = fmt.Errorf("scan panic: %v", r)
            s.logger.Error("scanner", "Panic during scan", err)
        }
    }()
    // ... scan logic
}
```

## Logging

```go
// Scan cycle start
logger.Info("scanner", "Periodic scan starting", logging.F("interval", s.interval))

// Scan skipped
logger.Warn("scanner", "Periodic scan skipped - previous scan still running",
    logging.F("skipped_ticks", s.skippedTicks))

// Watch directory results
logger.Info("scanner", "Watch directory scan complete",
    logging.F("processed", processed), logging.F("errors", errorCount))

// Reconciliation results
logger.Info("scanner", "Activity reconciliation complete",
    logging.F("retried", retriedCount), logging.F("cleaned", cleanedCount))

// Scan complete
logger.Info("scanner", "Periodic scan complete",
    logging.F("duration_ms", elapsed.Milliseconds()))

// Scan failure
logger.Error("scanner", "Periodic scan failed", err,
    logging.F("will_retry_next_interval", true))
```

## Installer Changes

### 1. Update Scan Frequency Options

**File:** `cmd/installer/types.go`

```go
// Before
var scanFrequencyOptions = []string{
    "Every 5 minutes",
    "Every 10 minutes",
    "Every 30 minutes",
    "Hourly",
    "Daily (3:00 AM)",  // Misleading
}

// After
var scanFrequencyOptions = []string{
    "Every 5 minutes",
    "Every 10 minutes",
    "Every 30 minutes",
    "Hourly",
    "Every 24 hours",  // Accurate
}
```

### 2. Update Daemon Screen Description

**File:** `cmd/installer/screens.go`

```go
// Before
"The daemon monitors watch folders and organizes new media"

// After
"The daemon monitors watch folders and runs periodic scans to catch missed files"
```

### 3. Service File

No changes required - scan frequency read from config.toml.

## Integration in jellywatchd

```go
// In runDaemon(), after creating handler:

scanInterval, err := time.ParseDuration(cfg.Daemon.ScanFrequency)
if err != nil {
    scanInterval = 5 * time.Minute // default
}

scanner := scanner.NewPeriodicScanner(scanner.ScannerConfig{
    Interval:    scanInterval,
    WatchPaths:  watchPaths,
    Handler:     handler,
    Logger:      logger,
    ActivityDir: filepath.Join(configDir, "activity"),
})

// Add to goroutine pool:
go func() {
    errChan <- scanner.Start(ctx)
}()

// Update health server to include scanner:
healthServer := daemon.NewServer(handler, scanner, healthAddr, logger)
```

## Testing Strategy

### Unit Tests

- `TestPeriodicScanner_SkipsWhenBusy` - Verify tick skipped when scan in progress
- `TestPeriodicScanner_RunsScanOnTick` - Verify files processed on tick
- `TestPeriodicScanner_HealthDegradedOnError` - Verify health state on failure
- `TestPeriodicScanner_HealthRestoredOnSuccess` - Verify recovery after success

### Reconciliation Tests

- `TestReconcile_RetriesRecentFailures` - Entries < 24h retried
- `TestReconcile_IgnoresOldFailures` - Entries > 24h not retried
- `TestReconcile_CleansVeryOldFailures` - Entries > 7 days removed
- `TestReconcile_PreservesSuccessfulEntries` - Successful entries kept

### Manual Testing Checklist

- [ ] Daemon starts, logs show scan interval
- [ ] Periodic scans run at configured interval
- [ ] Logs show scan start/complete messages
- [ ] Health endpoint shows scanner status
- [ ] Skipped ticks logged when scan takes longer than interval
- [ ] Failed transfers retried within 24h window
- [ ] Old failures cleaned up after 7 days

## Files to Create/Modify

| File | Action |
|------|--------|
| `internal/scanner/periodic.go` | Create |
| `internal/scanner/reconcile.go` | Create |
| `internal/scanner/periodic_test.go` | Create |
| `cmd/jellywatchd/main.go` | Modify - integrate scanner |
| `internal/daemon/server.go` | Modify - add scanner status to health |
| `cmd/installer/types.go` | Modify - fix "Daily" option text |
| `cmd/installer/screens.go` | Modify - update description |
