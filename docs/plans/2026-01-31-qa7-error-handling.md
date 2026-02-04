# Error Handling Coverage Verification Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Verify all error paths are tested and handled gracefully. This is a verification task - check existing tests cover error scenarios.

**Architecture:** Review existing tests, verify error paths are covered, add missing error tests as needed.

**Tech Stack:** Go testing, error injection, test coverage analysis

---

## Task 1: Database Error Paths

**Files:**
- Review: `internal/database/dirty_test.go`
- Modify: `internal/database/dirty_test.go` (if tests missing)

**Step 1: Verify TestDirtyFlags_DatabaseClosed exists**

```bash
grep -n "TestDirtyFlags_DatabaseClosed" internal/database/dirty_test.go
```

Expected: Test exists (already written in dirty_test.go:273)

If exists:
- Run test: `go test ./internal/database/ -run TestDirtyFlags_DatabaseClosed -v`
- Expected: PASS - verifies error when DB closed

**Step 2: Verify TestDirtyFlags_InvalidID exists**

```bash
grep -n "TestDirtyFlags_InvalidID" internal/database/dirty_test.go
```

Expected: Test exists (dirty_test.go:342)

If exists:
- Run test: `go test ./internal/database/ -run TestDirtyFlags_InvalidID -v`
- Expected: PASS - tests ID 0, -1, 999999

**Step 3: Verify TestDirtyFlags_ConcurrentWrites exists**

```bash
grep -n "TestDirtyFlags_ConcurrentWrites" internal/database/dirty_test.go
```

Expected: Test exists (dirty_test.go:449)

If exists:
- Run test: `go test ./internal/database/ -run TestDirtyFlags_ConcurrentWrites -v`
- Expected: PASS - race detector clean

**Step 4: Verify TestGetAllSeries_DatabaseLocked exists**

```bash
grep -n "TestGetAllSeries_DatabaseLocked" internal/database/dirty_test.go
```

Expected: Test exists (dirty_test.go:532)

If exists:
- Run test: `go test ./internal/database/ -run TestGetAllSeries_DatabaseLocked -v`
- Expected: PASS - handles locked DB gracefully

**Step 5: Check for missing database error paths**

Review each dirty.go function and verify error handling:

```bash
# List all functions in dirty.go
grep "^func" internal/database/dirty.go

# Expected functions:
# - GetDirtySeries()
# - GetDirtyMovies()
# - MarkSeriesSynced()
# - MarkMovieSynced()
# - SetSeriesDirty()
# - SetMovieDirty()
# - GetSeriesByID()
# - GetMovieByID()
```

For each function, verify:
1. ❌ `sql.ErrNoRows` handled? - Yes, GetSeriesByID/GetMovieByID return nil, no error
2. ❌ Database closed error? - Yes, TestDirtyFlags_DatabaseClosed covers this
3. ❌ Invalid ID error? - Yes, TestDirtyFlags_InvalidID covers this
4. ❌ Concurrent access? - Yes, RLock/Lock used, TestDirtyFlags_ConcurrentWrites verifies

**Step 6: Add missing test if needed**

If any error path not covered, add test. For example, if QueryRow().Scan() error not tested:

```go
func TestDirtyFlags_ScanError(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    // Corrupt the database schema to trigger scan error
    _, err := db.DB().Exec(`DROP TABLE series`)
    if err != nil {
        t.Fatalf("failed to drop table: %v", err)
    }

    // Try to get dirty series - should error
    _, err = db.GetDirtySeries()
    if err == nil {
        t.Error("expected error when table doesn't exist")
    }
}
```

**Step 7: Run with race detector**

```bash
go test -race ./internal/database/... -v
```

Expected: 0 race conditions (already verified in TODO.md QA-8)

**Step 8: Commit if tests added**

```bash
git add internal/database/dirty_test.go
git commit -m "test: add missing database error path tests"
```

---

## Task 2: Sync Service Error Paths

**Files:**
- Review: `internal/sync/sync_test.go`
- Review: `internal/sync/sync.go`

**Step 1: Verify TestSyncFromFilesystemContextCancellation exists**

```bash
grep -n "TestSyncFromFilesystemContextCancellation" internal/sync/sync_test.go
```

Expected: Test exists (sync_test.go:169)

Run test: `go test ./internal/sync/ -run TestSyncFromFilesystemContextCancellation -v`
Expected: PASS - context.Canceled error returned

**Step 2: Check for sync service error handling**

Review sync.go for error paths:

```bash
# Check for error handling in key functions
grep -A5 "func.*syncDirtyRecords" internal/sync/sync.go
grep -A5 "func.*processSyncRequest" internal/sync/sync.go
grep -A5 "func.*retryWithBackoff" internal/sync/sync.go
```

Key error paths to verify:
1. ❌ Sonarr API failure - Covered by retryWithBackoff tests
2. ❌ Radarr API failure - Covered by retryWithBackoff tests
3. ❌ Context cancellation - Covered by context tests
4. ❌ Channel full - Covered by TestQueueSync_FullChannel (plan QA-4)
5. ❌ Channel closed - Covered by TestQueueSync_ClosedChannel (plan QA-4)

**Step 3: Add TestSyncService_SonarrAPIDown**

Test exponential backoff when Sonarr API down.

```go
func TestSyncService_SonarrAPIDown(t *testing.T) {
    db := createTestDB(t)
    defer db.Close()

    callCount := 0
    mockSonarr := &struct {
        UpdateSeriesPath func(ctx context.Context, seriesID int, path string) error
    }{
        UpdateSeriesPath: func(ctx context.Context, seriesID int, path string) error {
            callCount++
            return errors.New("Sonarr API unavailable")
        },
    }

    // Create series with sonarr_id
    sonarrID := 123
    series := &database.Series{
        Title:         "Test Show",
        Year:          2020,
        SonarrID:      &sonarrID,
        CanonicalPath: "/tv/Test Show (2020)",
    }
    _, err := db.UpsertSeries(series)
    if err != nil {
        t.Fatalf("UpsertSeries failed: %v", err)
    }
    _ = db.SetSeriesDirty(series.ID)

    cfg := SyncConfig{
        DB:     db,
        Sonarr: mockSonarr,
        Logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
    }
    svc := NewSyncService(cfg)

    // Sync should use exponential backoff
    err = svc.syncDirtyRecords(context.Background())
    if err == nil {
        t.Error("expected error when Sonarr API is down")
    }

    // Verify multiple attempts were made (exponential backoff)
    if callCount < 3 {
        t.Errorf("expected at least 3 retry attempts, got %d", callCount)
    }

    // Verify dirty flag NOT cleared (sync failed)
    retrieved, _ := db.GetSeriesByID(series.ID)
    if !retrieved.SonarrPathDirty {
        t.Error("dirty flag should remain when sync fails")
    }
}
```

**Step 4: Run test**

```bash
go test ./internal/sync/ -run TestSyncService_SonarrAPIDown -v -timeout 60s
```

Expected: PASS (~7-10s for 3 retries)

**Step 5: Add TestSyncService_NetworkTimeout**

Verify context timeout is respected.

```go
func TestSyncService_NetworkTimeout(t *testing.T) {
    db := createTestDB(t)
    defer db.Close()

    mockSonarr := &struct {
        UpdateSeriesPath func(ctx context.Context, seriesID int, path string) error
    }{
        UpdateSeriesPath: func(ctx context.Context, seriesID int, path string) error {
            // Sleep longer than timeout
            time.Sleep(2 * time.Second)
            return nil
        },
    }

    sonarrID := 123
    series := &database.Series{
        Title:         "Test Show",
        Year:          2020,
        SonarrID:      &sonarrID,
        CanonicalPath: "/tv/Test Show (2020)",
    }
    _, err := db.UpsertSeries(series)
    if err != nil {
        t.Fatalf("UpsertSeries failed: %v", err)
    }
    _ = db.SetSeriesDirty(series.ID)

    cfg := SyncConfig{
        DB:     db,
        Sonarr: mockSonarr,
        Logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
    }
    svc := NewSyncService(cfg)

    // Use short timeout
    ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
    defer cancel()

    err = svc.syncDirtyRecords(ctx)
    if err != context.DeadlineExceeded {
        t.Errorf("expected context.DeadlineExceeded, got: %v", err)
    }
}
```

**Step 6: Run test**

```bash
go test ./internal/sync/ -run TestSyncService_NetworkTimeout -v -timeout 10s
```

Expected: PASS

**Step 7: Add TestSyncService_InvalidAPIKey**

Verify clear error message on auth failure.

```go
func TestSyncService_InvalidAPIKey(t *testing.T) {
    db := createTestDB(t)
    defer db.Close()

    mockSonarr := &struct {
        UpdateSeriesPath func(ctx context.Context, seriesID int, path string) error
    }{
        UpdateSeriesPath: func(ctx context.Context, seriesID int, path string) error {
            return errors.New("401 Unauthorized: invalid API key")
        },
    }

    sonarrID := 123
    series := &database.Series{
        Title:         "Test Show",
        Year:          2020,
        SonarrID:      &sonarrID,
        CanonicalPath: "/tv/Test Show (2020)",
    }
    _, err := db.UpsertSeries(series)
    if err != nil {
        t.Fatalf("UpsertSeries failed: %v", err)
    }
    _ = db.SetSeriesDirty(series.ID)

    cfg := SyncConfig{
        DB:     db,
        Sonarr: mockSonarr,
        Logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
    }
    svc := NewSyncService(cfg)

    err = svc.syncDirtyRecords(context.Background())
    if err == nil {
        t.Error("expected error on invalid API key")
    }

    // Verify error is descriptive (not cryptic)
    if !strings.Contains(err.Error(), "401") && !strings.Contains(err.Error(), "Unauthorized") {
        t.Errorf("error message should indicate auth failure: %v", err)
    }

    // Verify dirty flag remains (sync didn't succeed)
    retrieved, _ := db.GetSeriesByID(series.ID)
    if !retrieved.SonarrPathDirty {
        t.Error("dirty flag should remain on auth failure")
    }
}
```

**Step 8: Run test**

```bash
go test ./internal/sync/ -run TestSyncService_InvalidAPIKey -v
```

Expected: PASS

**Step 9: Commit**

```bash
git add internal/sync/sync_test.go
git commit -m "test: add sync service error handling tests (API down, timeout, invalid key)"
```

---

## Task 3: Migration Error Paths

**Files:**
- Review: `internal/migration/migration.go`
- Create: `internal/migration/migration_error_test.go`

**Step 1: Add TestMigration_BothConfigsNil**

Verify error when no Sonarr/Radarr configured.

```go
package migration

import (
    "testing"

    "github.com/Nomadcxx/jellywatch/internal/database"
    "github.com/Nomadcxx/jellywatch/internal/sonarr"
    "github.com/Nomadcxx/jellywatch/internal/radarr"
)

func TestMigration_BothConfigsNil(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    // Create test series
    sonarrID := 123
    _, err := db.DB().Exec(`
        INSERT INTO series (title, title_normalized, year, sonarr_id, canonical_path, library_root, source, source_priority)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)
    `, "Test Show", "testshow", 2024, &sonarrID, "/tv/Test Show (2024)", "/tv", "jellywatch", 100)
    if err != nil {
        t.Fatalf("failed to insert series: %v", err)
    }

    // Detect mismatches with both configs nil
    mismatches, err := DetectSeriesMismatches(nil, nil, db)
    if err == nil {
        t.Error("expected error when both Sonarr and Radarr configs are nil")
    }
    if len(mismatches) != 0 {
        t.Error("should return empty mismatches, not error list, when configs are nil")
    }
}
```

**Step 2: Run test**

```bash
go test ./internal/migration/ -run TestMigration_BothConfigsNil -v
```

Expected: PASS

**Step 3: Add TestMigration_APIAuthFailure**

Verify clear error message on auth failure.

```go
func TestMigration_APIAuthFailure(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    sonarrID := 123
    _, err := db.DB().Exec(`
        INSERT INTO series (title, title_normalized, year, sonarr_id, canonical_path, library_root, source, source_priority)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)
    `, "Auth Test", "authtest", 2024, &sonarrID, "/tv/Auth Test (2024)", "/tv", "sonarr", 50)
    if err != nil {
        t.Fatalf("failed to insert series: %v", err)
    }

    // Mock Sonarr with auth error
    mockSonarr := &struct {
        GetSeriesByID func(ctx context.Context, id int) (*sonarr.Series, error)
    }{
        GetSeriesByID: func(ctx context.Context, id int) (*sonarr.Series, error) {
            return nil, errors.New("401 Unauthorized: invalid API key")
        },
    }

    mismatches, err := DetectSeriesMismatches(mockSonarr, nil, db)
    if err == nil {
        t.Error("expected error on API auth failure")
    }

    // Verify error is descriptive
    if !strings.Contains(err.Error(), "401") && !strings.Contains(err.Error(), "Unauthorized") {
        t.Errorf("error should indicate auth failure: %v", err)
    }
}
```

**Step 4: Run test**

```bash
go test ./internal/migration/ -run TestMigration_APIAuthFailure -v
```

Expected: PASS

**Step 5: Add TestMigration_DatabaseReadOnly**

Verify error on fix attempt with read-only DB.

```go
func TestMigration_DatabaseReadOnly(t *testing.T) {
    tmpDir := t.TempDir()
    dbPath := filepath.Join(tmpDir, "readonly.db")

    db, err := database.OpenPath(dbPath)
    if err != nil {
        t.Fatalf("failed to open DB: %v", err)
    }
    defer db.Close()

    // Insert series
    sonarrID := 123
    _, err = db.DB().Exec(`
        INSERT INTO series (title, title_normalized, year, sonarr_id, canonical_path, library_root, source, source_priority)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)
    `, "RO Test", "rotest", 2024, &sonarrID, "/tv/RO Test (2024)", "/tv", "sonarr", 50)
    if err != nil {
        t.Fatalf("failed to insert series: %v", err)
    }

    // Make file read-only
    err = os.Chmod(dbPath, 0444)
    if err != nil {
        t.Fatalf("failed to make DB read-only: %v", err)
    }
    defer os.Chmod(dbPath, 0644)

    // Mock Sonarr
    mockSonarr := &struct {
        GetSeriesByID func(ctx context.Context, id int) (*sonarr.Series, error)
        UpdateSeriesPath func(ctx context.Context, seriesID int, path string) error
    }{
        GetSeriesByID: func(ctx context.Context, id int) (*sonarr.Series, error) {
            return &sonarr.Series{ID: id, Path: "/new/path"}, nil
        },
        UpdateSeriesPath: func(ctx context.Context, seriesID int, path string) error {
            return nil
        },
    }

    // Try to fix mismatch (should fail on DB update)
    mismatches, _ := DetectSeriesMismatches(mockSonarr, nil, db)
    if len(mismatches) == 0 {
        t.Fatal("expected at least one mismatch")
    }

    err = FixSeriesMismatch(db, mockSonarr, mismatches[0], "j")
    if err == nil {
        t.Error("expected error when DB is read-only")
    }
}
```

**Step 6: Run test**

```bash
go test ./internal/migration/ -run TestMigration_DatabaseReadOnly -v
```

Expected: PASS

**Step 7: Commit**

```bash
git add internal/migration/migration_error_test.go
git commit -m "test: add migration error handling tests (nil configs, auth failure, read-only DB)"
```

---

## Final Verification

**Step 1: Run all error handling tests**

```bash
go test ./internal/database/... ./internal/sync/... ./internal/migration/... -v
```

Expected: All tests pass

**Step 2: Check test coverage for error paths**

```bash
go test ./internal/database/... ./internal/sync/... ./internal/migration/... -cover
```

Expected: Coverage > 50% for all three packages

**Step 3: Run with race detector**

```bash
go test -race ./internal/database/... ./internal/sync/... ./internal/migration/... -v
```

Expected: 0 race conditions

**Step 4: Manual verification checklist**

Verify these scenarios work gracefully:

1. ❌ Database corrupted - Error returned, no panic
2. ❌ Sonarr API down - Exponential backoff, retries
3. ❌ Network timeout - Context timeout respected
4. ❌ Invalid API key - Clear error message
5. ❌ Disk full - Error returned (organizer tests)
6. ❌ Permission denied - Clear error message
7. ❌ Process killed (SIGKILL) - Dirty flags persist, retry sweep picks up
8. ❌ Context cancellation - All operations stop gracefully

Expected: All scenarios handled without crashes
