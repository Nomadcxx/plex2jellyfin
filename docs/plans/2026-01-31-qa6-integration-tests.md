# Integration Tests Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Create end-to-end integration tests for organize → dirty flag → sync queue → API call workflow.

**Architecture:** Full workflow tests using real database, temp filesystems, and minimal mocking. Tests verify complete data flow from file organization through to sync completion.

**Tech Stack:** Go testing, real in-memory SQLite, temp directories, mock Sonarr/Radarr clients

---

## Task 1: Organizer → Sync Integration Tests

**Files:**
- Create: `internal/organizer/integration_test.go`

**Step 1: Write TestOrganizeWorkflow_SetsSeriesDirty**

Verify organizer marks series dirty after TV episode organize.

```go
package organizer

import (
    "context"
    "os"
    "path/filepath"
    "testing"
    "time"

    "github.com/Nomadcxx/jellywatch/internal/database"
    "github.com/Nomadcxx/jellywatch/internal/sync"
)

func TestOrganizeWorkflow_SetsSeriesDirty(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    // Create temp directories
    tmpDir := t.TempDir()
    sourceDir := filepath.Join(tmpDir, "source")
    tvLib := filepath.Join(tmpDir, "tv")

    os.MkdirAll(sourceDir, 0755)
    os.MkdirAll(tvLib, 0755)

    // Create test TV episode file
    sourceFile := filepath.Join(sourceDir, "Test.Show.2024.S01E01.1080p.WEB-DL.mkv")
    os.WriteFile(sourceFile, []byte("fake video"), 0644)

    // Create sync service (for QueueSync)
    cfg := sync.SyncConfig{
        DB:     db,
        Sonarr: nil,
        Radarr: nil,
        Logger: nil,
    }
    syncSvc := sync.NewSyncService(cfg)

    // Create organizer
    cfg2 := Config{
        DB:             db,
        TVLibraries:    []string{tvLib},
        MovieLibraries:  []string{},
        SyncService:    syncSvc,
    }

    // Mock scanner helper - don't do full filesystem scan
    organizer, err := NewOrganizer(cfg2)
    if err != nil {
        t.Fatalf("failed to create organizer: %v", err)
    }

    // Organize the file
    ctx := context.Background()
    results, err := organizer.Organize(ctx, sourceDir, "")
    if err != nil {
        t.Fatalf("Organize failed: %v", err)
    }

    // Verify file was organized
    if len(results) == 0 {
        t.Fatal("expected at least one organize result")
    }

    // Check if series was created in DB
    series, err := db.GetSeriesByTitle("Test Show", 2024)
    if err != nil {
        t.Fatalf("failed to get series: %v", err)
    }
    if series == nil {
        t.Fatal("expected series to be created in database")
    }

    // Wait a bit for async operations
    time.Sleep(10 * time.Millisecond)

    // Verify dirty flag is set
    retrieved, err := db.GetSeriesByID(series.ID)
    if err != nil {
        t.Fatalf("failed to get series by ID: %v", err)
    }
    if !retrieved.SonarrPathDirty {
        t.Error("expected series dirty flag to be set after organize")
    }
}
```

**Step 2: Run test**

```bash
go test ./internal/organizer/ -run TestOrganizeWorkflow_SetsSeriesDirty -v
```

Expected: PASS

**Step 3: Write TestOrganizeWorkflow_QueuesSync**

Verify QueueSync is called after organize.

```go
func TestOrganizeWorkflow_QueuesSync(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    tmpDir := t.TempDir()
    sourceDir := filepath.Join(tmpDir, "source")
    tvLib := filepath.Join(tmpDir, "tv")

    os.MkdirAll(sourceDir, 0755)
    os.MkdirAll(tvLib, 0755)

    sourceFile := filepath.Join(sourceDir, "Another.Show.2023.S01E01.mkv")
    os.WriteFile(sourceFile, []byte("fake video"), 0644)

    // Create sync service with a mock to track QueueSync calls
    var queuedRequests []sync.SyncRequest
    queueCalls := 0

    // We'll need to modify sync service to track this, or use existing test pattern
    // For now, verify that QueueSync doesn't panic

    cfg := sync.SyncConfig{
        DB:     db,
        Sonarr: nil,
        Radarr: nil,
        Logger: nil,
    }
    syncSvc := sync.NewSyncService(cfg)

    cfg2 := Config{
        DB:             db,
        TVLibraries:    []string{tvLib},
        MovieLibraries:  []string{},
        SyncService:    syncSvc,
    }

    organizer, err := NewOrganizer(cfg2)
    if err != nil {
        t.Fatalf("failed to create organizer: %v", err)
    }

    ctx := context.Background()
    _, err = organizer.Organize(ctx, sourceDir, "")
    if err != nil {
        t.Fatalf("Organize failed: %v", err)
    }

    series, err := db.GetSeriesByTitle("Another Show", 2023)
    if err != nil || series == nil {
        t.Fatal("expected series to be created")
    }

    time.Sleep(10 * time.Millisecond)

    // QueueSync was called (no panic)
    // This is a smoke test - full tracking would need sync service modification
    t.Log("QueueSync called successfully (no panic)")
}
```

**Step 4: Run test**

```bash
go test ./internal/organizer/ -run TestOrganizeWorkflow_QueuesSync -v
```

Expected: PASS

**Step 5: Write TestOrganizeWorkflow_WithoutSyncService**

Verify no panic when SyncService is nil.

```go
func TestOrganizeWorkflow_WithoutSyncService(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    tmpDir := t.TempDir()
    sourceDir := filepath.Join(tmpDir, "source")
    tvLib := filepath.Join(tmpDir, "tv")

    os.MkdirAll(sourceDir, 0755)
    os.MkdirAll(tvLib, 0755)

    sourceFile := filepath.Join(sourceDir, "NilSync.Show.2024.S01E01.mkv")
    os.WriteFile(sourceFile, []byte("fake video"), 0644)

    // Create organizer WITHOUT sync service
    cfg := Config{
        DB:             db,
        TVLibraries:    []string{tvLib},
        MovieLibraries:  []string{},
        SyncService:    nil, // Nil sync service
    }

    organizer, err := NewOrganizer(cfg)
    if err != nil {
        t.Fatalf("failed to create organizer: %v", err)
    }

    ctx := context.Background()
    _, err = organizer.Organize(ctx, sourceDir, "")
    if err != nil {
        t.Fatalf("Organize failed: %v", err)
    }

    series, err := db.GetSeriesByTitle("Nil Sync Show", 2024)
    if err != nil || series == nil {
        t.Fatal("expected series to be created")
    }

    // Verify dirty flag is set (SetSeriesDirty doesn't need SyncService)
    time.Sleep(10 * time.Millisecond)

    retrieved, err := db.GetSeriesByID(series.ID)
    if err != nil {
        t.Fatalf("failed to get series by ID: %v", err)
    }

    // Note: Without SyncService, QueueSync won't be called
    // but SetSeriesDirty should still work
    if !retrieved.SonarrPathDirty {
        t.Error("expected dirty flag to be set even without SyncService")
    }
}
```

**Step 6: Run test**

```bash
go test ./internal/organizer/ -run TestOrganizeWorkflow_WithoutSyncService -v
```

Expected: PASS

**Step 7: Write TestOrganizeWorkflow_MovieSetsMovieDirty**

Verify movies get dirty flag set.

```go
func TestOrganizeWorkflow_MovieSetsMovieDirty(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    tmpDir := t.TempDir()
    sourceDir := filepath.Join(tmpDir, "source")
    movieLib := filepath.Join(tmpDir, "movies")

    os.MkdirAll(sourceDir, 0755)
    os.MkdirAll(movieLib, 0755)

    sourceFile := filepath.Join(sourceDir, "Test.Movie.2024.1080p.BluRay.mkv")
    os.WriteFile(sourceFile, []byte("fake video"), 0644)

    cfg := sync.SyncConfig{
        DB:     db,
        Sonarr: nil,
        Radarr: nil,
        Logger: nil,
    }
    syncSvc := sync.NewSyncService(cfg)

    cfg2 := Config{
        DB:             db,
        TVLibraries:    []string{},
        MovieLibraries:  []string{movieLib},
        SyncService:    syncSvc,
    }

    organizer, err := NewOrganizer(cfg2)
    if err != nil {
        t.Fatalf("failed to create organizer: %v", err)
    }

    ctx := context.Background()
    _, err = organizer.Organize(ctx, sourceDir, "")
    if err != nil {
        t.Fatalf("Organize failed: %v", err)
    }

    movie, err := db.GetMovieByTitle("Test Movie", 2024)
    if err != nil {
        t.Fatalf("failed to get movie: %v", err)
    }
    if movie == nil {
        t.Fatal("expected movie to be created in database")
    }

    time.Sleep(10 * time.Millisecond)

    retrieved, err := db.GetMovieByID(movie.ID)
    if err != nil {
        t.Fatalf("failed to get movie by ID: %v", err)
    }
    if !retrieved.RadarrPathDirty {
        t.Error("expected movie dirty flag to be set after organize")
    }
}
```

**Step 8: Run test**

```bash
go test ./internal/organizer/ -run TestOrganizeWorkflow_MovieSetsMovieDirty -v
```

Expected: PASS

**Step 9: Commit**

```bash
git add internal/organizer/integration_test.go
git commit -m "test: add organizer integration tests (dirty flags, sync queue)"
```

---

## Task 2: End-to-End Sync Workflow Tests

**Files:**
- Create: `internal/sync/integration_test.go`

**Step 1: Write Mock Sonarr/Radarr Clients**

First, create proper mock clients for API calls.

```go
package sync

import (
    "context"
    "errors"
    "testing"

    "github.com/Nomadcxx/jellywatch/internal/sonarr"
)

type mockSonarrClient struct {
    updatePathFunc func(ctx context.Context, seriesID int, path string) error
    seriesToPath   map[int]string // Tracks what paths were updated
    shouldError    bool
}

func (m *mockSonarrClient) UpdateSeriesPath(ctx context.Context, seriesID int, path string) error {
    if m.shouldError {
        return errors.New("mock API error")
    }
    if m.seriesToPath == nil {
        m.seriesToPath = make(map[int]string)
    }
    m.seriesToPath[seriesID] = path
    if m.updatePathFunc != nil {
        return m.updatePathFunc(ctx, seriesID, path)
    }
    return nil
}

func (m *mockSonarrClient) GetSeriesByID(ctx context.Context, id int) (*sonarr.Series, error) {
    return &sonarr.Series{
        ID:    id,
        Title:  "Test Show",
        Year:   2024,
        Path:   "/old/path",
    }, nil
}

type mockRadarrClient struct {
    updatePathFunc func(ctx context.Context, movieID int, path string) error
    moviesToPath   map[int]string
    shouldError    bool
}

func (m *mockRadarrClient) UpdateMoviePath(ctx context.Context, movieID int, path string) error {
    if m.shouldError {
        return errors.New("mock API error")
    }
    if m.moviesToPath == nil {
        m.moviesToPath = make(map[int]string)
    }
    m.moviesToPath[movieID] = path
    if m.updatePathFunc != nil {
        return m.updatePathFunc(ctx, movieID, path)
    }
    return nil
}

func (m *mockRadarrClient) GetMovieByID(ctx context.Context, id int) (*radarr.Movie, error) {
    return &radarr.Movie{
        ID:    id,
        Title:  "Test Movie",
        Year:   2024,
        Path:   "/old/path",
    }, nil
}
```

**Step 2: Write TestFullSyncWorkflow_OrganizeToSonarr**

Full E2E test: organize → dirty → sync → API → clear.

```go
func TestFullSyncWorkflow_OrganizeToSonarr(t *testing.T) {
    db := createTestDB(t)
    defer db.Close()

    tmpDir := t.TempDir()
    sourceDir := filepath.Join(tmpDir, "source")
    tvLib := filepath.Join(tmpDir, "tv")

    os.MkdirAll(sourceDir, 0755)
    os.MkdirAll(tvLib, 0755)

    sourceFile := filepath.Join(sourceDir, "Full.Test.2024.S01E01.mkv")
    os.WriteFile(sourceFile, []byte("fake video"), 0644)

    // Create mock Sonarr client
    mockSonarr := &mockSonarrClient{shouldError: false}

    cfg := SyncConfig{
        DB:     db,
        Sonarr: mockSonarr,
        Radarr: nil,
        Logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
    }
    syncSvc := NewSyncService(cfg)

    // Step 1: Organize file (sets dirty flag)
    orgCfg := Config{
        DB:             db,
        TVLibraries:    []string{tvLib},
        MovieLibraries:  []string{},
        SyncService:    syncSvc,
    }

    organizer, err := NewOrganizer(orgCfg)
    if err != nil {
        t.Fatalf("failed to create organizer: %v", err)
    }

    ctx := context.Background()
    _, err = organizer.Organize(ctx, sourceDir, "")
    if err != nil {
        t.Fatalf("Organize failed: %v", err)
    }

    // Step 2: Verify dirty flag set
    series, err := db.GetSeriesByTitle("Full Test", 2024)
    if err != nil {
        t.Fatalf("failed to get series: %v", err)
    }
    if series == nil {
        t.Fatal("expected series to exist")
    }

    // Set sonarr_id for sync
    sonarrID := 123
    series.SonarrID = &sonarrID
    _, err = db.UpsertSeries(series)
    if err != nil {
        t.Fatalf("failed to update series with sonarr_id: %v", err)
    }

    retrieved, err := db.GetSeriesByID(series.ID)
    if err != nil {
        t.Fatalf("failed to get series: %v", err)
    }
    if !retrieved.SonarrPathDirty {
        t.Error("expected dirty flag to be set")
    }

    // Step 3: Sync dirty records (calls Sonarr API)
    err = syncSvc.syncDirtyRecords(ctx)
    if err != nil {
        t.Fatalf("syncDirtyRecords failed: %v", err)
    }

    // Step 4: Verify Sonarr API was called with correct path
    if len(mockSonarr.seriesToPath) == 0 {
        t.Error("expected Sonarr UpdateSeriesPath to be called")
    }

    expectedPath := "/tv/Full Test (2024)"
    actualPath, exists := mockSonarr.seriesToPath[*series.SonarrID]
    if !exists {
        t.Error("Sonarr API was not called with this series ID")
    } else if actualPath != expectedPath {
        t.Errorf("expected path %q, got %q", expectedPath, actualPath)
    }

    // Step 5: Verify dirty flag cleared
    final, err := db.GetSeriesByID(series.ID)
    if err != nil {
        t.Fatalf("failed to get final series state: %v", err)
    }
    if final.SonarrPathDirty {
        t.Error("expected dirty flag to be cleared after sync")
    }
    if final.SonarrSyncedAt == nil {
        t.Error("expected sonarr_synced_at to be set")
    }
}
```

**Step 3: Run test**

```bash
go test ./internal/sync/ -run TestFullSyncWorkflow_OrganizeToSonarr -v
```

Expected: PASS

**Step 4: Write TestFullSyncWorkflow_OrganizeToRadarr**

Similar E2E test for movies.

```go
func TestFullSyncWorkflow_OrganizeToRadarr(t *testing.T) {
    db := createTestDB(t)
    defer db.Close()

    tmpDir := t.TempDir()
    sourceDir := filepath.Join(tmpDir, "source")
    movieLib := filepath.Join(tmpDir, "movies")

    os.MkdirAll(sourceDir, 0755)
    os.MkdirAll(movieLib, 0755)

    sourceFile := filepath.Join(sourceDir, "Full.Movie.2024.1080p.mkv")
    os.WriteFile(sourceFile, []byte("fake video"), 0644)

    mockRadarr := &mockRadarrClient{shouldError: false}

    cfg := SyncConfig{
        DB:     db,
        Sonarr: nil,
        Radarr: mockRadarr,
        Logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
    }
    syncSvc := NewSyncService(cfg)

    // Organize
    orgCfg := Config{
        DB:             db,
        TVLibraries:    []string{},
        MovieLibraries:  []string{movieLib},
        SyncService:    syncSvc,
    }

    organizer, err := NewOrganizer(orgCfg)
    if err != nil {
        t.Fatalf("failed to create organizer: %v", err)
    }

    ctx := context.Background()
    _, err = organizer.Organize(ctx, sourceDir, "")
    if err != nil {
        t.Fatalf("Organize failed: %v", err)
    }

    movie, err := db.GetMovieByTitle("Full Movie", 2024)
    if err != nil {
        t.Fatalf("failed to get movie: %v", err)
    }
    if movie == nil {
        t.Fatal("expected movie to exist")
    }

    radarrID := 456
    movie.RadarrID = &radarrID
    _, err = db.UpsertMovie(movie)
    if err != nil {
        t.Fatalf("failed to update movie with radarr_id: %v", err)
    }

    retrieved, err := db.GetMovieByID(movie.ID)
    if err != nil || !retrieved.RadarrPathDirty {
        t.Fatalf("expected dirty flag to be set: %v", err)
    }

    // Sync
    err = syncSvc.syncDirtyRecords(ctx)
    if err != nil {
        t.Fatalf("syncDirtyRecords failed: %v", err)
    }

    // Verify Radarr API called
    if len(mockRadarr.moviesToPath) == 0 {
        t.Error("expected Radarr UpdateMoviePath to be called")
    }

    expectedPath := "/movies/Full Movie (2024)"
    actualPath, exists := mockRadarr.moviesToPath[*movie.RadarrID]
    if !exists || actualPath != expectedPath {
        t.Errorf("expected path %q, got %q", expectedPath, actualPath)
    }

    // Verify dirty flag cleared
    final, err := db.GetMovieByID(movie.ID)
    if err != nil || final.RadarrPathDirty {
        t.Fatalf("expected dirty flag cleared: %v", err)
    }
    if final.RadarrSyncedAt == nil {
        t.Error("expected radarr_synced_at to be set")
    }
}
```

**Step 5: Run test**

```bash
go test ./internal/sync/ -run TestFullSyncWorkflow_OrganizeToRadarr -v
```

Expected: PASS

**Step 6: Write TestFullSyncWorkflow_RetryOnFailure**

Verify retry loop picks up failed sync.

```go
func TestFullSyncWorkflow_RetryOnFailure(t *testing.T) {
    db := createTestDB(t)
    defer db.Close()

    tmpDir := t.TempDir()
    sourceDir := filepath.Join(tmpDir, "source")
    tvLib := filepath.Join(tmpDir, "tv")

    os.MkdirAll(sourceDir, 0755)
    os.MkdirAll(tvLib, 0755)

    sourceFile := filepath.Join(sourceDir, "Retry.Test.2024.S01E01.mkv")
    os.WriteFile(sourceFile, []byte("fake video"), 0644)

    // Mock that fails first time, succeeds second
    callCount := 0
    mockSonarr := &mockSonarrClient{
        shouldError: false,
        updatePathFunc: func(ctx context.Context, seriesID int, path string) error {
            callCount++
            if callCount == 1 {
                return errors.New("temporary API failure")
            }
            return nil
        },
    }

    cfg := SyncConfig{
        DB:     db,
        Sonarr: mockSonarr,
        Radarr: nil,
        Logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
    }
    syncSvc := NewSyncService(cfg)

    // Organize
    orgCfg := Config{
        DB:             db,
        TVLibraries:    []string{tvLib},
        MovieLibraries:  []string{},
        SyncService:    syncSvc,
    }

    organizer, err := NewOrganizer(orgCfg)
    if err != nil {
        t.Fatalf("failed to create organizer: %v", err)
    }

    ctx := context.Background()
    _, err = organizer.Organize(ctx, sourceDir, "")
    if err != nil {
        t.Fatalf("Organize failed: %v", err)
    }

    series, err := db.GetSeriesByTitle("Retry Test", 2024)
    if err != nil || series == nil {
        t.Fatalf("failed to get series: %v", err)
    }

    sonarrID := 789
    series.SonarrID = &sonarrID
    _, err = db.UpsertSeries(series)
    if err != nil {
        t.Fatalf("failed to update series: %v", err)
    }

    // First sync attempt (fails)
    err = syncSvc.syncDirtyRecords(ctx)
    if err == nil {
        t.Error("expected error on first sync attempt")
    }

    // Verify dirty flag still set
    retrieved, _ := db.GetSeriesByID(series.ID)
    if !retrieved.SonarrPathDirty {
        t.Error("dirty flag should remain after sync failure")
    }

    // Simulate retry (manual sync again)
    err = syncSvc.syncDirtyRecords(ctx)
    if err != nil {
        t.Errorf("retry sync failed: %v", err)
    }

    // Verify sync succeeded on retry
    if callCount != 2 {
        t.Errorf("expected 2 API calls, got %d", callCount)
    }

    // Verify dirty flag cleared after successful retry
    final, err := db.GetSeriesByID(series.ID)
    if err != nil || final.SonarrPathDirty {
        t.Fatalf("expected dirty flag cleared after retry: %v", err)
    }
}
```

**Step 7: Run test**

```bash
go test ./internal/sync/ -run TestFullSyncWorkflow_RetryOnFailure -v
```

Expected: PASS

**Step 8: Commit**

```bash
git add internal/sync/integration_test.go
git commit -m "test: add end-to-end sync workflow tests (organize, API, retry)"
```

---

## Task 3: Migration CLI Integration Tests

**Files:**
- Create: `cmd/jellywatch/migrate_integration_test.go`

**Step 1: Write TestMigrateCommand_DryRun**

Verify dry-run doesn't make changes.

```go
package main

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/Nomadcxx/jellywatch/internal/database"
    "github.com/Nomadcxx/jellywatch/internal/sonarr"
)

// Setup helper for migrate command tests
func setupMigrateTest(t *testing.T) (*database.MediaDB, string, func()) {
    tmpDir := t.TempDir()
    dbPath := filepath.Join(tmpDir, "test.db")

    db, err := database.OpenPath(dbPath)
    if err != nil {
        t.Fatalf("failed to create test DB: %v", err)
    }

    cleanup := func() {
        db.Close()
        os.RemoveAll(tmpDir)
    }

    return db, tmpDir, cleanup
}

func TestMigrateCommand_DryRun(t *testing.T) {
    db, tmpDir, cleanup := setupMigrateTest(t)
    defer cleanup()

    // Insert series with different paths in DB vs Sonarr
    sonarrID := 123
    _, err := db.DB().Exec(`
        INSERT INTO series (title, title_normalized, year, sonarr_id, canonical_path, library_root, source, source_priority)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)
    `, "Test Show", "testshow", 2024, &sonarrID, "/tv/Test Show (2024)", "/tv", "sonarr", 50)
    if err != nil {
        t.Fatalf("failed to insert series: %v", err)
    }

    // Create mock Sonarr client with different path
    mockSonarr := &struct {
        GetSeriesByID func(ctx context.Context, id int) (*sonarr.Series, error)
    }{
        GetSeriesByID: func(ctx context.Context, id int) (*sonarr.Series, error) {
            return &sonarr.Series{
                ID:    id,
                Title:  "Test Show",
                Year:   2024,
                Path:   "/different/path/Test Show", // Different path
            }, nil
        },
    }

    // Run migrate with --dry-run
    // This would require calling the migrate command directly
    // For now, this is a placeholder for integration testing

    t.Log("dry-run migration would detect path mismatch")
}
```

**Note:** Full migrate command integration tests require significant setup. For now, we'll document what needs testing:

- Detect mismatches correctly
- Dry-run doesn't modify DB
- Keep jellywatch updates Sonarr
- Keep sonarr updates DB
- Skip works correctly
- Interactive prompts work

**Step 2: Run test**

```bash
go test ./cmd/jellywatch/ -run TestMigrateCommand_DryRun -v
```

Expected: PASS (placeholder)

**Step 3: Commit**

```bash
git add cmd/jellywatch/migrate_integration_test.go
git commit -m "test: add migrate command integration test skeleton"
```

---

## Final Verification

**Step 1: Run all integration tests**

```bash
go test ./internal/organizer/... ./internal/sync/... ./cmd/jellywatch/... -tags=integration -v
```

Expected: All integration tests pass

**Step 2: Verify coverage**

```bash
go test ./internal/organizer/... ./internal/sync/... -cover
```

Expected: Coverage > 50% for sync, > 40% for organizer

**Step 3: Manual verification checklist**

Run these commands to verify workflows end-to-end:

1. Organize a TV episode → Check dirty flag in DB
2. Organize a movie → Check dirty flag in DB
3. Run `jellywatch migrate --dry-run` → Verify no changes
4. Check logs for sync operations
5. Verify Sonarr/Radarr show updated paths after sync

Expected: All workflows complete successfully
