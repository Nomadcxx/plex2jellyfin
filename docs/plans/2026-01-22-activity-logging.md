# Activity Logging and Monitoring Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Create an audit logging system for jellywatchd that tracks file organization activity with parse method, AI usage, and notification results, enabling post-hoc analysis for beta testing.

**Architecture:** Lightweight JSONL-based activity log with daily rotation. Internal/activity package writes structured entries; MediaHandler invokes logger after each operation; New CLI command `jellywatch monitor` queries and formats logs.

**Tech Stack:** Go 1.23, Cobra CLI, JSONL format, time-based file rotation

---

## Task 1: Create activity logger package and Entry struct

**Files:**
- Create: `internal/activity/logger.go`
- Create: `internal/activity/logger_test.go`

**Step 1: Write Entry struct and Logger**

```go
// Package: internal/activity/logger.go
package activity

import (
    "encoding/json"
    "os"
    "path/filepath"
    "strings"
    "sync"
    "time"
)

type ParseMethod string

const (
    MethodRegex ParseMethod = "regex"
    MethodAI    ParseMethod = "ai"
    MethodCache ParseMethod = "cache"
)

type Entry struct {
    Timestamp      time.Time  `json:"ts"`
    Action         string     `json:"action"`
    Source         string     `json:"source"`
    Target         string     `json:"target,omitempty"`
    MediaType      string     `json:"media_type"`
    ParseMethod    ParseMethod `json:"parse_method"`
    ParsedTitle    string     `json:"parsed_title"`
    ParsedYear     *int       `json:"parsed_year,omitempty"`
    AIConfidence   *float64   `json:"ai_confidence,omitempty"`
    Success        bool       `json:"success"`
    Bytes          int64      `json:"bytes,omitempty"`
    DurationMs     int64      `json:"duration_ms,omitempty"`
    SonarrNotified bool       `json:"sonarr_notified"`
    RadarrNotified bool       `json:"radarr_notified"`
    Error          string     `json:"error,omitempty"`
}

type Logger struct {
    configDir   string
    logDir      string
    currentFile *os.File
    currentDate string
    mu          sync.Mutex
}

func NewLogger(configDir string) (*Logger, error) {
    logDir := filepath.Join(configDir, "activity")

    if err := os.MkdirAll(logDir, 0755); err != nil {
        return nil, err
    }

    return &Logger{
        configDir: configDir,
        logDir:    logDir,
    }, nil
}

func (l *Logger) Log(entry Entry) error {
    l.mu.Lock()
    defer l.mu.Unlock()

    entry.Timestamp = time.Now()

    line, err := json.Marshal(entry)
    if err != nil {
        return err
    }

    today := time.Now().Format("2006-01-02")

    if l.currentDate != today || l.currentFile == nil {
        if err := l.rotateFile(today); err != nil {
            return err
        }
    }

    if l.currentFile == nil {
        return nil
    }

    _, err = l.currentFile.Write(append(line, '\n'))
    return err
}

func (l *Logger) Close() error {
    if l.currentFile != nil {
        return l.currentFile.Close()
    }
    return nil
}

func (l *Logger) PruneOld(retentionDays int) error {
    cutoff := time.Now().AddDate(0, 0, -retentionDays)

    entries, err := os.ReadDir(l.logDir)
    if err != nil {
        return err
    }

    for _, entry := range entries {
        if !strings.HasPrefix(entry.Name(), "activity-") || !strings.HasSuffix(entry.Name(), ".jsonl") {
            continue
        }

        if entry.IsDir() {
            continue
        }

        name := strings.TrimPrefix(entry.Name(), "activity-")
        name = strings.TrimSuffix(name, ".jsonl")

        fileDate, err := time.Parse("2006-01-02", name)
        if err != nil {
            continue
        }

        if fileDate.Before(cutoff) {
            os.Remove(filepath.Join(l.logDir, entry.Name()))
        }
    }

    return nil
}

func (l *Logger) rotateFile(date string) error {
    if l.currentFile != nil {
        l.currentFile.Close()
    }

    filePath := filepath.Join(l.logDir, "activity-"+date+".jsonl")

    file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
    if err != nil {
        return err
    }

    l.currentFile = file
    l.currentDate = date

    return nil
}

func (l *Logger) GetLogDir() string {
    return l.logDir
}
```

**Step 2: Write basic tests**

```go
// File: internal/activity/logger_test.go
package activity

import (
    "encoding/json"
    "os"
    "path/filepath"
    "testing"
    "time"
)

func TestNewLogger(t *testing.T) {
    tmpDir, err := os.MkdirTemp("", "jellywatch-activity-*")
    if err != nil {
        t.Fatal(err)
    }
    defer os.RemoveAll(tmpDir)

    logger, err := NewLogger(tmpDir)
    if err != nil {
        t.Fatalf("failed to create logger: %v", err)
    }
    defer logger.Close()

    if logger.GetLogDir() != filepath.Join(tmpDir, "activity") {
        t.Errorf("expected log dir %s, got %s", filepath.Join(tmpDir, "activity"), logger.GetLogDir())
    }
}

func TestLogEntry(t *testing.T) {
    tmpDir, err := os.MkdirTemp("", "jellywatch-activity-*")
    if err != nil {
        t.Fatal(err)
    }
    defer os.RemoveAll(tmpDir)

    logger, err := NewLogger(tmpDir)
    if err != nil {
        t.Fatal(err)
    }
    defer logger.Close()

    year := 2024
    confidence := 0.92
    entry := Entry{
        Action:         "organize",
        Source:         "/downloads/test.mkv",
        Target:         "/movies/Test (2024)/test.mkv",
        MediaType:      "movie",
        ParseMethod:    MethodAI,
        ParsedTitle:    "Test Movie",
        ParsedYear:     &year,
        AIConfidence:   &confidence,
        Success:        true,
        Bytes:          1234567,
        DurationMs:      1000,
        SonarrNotified: false,
        RadarrNotified: true,
    }

    if err := logger.Log(entry); err != nil {
        t.Fatalf("failed to log entry: %v", err)
    }

    logFile := filepath.Join(tmpDir, "activity-"+time.Now().Format("2006-01-02")+".jsonl")
    content, err := os.ReadFile(logFile)
    if err != nil {
        t.Fatalf("failed to read log file: %v", err)
    }

    var logged Entry
    if err := json.Unmarshal(content, &logged); err != nil {
        t.Fatalf("failed to parse logged entry: %v", err)
    }

    if logged.Action != entry.Action {
        t.Errorf("expected action %s, got %s", entry.Action, logged.Action)
    }

    if logged.AIConfidence == nil || *logged.AIConfidence != confidence {
        t.Errorf("expected confidence %f, got %v", confidence, logged.AIConfidence)
    }
}

func TestPruneOld(t *testing.T) {
    tmpDir, err := os.MkdirTemp("", "jellywatch-activity-*")
    if err != nil {
        t.Fatal(err)
    }
    defer os.RemoveAll(tmpDir)

    logger, err := NewLogger(tmpDir)
    if err != nil {
        t.Fatal(err)
    }
    defer logger.Close()

    oldDate := time.Now().AddDate(0, 0, -10)
    oldFile := filepath.Join(tmpDir, "activity", "activity-"+oldDate.Format("2006-01-02")+".jsonl")
    if err := os.WriteFile(oldFile, []byte("test"), 0644); err != nil {
        t.Fatal(err)
    }

    recentDate := time.Now()
    recentFile := filepath.Join(tmpDir, "activity", "activity-"+recentDate.Format("2006-01-02")+".jsonl")
    if err := os.WriteFile(recentFile, []byte("test"), 0644); err != nil {
        t.Fatal(err)
    }

    if err := logger.PruneOld(7); err != nil {
        t.Fatalf("failed to prune old logs: %v", err)
    }

    if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
        t.Error("old file should have been pruned")
    }

    if _, err := os.Stat(recentFile); err != nil {
        t.Error("recent file should still exist")
    }
}
```

**Step 3: Run tests to verify they pass**

```bash
go test ./internal/activity/... -v
```

Expected: All 3 tests PASS

**Step 4: Commit**

```bash
git add internal/activity/
git commit -m "feat: add activity logger package with JSONL logging"
```

---

## Task 2: Integrate activity logger into MediaHandler

**Files:**
- Modify: `internal/daemon/handler.go:1-316`
- Modify: `internal/daemon/handler.go:108` (Add activity logger field)

**Step 1: Add activity logger field to MediaHandler struct**

```go
// In internal/daemon/handler.go
import "github.com/Nomadcxx/jellywatch/internal/activity"

type MediaHandler struct {
    // ... existing fields ...
    activityLogger *activity.Logger  // Add this line
}
```

**Step 2: Update MediaHandlerConfig to include activity logger**

```go
// Add to MediaHandlerConfig
type MediaHandlerConfig struct {
    // ... existing fields ...
    ConfigDir string  // Add this line
}
```

**Step 3: Update NewMediaHandler to initialize activity logger**

```go
func NewMediaHandler(cfg MediaHandlerConfig) *MediaHandler {
    // ... existing setup code ...

    var activityLogger *activity.Logger
    if cfg.ConfigDir != "" {
        var err error
        activityLogger, err = activity.NewLogger(cfg.ConfigDir)
        if err != nil {
            logger.Warn("handler", "Failed to create activity logger", logging.F("error", err.Error()))
        } else {
            logger.Info("handler", "Activity logger initialized", logging.F("log_dir", activityLogger.GetLogDir()))
        }
    }

    return &MediaHandler{
        // ... existing fields ...
        activityLogger: activityLogger,  // Add this
    }
}
```

**Step 4: Add logEntry helper method to MediaHandler**

```go
func (h *MediaHandler) logEntry(result *organizer.OrganizationResult, mediaType notify.MediaType, parseMethod activity.ParseMethod, parsedTitle string, parsedYear *int, aiConfidence float64, duration time.Duration) {
    if h.activityLogger == nil {
        return
    }

    entry := activity.Entry{
        Action:      "organize",
        Source:      result.SourcePath,
        MediaType:   string(mediaType),
        ParseMethod:  parseMethod,
        ParsedTitle: parsedTitle,
        ParsedYear:  parsedYear,
        Success:     result.Success,
        DurationMs:  duration.Milliseconds(),
    }

    if result.Success {
        entry.Target = result.TargetPath
        entry.Bytes = result.BytesCopied
        entry.SonarrNotified = h.notifyManager != nil && mediaType == notify.MediaTypeTVEpisode
        entry.RadarrNotified = h.notifyManager != nil && mediaType == notify.MediaTypeMovie
    }

    if aiConfidence > 0 {
        entry.AIConfidence = &aiConfidence
    }

    if !result.Success && result.Error != nil {
        entry.Error = result.Error.Error()
    }

    _ = h.activityLogger.Log(entry)
}
```

**Step 5: Call logEntry in processFile after organization**

```go
func (h *MediaHandler) processFile(path string) {
    startTime := time.Now()  // Add at start of function

    // ... existing code that parses and organizes ...
    // Assume we have: result, mediaType, parsedTitle, parsedYear from existing code

    var parseMethod activity.ParseMethod = activity.MethodRegex
    var aiConfidence float64

    // If AI integrator exists, add logic to detect AI usage
    // For now, default to regex

    duration := time.Since(startTime)

    h.logEntry(result, mediaType, parseMethod, parsedTitle, parsedYear, aiConfidence, duration)

    // ... rest of existing code
}
```

**Step 6: Update Shutdown to close activity logger**

```go
func (h *MediaHandler) Shutdown() {
    h.mu.Lock()
    defer h.mu.Unlock()

    // ... existing cleanup ...

    if h.activityLogger != nil {
        h.activityLogger.Close()
    }
}
```

**Step 7: Run tests**

```bash
go test ./internal/daemon/... -v
```

Expected: Tests still pass (we haven't added activity-specific tests to handler yet)

**Step 8: Commit**

```bash
git add internal/daemon/handler.go
git commit -m "feat: integrate activity logger into MediaHandler"
```

---

## Task 3: Update main.go to pass ConfigDir and prune logs on startup

**Files:**
- Modify: `cmd/jellywatchd/main.go:52-218`

**Step 1: Get config directory path**

```go
import "path/filepath"

// In runDaemon, after loading config
configDir := filepath.Dir(cfgFile)
if configDir == "" {
    configDir = filepath.Join(os.Getenv("HOME"), ".config", "jellywatch")
}
```

**Step 2: Pass configDir to MediaHandlerConfig**

```go
handler := daemon.NewMediaHandler(daemon.MediaHandlerConfig{
    // ... existing fields ...
    ConfigDir: configDir,  // Add this
})
```

**Step 3: Add log pruning after handler creation**

```go
handler := daemon.NewMediaHandler(...)

logger.Info("daemon", "Pruning old activity logs (7 days retention)")
if err := handler.PruneActivityLogs(7); err != nil {
    logger.Warn("daemon", "Failed to prune old activity logs", logging.F("error", err.Error()))
}
```

**Step 4: Add PruneActivityLogs method to MediaHandler**

```go
func (h *MediaHandler) PruneActivityLogs(days int) error {
    if h.activityLogger == nil {
        return nil
    }
    return h.activityLogger.PruneOld(days)
}
```

**Step 5: Build and test**

```bash
go build ./cmd/jellywatchd/
```

Expected: Build succeeds

**Step 6: Commit**

```bash
git add cmd/jellywatchd/main.go internal/daemon/handler.go
git commit -m "feat: prune old activity logs on daemon startup"
```

---

## Task 4: Create jellywatch monitor CLI command

**Files:**
- Create: `cmd/jellywatch/monitor.go`
- Modify: `cmd/jellywatch/main.go`

**Step 1: Create monitor command**

```go
// cmd/jellywatch/monitor.go
package main

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "sort"
    "strings"
    "time"

    "github.com/Nomadcxx/jellywatch/internal/activity"
    "github.com/spf13/cobra"
)

var (
    days       int
    filterAction string
    filterMethod string
    filterSuccess *bool
    showDetails bool
)

var monitorCmd = &cobra.Command{
    Use:   "monitor",
    Short: "View activity logs from jellywatchd",
    RunE: runMonitor,
}

func newMonitorCmd() *cobra.Command {
    var (
        days          int
        filterAction  string
        filterMethod  string
        filterSuccess *bool
        showDetails   bool
    )

    cmd := &cobra.Command{
        Use:   "monitor",
        Short: "View activity logs from jellywatchd",
        RunE: func(cmd *cobra.Command, args []string) error {
            return runMonitor(days, filterAction, filterMethod, filterSuccess, showDetails)
        },
    }

    cmd.Flags().IntVarP(&days, "days", "d", 3, "Days of logs to show (default: 3)")
    cmd.Flags().StringVarP(&filterAction, "action", "a", "", "Filter by action")
    cmd.Flags().StringVarP(&filterMethod, "method", "m", "", "Filter by parse method (regex|ai|cache)")
    cmd.Flags().BoolVarP(filterSuccess, "success", "s", false, "Filter by success (true/false)")
    cmd.Flags().BoolVarP(&showDetails, "details", "v", false, "Show detailed JSON output")

    return cmd
}

// In main.go, add: rootCmd.AddCommand(newMonitorCmd())

func runMonitor(days int, filterAction, filterMethod string, filterSuccess *bool, showDetails bool) error {
    configDir := filepath.Join(os.Getenv("HOME"), ".config", "jellywatch")
    activityDir := filepath.Join(configDir, "activity")

    entries, err := loadActivityEntries(activityDir, days)
    if err != nil {
        return fmt.Errorf("failed to load activity logs: %w", err)
    }

    entries = filterEntries(entries, filterAction, filterMethod, filterSuccess)

    if showDetails {
        for _, entry := range entries {
            data, _ := json.MarshalIndent(entry, "", "  ")
            fmt.Println(string(data))
        }
        return nil
    }

    printSummary(entries, days)
    return nil
}

func loadActivityEntries(activityDir string, daysBack int) ([]activity.Entry, error) {
    var entries []activity.Entry

    cutoff := time.Now().AddDate(0, 0, -daysBack)

    dirEntries, err := os.ReadDir(activityDir)
    if err != nil {
        return nil, err
    }

    for _, dirEntry := range dirEntries {
        if strings.HasPrefix(dirEntry.Name(), "activity-") && strings.HasSuffix(dirEntry.Name(), ".jsonl") {
            filePath := filepath.Join(activityDir, dirEntry.Name())

            fileDate, err := time.Parse("2006-01-02", strings.TrimPrefix(strings.TrimSuffix(dirEntry.Name(), ".jsonl"), "activity-"))
            if err != nil {
                continue
            }

            if fileDate.Before(cutoff) {
                continue
            }

            fileEntries, err := loadFileEntries(filePath)
            if err != nil {
                return nil, err
            }

            entries = append(entries, fileEntries...)
        }
    }

    sort.Slice(entries, func(i, j int) bool {
        return entries[i].Timestamp.Before(entries[j].Timestamp)
    })

    return entries, nil
}

func loadFileEntries(filePath string) ([]activity.Entry, error) {
    content, err := os.ReadFile(filePath)
    if err != nil {
        return nil, err
    }

    var entries []activity.Entry
    lines := strings.Split(string(content), "\n")

    for _, line := range lines {
        line = strings.TrimSpace(line)
        if line == "" {
            continue
        }

        var entry activity.Entry
        if err := json.Unmarshal([]byte(line), &entry); err != nil {
            continue
        }

        entries = append(entries, entry)
    }

    return entries, nil
}

func filterEntries(entries []activity.Entry, filterAction, filterMethod string, filterSuccess *bool) []activity.Entry {
    var filtered []activity.Entry

    for _, entry := range entries {
        if filterAction != "" && entry.Action != filterAction {
            continue
        }

        if filterMethod != "" && string(entry.ParseMethod) != filterMethod {
            continue
        }

        if filterSuccess != nil {
            if *filterSuccess != entry.Success {
                continue
            }
        }

        filtered = append(filtered, entry)
    }

    return filtered
}

func printSummary(entries []activity.Entry, days int) {
    if len(entries) == 0 {
        fmt.Println("No activity entries found")
        return
    }

    var totalSuccess, totalFailed int64
    var methodCounts = make(map[string]int64)
    var totalBytes int64
    var totalDuration int64

    for _, entry := range entries {
        if entry.Success {
            totalSuccess++
            methodCounts[string(entry.ParseMethod)]++
            totalBytes += entry.Bytes
            totalDuration += entry.DurationMs
        } else {
            totalFailed++
        }
    }

    fmt.Printf("\n=== Activity Summary (%d days) ===\n", days)
    fmt.Printf("Total operations: %d\n", totalSuccess+totalFailed)
    fmt.Printf("  Success: %d\n", totalSuccess)
    fmt.Printf("  Failed: %d\n", totalFailed)

    if totalSuccess > 0 {
        successRate := float64(totalSuccess) / float64(totalSuccess+totalFailed) * 100
        fmt.Printf("  Success rate: %.1f%%\n", successRate)
    }

    fmt.Printf("\n--- Parse Methods ---\n")
    methods := []string{"regex", "ai", "cache"}
    for _, method := range methods {
        count := methodCounts[method]
        if count > 0 {
            pct := float64(count) / float64(totalSuccess) * 100
            fmt.Printf("  %s: %d (%.1f%%)\n", method, count, pct)
        }
    }

    if totalBytes > 0 {
        fmt.Printf("\n--- Data Transfer ---\n")
        fmt.Printf("  Total: %.2f GB\n", float64(totalBytes)/(1024*1024*1024))
        fmt.Printf("  Avg per op: %.2f MB\n", float64(totalBytes)/float64(totalSuccess)/(1024*1024))
    }

    if totalDuration > 0 {
        fmt.Printf("\n--- Performance ---\n")
        avgMs := totalDuration / totalSuccess
        fmt.Printf("  Avg duration: %d ms\n", avgMs)
        fmt.Printf("  Total time: %s\n", time.Duration(totalDuration)*time.Millisecond)
    }

    fmt.Printf("\n=== Recent Failures ===\n")
    failureCount := 0
    for i := len(entries) - 1; i >= 0 && failureCount < 10; i-- {
        entry := entries[i]
        if !entry.Success {
            fmt.Printf("  [%s] %s\n", entry.Timestamp.Format("2006-01-02 15:04"), entry.Error)
            failureCount++
        }
    }
}

// Note: Uses activity.Entry from internal/activity package (no duplicate struct needed)
```

**Step 2: Test monitor command**

```bash
go run ./cmd/jellywatch monitor --help
go run ./cmd/jellywatch monitor --days 1
```

Expected: Help text displays, then runs with 1 day filter

**Step 3: Commit**

```bash
git add cmd/jellywatch/monitor.go cmd/jellywatch/main.go
git commit -m "feat: add jellywatch monitor command for activity log viewing"
```

---

## Task 5: End-to-end integration test

**Files:**
- Create: `scripts/test-activity-logging.sh`

**Step 1: Create integration test script**

```bash
#!/bin/bash
# scripts/test-activity-logging.sh

set -e

echo "Building jellywatchd..."
go build -o /tmp/jellywatchd-test ./cmd/jellywatchd

echo "Building jellywatch CLI..."
go build -o /tmp/jellywatch-test ./cmd/jellywatch

echo "Creating test directories..."
TMPDIR=$(mktemp -d)
mkdir -p "$TMPDIR/downloads"
mkdir -p "$TMPDIR/movies"
mkdir -p "$TMPDIR/config/jellywatch"

echo "Writing minimal config..."
cat > "$TMPDIR/config/jellywatch/config.toml" << EOF
[watch]
movies = ["$TMPDIR/downloads"]

[libraries]
movies = ["$TMPDIR/movies"]

[logging]
file = "$TMPDIR/logs/jellywatchd.log"

[options]
dry_run = false
EOF

echo "Starting jellywatchd (background)..."
/tmp/jellywatchd-test --config "$TMPDIR/config/jellywatch/config.toml" &
DAEMON_PID=$!
sleep 2

echo "Creating test movie file..."
dd if=/dev/zero of="$TMPDIR/downloads/Test Movie 2024.mkv" bs=1M count=100

echo "Waiting for processing..."
sleep 3

echo "Stopping jellywatchd..."
kill $DAEMON_PID
wait $DAEMON_PID 2>/dev/null || true

echo "Running jellywatch monitor..."
/tmp/jellywatch-test monitor --days 1

echo "Checking activity log..."
ls -la "$TMPDIR/config/jellywatch/activity/"

echo "Cleanup..."
rm -rf "$TMPDIR"

echo "Integration test complete!"
```

**Step 2: Run integration test**

```bash
chmod +x scripts/test-activity-logging.sh
./scripts/test-activity-logging.sh
```

Expected: Movie file organized, activity log entry created, monitor shows summary

**Step 3: Commit**

```bash
git add scripts/test-activity-logging.sh
git commit -m "test: add integration test for activity logging"
```

---

## Task 6: Documentation

**Files:**
- Create: `docs/activity-logging.md`

**Step 1: Write documentation**

```markdown
# Activity Logging

## Overview

JellyWatch automatically logs all file organization operations to daily JSONL files for post-hoc analysis.

## Log Location

Logs are stored in: `~/.config/jellywatch/activity/`

Format: `activity-YYYY-MM-DD.jsonl`

## Log Entry Fields

| Field | Description |
|--------|-------------|
| `ts` | ISO 8601 timestamp |
| `action` | Operation type (currently only "organize") |
| `source` | Original file path |
| `target` | Destination path (if successful) |
| `media_type` | "movie" or "tv" |
| `parse_method` | "regex", "ai", or "cache" |
| `parsed_title` | Extracted title |
| `parsed_year` | Extracted year (if applicable) |
| `ai_confidence` | AI confidence score (if AI used) |
| `success` | `true` if operation succeeded |
| `bytes` | Bytes transferred (if successful) |
| `duration_ms` | Operation duration |
| `sonarr_notified` | `true` if Sonarr was notified |
| `radarr_notified` | `true` if Radarr was notified |
| `error` | Error message (if failed) |

## Viewing Logs

```bash
# Show last 3 days
jellywatch monitor

# Show last 7 days
jellywatch monitor --days 7

# Show only AI-parsed entries
jellywatch monitor --method ai

# Show only failures
jellywatch monitor --success false

# Show detailed JSON
jellywatch monitor --details
```

## Log Rotation

- New file created each day at midnight
- Files older than 7 days automatically pruned on daemon startup
- Retention period configurable in `jellywatchd` startup code

## Example Entry

```json
{
  "ts": "2026-01-22T22:30:00Z",
  "action": "organize",
  "source": "/downloads/Movie.Name.2024.1080p.mkv",
  "target": "/movies/Movie Name (2024)/Movie Name (2024).mkv",
  "media_type": "movie",
  "parse_method": "ai",
  "parsed_title": "Movie Name",
  "parsed_year": 2024,
  "ai_confidence": 0.92,
  "success": true,
  "bytes": 4521000000,
  "duration_ms": 12340,
  "sonarr_notified": false,
  "radarr_notified": true,
  "error": null
}
```

## Disabling Logging

To disable activity logging, remove or rename the `~/.config/jellywatch/activity/` directory.
```

**Step 2: Commit**

```bash
git add docs/activity-logging.md
git commit -m "docs: add activity logging documentation"
```

---

## Summary

This plan adds:

1. **Activity logger package** (`internal/activity/logger.go`) - Writes JSONL entries with daily rotation
2. **MediaHandler integration** - Logs every organization operation with parse method, AI confidence, notification status
3. **CLI monitor command** - `jellywatch monitor` for viewing and filtering logs
4. **Log pruning** - Automatic 7-day retention cleanup on daemon startup
5. **Integration test** - Full E2E test script
6. **Documentation** - User guide for activity logging

**Total estimated time:** ~2 hours

**Files created:** 4
**Files modified:** 3
**Test coverage:** Basic unit tests + integration test

---

Plan complete and saved to `docs/plans/2026-01-22-activity-logging.md`. Two execution options:

**1. Subagent-Driven (this session)** - I dispatch fresh subagent per task, review between tasks, fast iteration

**2. Parallel Session (separate)** - Open new session with executing-plans, batch execution with checkpoints

Which approach?
