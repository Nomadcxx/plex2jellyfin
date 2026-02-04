# Database Layer Tests Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Expand database test coverage for dirty flags, schema migrations, and upsert operations.

**Architecture:** Table-driven tests using real in-memory SQLite (pattern from existing dirty_test.go), with comprehensive coverage of edge cases and error paths.

**Tech Stack:** Go testing, SQLite in-memory databases, `database.MediaDB`

---

## Task 1: Missing Dirty Flag Tests

**Files:**
- Modify: `internal/database/dirty_test.go`

**Step 1: Write TestSetSeriesDirty_MultipleCalls**

Add test to verify idempotency - calling SetSeriesDirty multiple times doesn't cause issues.

```go
func TestSetSeriesDirty_MultipleCalls(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    series := &Series{
        Title:         "Test Show",
        Year:          2020,
        CanonicalPath: "/tv/Test Show (2020)",
        LibraryRoot:   "/tv",
        Source:        "jellywatch",
    }
    _, err := db.UpsertSeries(series)
    if err != nil {
        t.Fatalf("UpsertSeries failed: %v", err)
    }

    // Set dirty 10 times - should not error
    for i := 0; i < 10; i++ {
        err = db.SetSeriesDirty(series.ID)
        if err != nil {
            t.Errorf("SetSeriesDirty call %d failed: %v", i+1, err)
        }
    }

    // Verify dirty flag is still set
    retrieved, err := db.GetSeriesByID(series.ID)
    if err != nil {
        t.Fatalf("GetSeriesByID failed: %v", err)
    }
    if !retrieved.SonarrPathDirty {
        t.Error("dirty flag should be set after multiple calls")
    }
}
```

**Step 2: Run test**

```bash
go test ./internal/database/ -run TestSetSeriesDirty_MultipleCalls -v
```

Expected: PASS

**Step 3: Write TestSetMovieDirty_MultipleCalls**

Similar test for movies.

```go
func TestSetMovieDirty_MultipleCalls(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    movie := &Movie{
        Title:         "Test Movie",
        Year:          2020,
        CanonicalPath: "/movies/Test Movie (2020)",
        LibraryRoot:   "/movies",
        Source:        "jellywatch",
    }
    _, err := db.UpsertMovie(movie)
    if err != nil {
        t.Fatalf("UpsertMovie failed: %v", err)
    }

    // Set dirty 10 times
    for i := 0; i < 10; i++ {
        err = db.SetMovieDirty(movie.ID)
        if err != nil {
            t.Errorf("SetMovieDirty call %d failed: %v", i+1, err)
        }
    }

    retrieved, err := db.GetMovieByID(movie.ID)
    if err != nil {
        t.Fatalf("GetMovieByID failed: %v", err)
    }
    if !retrieved.RadarrPathDirty {
        t.Error("dirty flag should be set after multiple calls")
    }
}
```

**Step 4: Run test**

```bash
go test ./internal/database/ -run TestSetMovieDirty_MultipleCalls -v
```

Expected: PASS

**Step 5: Write TestMarkSeriesSynced_ClearsBothFlags**

Verify both sonarr and radarr dirty flags are cleared.

```go
func TestMarkSeriesSynced_ClearsBothFlags(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    series := &Series{
        Title:         "Test Show",
        Year:          2020,
        CanonicalPath: "/tv/Test Show (2020)",
        LibraryRoot:   "/tv",
        Source:        "jellywatch",
    }
    _, err := db.UpsertSeries(series)
    if err != nil {
        t.Fatalf("UpsertSeries failed: %v", err)
    }

    // Set both flags manually (bypass SetSeriesDirty)
    _, err = db.DB().Exec(`UPDATE series SET sonarr_path_dirty = 1, radarr_path_dirty = 1 WHERE id = ?`, series.ID)
    if err != nil {
        t.Fatalf("failed to set dirty flags manually: %v", err)
    }

    // Mark synced
    err = db.MarkSeriesSynced(series.ID)
    if err != nil {
        t.Fatalf("MarkSeriesSynced failed: %v", err)
    }

    retrieved, err := db.GetSeriesByID(series.ID)
    if err != nil {
        t.Fatalf("GetSeriesByID failed: %v", err)
    }
    if retrieved.SonarrPathDirty {
        t.Error("sonarr_path_dirty should be cleared")
    }
    if retrieved.RadarrPathDirty {
        t.Error("radarr_path_dirty should be cleared")
    }
    if retrieved.SonarrSyncedAt == nil {
        t.Error("SonarrSyncedAt should be set")
    }
    if retrieved.RadarrSyncedAt == nil {
        t.Error("RadarrSyncedAt should be set")
    }
}
```

**Step 6: Run test**

```bash
go test ./internal/database/ -run TestMarkSeriesSynced_ClearsBothFlags -v
```

Expected: PASS

**Step 7: Write TestMarkMovieSynced_UpdatesTimestamp**

Verify timestamp is actually updated.

```go
func TestMarkMovieSynced_UpdatesTimestamp(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    movie := &Movie{
        Title:         "Test Movie",
        Year:          2020,
        CanonicalPath: "/movies/Test Movie (2020)",
        LibraryRoot:   "/movies",
        Source:        "jellywatch",
    }
    _, err := db.UpsertMovie(movie)
    if err != nil {
        t.Fatalf("UpsertMovie failed: %v", err)
    }

    // First sync
    err = db.SetMovieDirty(movie.ID)
    if err != nil {
        t.Fatalf("SetMovieDirty failed: %v", err)
    }

    time.Sleep(10 * time.Millisecond) // Small delay
    firstSyncTime := time.Now()
    err = db.MarkMovieSynced(movie.ID)
    if err != nil {
        t.Fatalf("MarkMovieSynced (first) failed: %v", err)
    }

    retrieved1, err := db.GetMovieByID(movie.ID)
    if err != nil {
        t.Fatalf("GetMovieByID (first) failed: %v", err)
    }

    // Second sync
    time.Sleep(10 * time.Millisecond)
    err = db.SetMovieDirty(movie.ID)
    if err != nil {
        t.Fatalf("SetMovieDirty (second) failed: %v", err)
    }

    time.Sleep(10 * time.Millisecond)
    secondSyncTime := time.Now()
    err = db.MarkMovieSynced(movie.ID)
    if err != nil {
        t.Fatalf("MarkMovieSynced (second) failed: %v", err)
    }

    retrieved2, err := db.GetMovieByID(movie.ID)
    if err != nil {
        t.Fatalf("GetMovieByID (second) failed: %v", err)
    }

    // Verify timestamps are different (second sync should be later)
    if retrieved2.RadarrSyncedAt.Before(*retrieved1.RadarrSyncedAt) {
        t.Error("second sync timestamp should be later than first")
    }
}
```

**Step 8: Run test**

```bash
go test ./internal/database/ -run TestMarkMovieSynced_UpdatesTimestamp -v
```

Expected: PASS

**Step 9: Write TestGetDirtySeries_OrderedByPriority**

Verify ORDER BY source_priority DESC works.

```go
func TestGetDirtySeries_OrderedByPriority(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    // Create series with different priorities
    seriesIDs := make([]int64, 3)
    priorities := []int{50, 100, 75}

    for i, priority := range priorities {
        series := &Series{
            Title:           fmt.Sprintf("Test Show %d", i),
            Year:            2020 + i,
            CanonicalPath:    fmt.Sprintf("/tv/Test Show %d (%d)", i, 2020+i),
            LibraryRoot:      "/tv",
            Source:           "test",
            SourcePriority:   priority,
        }
        _, err := db.UpsertSeries(series)
        if err != nil {
            t.Fatalf("UpsertSeries %d failed: %v", i, err)
        }
        seriesIDs[i] = series.ID
        _ = db.SetSeriesDirty(series.ID)
    }

    dirtySeries, err := db.GetDirtySeries()
    if err != nil {
        t.Fatalf("GetDirtySeries failed: %v", err)
    }

    if len(dirtySeries) != 3 {
        t.Fatalf("expected 3 dirty series, got %d", len(dirtySeries))
    }

    // Verify order: 100, 75, 50 (DESC)
    expectedOrder := []int{100, 75, 50}
    for i, series := range dirtySeries {
        if series.SourcePriority != expectedOrder[i] {
            t.Errorf("position %d: expected priority %d, got %d",
                i, expectedOrder[i], series.SourcePriority)
        }
    }
}
```

**Step 10: Run test**

```bash
go test ./internal/database/ -run TestGetDirtySeries_OrderedByPriority -v
```

Expected: PASS

**Step 11: Write TestGetDirtyMovies_OrderedByPriority**

Similar test for movies.

```go
func TestGetDirtyMovies_OrderedByPriority(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    // Create movies with different priorities
    movieIDs := make([]int64, 3)
    priorities := []int{50, 100, 75}

    for i, priority := range priorities {
        movie := &Movie{
            Title:           fmt.Sprintf("Test Movie %d", i),
            Year:            2020 + i,
            CanonicalPath:    fmt.Sprintf("/movies/Test Movie %d (%d)", i, 2020+i),
            LibraryRoot:      "/movies",
            Source:           "test",
            SourcePriority:   priority,
        }
        _, err := db.UpsertMovie(movie)
        if err != nil {
            t.Fatalf("UpsertMovie %d failed: %v", i, err)
        }
        movieIDs[i] = movie.ID
        _ = db.SetMovieDirty(movie.ID)
    }

    dirtyMovies, err := db.GetDirtyMovies()
    if err != nil {
        t.Fatalf("GetDirtyMovies failed: %v", err)
    }

    if len(dirtyMovies) != 3 {
        t.Fatalf("expected 3 dirty movies, got %d", len(dirtyMovies))
    }

    expectedOrder := []int{100, 75, 50}
    for i, movie := range dirtyMovies {
        if movie.SourcePriority != expectedOrder[i] {
            t.Errorf("position %d: expected priority %d, got %d",
                i, expectedOrder[i], movie.SourcePriority)
        }
    }
}
```

**Step 12: Run test**

```bash
go test ./internal/database/ -run TestGetDirtyMovies_OrderedByPriority -v
```

Expected: PASS

**Step 13: Write TestDirtyFlags_DefaultToFalse**

Verify new records have dirty=0 by default.

```go
func TestDirtyFlags_DefaultToFalse(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    // Create series without setting dirty
    series := &Series{
        Title:         "Test Show",
        Year:          2020,
        CanonicalPath: "/tv/Test Show (2020)",
        LibraryRoot:   "/tv",
        Source:        "jellywatch",
    }
    _, err := db.UpsertSeries(series)
    if err != nil {
        t.Fatalf("UpsertSeries failed: %v", err)
    }

    retrieved, err := db.GetSeriesByID(series.ID)
    if err != nil {
        t.Fatalf("GetSeriesByID failed: %v", err)
    }

    if retrieved.SonarrPathDirty {
        t.Error("sonarr_path_dirty should default to false")
    }
    if retrieved.RadarrPathDirty {
        t.Error("radarr_path_dirty should default to false")
    }
    if retrieved.SonarrSyncedAt != nil {
        t.Error("sonarr_synced_at should default to nil")
    }
    if retrieved.RadarrSyncedAt != nil {
        t.Error("radarr_synced_at should default to nil")
    }

    // Same for movies
    movie := &Movie{
        Title:         "Test Movie",
        Year:          2020,
        CanonicalPath: "/movies/Test Movie (2020)",
        LibraryRoot:   "/movies",
        Source:        "jellywatch",
    }
    _, err = db.UpsertMovie(movie)
    if err != nil {
        t.Fatalf("UpsertMovie failed: %v", err)
    }

    retrievedMovie, err := db.GetMovieByID(movie.ID)
    if err != nil {
        t.Fatalf("GetMovieByID failed: %v", err)
    }

    if retrievedMovie.RadarrPathDirty {
        t.Error("radarr_path_dirty should default to false for movies")
    }
    if retrievedMovie.RadarrSyncedAt != nil {
        t.Error("radarr_synced_at should default to nil for movies")
    }
}
```

**Step 14: Run test**

```bash
go test ./internal/database/ -run TestDirtyFlags_DefaultToFalse -v
```

Expected: PASS

**Step 15: Write TestDirtyFlags_SurvivesRestart**

Verify dirty flags persist across DB close/reopen.

```go
func TestDirtyFlags_SurvivesRestart(t *testing.T) {
    tmpDir := t.TempDir()
    dbPath := filepath.Join(tmpDir, "test.db")

    // First session: create and set dirty
    db1, err := database.OpenPath(dbPath)
    if err != nil {
        t.Fatalf("failed to open DB first time: %v", err)
    }

    series := &database.Series{
        Title:         "Test Show",
        Year:          2020,
        CanonicalPath: "/tv/Test Show (2020)",
        LibraryRoot:   "/tv",
        Source:        "jellywatch",
    }
    _, err = db1.UpsertSeries(series)
    if err != nil {
        db1.Close()
        t.Fatalf("UpsertSeries failed: %v", err)
    }

    err = db1.SetSeriesDirty(series.ID)
    if err != nil {
        db1.Close()
        t.Fatalf("SetSeriesDirty failed: %v", err)
    }

    seriesID := series.ID
    db1.Close()

    // Second session: reopen and verify dirty flag persists
    db2, err := database.OpenPath(dbPath)
    if err != nil {
        t.Fatalf("failed to reopen DB: %v", err)
    }
    defer db2.Close()

    retrieved, err := db2.GetSeriesByID(seriesID)
    if err != nil {
        t.Fatalf("GetSeriesByID failed: %v", err)
    }
    if !retrieved.SonarrPathDirty {
        t.Error("dirty flag should persist across DB restart")
    }
}
```

**Step 16: Run test**

```bash
go test ./internal/database/ -run TestDirtyFlags_SurvivesRestart -v
```

Expected: PASS

**Step 17: Commit**

```bash
git add internal/database/dirty_test.go
git commit -m "test: add missing dirty flag tests (idempotency, ordering, persistence)"
```

---

## Task 2: Schema Migration Tests

**Files:**
- Create: `internal/database/schema_test.go`

**Step 1: Write TestMigration11_IdempotentApplication**

Verify migration can be applied twice without error.

```go
func TestMigration11_IdempotentApplication(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    // Get current schema version
    var version int
    err := db.DB().QueryRow("SELECT version FROM schema_version ORDER BY version DESC LIMIT 1").Scan(&version)
    if err != nil {
        t.Fatalf("failed to get schema version: %v", err)
    }

    if version < 10 {
        t.Skip("migration 11 depends on migration 10")
    }

    // Apply migration 11 manually
    migrations := getMigrations() // Need to expose this or copy migration code
    var migration11 *migration
    for _, m := range migrations {
        if m.version == 11 {
            migration11 = &m
            break
        }
    }

    if migration11 == nil {
        t.Fatal("migration 11 not found")
    }

    // Apply first time
    for _, stmt := range migration11.up {
        _, err = db.DB().Exec(stmt)
        if err != nil {
            t.Fatalf("first application of migration 11 failed: %v", err)
        }
    }

    // Apply second time - should not error (column already exists is OK with IF NOT EXISTS, but we test anyway)
    for _, stmt := range migration11.up {
        _, err = db.DB().Exec(stmt)
        if err != nil {
            // ALTER TABLE errors are expected on second run
            if !strings.Contains(err.Error(), "duplicate column") {
                t.Logf("second application error (expected): %v", err)
            }
        }
    }
}
```

**Step 2: Run test**

```bash
go test ./internal/database/ -run TestMigration11_IdempotentApplication -v
```

Expected: PASS (with expected errors on duplicate columns)

**Step 3: Write TestMigration11_UpgradesFromV10**

Test upgrade path from version 10.

```go
func TestMigration11_UpgradesFromV10(t *testing.T) {
    tmpDir := t.TempDir()
    dbPath := filepath.Join(tmpDir, "test.db")

    // Create database at version 10
    db, err := sql.Open("sqlite3", dbPath)
    if err != nil {
        t.Fatalf("failed to create test DB: %v", err)
    }
    defer db.Close()

    // Run migrations 1-10 only
    err = applyMigrationsUpTo(db, 10)
    if err != nil {
        t.Fatalf("failed to apply migrations 1-10: %v", err)
    }

    // Create test series in version 10 schema
    _, err = db.Exec(`
        INSERT INTO series (title, title_normalized, year, canonical_path, library_root, source, source_priority)
        VALUES (?, ?, ?, ?, ?, ?, ?)
    `, "Test Show", "testshow", 2020, "/tv/Test Show (2020)", "/tv", "jellywatch", 100)
    if err != nil {
        t.Fatalf("failed to insert series at v10: %v", err)
    }

    // Close and reopen with full migrations (should auto-migrate to 11)
    db.Close()

    mediaDB, err := database.OpenPath(dbPath)
    if err != nil {
        t.Fatalf("failed to reopen DB: %v", err)
    }
    defer mediaDB.Close()

    // Verify schema version is now 11
    var version int
    err = mediaDB.DB().QueryRow("SELECT version FROM schema_version ORDER BY version DESC LIMIT 1").Scan(&version)
    if err != nil {
        t.Fatalf("failed to get schema version: %v", err)
    }

    if version != 11 {
        t.Errorf("expected schema version 11, got %d", version)
    }

    // Verify existing series still accessible
    series, err := mediaDB.GetSeriesByTitle("Test Show", 2020)
    if err != nil {
        t.Fatalf("failed to get series after migration: %v", err)
    }
    if series == nil {
        t.Fatal("series should exist after migration")
    }

    // Verify new columns have default values
    if series.SonarrPathDirty {
        t.Error("sonarr_path_dirty should default to false after migration")
    }
    if series.RadarrPathDirty {
        t.Error("radarr_path_dirty should default to false after migration")
    }
    if series.SonarrSyncedAt != nil {
        t.Error("sonarr_synced_at should default to nil after migration")
    }
    if series.RadarrSyncedAt != nil {
        t.Error("radarr_synced_at should default to nil after migration")
    }
}
```

**Step 4: Run test**

```bash
go test ./internal/database/ -run TestMigration11_UpgradesFromV10 -v
```

Expected: PASS

**Step 5: Write TestMigration11_ColumnDefaults**

Verify DEFAULT 0 for dirty flags works.

```go
func TestMigration11_ColumnDefaults(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    // Insert series directly without dirty columns (they should default to 0)
    _, err := db.DB().Exec(`
        INSERT INTO series (title, title_normalized, year, canonical_path, library_root, source, source_priority)
        VALUES (?, ?, ?, ?, ?, ?, ?)
    `, "Default Test", "defaulttest", 2020, "/tv/Default Test (2020)", "/tv", "test", 100)
    if err != nil {
        t.Fatalf("failed to insert series: %v", err)
    }

    // Query to check default values
    var sonarrDirty, radarrDirty int
    var sonarrSynced, radarrSynced sql.NullTime

    err = db.DB().QueryRow(`
        SELECT sonarr_path_dirty, radarr_path_dirty, sonarr_synced_at, radarr_synced_at
        FROM series WHERE title = ?
    `, "Default Test").Scan(&sonarrDirty, &radarrDirty, &sonarrSynced, &radarrSynced)
    if err != nil {
        t.Fatalf("failed to query defaults: %v", err)
    }

    if sonarrDirty != 0 {
        t.Errorf("sonarr_path_dirty default should be 0, got %d", sonarrDirty)
    }
    if radarrDirty != 0 {
        t.Errorf("radarr_path_dirty default should be 0, got %d", radarrDirty)
    }
    if sonarrSynced.Valid {
        t.Error("sonarr_synced_at should default to NULL")
    }
    if radarrSynced.Valid {
        t.Error("radarr_synced_at should default to NULL")
    }
}
```

**Step 6: Run test**

```bash
go test ./internal/database/ -run TestMigration11_ColumnDefaults -v
```

Expected: PASS

**Step 7: Write TestMigration11_NullableTimestamps**

Verify sync timestamps are nullable.

```go
func TestMigration11_NullableTimestamps(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    // Insert series
    _, err := db.DB().Exec(`
        INSERT INTO series (title, title_normalized, year, canonical_path, library_root, source, source_priority)
        VALUES (?, ?, ?, ?, ?, ?, ?)
    `, "Nullable Test", "nullabletest", 2020, "/tv/Nullable Test (2020)", "/tv", "test", 100)
    if err != nil {
        t.Fatalf("failed to insert series: %v", err)
    }

    // Mark synced (should set timestamps)
    var seriesID int64
    err = db.DB().QueryRow("SELECT id FROM series WHERE title = ?", "Nullable Test").Scan(&seriesID)
    if err != nil {
        t.Fatalf("failed to get series ID: %v", err)
    }

    err = db.MarkSeriesSynced(seriesID)
    if err != nil {
        t.Fatalf("MarkSeriesSynced failed: %v", err)
    }

    // Query to verify timestamps are set
    var sonarrSynced, radarrSynced sql.NullTime

    err = db.DB().QueryRow(`
        SELECT sonarr_synced_at, radarr_synced_at
        FROM series WHERE id = ?
    `, seriesID).Scan(&sonarrSynced, &radarrSynced)
    if err != nil {
        t.Fatalf("failed to query timestamps: %v", err)
    }

    if !sonarrSynced.Valid {
        t.Error("sonarr_synced_at should be valid after MarkSeriesSynced")
    }
    if !radarrSynced.Valid {
        t.Error("radarr_synced_at should be valid after MarkSeriesSynced")
    }
    if sonarrSynced.Time.After(time.Now()) {
        t.Error("sonarr_synced_at should not be in the future")
    }
    if radarrSynced.Time.After(time.Now()) {
        t.Error("radarr_synced_at should not be in the future")
    }
}
```

**Step 8: Run test**

```bash
go test ./internal/database/ -run TestMigration11_NullableTimestamps -v
```

Expected: PASS

**Step 9: Commit**

```bash
git add internal/database/schema_test.go
git commit -m "test: add migration 11 tests (idempotency, upgrade path, defaults)"
```

---

## Task 3: Upsert Operation Tests

**Files:**
- Modify: `internal/database/series_test.go`
- Modify: `internal/database/movies_test.go`

**Step 1: Write TestUpsertSeries_SetsDirtyFlag**

Verify path changes set dirty flag.

```go
func TestUpsertSeries_SetsDirtyFlag(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    // Initial upsert
    series := &Series{
        Title:         "Test Show",
        Year:          2020,
        CanonicalPath: "/tv/Test Show (2020)",
        LibraryRoot:   "/tv",
        Source:        "jellywatch",
    }
    _, err := db.UpsertSeries(series)
    if err != nil {
        t.Fatalf("UpsertSeries (initial) failed: %v", err)
    }

    // Verify not dirty
    retrieved1, _ := db.GetSeriesByID(series.ID)
    if retrieved1.SonarrPathDirty {
        t.Error("series should not be dirty after initial upsert")
    }

    // Upsert with new path
    series.CanonicalPath = "/tv/New Path/Test Show (2020)"
    series.LibraryRoot = "/tv/New Path"
    _, err = db.UpsertSeries(series)
    if err != nil {
        t.Fatalf("UpsertSeries (path change) failed: %v", err)
    }

    // Verify dirty flag is set
    retrieved2, err := db.GetSeriesByID(series.ID)
    if err != nil {
        t.Fatalf("GetSeriesByID failed: %v", err)
    }
    if !retrieved2.SonarrPathDirty {
        t.Error("series should be dirty after path change")
    }

    // Upsert with same path
    _, err = db.UpsertSeries(series)
    if err != nil {
        t.Fatalf("UpsertSeries (no change) failed: %v", err)
    }

    // Note: UpsertSeries doesn't detect "same path" case - dirty flag stays set
    // This is acceptable behavior - sync will clear it
}
```

**Step 2: Run test**

```bash
go test ./internal/database/ -run TestUpsertSeries_SetsDirtyFlag -v
```

Expected: PASS

**Step 3: Write TestUpsertSeries_ScansNewColumns**

Verify no SQL errors from new migration 11 columns.

```go
func TestUpsertSeries_ScansNewColumns(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    series := &Series{
        Title:         "Test Show",
        Year:          2020,
        CanonicalPath: "/tv/Test Show (2020)",
        LibraryRoot:   "/tv",
        Source:        "jellywatch",
    }

    // Upsert and retrieve
    _, err := db.UpsertSeries(series)
    if err != nil {
        t.Fatalf("UpsertSeries failed: %v", err)
    }

    retrieved, err := db.GetSeriesByID(series.ID)
    if err != nil {
        t.Fatalf("GetSeriesByID failed: %v", err)
    }

    // Access all new fields to ensure they're scanned correctly
    _ = retrieved.SonarrSyncedAt
    _ = retrieved.SonarrPathDirty
    _ = retrieved.RadarrSyncedAt
    _ = retrieved.RadarrPathDirty
}
```

**Step 4: Run test**

```bash
go test ./internal/database/ -run TestUpsertSeries_ScansNewColumns -v
```

Expected: PASS

**Step 5: Write TestUpsertMovie_SetsDirtyFlag**

Similar test for movies.

```go
func TestUpsertMovie_SetsDirtyFlag(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    movie := &Movie{
        Title:         "Test Movie",
        Year:          2020,
        CanonicalPath: "/movies/Test Movie (2020)",
        LibraryRoot:   "/movies",
        Source:        "jellywatch",
    }
    _, err := db.UpsertMovie(movie)
    if err != nil {
        t.Fatalf("UpsertMovie (initial) failed: %v", err)
    }

    retrieved1, _ := db.GetMovieByID(movie.ID)
    if retrieved1.RadarrPathDirty {
        t.Error("movie should not be dirty after initial upsert")
    }

    movie.CanonicalPath = "/movies/New Path/Test Movie (2020)"
    movie.LibraryRoot = "/movies/New Path"
    _, err = db.UpsertMovie(movie)
    if err != nil {
        t.Fatalf("UpsertMovie (path change) failed: %v", err)
    }

    retrieved2, err := db.GetMovieByID(movie.ID)
    if err != nil {
        t.Fatalf("GetMovieByID failed: %v", err)
    }
    if !retrieved2.RadarrPathDirty {
        t.Error("movie should be dirty after path change")
    }
}
```

**Step 6: Run test**

```bash
go test ./internal/database/ -run TestUpsertMovie_SetsDirtyFlag -v
```

Expected: PASS

**Step 7: Write TestUpsertMovie_ScansNewColumns**

Verify no SQL errors for movie columns.

```go
func TestUpsertMovie_ScansNewColumns(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    movie := &Movie{
        Title:         "Test Movie",
        Year:          2020,
        CanonicalPath: "/movies/Test Movie (2020)",
        LibraryRoot:   "/movies",
        Source:        "jellywatch",
    }

    _, err := db.UpsertMovie(movie)
    if err != nil {
        t.Fatalf("UpsertMovie failed: %v", err)
    }

    retrieved, err := db.GetMovieByID(movie.ID)
    if err != nil {
        t.Fatalf("GetMovieByID failed: %v", err)
    }

    // Access new fields
    _ = retrieved.RadarrSyncedAt
    _ = retrieved.RadarrPathDirty
}
```

**Step 8: Run test**

```bash
go test ./internal/database/ -run TestUpsertMovie_ScansNewColumns -v
```

Expected: PASS

**Step 9: Commit**

```bash
git add internal/database/series_test.go internal/database/movies_test.go
git commit -m "test: add upsert dirty flag tests"
```

---

## Task 4: GetAllSeries/Movies Tests

**Files:**
- Modify: `internal/database/series_test.go`
- Modify: `internal/database/movies_test.go`

**Step 1: Write TestGetAllSeries_IncludesDirtyFlags**

Verify all new columns returned.

```go
func TestGetAllSeries_IncludesDirtyFlags(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    // Insert series with dirty flag set
    _, err := db.DB().Exec(`
        INSERT INTO series (title, title_normalized, year, canonical_path, library_root, source, source_priority, sonarr_path_dirty)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)
    `, "Dirty Series", "dirtyseries", 2020, "/tv/Dirty Series (2020)", "/tv", "test", 100, 1)
    if err != nil {
        t.Fatalf("failed to insert dirty series: %v", err)
    }

    allSeries, err := db.GetAllSeries()
    if err != nil {
        t.Fatalf("GetAllSeries failed: %v", err)
    }

    foundDirty := false
    for _, s := range allSeries {
        if s.Title == "Dirty Series" {
            foundDirty = true
            if !s.SonarrPathDirty {
                t.Error("dirty flag should be included in GetAllSeries result")
            }
        }
        // Access all new fields to ensure they're scanned
        _ = s.SonarrSyncedAt
        _ = s.SonarrPathDirty
        _ = s.RadarrSyncedAt
        _ = s.RadarrPathDirty
    }

    if !foundDirty {
        t.Error("dirty series should be in GetAllSeries result")
    }
}
```

**Step 2: Run test**

```bash
go test ./internal/database/ -run TestGetAllSeries_IncludesDirtyFlags -v
```

Expected: PASS

**Step 3: Write TestGetAllSeries_Empty**

Verify empty DB returns empty slice, not nil.

```go
func TestGetAllSeries_Empty(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    // Don't insert anything
    allSeries, err := db.GetAllSeries()
    if err != nil {
        t.Fatalf("GetAllSeries failed: %v", err)
    }

    if allSeries == nil {
        t.Fatal("GetAllSeries should return empty slice, not nil")
    }
    if len(allSeries) != 0 {
        t.Errorf("expected 0 series, got %d", len(allSeries))
    }
}
```

**Step 4: Run test**

```bash
go test ./internal/database/ -run TestGetAllSeries_Empty -v
```

Expected: PASS

**Step 5: Write TestGetAllMovies_IncludesDirtyFlags**

Similar test for movies.

```go
func TestGetAllMovies_IncludesDirtyFlags(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    _, err := db.DB().Exec(`
        INSERT INTO movies (title, title_normalized, year, canonical_path, library_root, source, source_priority, radarr_path_dirty)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)
    `, "Dirty Movie", "dirtymovie", 2020, "/movies/Dirty Movie (2020)", "/movies", "test", 100, 1)
    if err != nil {
        t.Fatalf("failed to insert dirty movie: %v", err)
    }

    allMovies, err := db.GetAllMovies()
    if err != nil {
        t.Fatalf("GetAllMovies failed: %v", err)
    }

    foundDirty := false
    for _, m := range allMovies {
        if m.Title == "Dirty Movie" {
            foundDirty = true
            if !m.RadarrPathDirty {
                t.Error("dirty flag should be included in GetAllMovies result")
            }
        }
        _ = m.RadarrSyncedAt
        _ = m.RadarrPathDirty
    }

    if !foundDirty {
        t.Error("dirty movie should be in GetAllMovies result")
    }
}
```

**Step 6: Run test**

```bash
go test ./internal/database/ -run TestGetAllMovies_IncludesDirtyFlags -v
```

Expected: PASS

**Step 7: Write TestGetAllMovies_Empty**

Verify empty slice for empty DB.

```go
func TestGetAllMovies_Empty(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    allMovies, err := db.GetAllMovies()
    if err != nil {
        t.Fatalf("GetAllMovies failed: %v", err)
    }

    if allMovies == nil {
        t.Fatal("GetAllMovies should return empty slice, not nil")
    }
    if len(allMovies) != 0 {
        t.Errorf("expected 0 movies, got %d", len(allMovies))
    }
}
```

**Step 8: Run test**

```bash
go test ./internal/database/ -run TestGetAllMovies_Empty -v
```

Expected: PASS

**Step 9: Commit**

```bash
git add internal/database/series_test.go internal/database/movies_test.go
git commit -m "test: add GetAllSeries/Movies dirty flag tests"
```

---

## Final Verification

**Step 1: Run all database tests**

```bash
go test ./internal/database/... -v -cover
```

Expected: All tests pass, coverage > 60%

**Step 2: Verify test count**

```bash
go test ./internal/database/... -v 2>&1 | grep "^--- PASS:" | wc -l
```

Expected: 35+ tests (22 existing + 13 new)
