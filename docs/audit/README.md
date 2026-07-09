# Plex2Jellyfin Ponytail Audit — Master Summary

## Totals

| Metric | Count |
|--------|-------|
| Domains audited | 9 |
| Findings | 280 |
| Lines saveable | ~111,400 |
| Cross-package research needed | 63 |

## Top Findings by Domain

### Parser Brain
| Rank | What | Lines |
|------|------|-------|
| 1 | `KnownReleaseGroups` — 10,992-entry map (105KB) from srrDB.com. 80-entry map covers 95%+ of real cases. | ~105,000 |
| 2 | `advanced.go:init()` duplicates ~80% of `naming.go:init()` regex list. Two init blocks, same package. | ~150 |
| 3 | `contains`, `stringContains`, `indexOfSubstring`, `toLower` — hand-rolled reimplementations of `strings.Contains`/`strings.ToLower`. | ~30 |

### Pipeline
| Rank | What | Lines |
|------|------|-------|
| 1 | `OrganizeMovie`/`OrganizeMovieWithParsed` and `OrganizeTVEpisode`/`OrganizeTVWithParsed` are ~90% identical pairs. | ~300 |
| 2 | Video extension map duplicated 7× across cleanup, dedup, organizer, scanner, watcher, pv. | ~60 |
| 3 | `OrphanChecker` interface with one implementation + `isNilOrphanChecker` reflection hack. | ~15 |

### Daemon Core
| Rank | What | Lines |
|------|------|-------|
| 1 | `daemon.go` — entire file dead. `Daemon` struct, `NewDaemon`, `Start`, `Stop`, `Run` have zero callers. | ~80 |
| 2 | `organizeWithRegexFallback` duplicates the regex path from `processFile` (~90 lines verbatim). | ~90 |
| 3 | `processFile` TV/Movie branches share ~80% structure — parse→DB→AI→organize flow is identical. | ~50 |

### Background Loops
| Rank | What | Lines |
|------|------|-------|
| 1 | Two parallel consolidation systems: `Consolidator`+`Plan`+`Operation` vs `Executor`+`Planner`+`ConsolidationPlan`. Different structs, DB tables, same purpose. | ~500 |
| 2 | Save/Load/Delete/Archive repeated 3× for consolidate, duplicate, audit plans. Identical pattern, different filenames. | ~150 |
| 3 | `execParserDriftRename` and `execParserDriftTVRename` share ~80% structure. | ~100 |

### DB Layer
| Rank | What | Lines |
|------|------|-------|
| 1 | `UpdateMediaFile` duplicates `UpsertMediaFile`'s 25-column UPDATE clause. `ON CONFLICT DO UPDATE` already handles it. | ~68 |
| 2 | `GetMediaFile` + `GetMediaFileByID` — copy-paste with `WHERE path = ?` vs `WHERE id = ?`. 60+ lines each. | ~50 |
| 3 | `detectSeriesConflicts` + `detectMovieConflicts` — copy-paste with series/movie swapped. 43 lines each. | ~40 |

### *arr & Jellyfin
| Rank | What | Lines |
|------|------|-------|
| 1 | Context-wrapper pattern: every Radarr/Sonarr method has a `*Context` variant that just calls the context version with `context.Background()`. | ~120 |
| 2 | `PluginClient` duplicates ~80% of `Client` struct fields, auth, get/post methods. Only difference is retry logic. | ~50 |
| 3 | `isMediaFile` duplicated 4× across jellyfin/verify.go, cmd/plex2jellyfin, cmd/plex2jellyfin-daemon, consolidate. | ~40 |

### CLI & Commands
| Rank | What | Lines |
|------|------|-------|
| 1 | `audit_preview.go` — TUI preview using charmbracelet/bubbles/tea/lipgloss (210 lines). Same info could be plain text. | ~210 |
| 2 | `classification-audit/main.go` — standalone binary for one-off testing with hardcoded paths. YAGNI. | ~219 |
| 3 | 3 separate `ProgressBar` implementations across audit_validation, scan_progress, audit_cmd. | ~100 |

### Web & Config
| Rank | What | Lines |
|------|------|-------|
| 1 | `BeamsTextEffect` — 771-line custom ASCII animation engine for a one-time installer header. | ~771 |
| 2 | 6 nearly identical `init*Inputs`/`save*Inputs` function pairs in installer — same textinput styling boilerplate repeated ~30 times. | ~200 |
| 3 | `ToTOML()` — 200-line hand-rolled TOML serializer. Viper (already imported) has `WriteConfig()`. | ~180 |

### AI & Infra
| Rank | What | Lines |
|------|------|-------|
| 1 | Custom log rotation (198 lines) — reinvents lumberjack. | ~190 |
| 2 | `getSystemPrompt` — 100-line string literal embedded in Go source. | ~100 (moved) |
| 3 | `LLMProvider` + `MediaManager` duplicated registry pattern — identical `map[string]T` with Register/Get/All/AllInfo. | ~50 |

## Cross-Package Research Needed

### Media File Detection (5 items)
- **C1** (Pipeline): `isVideoFile` (scanner.go) vs `Handler.IsMediaFile` (watcher.go) — two different video-file detection paths. Verify all Handler implementations use the same extension set.
- **C1** (CLI): `isMediaFile`/`isVideoFile` spans cmd/plex2jellyfin, cmd/plex2jellyfin-daemon, cmd/plex2jellyfin/cleanup.go, and possibly internal/ packages.
- **#3** (*arr): `isMediaFile` duplicated 4× across jellyfin/verify.go, cmd/plex2jellyfin, cmd/plex2jellyfin-daemon, consolidate.
- **CR3** (AI): `videoExtensions` map exists in analyzer, library, service — 3+ places.
- **R1** (Parser): `KnownReleaseGroups` (10,992 entries) vs `knownReleaseGroups` (~80 entries) — measure accuracy on real filenames.

### Consolidation/Duplicate Systems (4 items)
- **#1** (Background): `Consolidator` vs `Executor`+`Planner` vs `CleanupService` — three systems for duplicate detection. Trace CLI commands and daemon paths.
- **C4** (Pipeline): `reconcileActivity` reads JSONL activity files — verify activity package writes all fields used by retry logic.
- **C4** (CLI): `newConsolidateCmd` / `newDuplicatesCmd` structural duplication — both have generate/execute/dry-run subcommands.
- **#5** (Background): `executeDelete` triplication — 3 delete implementations differ in permission-checking and dry-run handling.

### Config & Serialization (3 items)
- **C1** (Web): `ToTOML()` replacement touches config save pipeline — verify viper round-trips all fields correctly.
- **C4** (Web): `BeamsTextEffect` deletion — check installer update.go and view.go for all references to beams/ticker.
- **#7** (*arr): `MetadataRecoveryConfig` name collision — config/config.go and jellyfin/metadata_recovery.go both define it with different fields.

### Interface/Abstraction Cleanup (8 items)
- **C3** (Pipeline): `Transferer` interface — verify `VolumeLimitedTransferer` is used in production paths.
- **C5** (Pipeline): `checkPlaybackSafetyWithOp` — verify both webhook locks and API polling paths are used in production.
- **#5** (Daemon): `Stats` atomic migration — verify no caller depends on mutual exclusion across multiple fields.
- **#6** (Daemon): `CmdDupScan`, `CmdVerifyFlagged`, `CmdTask*` — sprawling command set (30+ commands).
- **#5** (*arr): `MetadataClient`/`MetadataRepairClient`/`MetadataStore` interface removal — check test impact.
- **#2** (*arr): `PluginClient` vs `Client` merge — check all PluginClient call sites.
- **CR5** (AI): `DatabaseProvider` interface — one method, one implementation. Check polymorphic usage.
- **CR9** (AI): `LLMProvider` interface — is `ProviderRegistry` used anywhere outside tests?

### DB Layer Caller Analysis (6 items)
- **#1** (DB): `AIImprovement` entire subsystem — check which methods have callers outside database package.
- **#4** (DB): `Stats` / `GetStats` legacy — search for callers across all packages.
- **#7** (DB): `DB()` vs `SQL()` callers — grep all packages before deleting.
- **#8** (DB): `UpdateMediaFile` vs `UpsertMediaFile` — verify caller behavior.
- **#9** (DB): `DeleteMediaFileByID` callers — check if any code deletes by ID.
- **#11** (DB): `IssueType` constants usage — search all packages for references.

### Shared Utilities (5 items)
- **CR1** (AI): `normalizeTitle` in library/scanner.go vs `database.NormalizeForMatch` — two normalization functions may diverge.
- **CR4** (AI): `formatBytes` exists in 2 places, `min` in 2 places — shared utilities scattered.
- **#2** (Background): `isAllDigits`/`allDigits`/`isAllDigitsOrPunct` — may exist in naming or database packages.
- **#3** (Background): `isEmptyDir` / `removeDirIfEmpty` / `removeIfEmptyTree` / `CleanupEmptyDir` — scattered across housekeeping and consolidate.
- **#4** (Background): `formatBytes` — check if internal/format already has this.

### Parser Brain Deep Dives (5 items)
- **R2** (Parser): `advanced.go` regex list vs `naming.go` regex list — map which callers use which.
- **R3** (Parser): `StripReleaseGroup` vs `stripReleaseMarkers` — trace all callers.
- **R4** (Parser): `titleCaseWithOrdinals` external dependency — check if `golang.org/x/text` is used elsewhere.
- **R5** (Parser): `ParseFromPath` vs `ExtractMetadata` parent-dir walking — check callers.
- **R7** (Parser): `SimilarityRatio` / `levenshteinDistance` callers — check if simpler algorithm suffices.

### *arr/Jellyfin Deep Dives (4 items)
- **#1** (*arr): `DeferredQueue.GetAll()` / `PlaybackLockManager.GetAll()` / `Count()` usage — may be test-only.
- **#4** (*arr): Radarr context-wrapper pattern scope — verify no external caller depends on non-Context variants.
- **#6** (*arr): `joinURL` dedup placement — weigh package overhead vs duplication.
- **C3** (Web): `*arr` test connection duplication spans API test_handlers, installer validation, and underlying client packages.

### CLI/Web Deep Dives (4 items)
- **C2** (CLI): ProgressBar proliferation — check internal/scanner for yet another copy.
- **C3** (CLI): Plan file management — 11 plan-file functions for 3 plan types.
- **C5** (CLI): `audit_preview.go` charmbracelet dependency — is bubbles/tea/lipgloss used elsewhere?
- **C6** (CLI): `classification-audit` and `test-hint` binaries — referenced by Makefile, CI, or docs?

### AI/Infra Deep Dives (5 items)
- **CR2** (AI): `SonarrAdapter` vs `RadarrAdapter` — ~90% structural copy. Could a single generic adapter work?
- **CR6** (AI): `SeriesCache.ForceRefresh` — is it called anywhere?
- **CR7** (AI): `BackgroundQueue.Output()` — is the output channel consumed?
- **CR8** (AI): `NotifyManager.Results()` — is the async result channel consumed?
- **CR10** (AI): `MediaManager` interface — is Registry used polymorphically?

### Daemon/Process Deep Dives (4 items)
- **#1** (Daemon): `ReadFlaggedForReview` in enhancelog.go — lives in daemon package but only called from CLI.
- **#2** (Daemon): `tvOrganizer`/`movieOrganizer` duplication — two Organizer instances with nearly identical option chains.
- **#3** (Daemon): `processFile` regex path vs `organizeWithRegexFallback` — verify debounce/pending maps don't interfere.
- **#4** (Daemon): `runJellyfinVerificationPass` O(n) queries — needs batch DB method.

### DB Schema/Data (3 items)
- **#3** (DB): `GetLibraryStats` dead table queries — confirm `movie_duplicates`/`episode_duplicates` tables don't exist.
- **#5** (DB): `Checker.libraryRoot` dead field — verify no external package accesses it.
- **#10** (DB): Migration v9 no-op — verify no code references specific version numbers.

### Other (2 items)
- **#2** (DB): `NormalizeTitleFromFilename` — check if callers could use naming package directly.
- **C2** (Web): Ollama test logic duplication spans API ai_custom.go, installer validation.go, and possibly internal/ai/.

## Next Steps

1. Review findings, decide which to fix
2. Dispatch follow-up agents for cross-package items
3. Create implementation plan for approved fixes
