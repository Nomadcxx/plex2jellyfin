# DB Layer Ponytail Audit

## Findings

| Rank | File:Line | What | Why | Fix | Lines Saved |
|------|-----------|------|-----|-----|-------------|
| 1 | database.go:82-84 | `DB()` method | Duplicate of `SQL()` (line 87). Two methods returning same `*sql.DB`. Comment on `SQL()` says "Preferred over DB() for new code." | Delete `DB()`. Replace callers with `SQL()`. | 3 |
| 1 | database.go:78-80 | `migrate()` method | 1-line wrapper: `return applyMigrations(m.db)`. Zero value-add. | Delete. Call `applyMigrations(m.db)` directly in `OpenPath`. | 3 |
| 1 | housekeeping.go:235-247 | `containsAny()` | Hand-rolled substring search. `strings.Contains` exists in stdlib. | Replace with `strings.Contains(s, sub)` for each sub. | 13 |
| 1 | housekeeping.go:68-70 | `CountDuplicateManualReviewFailures()` | Exists only to report count before `CollapseDuplicateManualReviewFailures()`. Same complex SQL duplicated. | Merge: have `CollapseDuplicateManualReviewFailures` return `(canceled, total int64, error)`. | 25 |
| 1 | parse_decisions.go:489-506 | `AuditParseDecisionsPage()` | No-op pass. Comment admits: "The underlying audit logic lives in cmd/jellywatch (CLI) and labeling/runner.go. This page-iterator simply scans rows so the operator sees granular progress." Dead code pretending to be useful. | Delete. | 18 |
| 1 | stats.go:38-41 | `Stats` struct + `GetStats()` | Marked "legacy, kept for compatibility." `LibraryStats` supersedes it. | Delete both if no callers remain. | 15 |
| 1 | stats.go:68-93 | `GetLibraryStats()` duplicate queries | Queries `movie_duplicates` and `episode_duplicates` tables that do not exist in schema.go. Error handlers default to 0 — dead code paths that always take the error branch. | Delete the dead table queries. Use `FindDuplicateMovies`/`FindDuplicateEpisodes` results already computed or compute from `media_files`. | 25 |
| 1 | compliance.go:18-26 | `IssueType` constants | 8 consts defined (`IssueInvalidFilename`, etc.) but never referenced by name. All code uses raw string literals like `"invalid_filename"`. | Either use the consts in CheckMovie/CheckEpisode (replace raw strings) or delete the const block. | 8 |
| 1 | compliance.go:29-31 | `Checker.libraryRoot` field | Stored in struct, passed to `NewChecker`, never read by any method (`CheckMovie`, `CheckEpisode`, `CheckFile`). Dead field. | Delete field from struct and `NewChecker` parameter. | 3 |
| 2 | normalize.go:44-57 | `NormalizeForMatch()` | 95% identical to `NormalizeTitle()`. Only differences: adds `\u2019` (right single quote) to replacer, does NOT strip year suffix. | Merge: add `\u2019` to `NormalizeTitle`'s replacer, add a `keepYear bool` param or have callers strip year before calling. | 14 |
| 2 | normalize.go:78-97 | `ExtractYearFlexible()` | Calls `ExtractYear()` first, then does bare-year fallback. Two functions for one concept. | Merge bare-year logic into `ExtractYear()`. Delete `ExtractYearFlexible`. | 20 |
| 2 | maintenance.go:57-68 | `listUserTables()` double filter | SQL already has `WHERE name NOT LIKE 'sqlite_%'`. Go loop also checks `strings.HasPrefix(n, "sqlite_")`. Redundant. | Remove the Go-level prefix check. | 4 |
| 3 | housekeeping.go:361-363 | `hkScanner` interface | Single-implementation interface abstracting `*sql.Row` and `*sql.Rows`. Both already have `Scan`. Exists only so `scanHousekeepingTask` accepts both. | Delete interface. Make `scanHousekeepingTask` take `...interface{}` from the caller's row.Scan, or write two tiny wrappers. | 3 |
| 3 | parse_decisions.go:436-438 | `scanner` interface | Same pattern as `hkScanner` — single-implementation interface for `*sql.Row`/`*sql.Rows`. | Delete. Same fix as above. | 3 |
| 3 | conflicts.go:37-39 | `getUnresolvedConflictsLocked()` | Private "locked" variant called from exactly 2 places (`GetUnresolvedConflicts` and `DetectConflicts`). Adds a convention (lock-held variants) without reducing duplication. | Inline into the two callers, or just have `GetUnresolvedConflicts` be the only version and `DetectConflicts` call it after acquiring write lock. | 3 |
| 3 | compliance_helpers.go:14-44 | `CheckCompliance`, `CheckMovieCompliance`, `CheckEpisodeCompliance` | Three thin wrappers. Each creates `compliance.NewChecker(libraryRoot)` and calls one method. Zero logic. | Delete file. Callers use `compliance.NewChecker(root).CheckFile(path)` etc. directly. | 30 |
| 3 | scheduled_jobs.go:95-97 | `scanScheduledJob` reuses `hkScanner` | Cross-file coupling to an interface in housekeeping.go. If `hkScanner` is deleted, this breaks. | Inline the scan or use `*sql.Row`/`*sql.Rows` directly. | 0 (refactor) |
| 4 | conflicts.go:88-130 | `detectSeriesConflicts()` + `detectMovieConflicts()` | Copy-paste with `series`/`movies` and `"series"`/`"movie"` swapped. 43 lines each. | One function parameterized by table name and media type string. | 40 |
| 4 | housekeeping.go:289-355 | `BulkRetry`, `BulkCancel`, `BulkApprove` | Three methods with identical tx+prepare+loop+count pattern. Differ only in the UPDATE SQL and status filter. | One `bulkUpdateHousekeepingTasks(ids, query string)` helper. | 40 |
| 4 | migration.go:78-155 | `FixSeriesMismatch()` + `FixMovieMismatch()` | Copy-paste with series/movie, Sonarr/Radarr, `GetSeriesByID`/`GetMovieByID`, `UpsertSeries`/`UpsertMovie` swapped. | One function parameterized by media type and client interface. | 40 |
| 4 | migration.go:23-76 | `DetectSeriesMismatches()` + `DetectMovieMismatches()` | Copy-paste with series/movie, Sonarr/Radarr swapped. | One function parameterized by type and client. | 30 |
| 4 | parse_decisions.go:330-370 | `QueryRecentSuccessfulMovieImports()` + `QueryRecentSuccessfulTVImports()` | Copy-paste with `'movie'`/`'tv'` swapped. | One function with `mediaType` param. | 20 |
| 4 | parse_decisions.go:296-328 | `GetUnresolvedDecisionByTargetPath()` + `GetDecisionByTargetPath()` | Differ by one WHERE clause (`jellyfin_resolved_at IS NULL`). | One function with `unresolvedOnly bool` param. | 15 |
| 4 | media_files.go:107-170 | `GetMediaFile()` + `GetMediaFileByID()` | Copy-paste with `WHERE path = ?` vs `WHERE id = ?`. 60+ lines each. | Shared `scanMediaFile(row)` helper, two 5-line query methods. | 50 |
| 4 | media_files.go:172-240 | `UpdateMediaFile()` | Duplicates `UpsertMediaFile`'s UPDATE SET clause (25 columns). | `UpdateMediaFile` calls `UpsertMediaFile` (which has `ON CONFLICT(path) DO UPDATE`). If path unchanged, it's an update. Delete `UpdateMediaFile`. | 68 |
| 4 | queries.go:30-78 | `FindDuplicateMovies()` + `FindDuplicateEpisodes()` | Copy-paste with `'movie'`/`'episode'` and season/episode NULL handling swapped. | One parameterized function. | 25 |
| 4 | series_repair.go:218-228 | `nullInt64Interface()` + `nullStringInterface()` | Duplicate of the null-handling pattern from parse_decisions.go (`nullIntPtr`/`nullStr`) but for `sql.Null*` types instead of `*int`/`string`. | Use `nullInt64ToInterface(n sql.NullInt64) interface{}` consistently, or just inline the 3-line conversion at the single call site. | 6 |
| 5 | ai_improvements.go:189-230 | `GetAIImprovementsByModel()` | Check callers. If unused, YAGNI. | Delete if no callers. | 42 |
| 5 | ai_improvements.go:233-240 | `CountAIImprovementsByStatus()` | Check callers. If unused, YAGNI. | Delete if no callers. | 8 |
| 5 | ai_improvements.go:163-168 | `DeleteAIImprovement()` | Check callers. If unused, YAGNI. | Delete if no callers. | 6 |
| 5 | media_files.go:438-480 | `AICoverageStats` + `GetAICoverageStats()` | Check callers. If unused, YAGNI. | Delete if no callers. | 43 |
| 5 | media_files.go:482-530 | `GetFilesNeverAIParsed()` | Check callers. If unused, YAGNI. | Delete if no callers. | 49 |
| 5 | media_files.go:242-248 | `DeleteMediaFileByID()` | Check callers. If all deletes use path, this is dead. | Delete if no callers. | 7 |
| 5 | schema.go:189-192 | Migration v9 | No-op migration: "Migration removed - plans are now stored in JSON files." Exists only to bump version number. | Delete the migration entry. Renumber subsequent migrations (or leave gap — `applyMigrations` only cares about version > currentVersion). | 4 |

## Needs Deeper Research

These findings require cross-package caller analysis to confirm:

1. **`AIImprovement` entire subsystem** (`ai_improvements.go`): The struct has 18 fields. Methods: `UpsertAIImprovement`, `GetAIImprovement`, `GetPendingAIImprovements`, `UpdateAIImprovementStatus`, `DeleteAIImprovement`, `GetAIImprovementsByModel`, `CountAIImprovementsByStatus`. Check which methods have callers outside `internal/database`. Unused methods = dead code. The whole table may be vestigial if the AI enhancement pipeline was redesigned.

2. **`NormalizeTitleFromFilename`** (`compliance_helpers.go:46-68`): Wraps `naming.IsTVEpisodeFilename`, `naming.ParseTVShowName`, `naming.IsMovieFilename`, `naming.ParseMovieName`. Check if callers could use the `naming` package directly. If this is the only caller of those naming functions, the dependency direction may be inverted.

3. **`GetLibraryStats` dead table queries** (`stats.go:68-93`): Queries `movie_duplicates` and `episode_duplicates` tables. These tables are not created in `schema.go` migrations. Confirm they don't exist in any migration. If confirmed, the error-handling fallback-to-zero branches are unreachable dead code.

4. **`Stats` / `GetStats` legacy** (`stats.go:38-55`): Marked "legacy, kept for compatibility." Search for callers of `GetStats()` across all packages. If zero callers, delete both struct and method.

5. **`Checker.libraryRoot` dead field** (`compliance/compliance.go:29-31`): Field is stored but never read. Before deleting, verify no external package accesses it via reflection or struct field access (unlikely but check).

6. **`hkScanner` / `scanner` interface deletion impact**: Both `housekeeping.go` and `parse_decisions.go` define a `scanner` interface. `scheduled_jobs.go` imports `hkScanner` from housekeeping.go. Deleting these interfaces requires touching 3 files. The `scanHousekeepingTask`, `scanDecision`, and `scanScheduledJob` functions all use the pattern. Refactor to accept `*sql.Row` and `*sql.Rows` directly (two overloads each) or use a common pattern.

7. **`DB()` vs `SQL()` callers**: Before deleting `DB()`, grep all packages for `.DB()` calls on `*MediaDB`. Replace with `.SQL()`.

8. **`UpdateMediaFile` vs `UpsertMediaFile`**: `UpdateMediaFile` does a full UPDATE of all 25 columns by ID. `UpsertMediaFile` does INSERT OR UPDATE by path. If `UpdateMediaFile` callers always pass the same path, `UpsertMediaFile` already handles it via `ON CONFLICT(path) DO UPDATE`. Verify caller behavior before deleting.

9. **`DeleteMediaFileByID` callers**: Check if any code deletes by ID rather than path. If all deletions use path, this method is dead.

10. **Migration v9 no-op**: Removing it means renumbering v10→v22 as v9→v21, or leaving a version gap. `applyMigrations` iterates `migrations` slice in order and skips `version <= currentVersion`, so a gap is harmless. But verify no code references specific version numbers.

11. **`IssueType` constants usage**: Search all packages for references to `compliance.IssueInvalidFilename` etc. If zero, the consts are dead. The raw strings are used inline in `CheckMovie`/`CheckEpisode`.

12. **`media_files.go:getLowConfidenceFiles` NeedsReview scan bug**: At line 432, `rows.Scan` reads `&file.NeedsReview` (a `bool`) directly instead of using an `int` intermediate like every other scan in the file. This is a correctness bug (SQLite stores 0/1, Go `Scan` into `bool` may fail). Route to normal review, not over-engineering.

**Total estimated savings: ~650 lines deletable, ~250 lines shrinkable through deduplication. Net: ~900 lines, 0 dependency changes.**
