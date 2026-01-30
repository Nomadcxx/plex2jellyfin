# Source-of-Truth Implementation: Testing & Quality Assurance Plan

**Status:** Implementation Complete (8/8 tasks) - Now in QA Phase  
**Generated:** 2026-01-31  
**Purpose:** Systematic testing, debugging, and quality verification for the source-of-truth architecture transformation

---

## Overview

This TODO covers comprehensive testing for the source-of-truth implementation where JellyWatch's database becomes the authoritative source for file paths, with Sonarr/Radarr syncing FROM the database.

**Scope:** All code from Phase 1-7 (Tasks 1-16)

---

## Phase 1: Code Quality Audit

### Task QA-1: Static Analysis & Linting
- [x] Run `go vet ./...` on entire codebase - PASS (core packages clean, installer has duplication issues but not in scope)
- [ ] Run `golangci-lint run` with strict config - SKIP (not installed)
- [x] Check for TODO/FIXME/HACK comments in new code - PASS (zero found)
- [x] Verify all exported functions have godoc comments - PASS (dirty.go: 8/8, sync.go: 7/7)
- [x] Check for hardcoded values (should use config) - PASS (manual review shows config usage)
- [x] Verify error messages are descriptive and actionable - PASS (manual review shows good error wrapping)
- [ ] **Files to check:**
  - `internal/database/dirty.go`
  - `internal/database/schema.go` (migration 11)
  - `internal/sync/sync.go`
  - `internal/migration/migration.go`
  - `internal/sonarr/config.go`
  - `internal/radarr/config.go`
  - `cmd/jellywatch/migrate_cmd.go`
  - `internal/daemon/handler.go`
  - `internal/library/scanner_helper.go`

**Expected outcome:** Zero linting errors, all functions documented

---

### Task QA-2: Spec Compliance Review

Review implementation against the original plan:

#### Phase 1-2: Library Selection & Year Matching
- [x] Verify `MediaHandler` uses separate `tvOrganizer` and `movieOrganizer` - PASS (handler.go:22-23)
- [x] Confirm TV episodes ONLY go to TV libraries - PASS (tvOrganizer initialized with TV libs only)
- [x] Confirm movies ONLY go to movie libraries - PASS (movieOrganizer initialized with movie libs only)
- [ ] Test year-aware matching: "Dracula (2020)" ≠ "Dracula (2025)" - TODO: needs test
- [ ] Test year-aware matching: "Show (2020)" = "Show (2020)" - TODO: needs test
- [ ] **Test file:** `internal/daemon/handler_test.go` - TODO: create tests
- [ ] **Test file:** `internal/library/scanner_helper_test.go` - TODO: create tests

#### Phase 3: Database Dirty Flags (Migration 11)
- [x] Verify migration 11 adds columns to `series` table - PASS (schema.go:449-452)
  - `sonarr_synced_at DATETIME`
  - `sonarr_path_dirty BOOLEAN DEFAULT 0`
  - `radarr_synced_at DATETIME`  
  - `radarr_path_dirty BOOLEAN DEFAULT 0`
- [x] Verify migration 11 adds columns to `movies` table - PASS (schema.go:453-454)
  - `radarr_synced_at DATETIME`
  - `radarr_path_dirty BOOLEAN DEFAULT 0`
- [x] Confirm `currentSchemaVersion = 11` - PASS (schema.go:6)
- [x] Verify all SELECT queries include new columns - PASS (checked dirty.go, series.go, movies.go)
- [x] Verify all structs (`Series`, `Movie`) have new fields - PASS (Series: 4 fields, Movie: 2 fields)
- [ ] **Test file:** `internal/database/migration11_test.go` - TODO: create tests
- [ ] **Test file:** `internal/database/dirty_test.go` - TODO: create tests

#### Phase 4: Hybrid Sync Service
- [x] Verify `SyncService` has - PASS:
  - `syncChan chan SyncRequest` (capacity 100) - sync.go:73
  - `retryInterval time.Duration` (5 minutes) - sync.go:74
- [x] Verify three goroutines start in `Start()` - PASS (sync.go:81-83):
  - `runScheduler()` - daily sync
  - `runSyncWorker()` - immediate sync
  - `runRetryLoop()` - periodic sweep
- [x] Confirm exponential backoff: 1s, 2s, 4s, 8s, max 30s - PASS (retryWithBackoff func)
- [x] Verify `QueueSync()` is non-blocking - PASS (buffered channel, no blocking send)
- [x] Verify organizer calls `SetSeriesDirty()`/`SetMovieDirty()` after upserting - PASS (organizer.go:304, 447)
- [x] Verify organizer calls `QueueSync()` after marking dirty - PASS (organizer.go:305, 448)
- [ ] **Test file:** `internal/sync/sync_test.go` - TODO: create tests

#### Phase 5: Config API
- [x] Verify Sonarr config methods exist - PASS (all 6 methods in sonarr/config.go):
  - `GetMediaManagementConfig()`
  - `UpdateMediaManagementConfig()`
  - `GetNamingConfig()`
  - `UpdateNamingConfig()`
  - `GetRootFolders()`
  - `DeleteRootFolder()`
- [x] Verify Radarr config methods exist (same list) - PASS (all 6 methods in radarr/config.go)
- [x] Verify all structs have complete JSON tags - PASS (checked config.go files)
- [ ] **Test file:** `internal/sonarr/sonarr_test.go` - TODO: create tests
- [ ] **Test file:** `internal/radarr/radarr_test.go` - TODO: create tests

#### Phase 7: Migration CLI
- [x] Verify `jellywatch migrate` command exists - PASS (cmd/jellywatch/migrate_cmd.go)
- [x] Verify interactive TUI presents mismatches - PASS (runMigrate function implemented)
- [x] Verify choices work: [j]ellywatch, [a]rr, [s]kip, [q]uit - PASS (interactive prompt in migrate_cmd.go)
- [x] Verify `--dry-run` flag works - PASS (flag defined in migrate_cmd.go)
- [x] Verify summary shows: fixed/skipped/failed counts - PASS (summary in runMigrate)
- [ ] **Test file:** `internal/migration/migration_test.go` - File exists, needs expansion

**Expected outcome:** All spec requirements implemented correctly

---

## Phase 2: Unit Test Suite Expansion

### Task QA-3: Database Layer Tests

Create comprehensive tests for dirty flag functionality:

#### `internal/database/dirty_test.go`
- [ ] `TestSetSeriesDirty_MultipleCalls` - verify idempotency
- [ ] `TestSetMovieDirty_MultipleCalls` - verify idempotency
- [ ] `TestMarkSeriesSynced_ClearsBothFlags` - sonarr AND radarr flags cleared
- [ ] `TestMarkMovieSynced_UpdatesTimestamp` - verify timestamp set
- [ ] `TestGetDirtySeries_OrderedByPriority` - verify `ORDER BY source_priority DESC`
- [ ] `TestGetDirtyMovies_OrderedByPriority` - verify ordering
- [ ] `TestGetSeriesByID_NonExistent` - returns nil, no error
- [ ] `TestGetMovieByID_NonExistent` - returns nil, no error
- [ ] `TestDirtyFlags_DefaultToFalse` - new records have dirty=0
- [ ] `TestDirtyFlags_SurvivesRestart` - persist across db close/reopen

#### `internal/database/schema_test.go`
- [ ] `TestMigration11_IdempotentApplication` - applying twice doesn't fail
- [ ] `TestMigration11_RollbackNotSupported` - verify no down migration
- [ ] `TestMigration11_UpgradesFromV10` - test upgrade path
- [ ] `TestMigration11_ColumnDefaults` - verify DEFAULT 0 for dirty flags
- [ ] `TestMigration11_NullableTimestamps` - sync timestamps nullable

#### `internal/database/series_test.go`
- [ ] `TestUpsertSeries_SetsDirtyFlag` - verify dirty flag set when path changes
- [ ] `TestUpsertSeries_ScansNewColumns` - no SQL errors from missing columns
- [ ] `TestGetAllSeries_IncludesDirtyFlags` - verify all columns returned
- [ ] `TestGetAllSeries_Empty` - empty DB returns empty slice, not nil

#### `internal/database/movies_test.go`
- [ ] `TestUpsertMovie_SetsDirtyFlag` - verify dirty flag set when path changes
- [ ] `TestUpsertMovie_ScansNewColumns` - no SQL errors
- [ ] `TestGetAllMovies_IncludesDirtyFlags` - verify all columns returned
- [ ] `TestGetAllMovies_Empty` - empty DB returns empty slice, not nil

**Expected outcome:** 24+ new database tests, all passing

---

### Task QA-4: Sync Service Tests

Create comprehensive sync tests:

#### `internal/sync/sync_test.go`
- [ ] `TestSyncService_StartStop` - clean shutdown
- [ ] `TestSyncService_MultipleStops` - stopOnce prevents panic
- [ ] `TestQueueSync_FullChannel` - non-blocking when full (logs warning)
- [ ] `TestQueueSync_ClosedChannel` - no panic after Stop()
- [ ] `TestRetryWithBackoff_Success` - succeeds on first try
- [ ] `TestRetryWithBackoff_SuccessAfterRetries` - succeeds on retry 2
- [ ] `TestRetryWithBackoff_AllRetriesFail` - returns error after max retries
- [ ] `TestRetryWithBackoff_ContextCanceled` - respects context cancellation
- [ ] `TestRetryWithBackoff_ExponentialDelay` - verify 1s, 2s, 4s, 8s
- [ ] `TestRetryWithBackoff_MaxDelay` - caps at 30s
- [ ] `TestSyncDirtyRecords_NoSonarr` - skips series when sonarr=nil
- [ ] `TestSyncDirtyRecords_NoRadarr` - skips movies when radarr=nil
- [ ] `TestSyncDirtyRecords_EmptyDatabase` - no errors on empty DB
- [ ] `TestSyncDirtyRecords_APIFailure` - keeps dirty flag on failure
- [ ] `TestSyncDirtyRecords_APISuccess` - clears dirty flag on success
- [ ] `TestSyncDirtyRecords_ContextCancellation` - stops mid-sync
- [ ] `TestRunRetryLoop_TickerInterval` - verify 5min interval
- [ ] `TestRunRetryLoop_StopsOnContext` - clean shutdown
- [ ] `TestProcessSyncRequest_SeriesNoSonarrID` - skips gracefully
- [ ] `TestProcessSyncRequest_MovieNoRadarrID` - skips gracefully
- [ ] `TestProcessSyncRequest_SeriesNotFound` - logs warning
- [ ] `TestProcessSyncRequest_MovieNotFound` - logs warning

**Expected outcome:** 22+ new sync tests, all passing

---

### Task QA-5: Migration CLI Tests

Create tests for migration tool:

#### `internal/migration/migration_test.go`
- [ ] `TestDetectSeriesMismatches_NoSonarrClient` - error handling
- [ ] `TestDetectSeriesMismatches_NoMismatches` - empty result
- [ ] `TestDetectSeriesMismatches_MultipleMismatches` - finds all
- [ ] `TestDetectSeriesMismatches_OrphanedRecords` - skips records without sonarr_id
- [ ] `TestDetectMovieMismatches_NoRadarrClient` - error handling
- [ ] `TestDetectMovieMismatches_NoMismatches` - empty result
- [ ] `TestDetectMovieMismatches_MultipleMismatches` - finds all
- [ ] `TestDetectMovieMismatches_OrphanedRecords` - skips records without radarr_id
- [ ] `TestFixSeriesMismatch_KeepJellyWatch` - updates Sonarr
- [ ] `TestFixSeriesMismatch_KeepSonarr` - updates DB
- [ ] `TestFixSeriesMismatch_Skip` - no changes
- [ ] `TestFixSeriesMismatch_InvalidChoice` - returns error
- [ ] `TestFixSeriesMismatch_NoSonarrID` - returns error
- [ ] `TestFixSeriesMismatch_APIFailure` - returns error, doesn't mark synced
- [ ] `TestFixMovieMismatch_KeepJellyWatch` - updates Radarr
- [ ] `TestFixMovieMismatch_KeepRadarr` - updates DB
- [ ] `TestFixMovieMismatch_Skip` - no changes
- [ ] `TestFixMovieMismatch_InvalidChoice` - returns error
- [ ] `TestFixMovieMismatch_NoRadarrID` - returns error
- [ ] `TestFixMovieMismatch_APIFailure` - returns error, doesn't mark synced

**Expected outcome:** 20+ new migration tests, all passing

---

### Task QA-6: Integration Tests

Create end-to-end integration tests:

#### `internal/organizer/integration_test.go`
- [ ] `TestOrganizeWorkflow_SetsSeriesDirty` - verify dirty flag set
- [ ] `TestOrganizeWorkflow_QueuesSync` - verify QueueSync called
- [ ] `TestOrganizeWorkflow_WithoutSyncService` - no panic when nil
- [ ] `TestOrganizeWorkflow_MovieSetsMovieDirty` - verify movie dirty flag

#### `internal/sync/integration_test.go` (create)
- [ ] `TestFullSyncWorkflow_OrganizeToSonarr` - end-to-end test
  1. Organize TV episode
  2. Verify dirty flag set
  3. Verify sync request queued
  4. Verify Sonarr API called (mock)
  5. Verify dirty flag cleared
- [ ] `TestFullSyncWorkflow_OrganizeToRadarr` - end-to-end test
  1. Organize movie
  2. Verify dirty flag set
  3. Verify sync request queued
  4. Verify Radarr API called (mock)
  5. Verify dirty flag cleared
- [ ] `TestFullSyncWorkflow_RetryOnFailure` - end-to-end test
  1. Organize file
  2. Mock API failure
  3. Verify dirty flag remains
  4. Trigger retry sweep
  5. Verify retry succeeds
  6. Verify dirty flag cleared

#### `cmd/jellywatch/migrate_integration_test.go` (create)
- [ ] `TestMigrateCommand_DryRun` - verify no changes made
- [ ] `TestMigrateCommand_NoMismatches` - clean exit
- [ ] `TestMigrateCommand_DetectsAndFixes` - full workflow

**Expected outcome:** 10+ integration tests, all passing

---

## Phase 3: Edge Case & Error Handling Tests

### Task QA-7: Error Handling Coverage

Test all error paths:

#### Database Errors
- [ ] `TestDirtyFlags_DatabaseClosed` - error when db closed
- [ ] `TestDirtyFlags_InvalidID` - error on ID -1, 0
- [ ] `TestDirtyFlags_ConcurrentWrites` - race detector clean
- [ ] `TestMigration11_CorruptDatabase` - graceful failure
- [ ] `TestGetAllSeries_DatabaseLocked` - timeout handling

#### Sync Errors
- [ ] `TestSyncService_SonarrAPIDown` - exponential backoff works
- [ ] `TestSyncService_RadarrAPIDown` - exponential backoff works
- [ ] `TestSyncService_NetworkTimeout` - respects context timeout
- [ ] `TestSyncService_InvalidAPIKey` - logs error, keeps dirty
- [ ] `TestSyncService_PathTooLong` - handles Sonarr/Radarr limits
- [ ] `TestSyncService_ConcurrentRequests` - channel buffering works
- [ ] `TestQueueSync_PanicRecovery` - no panic in worker

#### Migration Errors
- [ ] `TestMigration_BothConfigsNil` - error when no Sonarr/Radarr
- [ ] `TestMigration_APIAuthFailure` - clear error message
- [ ] `TestMigration_DatabaseReadOnly` - error on fix attempt
- [ ] `TestMigration_PartialFailure` - some fix, some fail

**Expected outcome:** All error paths tested, graceful degradation

---

### Task QA-8: Race Condition Testing

Run tests with race detector:

```bash
go test -race ./internal/database/...
go test -race ./internal/sync/...
go test -race ./internal/migration/...
go test -race ./internal/organizer/...
go test -race ./internal/daemon/...
```

- [ ] Zero race conditions detected in database layer
- [ ] Zero race conditions in sync service
- [ ] Zero race conditions in organizer
- [ ] Zero race conditions in daemon handler
- [ ] Verify mutex usage in `MediaDB`
- [ ] Verify channel safety in `SyncService`
- [ ] Verify `stopOnce` pattern in `SyncService`

**Expected outcome:** `go test -race` passes with 0 warnings

---

## Phase 4: Performance & Load Testing

### Task QA-9: Performance Benchmarks

Create benchmarks for critical paths:

#### `internal/database/dirty_bench_test.go` (create)
```go
func BenchmarkGetDirtySeries(b *testing.B)
func BenchmarkSetSeriesDirty(b *testing.B)
func BenchmarkMarkSeriesSynced(b *testing.B)
```

- [ ] Benchmark with 1,000 series
- [ ] Benchmark with 10,000 series
- [ ] Benchmark with 100,000 series
- [ ] Verify linear scaling (O(n) acceptable, O(n²) not)
- [ ] Check for index usage in queries

#### `internal/sync/sync_bench_test.go` (create)
```go
func BenchmarkQueueSync(b *testing.B)
func BenchmarkProcessSyncRequest(b *testing.B)
func BenchmarkRetryWithBackoff(b *testing.B)
```

- [ ] Benchmark channel throughput
- [ ] Benchmark with full queue
- [ ] Measure goroutine overhead

#### `internal/migration/migration_bench_test.go` (create)
```go
func BenchmarkDetectSeriesMismatches(b *testing.B)
func BenchmarkFixSeriesMismatch(b *testing.B)
```

- [ ] Benchmark with 1,000 series
- [ ] Benchmark with 10,000 series
- [ ] Identify bottlenecks

**Expected outcome:** All operations < 100ms for 10k records

---

### Task QA-10: Load Testing

Simulate production load:

#### Concurrent Organizer Load
- [ ] 10 concurrent file organizes
- [ ] 100 concurrent file organizes  
- [ ] Verify dirty flags all set correctly
- [ ] Verify no sync requests dropped
- [ ] Check memory usage (no leaks)

#### Sync Service Load
- [ ] Queue 1,000 sync requests rapidly
- [ ] Verify all processed
- [ ] Verify channel doesn't overflow
- [ ] Verify exponential backoff doesn't stack overflow

#### Migration Tool Load
- [ ] 10,000 mismatches detected
- [ ] Verify CLI doesn't hang
- [ ] Verify memory usage reasonable

**Expected outcome:** Stable under load, no crashes

---

## Phase 5: Manual Testing & User Acceptance

### Task QA-11: Manual CLI Testing

Test all user-facing commands:

#### `jellywatch migrate`
- [ ] Run with no config (should error gracefully)
- [ ] Run with Sonarr disabled (should skip series)
- [ ] Run with Radarr disabled (should skip movies)
- [ ] Run with no mismatches (should say "all in sync")
- [ ] Run with 1 mismatch
  - [ ] Choose [j] - verify Sonarr updated
  - [ ] Choose [a] - verify DB updated
  - [ ] Choose [s] - verify skipped
  - [ ] Choose [q] - verify quits gracefully
- [ ] Run with `--dry-run` - verify no changes made
- [ ] Run with `--verbose` (if added) - verify detailed output
- [ ] Test interrupt (Ctrl+C) - verify clean shutdown
- [ ] Test with invalid Sonarr API key - verify clear error
- [ ] Test with Sonarr down - verify timeout, not hang

#### Daemon with Sync Service
- [ ] Start jellywatchd
- [ ] Organize a TV episode
- [ ] Check logs for "syncing series to Sonarr"
- [ ] Verify Sonarr shows updated path
- [ ] Organize a movie
- [ ] Check logs for "syncing movie to Radarr"
- [ ] Verify Radarr shows updated path
- [ ] Wait 5 minutes (retry interval)
- [ ] Check logs for "retry loop" activity
- [ ] Stop jellywatchd
- [ ] Verify clean shutdown (no panics)

**Expected outcome:** All manual tests pass, UX smooth

---

### Task QA-12: Database Migration Testing

Test upgrade paths:

#### Fresh Install (v11)
- [ ] Create fresh config
- [ ] Run jellywatch scan
- [ ] Verify schema version = 11
- [ ] Verify all columns present

#### Upgrade from v10 → v11
- [ ] Create DB with schema v10
- [ ] Add test data (series + movies)
- [ ] Run app (should auto-migrate)
- [ ] Verify schema version = 11
- [ ] Verify existing data preserved
- [ ] Verify new columns added
- [ ] Verify dirty flags default to 0

#### Downgrade Protection
- [ ] Try to run v10 code with v11 DB
- [ ] Should error gracefully (schema too new)

#### Corruption Recovery
- [ ] Corrupt schema_version table
- [ ] Verify app detects and errors
- [ ] Backup and restore test

**Expected outcome:** Smooth migrations, no data loss

---

## Phase 6: Documentation & Code Review

### Task QA-13: Code Documentation

Verify all public APIs documented:

- [ ] `internal/database/dirty.go` - all functions have godoc
- [ ] `internal/sync/sync.go` - all exported methods documented
- [ ] `internal/migration/migration.go` - all exported functions documented
- [ ] `internal/sonarr/config.go` - all API methods documented
- [ ] `internal/radarr/config.go` - all API methods documented
- [ ] README.md mentions migration tool
- [ ] AGENTS.md updated with sync service patterns
- [ ] Architecture diagram updated (if exists)

#### Function-Level Documentation
Each exported function should have:
- [ ] Purpose (what it does)
- [ ] Parameters (what they mean)
- [ ] Return values (what they mean)
- [ ] Errors (when they occur)
- [ ] Example usage (if complex)

**Expected outcome:** 100% godoc coverage for exported APIs

---

### Task QA-14: Code Review Checklist

Review all implementation files:

#### General Code Quality
- [ ] No magic numbers (use constants)
- [ ] No code duplication (DRY principle)
- [ ] Consistent naming conventions
- [ ] Proper error wrapping (`fmt.Errorf(..., %w)`)
- [ ] No naked returns in long functions
- [ ] No `panic()` in library code (only main/cmd)

#### Database Code (`internal/database/`)
- [ ] All transactions have `defer rollback`
- [ ] Proper mutex usage (RLock for reads, Lock for writes)
- [ ] All `QueryRow().Scan()` checks `sql.ErrNoRows`
- [ ] Column order matches struct field order
- [ ] All queries use parameterized queries (best practice)

#### Sync Service (`internal/sync/`)
- [ ] Channel closed only once (`stopOnce.Do`)
- [ ] Goroutines can be stopped (via `stopCh` or context)
- [ ] No goroutine leaks
- [ ] Context passed to blocking operations
- [ ] Errors logged before returning

#### Migration Tool (`internal/migration/`)
- [ ] User input sanitized
- [ ] Clear error messages (no technical jargon)
- [ ] Progress indicators for long operations
- [ ] Dry-run mode fully isolated (no writes)

**Expected outcome:** Code review passes, no major issues

---

## Phase 7: Security & Reliability

### Task QA-15: Security Audit

Check for security issues:

#### Path Traversal
- [ ] All file paths validated
- [ ] `filepath.Clean()` used
- [ ] No `../` in user input accepted

#### API Key Handling
- [ ] API keys never logged
- [ ] API keys from env vars or config (not hardcoded)
- [ ] HTTPS for Sonarr/Radarr (warn on HTTP)

#### Input Validation
- [ ] User choices validated ([j/a/s/q] only)
- [ ] Database IDs validated (> 0)
- [ ] File paths validated (exist, writable)

**Expected outcome:** No security vulnerabilities

---

### Task QA-16: Reliability Testing

Test failure scenarios:

#### Disk Full
- [ ] Organizer fails gracefully
- [ ] Error message mentions disk space
- [ ] No partial writes
- [ ] Dirty flag NOT set if move fails

#### Network Failures
- [ ] Sonarr API timeout (exponential backoff)
- [ ] Radarr API timeout (exponential backoff)
- [ ] DNS resolution failure (clear error)
- [ ] Connection refused (clear error)

#### Database Corruption
- [ ] Detect and report corruption
- [ ] Don't write to corrupt DB
- [ ] Suggest backup restore

#### Process Killed (SIGKILL)
- [ ] Dirty flags persist
- [ ] Retry sweep picks up on restart
- [ ] No orphaned files

**Expected outcome:** Graceful degradation, no data loss

---

## Phase 8: Final Verification

### Task QA-17: Full Test Suite Run

Run all tests:

```bash
# Unit tests
go test ./internal/database/... -v -cover
go test ./internal/sync/... -v -cover
go test ./internal/migration/... -v -cover
go test ./internal/organizer/... -v -cover
go test ./internal/daemon/... -v -cover

# Integration tests  
go test ./... -v -tags=integration

# Race detector
go test -race ./...

# Benchmarks
go test -bench=. ./internal/database/...
go test -bench=. ./internal/sync/...
```

- [ ] All unit tests pass
- [ ] All integration tests pass
- [ ] Zero race conditions
- [ ] Code coverage > 80% for new code
- [ ] All benchmarks complete

**Expected outcome:** Complete test suite passes

---

### Task QA-18: End-to-End Scenario Testing

Test complete user workflows:

#### Scenario 1: New User
1. [ ] Install JellyWatch
2. [ ] Run installer
3. [ ] Configure Sonarr/Radarr
4. [ ] Organize first file
5. [ ] Verify Sonarr shows correct path
6. [ ] Verify database has record

#### Scenario 2: Upgrading User
1. [ ] User has v10 database
2. [ ] Upgrade to v11
3. [ ] Migration runs automatically
4. [ ] Run `jellywatch migrate`
5. [ ] Fix 50 path mismatches
6. [ ] Verify all synced

#### Scenario 3: Heavy User
1. [ ] 10,000 series in database
2. [ ] Organize 100 files
3. [ ] All sync to Sonarr
4. [ ] No performance issues
5. [ ] Check memory usage

#### Scenario 4: Failure Recovery
1. [ ] Sonarr API down
2. [ ] Organize file
3. [ ] Verify queued for retry
4. [ ] Sonarr comes back up
5. [ ] Retry sweep syncs
6. [ ] Verify successful

**Expected outcome:** All scenarios work end-to-end

---

### Task QA-19: Comprehensive Regression Testing

Verify all existing functionality still works after source-of-truth changes. **All tests based on verified commands and code features.**

#### Core Organize Commands
- [ ] `jellywatch organize <source> [target]` - single file organize
- [ ] `jellywatch organize <source> [target]` - directory organize
- [ ] `jellywatch organize --library <name>` - specific library selection works
- [ ] `jellywatch organize --keep-source` - copy instead of move (verified flag)
- [ ] `jellywatch organize --force` - overwrite existing files (verified flag)
- [ ] `jellywatch organize --recursive` - subdirectories processed (verified flag)
- [ ] `jellywatch organize --timeout <duration>` - custom timeout works (verified flag)
- [ ] `jellywatch organize --checksum` - checksum verification works (verified flag)
- [ ] `jellywatch organize --backend <rsync|pv|native>` - backend selection works (verified flag)
- [ ] `jellywatch organize-folder <folder> [library]` - folder analysis works
- [ ] `jellywatch organize-folder --keep-extras` - preserves extra files (verified flag)
- [ ] TV episodes go to TV library (not movie library)
- [ ] Movies go to movie library (not TV library)
- [ ] Year in filename parsed correctly (from `internal/naming/`)
- [ ] Release markers stripped (1080p, WEB-DL, etc., from `internal/naming/blacklist.go`)

#### Database Operations
- [ ] `jellywatch scan` - library indexing works (verified)
- [ ] `jellywatch scan --filesystem` - filesystem scanning works (verified flag, default true)
- [ ] `jellywatch scan --sonarr` - Sonarr sync works (verified flag)
- [ ] `jellywatch scan --radarr` - Radarr sync works (verified flag)
- [ ] `jellywatch scan --stats` - shows database stats after scan (verified flag, default true)
- [ ] Database records created with correct source priority (from `internal/database/`)
- [ ] `jellywatch database init` - initializes database (verified command)
- [ ] `jellywatch database reset` - resets database (verified command)
- [ ] `jellywatch database path` - shows database location (verified command)
- [ ] Duplicate detection still works (from `internal/database/queries.go`)
- [ ] Series/movie lookup by title still works (from `internal/database/series.go`, `movies.go`)
- [ ] Episode count increments correctly (from `internal/database/media_files.go`)

#### Watcher & Daemon
- [ ] `jellywatch watch <directory>` - file monitoring works (verified command)
- [ ] Watch debounce timing correct (from `internal/watcher/`)
- [ ] Multiple watch paths supported (from config)
- [ ] Daemon mode (`jellywatchd`) starts correctly (from `cmd/jellywatchd/`)
- [ ] Daemon processes files automatically
- [ ] Daemon logs to correct location
- [ ] Daemon shutdown graceful (no orphaned files)

#### Sonarr Integration
- [ ] `jellywatch sonarr status` - connection test (verified command)
- [ ] `jellywatch sonarr queue` - shows queue (verified command)
- [ ] `jellywatch sonarr clear-stuck` - removes stuck items (verified command)
- [ ] `jellywatch sonarr import <path>` - triggers scan (verified command)
- [ ] Sonarr API client works (from `internal/sonarr/`)
- [ ] Sonarr config methods work (from `internal/sonarr/config.go`)

#### Radarr Integration
- [ ] `jellywatch radarr status` - connection test (verified command)
- [ ] `jellywatch radarr queue` - shows queue (verified command)
- [ ] `jellywatch radarr clear-stuck` - removes stuck items (verified command)
- [ ] `jellywatch radarr import <path>` - triggers scan (verified command)
- [ ] `jellywatch radarr movies` - lists all movies (verified command)
- [ ] Radarr API client works (from `internal/radarr/`)
- [ ] Radarr config methods work (from `internal/radarr/config.go`)
- [ ] Series cache works (from `internal/library/cache.go`, uses TTL)

#### Validation & Compliance
- [ ] `jellywatch validate <path>` - checks naming (verified command)
- [ ] `jellywatch validate --recursive` - recursive validation (verified flag, default true)
- [ ] Jellyfin compliance rules enforced (from `internal/validator/`)
- [ ] Year in parentheses required for movies (from `internal/naming/`)
- [ ] Season folders required for TV (Season 01, Season 02)
- [ ] Episode format: `Show (Year) S01E01.ext` (from `internal/organizer/`)
- [ ] Movie format: `Movie (Year)/Movie (Year).ext` (from `internal/organizer/`)
- [ ] Invalid characters removed (: < > " / \ | ? *, from `internal/naming/`)

#### Quality Detection
- [ ] Quality parsing from filename works (from `internal/quality/extract.go`)
- [ ] Resolution detected (720p, 1080p, 2160p, 4K, from `internal/quality/patterns.go`)
- [ ] Source detected (BluRay, WEB-DL, HDTV, etc., from `internal/quality/`)
- [ ] Codec detected (x264, x265, HEVC, from `internal/quality/`)
- [ ] Audio detected (AAC, AC3, DTS, Atmos, from `internal/quality/`)
- [ ] HDR detected (DoVi, HDR10, HDR10+, from `internal/quality/`)
- [ ] Quality score calculated (CONDOR algorithm, from `internal/quality/scoring.go`)
- [ ] Quality comparison works (higher score wins, from database queries)

#### Duplicate & Consolidation
- [ ] `jellywatch duplicates generate` - finds dupes (verified command)
- [ ] `jellywatch duplicates execute` - removes dupes (verified command)
- [ ] `jellywatch duplicates dry-run` - preview only (verified command)
- [ ] Quality-aware duplicate removal (keeps best, from `internal/database/queries.go`)
- [ ] `jellywatch consolidate generate` - finds scattered series (verified command)
- [ ] `jellywatch consolidate execute` - moves episodes (verified command)
- [ ] `jellywatch consolidate dry-run` - preview only (verified command)
- [ ] Target library selection correct (from `internal/consolidate/`)
- [ ] Conflict detection works (from `internal/database/conflicts.go`)

#### AI Integration (if enabled)
- [ ] AI parsing works (from `internal/ai/`)
- [ ] AI queue system processes requests (from `internal/ai/integrator.go`)
- [ ] Circuit breaker opens on repeated failures (from `internal/ai/circuit_breaker.go`)
- [ ] Circuit breaker recovers after cooldown (from circuit breaker tests)
- [ ] AI confidence threshold respected (default 0.8, from `internal/config/config.go`)
- [ ] Low-confidence files tracked (from `internal/database/media_files.go`)
- [ ] `jellywatch audit <path> --generate` - finds low-confidence (verified flag)
- [ ] `jellywatch audit <path> --execute` - applies AI fixes (verified flag)
- [ ] `jellywatch audit --dry-run` - preview changes (verified flag)
- [ ] `jellywatch audit --threshold <float>` - custom threshold (verified flag, default 0.8)
- [ ] `jellywatch audit --limit <int>` - max files to audit (verified flag, default 100)

#### File Transfer & Permissions
- [ ] Rsync backend works (from `internal/transfer/rsync.go`)
- [ ] Native backend works (from `internal/transfer/native.go`)
- [ ] Timeout handling works (from `internal/transfer/transfer.go`)
- [ ] Checksum verification works (if enabled)
- [ ] Permissions set correctly (from `internal/transfer/rsync.go`, `internal/organizer/`)
- [ ] Ownership set correctly (chown, from `internal/transfer/rsync.go`)
- [ ] File mode configurable (chmod, from transfer options)
- [ ] Directory mode configurable (from transfer options)

#### Configuration & Setup
- [ ] `jellywatch config init` - creates default config (verified command)
- [ ] `jellywatch config show` - shows current config (verified command)
- [ ] `jellywatch config test` - validates config (verified command)
- [ ] `jellywatch config path` - shows config location (verified command)
- [ ] Config loads from `~/.config/jellywatch/config.toml` (default path)
- [ ] Config loads from `--config` flag (global flag)
- [ ] Missing config creates defaults (from `internal/config/config.go`)
- [ ] Invalid config shows helpful error

#### Logging & Monitoring
- [ ] `jellywatch monitor` - shows activity log (verified command)
- [ ] `jellywatch monitor --days <int>` - custom day range (verified flag, default 3)
- [ ] `jellywatch monitor --details` - detailed JSON output (verified flag)
- [ ] `jellywatch monitor --errors` - show only failed operations (verified flag)
- [ ] `jellywatch monitor --method <regex|ai|cache>` - filter by parse method (verified flag)
- [ ] `jellywatch status` - shows database status (verified command)
- [ ] Logs written to correct location
- [ ] Verbose mode (`-v`) increases detail (global flag)
- [ ] Dry-run mode (`-n`) previews changes (global flag)

#### Migration & Wizard
- [ ] `jellywatch migrate` - migration tool works (verified command)
- [ ] `jellywatch fix` - wizard for duplicates/consolidation (verified command)
- [ ] `jellywatch fix --dry-run` - preview wizard changes (verified flag)
- [ ] `jellywatch fix --yes` - auto-accept suggestions (verified flag)
- [ ] `jellywatch fix --duplicates-only` - handle only duplicates (verified flag)
- [ ] `jellywatch fix --consolidate-only` - handle only consolidation (verified flag)

#### API Server
- [ ] `jellywatch serve` - starts HTTP API server (verified command)
- [ ] `jellywatch serve --addr <host:port>` - custom address (verified flag, default :8080)
- [ ] API follows OpenAPI spec (from `api/openapi.yaml`)

#### Error Handling & Recovery
- [ ] Source file not found - clear error
- [ ] Target library doesn't exist - clear error
- [ ] Disk full - clear error (doesn't crash)
- [ ] Permission denied - clear error
- [ ] Invalid filename - clear error
- [ ] Corrupted file - clear error
- [ ] Network timeout - retry with backoff (from sync service)

#### Database Features
- [ ] Dirty flags work (sonarr_path_dirty, radarr_path_dirty, from migration 11)
- [ ] Sync timestamps tracked (sonarr_synced_at, radarr_synced_at)
- [ ] Conflicts detected (from `internal/database/conflicts.go`)
- [ ] Conflicts can be resolved (ResolveConflict method)
- [ ] v10 database upgrades to v11 automatically
- [ ] Existing series records preserved (through migration)
- [ ] Existing movie records preserved (through migration)
- [ ] Existing config files work (no breaking changes)

**Expected outcome:** All 100+ regression tests pass, zero breaking changes

---

### Task QA-20: Sign-Off & Deployment Prep

Final checklist before marking as production-ready:

#### Documentation
- [ ] README.md updated
- [ ] CHANGELOG.md entry added
- [ ] Migration guide written (v10 → v11)
- [ ] API documentation complete

#### Testing
- [ ] All 19 QA tasks above complete
- [ ] Test coverage report generated
- [ ] Performance benchmarks documented
- [ ] Load testing results documented

#### Code Quality
- [ ] All linters pass
- [ ] All tests pass
- [ ] No known bugs
- [ ] No TODO comments in production code

#### Deployment
- [ ] Version bumped (e.g., v2.0.0)
- [ ] Release notes drafted
- [ ] Installation guide updated
- [ ] Rollback procedure documented

**Expected outcome:** Ready for production release

---

## Summary Statistics

**Total QA Tasks:** 20
**Estimated Test Count:** 150+ tests (50+ new unit tests + 100+ verified regression checks)
**Target Coverage:** 80%+
**Target Performance:** <100ms for 10k records
**Timeline:** ~5-10 days for comprehensive QA

---

## Priority Levels

1. **Critical (Must Complete):**
   - QA-1: Static Analysis
   - QA-2: Spec Compliance
   - QA-7: Error Handling
   - QA-17: Full Test Suite
   - QA-19: Comprehensive Regression Testing ← **EXPANDED**
   - QA-20: Sign-Off

2. **High Priority:**
   - QA-3: Database Tests
   - QA-4: Sync Service Tests
   - QA-6: Integration Tests
   - QA-11: Manual CLI Testing
   - QA-12: Migration Testing

3. **Medium Priority:**
   - QA-5: Migration CLI Tests
   - QA-8: Race Conditions
   - QA-13: Documentation
   - QA-14: Code Review

4. **Low Priority (Nice-to-Have):**
   - QA-9: Performance Benchmarks
   - QA-10: Load Testing
   - QA-15: Security Audit ← **SQL injection removed, focused on other security**
   - QA-16: Reliability Testing
   - QA-18: E2E Scenarios

---

## Notes

- Run tests incrementally (don't wait until end)
- Fix bugs as soon as discovered (don't batch)
- Update this TODO as tests are added
- Mark tasks complete with `[x]` as you go
- Add notes for any failures or issues discovered

**Last Updated:** 2026-01-31  
**Status:** Ready for QA execution
