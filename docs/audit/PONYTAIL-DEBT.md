# Plex2Jellyfin Ponytail Debt Ledger

Generated 2026-06-24 from 9-domain ponytail audit. 280 findings across ~94K lines.

---

## HIGH IMPACT — Delete or simplify immediately

These are large line-count savings, dead code, or structural simplifications with clear fixes.

### Dead Code (delete now, zero risk)

| # | File | What | Lines | Fix |
|---|------|------|-------|-----|
| H1 | `internal/daemon/daemon.go` | Entire file: `Daemon` struct, `NewDaemon`, `Start`, `Stop`, `Run` — zero callers | ~80 | Delete file |
| H2 | `cmd/installer/animations.go` | `BeamsTextEffect` — 771-line ASCII animation engine for one-time installer header | ~771 | Delete file, use static `asciiHeader` from theme.go |
| H3 | `cmd/classification-audit/main.go` | Standalone binary with hardcoded paths, YAGNI | ~219 | Delete binary |
| H4 | `cmd/test-hint/main.go` | Standalone binary with hardcoded test path, YAGNI | ~44 | Delete binary |
| H5 | `internal/daemon/handler.go:1297` | `removeNonVideoContents` — defined, never called | ~38 | Delete method |
| H6 | `internal/organizer/organizer.go:197` | `Pause()`/`Resume()`/`IsPaused()` + `paused` field + `pauseMu` — never checked | ~20 | Delete all 3 methods + fields |
| H7 | `internal/organizer/organizer.go:235` | `shouldWaitForScan()`/`waitIfScanning()` — never called | ~25 | Delete both |
| H8 | `internal/transfer/transfer.go:24,27,89` | `ErrChecksumMismatch`, `ErrTransferFailed`, `TransferResult.Checksum`, `TransferOptions.Checksum` — never used | ~10 | Delete sentinels + fields |
| H9 | `internal/transfer/transfer.go:148` | `MustNew` — deprecated per own doc comment | ~7 | Delete |
| H10 | `internal/api/dashboard.go:12` | `DashboardData` local struct — never used, shadowed by generated type | ~40 | Delete struct + dependent types |
| H11 | `cmd/installer/utils.go:55` | `detectPackageManager()` — zero callers | ~15 | Delete |
| H12 | `cmd/plex2jellyfin-daemon/streaming_ops.go:340` | `notImplemented` function + unused `errors.New` import | ~10 | Delete both |
| H13 | `internal/daemon/classify.go:19` | `ChangeStructural` constant — "reserved for future use" | 2 | Delete |
| H14 | `internal/daemon/ratelimit.go:44,59` | `Record()` and `Caps()` — zero callers | 8 | Delete both |
| H15 | `internal/daemon/ipc/frame_ring.go:12` | `cursor` field — never read or written | 1 | Delete field |
| H16 | `internal/jellyfin/deferred_queue.go:16` | `DeferredOp.RetryCount` — never incremented or read | 1 | Delete field |
| H17 | `internal/jellyfin/remediate.go:12` | `RemediationResult.SeriesID` — never set | 1 | Delete field |
| H18 | `internal/ai/queue.go:48` | `outputChan` + `Output()` — nothing reads from it | ~10 | Delete channel + method |
| H19 | `internal/ai/integrator.go:52` | `shutdownCh` — duplicates `bgCtx`/`bgCancel` for same purpose | ~5 | Delete channel, use ctx only |
| H20 | `internal/ai/cache.go:170` | `NormalizeInput` — no-op identity function | 3 | Delete, inline identity |
| H21 | `internal/analyzer/analyzer.go:199` | `GetCleanupFilesPreserveExtras` — identical body to `GetCleanupFiles` | 7 | Delete duplicate |
| H22 | `internal/library/selector.go:27` | `CacheDuration` field — marked "deprecated - use DB" | 2 | Remove field |
| H23 | `internal/library/scanner_helper.go` | Single 15-line function in its own file | 15 | Inline into scanner.go, delete file |
| H24 | `internal/llm/types.go:7` | 4 speculative provider type constants (OpenAI, Anthropic, LMStudio, Custom) — only Ollama implemented | 4 | Delete unused constants |
| H25 | `internal/database/compliance.go:18` | `IssueType` constants — never referenced by name, all code uses raw strings | 8 | Delete const block |
| H26 | `internal/database/compliance.go:29` | `Checker.libraryRoot` — stored, never read | 3 | Delete field + param |
| H27 | `internal/database/database.go:82` | `DB()` — duplicate of `SQL()` | 3 | Delete, replace callers with `SQL()` |
| H28 | `internal/database/database.go:78` | `migrate()` — 1-line wrapper | 3 | Delete, call `applyMigrations` directly |
| H29 | `internal/database/parse_decisions.go:489` | `AuditParseDecisionsPage()` — no-op pass, comment admits it | 18 | Delete |
| H30 | `internal/database/stats.go:38` | `Stats` + `GetStats()` — marked "legacy, kept for compatibility" | 15 | Delete if no callers |
| H31 | `internal/database/stats.go:68` | `GetLibraryStats()` queries `movie_duplicates`/`episode_duplicates` tables that don't exist | 25 | Delete dead queries |
| H32 | `internal/database/compliance_helpers.go` | 3 thin wrappers (`CheckCompliance`, `CheckMovieCompliance`, `CheckEpisodeCompliance`) | 30 | Delete file, callers use `compliance.NewChecker` directly |
| H33 | `internal/database/housekeeping.go:68` | `CountDuplicateManualReviewFailures()` — exists only to report count before collapse | 25 | Merge into `CollapseDuplicateManualReviewFailures` |
| H34 | `internal/database/housekeeping.go:235` | `containsAny()` — hand-rolled `strings.Contains` | 13 | Replace with stdlib |
| H35 | `internal/database/schema.go:189` | Migration v9 — no-op, exists only to bump version | 4 | Delete entry |
| H36 | `internal/config/config.go:310` | `DefaultAIConfig()` — duplicates AI defaults with different values, unused | ~30 | Delete |
| H37 | `cmd/installer/theme.go:24` | `asciiHeaderLines` — duplicate of `asciiHeader` const, only used by beams animation | ~4 | Delete (goes with H2) |
| H38 | `internal/daemon/enhancelog.go:47` | `maxSize`, `maxBackups` struct fields — hardcoded, never configurable | 2 | Make package-level consts |
| H39 | `internal/daemon/handler.go:86` | `pendingAICap` field — hardcoded to 100, never changes | 1 | Make const |
| H40 | `internal/daemon/handler.go:80` | `transientRetryDelay`, `seasonPackEventSuppressWindow` — never mutated | 0 | Document as effectively-const |

### Structural Duplication (merge, save hundreds of lines)

| # | Files | What | Lines | Fix |
|---|-------|------|-------|-----|
| H41 | `consolidate/{consolidate,executor,planner,operations}.go` | Two parallel consolidation systems: `Consolidator`+`Plan`+`Operation` vs `Executor`+`Planner`+`ConsolidationPlan` | ~500 | Delete `Consolidator` system, keep `Executor`+`Planner` |
| H42 | `internal/organizer/organizer.go:230,372` | `OrganizeMovie`/`OrganizeMovieWithParsed` and `OrganizeTVEpisode`/`OrganizeTVWithParsed` — ~90% identical pairs | ~300 | Merge into single methods taking pre-parsed info |
| H43 | `internal/config/config.go:340` | `ToTOML()` — 200-line hand-rolled TOML serializer. Viper already has `WriteConfig()` | ~180 | Replace with Viper or BurntSushi/toml |
| H44 | `internal/logging/rotate.go` | Custom log rotation (198 lines) — reinvents lumberjack | ~190 | Replace with `lumberjack` (1 dep, 3 lines) or keep if zero-dep is hard requirement |
| H45 | `internal/plans/plans.go` | Save/Load/Delete/Archive repeated 3× for consolidate, duplicate, audit plans | ~150 | Single generic `SavePlan[T]`/`LoadPlan[T]`/`ArchivePlan` |
| H46 | `internal/daemon/handler.go:1700` | `organizeWithRegexFallback` duplicates ~90 lines from `processFile` verbatim | ~90 | Call `processFile` directly |
| H47 | `internal/housekeeping/engine.go:950` | `execParserDriftRename` and `execParserDriftTVRename` share ~80% structure | ~100 | Extract shared `execDriftRename` core |
| H48 | `cmd/plex2jellyfin/main.go:400` | Sonarr/Radarr command trees — ~150 lines of near-identical code | ~80 | Single `newArrCmd(name, clientFactory)` |
| H49 | `cmd/installer/inputs.go` | 6 nearly identical `init*Inputs`/`save*Inputs` function pairs — same textinput styling repeated ~30 times | ~200 | Extract `newStyledInput()` helper |
| H50 | `cmd/installer/validation.go:55` | `testSonarrCmd`/`testRadarrCmd`/`testJellyfinCmd` — 3 identical HTTP test functions | ~60 | Single `testServiceCmd()` |
| H51 | `internal/api/test_handlers.go:45` | Sonarr/Radarr/Jellyfin test handlers — same copy-paste as installer | ~30 | Single generic test handler |
| H52 | `internal/api/ai_custom.go:130` + `cmd/installer/validation.go:230` | `TestAIPrompt` and `testOllamaPromptCmd` duplicate Ollama prompt test logic | ~40 | Extract shared logic to `internal/ai/` |
| H53 | `internal/config/sections.go:70` | 11 identical `set*` functions — same 4-line pattern repeated | ~55 | Single generic `setSection[T]` |
| H54 | `internal/database/conflicts.go:88` | `detectSeriesConflicts()` + `detectMovieConflicts()` — copy-paste with series/movie swapped | 40 | One parameterized function |
| H55 | `internal/database/housekeeping.go:289` | `BulkRetry`/`BulkCancel`/`BulkApprove` — identical tx+prepare+loop pattern | 40 | One `bulkUpdateHousekeepingTasks` helper |
| H56 | `internal/database/migration.go:78,23` | `FixSeriesMismatch`/`FixMovieMismatch` + `DetectSeriesMismatches`/`DetectMovieMismatches` — copy-paste pairs | 70 | Parameterize by media type |
| H57 | `internal/database/media_files.go:107,172` | `GetMediaFile()` + `GetMediaFileByID()` — copy-paste with WHERE clause swapped | 50 | Shared `scanMediaFile(row)` helper |
| H58 | `internal/database/media_files.go:172` | `UpdateMediaFile()` duplicates `UpsertMediaFile`'s 25-column UPDATE | 68 | Delete `UpdateMediaFile`, use `UpsertMediaFile` (ON CONFLICT DO UPDATE) |
| H59 | `internal/database/parse_decisions.go:296,330` | `GetUnresolvedDecisionByTargetPath`/`GetDecisionByTargetPath` + `QueryRecentSuccessfulMovieImports`/`QueryRecentSuccessfulTVImports` | 35 | Parameterize by `unresolvedOnly` and `mediaType` |
| H60 | `internal/database/queries.go:30` | `FindDuplicateMovies()` + `FindDuplicateEpisodes()` — copy-paste | 25 | One parameterized function |
| H61 | `internal/radarr/client.go` | Context-wrapper pattern: every method has non-Context variant passing `context.Background()` | ~120 | Delete all non-Context wrappers |
| H62 | `internal/jellyfin/plugin.go:17` | `PluginClient` duplicates ~80% of `Client` struct, auth, get/post | ~50 | Embed `*Client`, override only `request` |
| H63 | `internal/jellyfin/client.go:56,88` | `request`→`requestWithAuth`→`requestCtx` 3-layer chain + `get`/`getCtx` duplicate | ~25 | Delete wrappers, callers pass ctx directly |
| H64 | `internal/ai/status.go` | `AIStatus` + `AIStatusSnapshot` — field-identical structs, snapshot exists only for RLock copy | ~30 | Return value copy of `AIStatus`, delete `AIStatusSnapshot` |
| H65 | `internal/llm/interface.go` + `internal/mediamanager/interface.go` | Duplicated registry pattern — identical `map[string]T` with Register/Get/All/AllInfo | ~50 | Extract one generic registry or use map directly |
| H66 | `internal/ai/matcher.go:340` | `getSystemPrompt` — 100-line string literal embedded in Go source | ~100 | Move to embedded file (`//go:embed prompt.txt`) |
| H67 | `internal/ai/matcher.go` | `ParseWithRetry`/`ParseWithCloud`/`ParseWithContext` — 4 parse variants, retry/cloud are composition concerns | ~30 | Keep `Parse`+`ParseWithContext`, fold retry into `parseWithModel` |
| H68 | `internal/naming/advanced.go:1` + `internal/naming/naming.go:32` | Two `init()` blocks compiling ~80% identical regex lists | ~150 | Merge into one `releasePatterns` list |
| H69 | `internal/naming/advanced.go:202,622` | `StripReleaseGroup` (150 lines) + `CleanMovieName` (80 lines) — duplicate `stripReleaseMarkers` pipeline | ~180 | Merge into `stripReleaseMarkers`, keep abbreviation preservation if proven needed |
| H70 | `internal/naming/advanced.go:352,382,392` | `ExtractYearAdvanced` + `removeYearAdvanced` + `removeSpecificYear` — duplicate `extractYear`/`removeYear` | ~51 | Delete, use naming.go versions |
| H71 | `internal/naming/advanced.go:422,522` | `stripOrphanedReleaseGroups` (100 lines) + `IsGarbageTitle` (100 lines) — fragile heuristic chains | ~130 | Simplify to known-group + known-title checks |
| H72 | `internal/naming/advanced.go:702` | `titleCaseWithOrdinals` — 80 lines with placeholder dance for ordinals/abbreviations/acronyms | ~50 | Use `cases.Title` directly, simple post-processing regex for ordinals |
| H73 | `internal/naming/confidence.go:12,52,118` | `CalculateTitleConfidence` (40 lines) + `shouldApplyGarbageTitlePenalty` (18 lines) + `hasDuplicateYear` (50 lines) — over-engineered scoring | ~83 | Simplify to binary trustworthy/untrustworthy, simpler year-duplicate check |
| H74 | `internal/quality/scoring.go:112,141` | `ResolutionToString`/`SourceToString`/`AudioToString` (40 lines) + `contains`/`stringContains`/`indexOfSubstring`/`toLower` (30 lines) — duplicate type methods + reinvented stdlib | ~68 | Use type `String()` methods, use `strings.Contains`/`strings.ToLower` |
| H75 | `internal/quality/patterns.go:147` + `internal/quality/extract.go:17` | `ParseFromPath` duplicates parent-dir walking from `ExtractMetadata` | ~20 | Delete `ParseFromPath`, consolidate into `ExtractMetadata` |
| H76 | `internal/quality/quality.go:202,262` | `QualityInfo.String()` and `Source.String()` duplicate Source switch | ~15 | `String()` calls `q.Source.String()` |
| H77 | `internal/naming/naming.go:252,317` | `NormalizeMovieName`/`NormalizeTVShowName` identical + `collectStrippedTokens` duplicates `stripReleaseMarkers` phases | ~28 | Merge normalize functions, refactor token collection |
| H78 | `internal/naming/deobfuscate.go:172,212,332` | `ParseTVShowFromPath`/`ParseMovieFromPath`/`IsMovieFromPath` — thin wrappers discarding return values | ~11 | Delete wrappers |
| H79 | `internal/naming/jellyfin_naming.go:52` + `internal/quality/patterns.go:172` | `getExtension`/`getExt` — hand-rolled `filepath.Ext` in two places | ~16 | Use `strings.TrimPrefix(filepath.Ext(path), ".")` |
| H80 | `internal/naming/blacklist.go` | `KnownReleaseGroups` — 10,992-entry map (105KB) from srrDB. 80-entry map covers 95%+ of real cases | ~105,000 | Measure accuracy, delete if 80-entry list suffices |
| H81 | `internal/naming/advanced.go:852` | `SimilarityRatio` + `levenshteinDistance` + `minThree` — custom Levenshtein, `minThree` could be `min(min(a,b),c)` | ~5 | Use built-in `min`, consider simpler Jaccard if only for fuzzy dedup |
| H82 | `internal/naming/confidence.go:202` | `clamp` — 8 lines, `math.Max(math.Min(value, max), min)` does it in one | ~7 | Delete, use stdlib |
| H83 | `internal/naming/confidence.go:87` | `isTechnicalTitleToken` — digit+letter heuristic could false-positive on "Se7en", "8MM" | ~8 | Drop digit+letter heuristic |
| H84 | `internal/naming/deobfuscate.go:17,77,87` | `IsObfuscatedFilename` fragile combo check + `isRandomAlphaToken` narrow heuristic + `isRandomAlphanumeric` arbitrary thresholds | ~33 | Simplify entropy checks, drop narrow heuristics |
| H85 | `internal/naming/deobfuscate.go:277` | `isGenericMovieContainerFolder` — 15-line switch, could be `map[string]bool` | ~8 | Use map lookup |
| H86 | `internal/naming/naming.go:502,522` | `extractEpisodeInfo` (check callers) + `normalizeSpaces` recompiles regex per call | ~10 | Delete if unused, move regex to package-level var |
| H87 | `internal/naming/naming.go:297` | `knownReleaseGroups` local 80-entry map — duplicates concept from `blacklist.go` | ~5 | Consolidate with `KnownReleaseGroups` |
| H88 | `internal/naming/types.go` + `internal/naming/naming.go:47` | `ErrParseFailed` + `MovieInfo` + `TVShowInfo` — fine, keep | 0 | Keep |

### Cross-File Duplication (shared utility needed)

| # | Files | What | Lines | Fix |
|---|-------|------|-------|-----|
| H89 | 7 files across organizer, scanner, watcher, cleanup, dedup, pv | Video extension map duplicated 7× with slight variations | ~60 | One `var VideoExtensions` in shared package |
| H90 | `cmd/plex2jellyfin/main.go`, `cmd/plex2jellyfin-daemon/main.go`, `cmd/plex2jellyfin/cleanup.go`, `internal/jellyfin/verify.go`, `internal/consolidate/consolidate.go` | `isMediaFile` duplicated 4-5× with different signatures | ~40 | Single shared function |
| H91 | `cmd/plex2jellyfin/audit_validation.go`, `cmd/plex2jellyfin/scan_progress.go`, `cmd/plex2jellyfin/audit_cmd.go` | 3 separate `ProgressBar` implementations | ~100 | One `ProgressBar` in `internal/progress/` |
| H92 | `internal/consolidate/operations.go`, `internal/plans/plans.go`, `internal/housekeeping/engine.go` | `executeDelete` triplicated — 3 delete implementations with slight variations | ~60 | Single `DeleteMediaFile(db, path, dryRun)` |
| H93 | `internal/housekeeping/engine.go`, `internal/consolidate/consolidate.go`, `internal/consolidate/safety.go` | `isAllDigits`/`allDigits`/`isAllDigitsOrPunct` + `isEmptyDir`/`CleanupEmptyDir` + `isSeasonDirectoryName`/`hasSeasonComponent` — scattered utilities | ~26 | Consolidate into shared location |
| H94 | `internal/wizard/wizard.go`, `internal/llm/ollama_adapter.go`, `internal/consolidate/operations.go` | `formatBytes` duplicated 3× | ~10 | Extract to `internal/format` |
| H95 | `internal/analyzer/analyzer.go`, `internal/library/scanner.go`, `internal/service/analysis.go` | `videoExtensions` map duplicated 3× | ~8 | Share one definition |
| H96 | `internal/ai/recovery.go`, `internal/library/selector.go` | Custom `min()` in 2 files — Go 1.26 has built-in `min`/`max` | 8 | Delete both, use built-in |
| H97 | `internal/sonarr/url.go`, `internal/radarr/url.go` | `joinURL` identical in two packages | ~20 | Move to shared `internal/arrutil` or inline |
| H98 | `internal/transfer/pv.go`, `internal/transfer/rsync.go` | `scanPVOutput` and `scanRsyncOutput` — identical `bufio.SplitFunc` | ~12 | One shared `scanLineOutput` |
| H99 | `internal/transfer/health.go`, `internal/transfer/native.go` | `StatWithTimeout`/`OpenWithTimeout`/`RemoveWithTimeout` — same goroutine+channel pattern 3× | ~30 | One generic `withTimeout[T]` helper |
| H100 | `internal/mediamanager/radarr_adapter.go`, `internal/mediamanager/sonarr_adapter.go` | `SonarrAdapter` (234 lines) vs `RadarrAdapter` (210 lines) — ~90% structural copy | ~100 | Single generic adapter or accept duplication for different APIs |
| H101 | `internal/postmortem/suspicious.go:73` | 24 hardcoded storage path translations — environment-specific config in code | ~25 | Move to config file or derive from library paths |
| H102 | `internal/postmortem/collector.go:131,247,268` | `parseDecisionEvidence` mirrors DB struct + `mediaInventory` O(n) scan + `configSnapshot` hand-picks fields | ~45 | Add json tags to DB struct, use SQL GROUP BY, serialize full config |
| H103 | `internal/library/selector.go:370` | `findRelatedContent` + `countMediaItems` — O(n) filesystem walk when DB has the data | ~50 | Query `media_files` table |
| H104 | `internal/service/analysis.go:72` | Double-pruning of missing files — `PruneMissingMediaFiles()` called twice | ~30 | Call once at start |
| H105 | `internal/ai/integrator.go:258` | `updateImprovementStatus` + `saveImprovementResult` — two separate DB writes to same table | ~15 | Merge into single upsert |
| H106 | `internal/notify/notify.go` | `Manager` async mode — `Results()` channel exposed but no caller drains it | ~20 | Remove async mode or add consumer |
| H107 | `internal/validator/validator.go` | `Validator` with functional options pattern for a single `allowMissingYear` bool | ~10 | Pass as constructor param directly |
| H108 | `internal/permissions/permissions.go` | `CanDelete` checks both dir AND file writability — on Linux only dir write matters | ~5 | Remove file writability check |
| H109 | `internal/wizard/consolidate.go:56` + `internal/wizard/duplicates.go:82` | TODO stubs — consolidation execution and swap not implemented | ~2 | Remove from wizard until ready |
| H110 | `internal/mediamanager/radarr_adapter.go:183` + `internal/mediamanager/sonarr_adapter.go:183` | `GetQualityProfiles`/`GetHistory` return `nil, nil` — dead stubs | 0 | Return `fmt.Errorf("not implemented")` or remove from interface |

### Pipeline Simplification

| # | Files | What | Lines | Fix |
|---|-------|------|-------|-----|
| H111 | `internal/scanner/scanner.go:175,99` | `scanPathWithProgress` duplicates `scanPathWithLibraryRoot` + `ScanLibraries` is subset of `ScanWithOptions` | ~55 | Merge with optional progress callback, delete `ScanLibraries` |
| H112 | `internal/daemon/handler.go:730` | `processFile` TV/Movie branches share ~80% structure | ~50 | Extract `processMediaFile(path, mediaType, parser, organizer)` |
| H113 | `internal/daemon/handler.go:1476` | `HandleJellyfinWebhookEvent` — `EventItemAdded`/`EventItemUpdated` cases ~90% identical | ~20 | Extract shared logic |
| H114 | `internal/daemon/handler.go:130` | `Stats` uses `sync.RWMutex` for 4 int64 counters — `sync/atomic.Int64` simpler, faster | ~15 | Replace with atomic |
| H115 | `internal/daemon/handler.go:148` | `StatsSnapshot` type — only used as return from `Snapshot()`, destructured immediately | 7 | Inline into `MetricsResponse` |
| H116 | `internal/daemon/handler.go:239,268,1380,252` | `allWatchPaths()`, `isInsideLibrary()`, `cleanupSourceDir()`, `getSourceHint()` — allocate/recompute on every hot-path call | ~17 | Pre-compute in constructor |
| H117 | `internal/daemon/handler.go:101,98` | `loggedErrors` and `transientWarned` maps grow unbounded — memory leak | 0 | Add LRU eviction or TTL cleanup |
| H118 | `internal/daemon/handler.go:1206` | `sendNotificationsWithTracking` — misleading name, doesn't track actual notifier results | ~4 | Rename or actually check notifier config |
| H119 | `internal/daemon/handler.go:1590` | `replayDeferredOperation` checks `h.logger != nil` twice | 1 | Remove second check |
| H120 | `internal/daemon/handler.go:68` | `aiRetryBackoff` allocates new slice every call | 3 | Make package-level `var` array |
| H121 | `internal/daemon/ipc/op_registry.go:107` | `List()` uses O(n²) bubble sort — `sort.Slice` is stdlib | 4 | Use `sort.Slice` |
| H122 | `internal/daemon/ratelimit.go:12` | `AIRateLimiter` stores mutable reset deadlines — compute arithmetically | ~4 | Use `now.Truncate(time.Hour).Add(time.Hour)` |
| H123 | `internal/daemon/reload/ai_reloadable.go:8` + `internal/daemon/reload/scanner_reloadable.go:8` | `AIReconfigurer` + `WatchPathReplacer` — interfaces with one implementation each | 6 | Take `func(...) error` directly |
| H124 | `internal/scanner/scanner.go:437` | `parseInt(s)` wraps `fmt.Sscanf` — `strconv.Atoi` is stdlib | ~5 | Replace with `strconv.Atoi` |
| H125 | `internal/scanner/ai_helper.go:48` | `tryParse` takes closure param — only 2 call sites, each wrapping single method | ~5 | Inline parse calls |
| H126 | `internal/scanner/reconcile.go:175` | `isDeterministicActivityFailure` — matches on hardcoded error substrings from other packages | ~10 | Use explicit error types/sentinels |
| H127 | `internal/organizer/organizer.go:68` | `Organizer` struct has 22 fields, 14 `With*` option functions — god struct | ~50 | Split into required core + optional extensions, or accept it |
| H128 | `internal/organizer/organizer.go:574,625` | `collectVideoFiles` and `findExistingMediaFile` each define inline video extension map | ~15 | Use shared constant (H89) |
| H129 | `internal/watcher/watcher.go:42` | `WithRecursive` option function + `Option` type for single boolean always true | ~10 | Remove options pattern, hardcode `recursive: true` |
| H130 | `internal/scanner/types.go:12` | `OrphanChecker` interface with one implementation + `isNilOrphanChecker` reflection hack | ~15 | Delete interface, use concrete type |
| H131 | `internal/transfer/fallback.go:68` | `fmt.Printf` side-effect print in library code | ~1 | Use injected logger |
| H132 | `internal/transfer/transfer.go:58` | `TransferOptions` has 14 fields, `Checksum` dead, `Progress`/`DeletePartial` only used by 2 backends | ~5 | Remove dead fields |
| H133 | `internal/consolidate/planner.go:80` | `generateDuplicateMoviePlansTx` + `generateDuplicateEpisodePlansTx` — ~90% identical | ~40 | One parameterized function |
| H134 | `internal/consolidate/executor.go:130` | `executeMove` + `executeRename` share ~70% code | ~50 | Merge into `executeRelocate` |
| H135 | `internal/housekeeping/engine.go:260` | `allDigits` and `isAllDigits` — identical functions defined twice in same file | 8 | Delete `allDigits`, rename callers |
| H136 | `internal/housekeeping/engine.go:1200` | `payloadInt64` + `payloadIntPtr` — near-duplicates | 15 | One function, callers wrap return |
| H137 | `internal/labeling/fuzzy.go:10` | `titleTokens` + `originalTokens` — ~90% identical | ~20 | One `tokenize(s, lower bool)` |
| H138 | `internal/labeling/runner.go:80` | `runStalePass` and `RunOnce` main loop share identical label+update logic | ~20 | Extract `labelBatch()` |
| H139 | `internal/activity/logger.go:200` | `JSONLScanner` — thin wrapper around `bufio.Scanner` + `json.Unmarshal` | 30 | Inline 3-line loop, delete type |
| H140 | `internal/scheduler/scheduler.go:230,240` | `parseEvery` 2-line wrapper + `sameMinute` compares 6 time fields | 9 | Inline `parseEvery`, use `a.Truncate(time.Minute).Equal(...)` |
| H141 | `internal/plans/plans.go:30` | `getConsolidatePlansPath`/`getDuplicatePlansPath`/`getAuditPlansPath` — identical | 15 | Single `planPath(name)` |
| H142 | `internal/consolidate/planner.go:180` | `GetPendingPlans` + `GetPlanByID` — identical SELECT + scan | ~15 | Extract `scanPlan(row)` |
| H143 | `internal/database/normalize.go:44,78` | `NormalizeForMatch()` 95% identical to `NormalizeTitle()` + `ExtractYearFlexible()` duplicates `ExtractYear()` | ~34 | Merge, add `keepYear` param |
| H144 | `internal/database/maintenance.go:57` | `listUserTables()` — SQL already filters `sqlite_`, Go loop also checks | 4 | Remove Go-level prefix check |
| H145 | `internal/database/housekeeping.go:361` + `internal/database/parse_decisions.go:436` | `hkScanner` + `scanner` — single-implementation interfaces for `*sql.Row`/`*sql.Rows` | 6 | Delete interfaces, use concrete types |
| H146 | `internal/database/conflicts.go:37` | `getUnresolvedConflictsLocked()` — private "locked" variant, 2 callers | 3 | Inline into callers |
| H147 | `internal/database/series_repair.go:218` | `nullInt64Interface()` + `nullStringInterface()` — duplicate null-handling | 6 | Use consistent pattern |
| H148 | `internal/database/media_files.go:438,482` | `AICoverageStats` + `GetAICoverageStats()` + `GetFilesNeverAIParsed()` — check callers | ~92 | Delete if unused |
| H149 | `internal/database/ai_improvements.go:163,189,233` | `DeleteAIImprovement()` + `GetAIImprovementsByModel()` + `CountAIImprovementsByStatus()` — check callers | ~56 | Delete if unused |
| H150 | `internal/database/media_files.go:242` | `DeleteMediaFileByID()` — check if any code deletes by ID | 7 | Delete if unused |
| H151 | `internal/jellyfin/client.go:127,143` | `Ping()` wraps `GetSystemInfo()` + `RemoteSearchResult` convenience fields duplicate `ProviderIDs` | ~15 | Delete wrapper + convenience fields |
| H152 | `internal/jellyfin/library.go:24` | `RefreshItemFullMetadata` + `RefreshItemFullMetadataRecursive` — differ only in `Recursive` flag | ~10 | Single function with `recursive bool` |
| H153 | `internal/jellyfin/verify.go:45` | `Logger` interface with `Printf` + `WithLogger` — `slog` already imported project-wide | ~20 | Use `slog` directly |
| H154 | `internal/jellyfin/pathmap.go:44` | `PathTranslator` stores two sorted copies of same mappings | ~5 | Store one list, sort on lookup |
| H155 | `internal/jellyfin/playback_lock.go:85` + `internal/jellyfin/metadata_recovery.go:82` + `internal/jellyfin/sweep.go:47` | `normalizePath` one-liner + nil-receiver guards + `sweepUnidentified` indirection | ~14 | Inline, delete guards, call directly |
| H156 | `internal/jellyfin/metadata_recovery.go:330` | `translateMetadataItem` copies entire `Item` struct for one field mutation | ~3 | Modify in place |
| H157 | `internal/jellyfin/items.go:30,117` | `GetItem` calls `GetItemsByIDs` with single-item slice + `ListItemsPage` wrapper | ~10 | Direct endpoint call, delete wrapper |
| H158 | `internal/sync/filesystem.go:243,311` | `parseTVShowDir`/`parseMovieDir` wrappers + `hasVideoFiles` one-liner | ~11 | Delete wrappers, inline |
| H159 | `internal/sync/filesystem.go:288` | `countVideoFiles` duplicates video extension list | ~10 | Use shared `isMediaFile` (H90) |
| H160 | `internal/sync/sync.go:233` + `internal/jellyfin/plugin.go:64` | Two different retry-with-backoff implementations | ~15 | Single shared retry helper |
| H161 | `internal/jellyfin/metadata_recovery.go:36` + `internal/config/config.go:157` | Two `MetadataRecoveryConfig` structs with same name, different fields | 0 | Rename jellyfin one to `ReconcilerConfig` |
| H162 | `cmd/plex2jellyfin/audit_preview.go` | TUI preview using charmbracelet/bubbles/tea/lipgloss (210 lines) — same info could be plain text | ~210 | Replace with `fmt.Print` + `--show-all` flag |
| H163 | `cmd/plex2jellyfin/status_cmd.go:200` | `deploymentDriftWarnings` — SHA256 comparison of binaries, CI concern not runtime | ~70 | Delete, move to `make deploy-check` |
| H164 | `cmd/plex2jellyfin/audit_validation.go:20,60` | `inferTypeFromLibraryRoot` keyword matching + `validateMediaType` live API calls during generation | ~80 | Use config membership, remove API validation |
| H165 | `cmd/plex2jellyfin/consolidate_cmd.go:200` | `updateDatabaseAfterMove` + `createMediaFileEntry` — DB logic in CLI layer | ~30 | Move to `internal/database/` |
| H166 | `cmd/plex2jellyfin/daemon_cmd.go:40` | `formatDaemonIPCError` — error classification via string matching | ~15 | Use `errors.Is`/`errors.As` with typed sentinels |
| H167 | `cmd/plex2jellyfin-daemon/control.go:200` + `cmd/plex2jellyfin-daemon/jobs_handlers.go:430` | `guardMutator` thin wrapper + `payloadIntPtrLocal` mirrors housekeeping | ~25 | Inline, import housekeeping version |
| H168 | `cmd/plex2jellyfin/audit_cmd.go:100` + `cmd/plex2jellyfin-daemon/streaming_ops.go:120` + `cmd/plex2jellyfin/orphans.go:18` | `dbProvider` + `aiBatcher` + `orphansClient` — interfaces with one implementation each | ~18 | Delete interfaces, use concrete types |
| H169 | `cmd/plex2jellyfin/scan_cmd.go:280` + `cmd/plex2jellyfin/consolidate_generate.go:140` + `cmd/plex2jellyfin/config.go:330` | `shouldRunPostScanAnalysis` + `shouldReportSkippedConsolidationPlan` + `separator` — one-line functions called once | ~9 | Inline at call sites |
| H170 | `internal/api/sse_relay.go:15` | `IPCAttacher` interface with one implementation (`*ipc.Client`) | ~4 | Delete interface, use concrete type |
| H171 | `internal/api/media_managers.go:155` | `convertSonarrQueueItems` + `convertRadarrQueueItems` — near-identical converters | ~20 | Unify or use generic converter |
| H172 | `internal/api/settings_handlers.go:110` | `preserveMaskedSectionSecrets` — repeated switch with same pattern | ~15 | Table-driven |
| H173 | `internal/config/config.go:70,122` | `ResolveUID`/`ResolveGID` near-identical + `ParseFileMode`/`ParseDirMode` identical | ~30 | Single `resolveID` + `parseMode` |
| H174 | `internal/api/handlers.go:380` | `generateEventID` uses `uuid.NewMD5` — cryptographic hash for display-only ID | ~3 | `fmt.Sprintf("%d-%s", ...)` |
| H175 | `internal/api/auth.go:28` + `internal/api/server.go:170` | `secureRandomRead = rand.Read` indirection + `getConfigDir()` 3-line wrapper | ~4 | Call directly, inline |
| H176 | `cmd/installer/utils.go:130` | `resolveInstallerProjectRoot()` — 3-line wrapper | ~3 | Inline |
| H177 | `internal/postmortem/collector.go:131` | `parseDecisionEvidence` mirrors `database.ParseDecision` field-for-field | ~20 | Add `json` tags to DB struct directly |
| H178 | `internal/service/analysis.go:287` | `isScatteredMediaFile` duplicates `isVideoFile` extension list | 7 | Use shared definition (H95) |
| H179 | `internal/ai/queue.go:247` | `QueueError` custom type — identical to `errors.New`/`fmt.Errorf` | 7 | Replace with `errors.New` sentinels |

---

## LOW IMPACT — Nice to have, low urgency

Small line-count savings, cosmetic improvements, or items requiring caller verification first.

| # | File | What | Lines | Fix |
|---|------|------|-------|-----|
| L1 | `internal/naming/advanced.go:412` | `isRomanNumeralTitleToken` — 10-line switch, could be map lookup | ~5 | `map[string]bool` |
| L2 | `internal/naming/advanced.go:782` | `NormalizeName` — calls `StripReleaseGroup` instead of `stripReleaseMarkers` | ~5 | Use simpler function |
| L3 | `internal/naming/advanced.go:832` | `ExtractResolution` — `strings.Contains` chain, fine as-is | 0 | Keep |
| L4 | `internal/naming/blacklist.go:1` | `buildReleaseGroupMap()` — builds map from slice, could be literal | ~3 | `var KnownReleaseGroups = map[string]bool{...}` |
| L5 | `internal/naming/confidence.go:1,112` | Pre-compiled regexes + `isTVPattern` wrapper — fine | 0 | Keep |
| L6 | `internal/naming/confidence.go:72` | `isStandaloneReleaseArtifact` — simple check, fine | 0 | Keep |
| L7 | `internal/naming/confidence.go:172` | `endsWithCodecOrSource`/`hasResolutionInTitle`/`hasReleaseMarkers` — simple checks | ~5 | Use `knownReleaseGroups` map for marker list |
| L8 | `internal/naming/deobfuscate.go:1,62,132` | Pre-compiled regexes + `isShortRandomAlphanumeric` + utility functions — fine | 0 | Keep |
| L9 | `internal/naming/deobfuscate.go:182,222,292,338` | `ParseTVShowFromPathVerbose`/`ParseMovieFromPathVerbose`/`IsTVEpisodeFromPath`/`ParseError` — necessary | 0 | Keep |
| L10 | `internal/naming/jellyfin_naming.go:1,17,62` | Pre-compiled regexes + `IsJellyfinCompliantFilename` + `hasReleaseGroupSuffix` — fine | 0 | Keep |
| L11 | `internal/naming/known_titles.go` | `knownMediaTitles` map + `IsKnownMediaTitle` — necessary | 0 | Keep |
| L12 | `internal/naming/naming.go:1,82,152,202,267,312,362,402,432` | Core types + parsing pipeline + formatting + regexes + `stripReleaseMarkers` + `extractYear`/`removeYear` + `findEpisodeMatch` — fine | 0 | Keep |
| L13 | `internal/quality/extract.go:1,17,62,72,92` | `QualityMetadata` + `ExtractMetadata` + wrappers + `CompareWithSize` + `FindBestFile` — mostly fine | ~24 | Delete `ExtractMovieMetadata`/`ExtractEpisodeMetadata`/`CompareWithSize` wrappers |
| L14 | `internal/quality/patterns.go:1,52,132` | Pre-compiled regexes + switch parsers + `CompareFiles`/`IsBetterFile`/`GetQualityString` — mostly fine | ~5 | Delete `GetQualityString` (inline `.String()`) |
| L15 | `internal/quality/quality.go:1,82,102,182` | Type definitions + `QualityInfo` + `ComputeScore` + `Compare`/`IsBetterThan` — necessary | 0 | Keep |
| L16 | `internal/quality/scoring.go:1,32,92,102,152` | CONDOR constants + `ScoreFile` + wrappers + `ShouldInclude*` + `CodecToString` — mostly fine | ~11 | Delete `ScoreMovie`/`ScoreEpisode` wrappers, fix `CodecToString` to use `strings.Contains` |
| L17 | `internal/organizer/organizer.go:443` | `languageSuffixes` global var — 30+ codes as slice, fine | 0 | Keep or convert to map |
| L18 | `internal/transfer/transfer.go:130` | `New()` factory builds fallback chain at construction — acceptable for daemon | 0 | Keep |
| L19 | `internal/transfer/health.go:44` | `CheckDiskHealthDetailed` — goroutine timeouts for hung syscalls, justified | 0 | Keep |
| L20 | `internal/scanner/scanner.go:28` | `progressReportInterval = 10` constant used once — trivial | 0 | Keep or inline |
| L21 | `internal/daemon/handler.go:1558` | `runJellyfinVerificationPass` O(n) individual DB queries — needs batch method | 0 | Add `GetJellyfinItemsByPaths([]string)` to DB |
| L22 | `internal/daemon/ipc/protocol.go` | `CmdDupScan`, `CmdVerifyFlagged`, `CmdTask*` — sprawling 30+ command set | 0 | Consider consolidating some commands |
| L23 | `internal/scheduler/scheduler.go:50` | `registeredJob` struct has 2 fields — could be folded | 3 | Use `map[string]*jobState` |
| L24 | `internal/consolidate/operations.go:310` | `formatBytes` standalone — move to shared package | 0 | Move only |
| L25 | `internal/database/scheduled_jobs.go:95` | `scanScheduledJob` reuses `hkScanner` — cross-file coupling | 0 | Refactor if H145 applied |
| L26 | `internal/database/migration.go` | Migration v9 removal — renumbering or gap, verify no code references version numbers | 0 | Verify before H35 |
| L27 | `internal/database/media_files.go:432` | `getLowConfidenceFiles` — `rows.Scan` reads `bool` directly instead of `int` intermediate (SQLite stores 0/1) | 0 | Fix scan bug (correctness, not over-engineering) |
| L28 | `internal/jellyfin/deferred_queue.go:82` + `internal/jellyfin/playback_lock.go:69` | `GetAll()`/`Count()` — check if test-only | ~0-10 | Delete if test-only |
| L29 | `internal/sonarr/series.go:38` + `internal/radarr/movies.go:56` | `FindSeriesByTitle`/`GetMovieByTmdbID` — O(n) client-side scan | 0 | Acceptable, add server-side lookup if library exceeds ~500 |
| L30 | `internal/sonarr/commands.go` + `internal/radarr/commands.go` | `WaitForCommand` structurally identical — acceptable duplication for different APIs | 0 | Keep |
| L31 | `internal/sonarr/types.go` + `internal/radarr/types.go` | `SystemStatus` structs ~80% identical — risk of API divergence | 0 | Keep separate |
| L32 | `internal/jellyfin/metadata_recovery.go:36` | `MetadataRecoveryConfig` name collision with config package | 0 | Rename for clarity |
| L33 | `cmd/plex2jellyfin-daemon/streaming_ops.go` | `aiBatcher` interface — one method, one impl | ~5 | Remove interface |
| L34 | `cmd/plex2jellyfin/orphans.go` | `orphansClient` interface — one method, one impl | ~5 | Remove interface |
| L35 | `cmd/plex2jellyfin/audit_cmd.go` | `dbProvider` struct — one method, one impl | ~8 | Inline |
| L36 | `internal/ai/types.go` | `FlexInt`/`FlexIntSlice` — complex but legitimate for Ollama's inconsistent JSON | 0 | Keep |
| L37 | `internal/ai/circuit_breaker.go` | Custom circuit breaker — standard pattern, well-implemented | 0 | Keep |
| L38 | `internal/ai/keepalive.go:88` + `internal/ai/cache.go:148` | `fmt.Printf` instead of structured logger | 0 | Use `logging.Logger` |
| L39 | `internal/notify/jellyfin.go` | `targetedRefresh` falls through to `refreshLibrary` — deliberate fallback, correct | 0 | Keep |
| L40 | `internal/library/cache.go` | `SeriesCache` custom TTL cache — needed for slow Sonarr API | 0 | Keep, check if `ForceRefresh` unused |
| L41 | `internal/service/convergence.go` | `GroupConfidence` complex heuristic — core safety gate, legitimate | 0 | Keep |
| L42 | `internal/privilege/escalate.go` | `Escalate` re-execs with sudo — clean, 36 lines | 0 | Keep |
| L43 | `internal/tmdb/verifier.go` | TMDB verifier with 3-tier lookup — well-structured | 0 | Keep |
| L44 | `internal/postmortem/collector.go` | Collector writes 11 files per run — legitimate for postmortem bundles | 0 | Keep |
| L45 | `internal/service/rebuild_movies.go` | `RebuildMoviesFromMediaFiles` — legitimate data repair operation | 0 | Keep |
| L46 | `internal/ai/integrator.go` | `Integrator` orchestrates 6 subsystems — high coupling but this is the integration point | 0 | Accept |
| L47 | `internal/ai/matcher.go` | `ParseWithRetry`/`ParseWithCloud` — composition concerns, not separate methods | ~30 | Fold into `parseWithModel` |

---

## NEEDS DEEPER RESEARCH — Cross-package verification required

These findings need caller analysis, cross-package tracing, or empirical measurement before acting.

### Media File Detection (5 items)

| # | Finding | Investigation |
|---|---------|---------------|
| R1 | `isMediaFile`/`isVideoFile` spans cmd/plex2jellyfin, cmd/plex2jellyfin-daemon, cmd/plex2jellyfin/cleanup.go, internal/jellyfin/verify.go, internal/consolidate/consolidate.go, internal/analyzer, internal/library, internal/service | Verify all copies use same extension set. Consolidate into one `internal/mediautil.IsMediaFile(path)`. |
| R2 | `KnownReleaseGroups` (10,992 entries) vs `knownReleaseGroups` (~80 entries) | Run both against 10,000+ real filenames. Measure accuracy and false positives. Delete 105KB file if 80-entry covers 95%+. |
| R3 | `videoExtensions` map in analyzer, library, service — 3+ places | Consolidate to one canonical source. |

### Consolidation/Duplicate Systems (4 items)

| # | Finding | Investigation |
|---|---------|---------------|
| R4 | `Consolidator` vs `Executor`+`Planner` vs `CleanupService` — three systems for duplicate detection | Trace CLI commands and daemon paths. Confirm `Consolidator` is truly dead before H41 deletion. |
| R5 | `reconcileActivity` reads JSONL activity files — verify activity package writes all fields used by retry logic | Check `activity.Entry` fields: `Deterministic`, `SourceMtime`, `Error`. |
| R6 | `newConsolidateCmd` / `newDuplicatesCmd` structural duplication | Both have generate/execute/dry-run subcommands. Check if audit follows same pattern. |
| R7 | `executeDelete` triplication — 3 implementations differ in permission-checking and dry-run handling | Verify unified `DeleteMediaFile` satisfies all callers without losing safety checks. |

### Config & Serialization (3 items)

| # | Finding | Investigation |
|---|---------|---------------|
| R8 | `ToTOML()` replacement touches config save pipeline | Verify viper round-trips all fields correctly, especially `PermissionsConfig` conditional block and `PasswordHash` append. |
| R9 | `BeamsTextEffect` deletion — check installer update.go and view.go for all references | Ticker and beams intertwined in update loop and view. Removing means simplifying `Update()`. |
| R10 | `MetadataRecoveryConfig` name collision — config/config.go and jellyfin/metadata_recovery.go both define it | Rename jellyfin one to `ReconcilerConfig`, update all references. |

### Interface/Abstraction Cleanup (8 items)

| # | Finding | Investigation |
|---|---------|---------------|
| R11 | `Transferer` interface — verify `VolumeLimitedTransferer` used in production paths | If only tests/edge case, mount-root detection logic may be dead. |
| R12 | `checkPlaybackSafetyWithOp` — verify both webhook locks and API polling paths used in production | If only one configured, other branch + fields are dead weight. |
| R13 | `Stats` atomic migration — verify no caller depends on mutual exclusion across multiple fields | Replace `sync.RWMutex` with `atomic.Int64` if safe. |
| R14 | `CmdDupScan`, `CmdVerifyFlagged`, `CmdTask*` — sprawling 30+ command set | Consider consolidating: `TASK_GET` + `TASKS_BULK` + `TASK_GROUP`. |
| R15 | `MetadataClient`/`MetadataRepairClient`/`MetadataStore` interface removal — check test impact | Can mock be concrete struct without interface? |
| R16 | `PluginClient` vs `Client` merge — check all PluginClient call sites | Confirm no behavioral differences beyond retry logic. |
| R17 | `DatabaseProvider` interface — one method, one implementation | Check polymorphic usage. If none, pass `*sql.DB` directly. |
| R18 | `LLMProvider` interface — is `ProviderRegistry` used anywhere outside tests? | If registry exists only for future multi-provider, YAGNI. |

### DB Layer Caller Analysis (6 items)

| # | Finding | Investigation |
|---|---------|---------------|
| R19 | `AIImprovement` entire subsystem — check which methods have callers outside database package | Unused methods = dead code. Whole table may be vestigial. |
| R20 | `Stats` / `GetStats` legacy — search for callers across all packages | Delete if zero callers. |
| R21 | `DB()` vs `SQL()` callers — grep all packages before deleting `DB()` | Replace with `SQL()`. |
| R22 | `UpdateMediaFile` vs `UpsertMediaFile` — verify caller behavior | If callers always pass same path, `UpsertMediaFile` handles it via ON CONFLICT. |
| R23 | `DeleteMediaFileByID` callers — check if any code deletes by ID | Delete method if all deletions use path. |
| R24 | `IssueType` constants usage — search all packages for references | Delete const block if zero references. |

### Shared Utilities (5 items)

| # | Finding | Investigation |
|---|---------|---------------|
| R25 | `normalizeTitle` in library/scanner.go vs `database.NormalizeForMatch` — two normalization functions | Check if they produce identical results. Delete library one if so. |
| R26 | `formatBytes` exists in 2 places, `min` in 2 places — shared utilities scattered | Consolidate `formatBytes` to `internal/format`. `min` is built-in (Go 1.26). |
| R27 | `isAllDigits`/`allDigits`/`isAllDigitsOrPunct` — may exist in naming or database packages | Cross-package grep before consolidating. |
| R28 | `isEmptyDir` / `removeDirIfEmpty` / `removeIfEmptyTree` / `CleanupEmptyDir` — scattered | Check if `internal/paths` already has these. |
| R29 | `formatBytes` — check if `internal/format` already exists | Move or create shared package. |

### Parser Brain Deep Dives (5 items)

| # | Finding | Investigation |
|---|---------|---------------|
| R30 | `advanced.go` regex list vs `naming.go` regex list — map which callers use which | Verify extra patterns in `advReleasePatterns` don't break `stripReleaseMarkers`. |
| R31 | `StripReleaseGroup` vs `stripReleaseMarkers` — trace all callers | Determine if `stripReleaseMarkers` can replace `StripReleaseGroup` for all callers. |
| R32 | `titleCaseWithOrdinals` external dependency — `golang.org/x/text` used elsewhere? | If not, replacing with simple title-caser removes external dep. |
| R33 | `ParseFromPath` vs `ExtractMetadata` parent-dir walking — check callers | Delete `ParseFromPath` if no external callers. |
| R34 | `SimilarityRatio` / `levenshteinDistance` callers — simpler algorithm? | Replace with Jaccard/token-overlap if only for fuzzy dedup. |

### *arr/Jellyfin Deep Dives (4 items)

| # | Finding | Investigation |
|---|---------|---------------|
| R35 | `DeferredQueue.GetAll()` / `PlaybackLockManager.GetAll()` / `Count()` usage | May be test-only. Cross-reference `internal/api/`, `internal/daemon/`. |
| R36 | Radarr context-wrapper pattern scope — verify no external caller depends on non-Context variants | Delete all non-Context wrappers if safe. |
| R37 | `joinURL` dedup placement — weigh package overhead vs duplication | New `internal/arrutil` for 8 lines vs inline at call sites. |
| R38 | `*arr` test connection duplication spans API test_handlers, installer validation, and client packages | Underlying clients already have `GetSystemStatus()`. Eliminate thin wrappers. |

### CLI/Web Deep Dives (4 items)

| # | Finding | Investigation |
|---|---------|---------------|
| R39 | ProgressBar proliferation — check `internal/scanner` for yet another copy | Consolidate into one `internal/progress` package. |
| R40 | Plan file management — 11 plan-file functions for 3 plan types | Could be 3 generic functions. Check `internal/plans/` for refactor. |
| R41 | `audit_preview.go` charmbracelet dependency — is bubbles/tea/lipgloss used elsewhere? | Check `cmd/plex2jellyfin/scan_progress.go` (also uses lipgloss) and `go.mod`. |
| R42 | `classification-audit` and `test-hint` binaries — referenced by Makefile, CI, or docs? | Check before deleting. |

### AI/Infra Deep Dives (5 items)

| # | Finding | Investigation |
|---|---------|---------------|
| R43 | `SonarrAdapter` vs `RadarrAdapter` — ~90% structural copy | Could single generic adapter work? *arr clients have similar APIs. |
| R44 | `SeriesCache.ForceRefresh` — is it called anywhere? | Delete if unused. |
| R45 | `BackgroundQueue.Output()` — is the output channel consumed? | Delete `outputChan` if unused. |
| R46 | `NotifyManager.Results()` — is the async result channel consumed? | Delete async mode if unused. |
| R47 | `MediaManager` interface — is Registry used polymorphically? | Two implementations exist (Sonarr/Radarr), which justifies interface. But check registry abstraction usage. |

### Daemon/Process Deep Dives (4 items)

| # | Finding | Investigation |
|---|---------|---------------|
| R48 | `ReadFlaggedForReview` in enhancelog.go — lives in daemon but only called from CLI | Move to shared package or CLI layer. |
| R49 | `tvOrganizer`/`movieOrganizer` duplication — two Organizer instances with nearly identical option chains | Organizer package could accept media type parameter. |
| R50 | `processFile` regex path vs `organizeWithRegexFallback` — verify debounce/pending maps don't interfere | Could `processFile` be called directly for fallback items? |
| R51 | `runJellyfinVerificationPass` O(n) queries — needs batch DB method | Add `GetJellyfinItemsByPaths([]string)`. |

### DB Schema/Data (3 items)

| # | Finding | Investigation |
|---|---------|---------------|
| R52 | `GetLibraryStats` dead table queries — confirm `movie_duplicates`/`episode_duplicates` tables don't exist | Check all migrations. |
| R53 | `Checker.libraryRoot` dead field — verify no external package accesses it | Unlikely but check before deleting. |
| R54 | Migration v9 no-op — verify no code references specific version numbers | `applyMigrations` skips `version <= currentVersion`, gap is harmless. |

### Other (2 items)

| # | Finding | Investigation |
|---|---------|---------------|
| R55 | `NormalizeTitleFromFilename` — check if callers could use naming package directly | If this is the only caller of those naming functions, dependency direction may be inverted. |
| R56 | Ollama test logic duplication spans API ai_custom.go, installer validation.go, and possibly internal/ai/ | Check if `internal/ai/` already has Ollama client both could share. |

---

## TOTALS

| Category | Count | Est. Lines Saveable |
|----------|-------|---------------------|
| HIGH — Dead Code | 40 | ~1,500 |
| HIGH — Structural Duplication | 48 | ~2,500 |
| HIGH — Cross-File Duplication | 22 | ~600 |
| HIGH — Pipeline Simplification | 69 | ~1,200 |
| LOW — Nice to Have | 47 | ~200 |
| RESEARCH — Cross-Package | 56 | TBD |
| **TOTAL** | **282** | **~6,000 + 105,000 (blacklist)** |

The 105K-line `KnownReleaseGroups` blacklist dominates the line count. Excluding it, ~6,000 lines of actionable savings across the codebase.

---

## Next Steps

1. **Wave 1: Dead code** — Delete H1-H40 immediately. Zero risk, instant savings.
2. **Wave 2: Research** — Dispatch agents for R1-R56 cross-package verification.
3. **Wave 3: Structural fixes** — Implement H41-H179 after research confirms safety.
4. **Wave 4: Low impact** — L1-L47 as time permits.
