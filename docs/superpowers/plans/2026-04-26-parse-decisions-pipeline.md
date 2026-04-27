# Parse-Decisions Pipeline Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task. For this plan, prefer `superpowers:subagent-driven-development` only when work is split by chunk with disjoint file ownership.

**Goal:** Build a reliable, queryable corpus of daemon parse/organize attempts, then use it to diagnose parser drift while separately fixing dedup and cleanup behavior.

**Architecture:** `parse_decisions` records one debounced daemon processing attempt, not one raw fsnotify event. The row is created in `MediaHandler.processFile` after early validation and is updated as parsing, organizing, Jellyfin resolution, and labeling complete. Jellyfin updates must be wired through the production daemon webhook path, with the API webhook path kept consistent where practical.

**Tech Stack:** Go, SQLite via `modernc.org/sqlite`, existing `internal/database.MediaDB`, existing daemon and Jellyfin webhook surfaces, Cobra CLI, package-local tests.

---

## Review Corrections Applied

This rewrite incorporates the review findings from the first plan pass:

- Nullable DB columns must scan through `sql.NullString`, `sql.NullInt64`, and `sql.NullTime`, or use `COALESCE`.
- Existing DB APIs are `database.Open`, `database.OpenPath`, and `paths.DatabasePath`; do not use nonexistent `NewMediaDB` or `MediaDBPath`.
- Existing DB test helper is `setupTestDB(t)` inside `internal/database`.
- New `MediaDB` methods must follow the existing `m.mu` locking pattern.
- The corpus is per processed attempt. Raw fsnotify events are debounced before `processFile`.
- AI-routed items must carry `ParseDecisionID` through `PendingItem`, `applyAIResult`, and regex fallback.
- Production Jellyfin webhooks are served by `internal/daemon/server.go` -> `MediaHandler.HandleJellyfinWebhookEvent`.
- Webhook tests must include the companion plugin's nested payload shape, not only flat Jellyfin plugin payloads.
- Sweep query windows must not use an inverted "older than lookback" filter.
- Cleanup must keep the existing source-present safety gate and must not recursively purge unrelated ancestor directories.
- Dedup must ignore subtitles and non-video files.
- Fuzzy title matching must satisfy its own negative tests and must not use naive substring matching.
- CLI and corpus tests must execute the command/harness being added, not just call lower-level DB methods.
- Smoke tests must not start the real daemon against the user's real config/database.

## File Structure

| File | Responsibility |
|---|---|
| `internal/database/schema.go` | Migration 14 and `currentSchemaVersion = 14` |
| `internal/database/parse_decisions.go` | New parse-decision model, updates, and query helpers |
| `internal/database/parse_decisions_test.go` | DB migration/CRUD/query tests |
| `internal/daemon/handler.go` | Create/update decision rows; carry IDs through AI paths; update from daemon webhooks |
| `internal/daemon/handler_test.go` | Process-attempt, AI-path, cleanup, and webhook tests |
| `internal/jellyfin/webhook_types.go` | Normalize flat and companion-plugin webhook payloads |
| `internal/organizer/dedup.go` | Episode-key dedup helpers that only consider video files |
| `internal/organizer/dedup_test.go` | Dedup helper tests |
| `internal/organizer/cleanup.go` | Strict video/subtitle allowlist purge helper |
| `internal/organizer/cleanup_test.go` | Purge helper tests |
| `internal/organizer/organizer.go` | Use canonical episode-key dedup; stem-match subtitle copy |
| `internal/organizer/organizer_test.go` | Dedup and subtitle regression tests |
| `internal/jellyfin/sweep.go` | Paginated unresolved-decision sweeper using existing Jellyfin client |
| `internal/jellyfin/sweep_test.go` | Sweep lookback, TTL, and pagination tests |
| `cmd/jellywatchd/main.go` | Start sweeper/labeler only when Jellyfin client is live |
| `cmd/jellywatch/parses_cmd.go` | `jellywatch parses` command |
| `cmd/jellywatch/parses_cmd_test.go` | CLI flag/output/override tests |
| `internal/naming/naming.go` | Verbose parse wrappers preserving path-aware parsing |
| `internal/naming/naming_test.go` | Stripped-token and obfuscated-path regression tests |
| `internal/naming/corpus_test.go` | Corpus regression harness |
| `internal/naming/testdata/parse_decisions_corpus.jsonl` | Small anonymized deterministic fixture |
| `internal/labeling/fuzzy.go` | Fuzzy title comparison |
| `internal/labeling/fuzzy_test.go` | Fuzzy matching tests |
| `internal/labeling/labeler.go` | `DeriveLabel` |
| `internal/labeling/runner.go` | Background labeler runner |
| `internal/labeling/runner_test.go` | Labeler runner tests |

## Dependency Graph

```text
Chunk 1: DB + daemon write path
  -> Chunk 4: Jellyfin outcome resolution
  -> Chunk 5: labeler
  -> Chunk 6: CLI and corpus visibility

Chunk 2: canonical dedup
  independent; can ship before Chunk 1

Chunk 3: cleanup allowlist and subtitle copy
  independent; can ship before Chunk 1
```

Recommended order: Chunk 1, Chunk 2, Chunk 3, Chunk 4, Chunk 5, Chunk 6. Chunks 2 and 3 may be pulled forward if immediate user-visible cleanup/dedup fixes are desired.

---

## Chunk 1: Parse-Decisions DB and Daemon Pipeline

### Task 1.1: Add Migration 14

**Files:**
- Modify: `internal/database/schema.go`
- Test: `internal/database/database_test.go`

**Step 1: Write the migration**

In `internal/database/schema.go`:

- Change `currentSchemaVersion` from `13` to `14`.
- Append a migration with `version: 14`.
- Keep the migration additive.

Migration SQL:

```sql
CREATE TABLE parse_decisions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_path TEXT NOT NULL,
    source_filename TEXT NOT NULL,
    event_at DATETIME NOT NULL,
    media_type_guessed TEXT,
    parse_method TEXT,
    parsed_title TEXT,
    parsed_year INTEGER,
    parsed_season INTEGER,
    parsed_episode INTEGER,
    parser_stripped_tokens TEXT,
    target_path TEXT,
    target_at DATETIME,
    existing_match_method TEXT,
    organize_outcome TEXT,
    organize_error TEXT,
    jellyfin_item_id TEXT,
    jellyfin_imdb_id TEXT,
    jellyfin_tmdb_id TEXT,
    jellyfin_tvdb_id TEXT,
    jellyfin_resolved_at DATETIME,
    auto_label TEXT,
    human_label_override TEXT
);
CREATE INDEX idx_pd_source_path ON parse_decisions(source_path);
CREATE INDEX idx_pd_target_path ON parse_decisions(target_path);
CREATE INDEX idx_pd_event_at ON parse_decisions(event_at);
CREATE INDEX idx_pd_auto_label ON parse_decisions(auto_label);
CREATE INDEX idx_pd_organize_outcome ON parse_decisions(organize_outcome);
INSERT INTO schema_version (version) VALUES (14);
```

**Step 2: Run migration tests**

Run:

```bash
go test ./internal/database -run 'TestDatabaseOpenClose|TestMediaFileMigrations' -v
```

Expected: PASS and schema version 14.

**Step 3: Commit**

```bash
git add internal/database/schema.go
git commit -m "feat(db): add parse_decisions table"
```

### Task 1.2: Add ParseDecision CRUD and Query Helpers

**Files:**
- Create: `internal/database/parse_decisions.go`
- Create: `internal/database/parse_decisions_test.go`

**Step 1: Write failing tests**

Create tests for:

- `InsertDecision` then `GetDecision` on a row with only required columns set.
- `UpdateParse` with nullable ints.
- `UpdateOrganize`.
- `UpdateOutcome`.
- `UpdateAutoLabel` and `UpdateHumanOverride`.
- `QueryDecisions` filters:
  - `OrganizeOutcome`
  - `AutoLabel`
  - `AutoLabelIsNull`
  - `JellyfinUnresolved`
  - `TargetPathNotEmpty`
  - `EventAfter`
  - `EventBefore`
  - `SourceContains`
- `GetUnresolvedDecisionByTargetPath` ignores already-resolved rows and failed organizes.

Use existing helper:

```go
db := setupTestDB(t)
defer db.Close()
```

Run:

```bash
go test ./internal/database -run 'TestParseDecision|TestQueryDecisions|TestGetUnresolvedDecisionByTargetPath' -v
```

Expected: FAIL because the APIs do not exist.

**Step 2: Implement the model and nullable scans**

Create `internal/database/parse_decisions.go` with these public types:

```go
type ParseDecision struct {
    ID                   int64
    SourcePath           string
    SourceFilename       string
    EventAt              time.Time
    MediaTypeGuessed     string
    ParseMethod          string
    ParsedTitle          string
    ParsedYear           *int
    ParsedSeason         *int
    ParsedEpisode        *int
    ParserStrippedTokens string
    TargetPath           string
    TargetAt             *time.Time
    ExistingMatchMethod  string
    OrganizeOutcome      string
    OrganizeError        string
    JellyfinItemID       string
    JellyfinImdbID       string
    JellyfinTmdbID       string
    JellyfinTvdbID       string
    JellyfinResolvedAt   *time.Time
    AutoLabel            string
    HumanLabelOverride   string
}

type ParseUpdate struct {
    ParseMethod          string
    ParsedTitle          string
    ParsedYear           *int
    ParsedSeason         *int
    ParsedEpisode        *int
    ParserStrippedTokens string
    MediaTypeGuessed     string
}

type OrganizeUpdate struct {
    TargetPath          string
    TargetAt            *time.Time
    ExistingMatchMethod string
    OrganizeOutcome     string
    OrganizeError       string
}

type OutcomeUpdate struct {
    JellyfinItemID     string
    JellyfinImdbID     string
    JellyfinTmdbID     string
    JellyfinTvdbID     string
    JellyfinResolvedAt *time.Time
}

type QueryFilter struct {
    OrganizeOutcome   string
    AutoLabel         string
    AutoLabelIsNull   bool
    ParseMethod       string
    StrippedToken     string
    SourceContains    string
    JellyfinUnresolved bool
    TargetPathNotEmpty bool
    EventAfter        *time.Time
    EventBefore       *time.Time
    Limit             int
}
```

Required methods:

```go
func (m *MediaDB) InsertDecision(d ParseDecision) (int64, error)
func (m *MediaDB) GetDecision(id int64) (*ParseDecision, error)
func (m *MediaDB) UpdateParse(id int64, u ParseUpdate) error
func (m *MediaDB) UpdateOrganize(id int64, u OrganizeUpdate) error
func (m *MediaDB) UpdateOutcome(id int64, u OutcomeUpdate) error
func (m *MediaDB) UpdateAutoLabel(id int64, label string) error
func (m *MediaDB) UpdateHumanOverride(id int64, label string) error
func (m *MediaDB) QueryDecisions(f QueryFilter) ([]*ParseDecision, error)
func (m *MediaDB) GetUnresolvedDecisionByTargetPath(targetPath string) (*ParseDecision, error)
```

Implementation requirements:

- Use `m.mu.Lock()` for writes and `m.mu.RLock()` for reads.
- Scan nullable text columns through `sql.NullString` and convert invalid values to `""`.
- Scan nullable ints through `sql.NullInt64`.
- Scan nullable times through `sql.NullTime`.
- `GetUnresolvedDecisionByTargetPath` must query:
  - `target_path = ?`
  - `organize_outcome = 'success'`
  - `jellyfin_resolved_at IS NULL`
  - order by `target_at DESC, event_at DESC, id DESC`
  - limit 1
- Query helpers must never string-concatenate user values into SQL; use placeholders.

**Step 3: Run tests**

Run:

```bash
go test ./internal/database -run 'TestParseDecision|TestQueryDecisions|TestGetUnresolvedDecisionByTargetPath' -v
```

Expected: PASS.

**Step 4: Commit**

```bash
git add internal/database/parse_decisions.go internal/database/parse_decisions_test.go
git commit -m "feat(db): add parse decision CRUD"
```

### Task 1.3: Add Verbose Path-Aware Naming Wrappers

**Files:**
- Modify: `internal/naming/naming.go`
- Modify: `internal/naming/deobfuscate.go`
- Modify: `internal/naming/naming_test.go`
- Modify: `internal/daemon/handler_test.go`

**Step 1: Write failing tests**

Add tests for:

- `ParseTVShowNameVerbose("Breaking.Bad.S01E01.1080p.WEB-DL.DDP5.1.H.264-FLUX.mkv")` returns normal TV info plus stripped tokens including quality/codec/group markers.
- `ParseMovieNameVerbose` returns movie info plus stripped tokens.
- `ParseTVShowFromPathVerbose` preserves parent-folder deobfuscation behavior currently covered by daemon path tests.
- Existing `ParseTVShowName`, `ParseMovieName`, `ParseTVShowFromPath`, and `ParseMovieFromPath` signatures still compile.

Run:

```bash
go test ./internal/naming -run 'TestParse.*Verbose|Test.*Obfuscated' -v
```

Expected: FAIL because verbose wrappers do not exist.

**Step 2: Implement verbose wrappers**

Implement:

```go
func ParseTVShowNameVerbose(filename string) (*TVShowInfo, []string, error)
func ParseMovieNameVerbose(filename string) (*MovieInfo, []string, error)
func ParseTVShowFromPathVerbose(path string) (*TVShowInfo, []string, error)
func ParseMovieFromPathVerbose(path string) (*MovieInfo, []string, error)
```

Refactor non-verbose functions to call verbose variants and discard tokens.

When serializing tokens later, use:

```go
b, err := json.Marshal(stripped)
if err == nil {
    update.ParserStrippedTokens = string(b)
}
```

Do not use `string(json.Marshal(stripped))`.

**Step 3: Run tests**

Run:

```bash
go test ./internal/naming ./internal/daemon -run 'TestParse.*Verbose|Test.*Obfuscated' -v
```

Expected: PASS.

**Step 4: Commit**

```bash
git add internal/naming/naming.go internal/naming/deobfuscate.go internal/naming/naming_test.go internal/daemon/handler_test.go
git commit -m "feat(naming): expose verbose path-aware parse metadata"
```

### Task 1.4: Wire Parse Decisions Through Regex and AI Daemon Paths

**Files:**
- Modify: `internal/daemon/handler.go`
- Modify: `internal/daemon/handler_test.go`

**Step 1: Write failing tests**

Add tests for:

- A regex TV file writes one decision row with parse columns and organize columns.
- A regex movie file writes one decision row.
- A file queued for AI stores `ParseDecisionID` on `PendingItem`.
- `applyAIResult` updates the same decision row with `parse_method = "ai"` and organize outcome.
- `organizeWithRegexFallback` updates the same decision row with fallback organize outcome.
- A failed organize writes `organize_outcome = "failed"` and `organize_error`.

Run:

```bash
go test ./internal/daemon -run 'TestProcessFile_WritesParseDecision|TestAI.*ParseDecision|TestRegexFallback.*ParseDecision' -v
```

Expected: FAIL.

**Step 2: Add decision ID plumbing**

Change `PendingItem` in `internal/daemon/handler.go` to include:

```go
ParseDecisionID int64
```

Create a small helper in `MediaHandler`:

```go
func (h *MediaHandler) updateDecisionOrganize(id int64, result *organizer.OrganizationResult, err error)
```

This helper must:

- no-op if `h.db == nil` or `id == 0`
- set `TargetAt` only when a result was attempted
- write `success`, `skipped`, or `failed`
- preserve target path for success/skipped where available
- log DB update failures as warnings

**Step 3: Create rows in `processFile`**

Inside `processFile`, after `_UNPACK_`, pending cleanup, dry-run check, and media type availability checks, insert:

```go
decisionID, err := h.db.InsertDecision(...)
```

Use `filepath.Base(path)` and `time.Now().UTC()`.

Clarify in comments that this is one debounced processing attempt, not one raw fsnotify event.

**Step 4: Update parse columns before AI early returns**

Use `ParseTVShowFromPathVerbose` and `ParseMovieFromPathVerbose`.

After a successful regex parse, call `UpdateParse` before `shouldQueueForAI` returns. When queuing AI, pass `decisionID` into `PendingItem`.

**Step 5: Update organize columns everywhere**

Call `updateDecisionOrganize`:

- after regex organize returns
- in the error path before returning
- in `applyAIResult`
- in `organizeWithRegexFallback`

For AI success, call `UpdateParse` with `ParseMethod: activity.MethodAI` or literal `"ai"` consistently with existing activity constants.

**Step 6: Run tests**

Run:

```bash
go test ./internal/daemon -run 'TestProcessFile_WritesParseDecision|TestAI.*ParseDecision|TestRegexFallback.*ParseDecision' -v
go test ./internal/daemon ./internal/database ./internal/naming
```

Expected: PASS.

**Step 7: Commit**

```bash
git add internal/daemon/handler.go internal/daemon/handler_test.go
git commit -m "feat(daemon): record parse decisions across regex and AI paths"
```

---

## Chunk 2: Dedup by Canonical Episode Key

### Task 2.1: Add Video-Only Episode-Key Helpers

**Files:**
- Create: `internal/organizer/dedup.go`
- Create: `internal/organizer/dedup_test.go`

**Step 1: Write failing tests**

Test:

- `S02E19`
- lowercase `s01e05`
- `2x07`
- date-based filenames if `naming.ParseTVShowName` maps them to a non-zero season/episode
- no marker returns false
- subtitle-only match is ignored by `FindEpisodeFile`
- when `.srt` and `.mkv` both exist, `.mkv` is selected

Run:

```bash
go test ./internal/organizer -run 'TestExtractEpisodeKey|TestFindEpisodeFile' -v
```

Expected: FAIL.

**Step 2: Implement helpers**

Create:

```go
func ExtractEpisodeKey(filename string) (season, episode int, ok bool)
func FindEpisodeFile(seasonDir string, season, episode int) (string, bool)
```

Implementation requirements:

- Prefer `naming.ParseTVShowName(filename)` first so date-based shows that already map to canonical season/episode are covered.
- Fall back to simple `SxxExx` and `NxNN` regex only if naming parse fails.
- `FindEpisodeFile` must ignore directories and non-video extensions.
- Use the same video extension set as `findExistingMediaFile`, or factor a shared helper in `dedup.go`.

**Step 3: Run tests**

Run:

```bash
go test ./internal/organizer -run 'TestExtractEpisodeKey|TestFindEpisodeFile' -v
```

Expected: PASS.

**Step 4: Commit**

```bash
git add internal/organizer/dedup.go internal/organizer/dedup_test.go
git commit -m "feat(organizer): add video-only episode-key lookup"
```

### Task 2.2: Replace TV Dedup Sites

**Files:**
- Modify: `internal/organizer/organizer.go`
- Modify: `internal/organizer/organizer_test.go`

**Step 1: Write failing tests**

Add tests for both `OrganizeTVEpisode` and `OrganizeTVWithParsed`:

- Existing file has different case/year suffix but same episode key.
- Existing subtitle with same episode key does not count as the existing media file.
- `forceOverwrite` removes/replaces the same episode file even when the normalized target filename differs.
- Date-based episode does not duplicate if parser yields canonical season/episode.

Run:

```bash
go test ./internal/organizer -run 'TestOrganizeTV.*Dedup|TestOrganizeTV.*ForceOverwrite|TestOrganizeTV.*DateBased' -v
```

Expected: FAIL.

**Step 2: Replace both dedup blocks**

At both `findExistingMediaFile(seasonDir)` TV call sites:

- Use `FindEpisodeFile(seasonDir, tv.Season, tv.Episode)` to identify same-episode media.
- Do this regardless of `forceOverwrite`; force overwrite should skip quality comparison, not skip same-episode cleanup.
- If not force overwrite, keep existing quality comparison.
- If force overwrite and existing path differs from target, remove the old same-episode file before transfer.
- Do not remove unrelated episodes.
- Set `ExistingQuality` from `quality.Parse(filepath.Base(existingFile))`.

**Step 3: Run tests**

Run:

```bash
go test ./internal/organizer -run 'TestOrganizeTV.*Dedup|TestOrganizeTV.*ForceOverwrite|TestOrganizeTV.*DateBased' -v
go test ./internal/organizer/...
```

Expected: PASS.

**Step 4: Commit**

```bash
git add internal/organizer/organizer.go internal/organizer/organizer_test.go
git commit -m "fix(organizer): dedup TV episodes by canonical episode key"
```

---

## Chunk 3: Cleanup Allowlist and Subtitle Copy

### Task 3.1: Add Strict Allowlist Purge Helper

**Files:**
- Create: `internal/organizer/cleanup.go`
- Create: `internal/organizer/cleanup_test.go`

**Step 1: Write failing tests**

Test:

- video extensions survive
- subtitle extensions survive
- `.exe`, `.nfo`, `.jpg`, `.txt`, and dot placeholders are removed
- nested empty directories are removed
- unreadable path errors do not abort the whole purge

Run:

```bash
go test ./internal/organizer -run TestPurgeNonAllowed -v
```

Expected: FAIL.

**Step 2: Implement `PurgeNonAllowed`**

Create:

```go
func PurgeNonAllowed(dir string) error
```

Requirements:

- Allow only video and subtitle extensions.
- Walk deepest-first.
- Remove disallowed files.
- Remove directories only if they are empty.
- Do not use `os.RemoveAll` on directories containing unknown contents.

**Step 3: Run tests and commit**

```bash
go test ./internal/organizer -run TestPurgeNonAllowed -v
git add internal/organizer/cleanup.go internal/organizer/cleanup_test.go
git commit -m "feat(organizer): add strict cleanup allowlist"
```

### Task 3.2: Wire Cleanup Without Removing Safety Gates

**Files:**
- Modify: `internal/daemon/handler.go`
- Modify: `internal/daemon/handler_test.go`

**Step 1: Write failing tests**

Add tests that prove:

- If `sourcePath` still exists, cleanup returns without deleting sibling junk.
- After a successful move, junk in the release directory is removed.
- If another video remains in the release directory, the release directory is not removed.
- Parent directories containing other active release folders are not recursively purged.
- Subtitle-only leftovers are preserved by the allowlist.

Run:

```bash
go test ./internal/daemon -run 'TestCleanup.*Allowlist|TestCleanup.*SourceStillExists|TestCleanup.*Parent' -v
```

Expected: FAIL for allowlist-specific cases only; existing source-present behavior should already pass.

**Step 2: Update `cleanupSourceDir` conservatively**

Keep the existing source-present gate:

```go
if _, err := os.Stat(sourcePath); err == nil {
    return
}
```

Then:

- For the starting release directory only, call `organizer.PurgeNonAllowed(dir)`.
- If video files remain after purge, return.
- If no video files remain, try `os.Remove(dir)`.
- Walk up parents using `os.Remove(parent)` only for empty directories.
- Do not call `PurgeNonAllowed` on parent directories.

This preserves cleanup boundaries and avoids deleting junk from unrelated active downloads.

**Step 3: Run tests and commit**

```bash
go test ./internal/daemon -run 'TestCleanup.*Allowlist|TestCleanup.*SourceStillExists|TestCleanup.*Parent' -v
go test ./internal/daemon ./internal/organizer
git add internal/daemon/handler.go internal/daemon/handler_test.go
git commit -m "feat(daemon): use allowlist cleanup within release boundaries"
```

### Task 3.3: Copy Only Stem-Matching Subtitles

**Files:**
- Modify: `internal/organizer/organizer.go`
- Modify: `internal/organizer/organizer_test.go`

**Step 1: Write failing test**

Test `copySubtitles` with:

- `Show.S01E01.srt` copied
- `Show.S01E01.eng.srt` copied
- `OtherShow.S01E01.srt` not copied
- `random.srt` not copied

Use `analysis.MainMediaFile`, not a nonexistent `analysis.MediaFile`.

Run:

```bash
go test ./internal/organizer -run TestCopySubtitles_OnlyStemMatching -v
```

Expected: FAIL.

**Step 2: Implement stem matching**

In `copySubtitles`, compute the video stem from `analysis.MainMediaFile.Name`. Strip one short language/flag suffix from subtitle stems, for example `.eng`, `.en`, `.forced`, `.sdh`. Only copy when the remaining subtitle stem equals the video stem case-insensitively.

**Step 3: Run tests and commit**

```bash
go test ./internal/organizer -run TestCopySubtitles_OnlyStemMatching -v
git add internal/organizer/organizer.go internal/organizer/organizer_test.go
git commit -m "feat(organizer): copy only stem-matching subtitles"
```

---

## Chunk 4: Jellyfin Outcome Plumbing

### Task 4.1: Normalize Flat and Companion Plugin Webhook Payloads

**Files:**
- Modify: `internal/jellyfin/webhook_types.go`
- Modify: `internal/daemon/server_test.go`
- Modify: `internal/api/webhooks_test.go`

**Step 1: Write failing HTTP-level tests**

Add tests for:

- Existing flat payload:
  - `NotificationType`
  - `ItemId`
  - `ItemPath`
  - `Provider_imdb`
- Companion-plugin nested payload:
  - `eventType`
  - `timestamp`
  - `payload.item.id`
  - `payload.item.path`
  - `payload.item.name`
  - `payload.item.type`
  - `payload.item.providerIds.Imdb/Tmdb/Tvdb`

Tests should POST to the daemon webhook handler and assert `HandleJellyfinWebhookEvent` receives a normalized `jellyfin.WebhookEvent`.

Run:

```bash
go test ./internal/daemon -run 'TestJellyfinWebhook.*Payload' -v
go test ./internal/api -run 'TestWebhook.*Payload' -v
```

Expected: FAIL for nested payload.

**Step 2: Implement custom normalization**

In `internal/jellyfin/webhook_types.go`, add a custom `UnmarshalJSON` on `WebhookEvent` or a `NormalizeWebhookPayload` helper used by both daemon and API servers.

Requirements:

- Preserve existing flat payload compatibility.
- Map companion plugin `eventType` to `NotificationType`.
- Map nested provider IDs into `ProviderImdb`, `ProviderTmdb`, `ProviderTvdb`.
- Keep unknown event types as strings; do not reject them.

**Step 3: Run tests and commit**

```bash
go test ./internal/daemon -run 'TestJellyfinWebhook.*Payload' -v
go test ./internal/api -run 'TestWebhook.*Payload' -v
git add internal/jellyfin/webhook_types.go internal/daemon/server_test.go internal/api/webhooks_test.go
git commit -m "feat(jellyfin): normalize webhook payload formats"
```

### Task 4.2: Update Parse Decisions From Production Daemon Webhooks

**Files:**
- Modify: `internal/daemon/handler.go`
- Modify: `internal/daemon/handler_test.go`
- Modify: `internal/api/webhooks.go`
- Modify: `internal/api/webhooks_test.go`

**Step 1: Write failing tests**

Add tests that:

- Seed a `parse_decisions` row with `organize_outcome = "success"` and `target_path`.
- Send `ItemAdded` through `MediaHandler.HandleJellyfinWebhookEvent`.
- Assert `jellyfin_item_id`, provider IDs, and `jellyfin_resolved_at` are updated.
- Seed a resolved row and an unresolved row with same target path; assert only unresolved row is updated.
- Keep existing `jellyfin_items` upsert behavior.

Run:

```bash
go test ./internal/daemon -run TestHandleJellyfinWebhookEvent_ItemAddedUpdatesParseDecision -v
go test ./internal/api -run TestHandleItemAdded_UpdatesParseDecision -v
```

Expected: FAIL.

**Step 2: Implement shared update logic**

In both daemon and API webhook paths, after successful `UpsertJellyfinItem`, call:

```go
dec, err := db.GetUnresolvedDecisionByTargetPath(path)
```

If found, update outcome with:

- item ID
- provider IDs
- `time.Now().UTC()`

Ignore `sql.ErrNoRows`; warn on other errors. Do not return early from webhook handling if parse-decision update fails after `jellyfin_items` was persisted.

**Step 3: Run tests and commit**

```bash
go test ./internal/daemon -run TestHandleJellyfinWebhookEvent_ItemAddedUpdatesParseDecision -v
go test ./internal/api -run TestHandleItemAdded_UpdatesParseDecision -v
git add internal/daemon/handler.go internal/daemon/handler_test.go internal/api/webhooks.go internal/api/webhooks_test.go
git commit -m "feat(jellyfin): correlate item-added events to parse decisions"
```

### Task 4.3: Add Paginated Jellyfin Sweep

**Files:**
- Modify: `internal/jellyfin/items.go`
- Create: `internal/jellyfin/sweep.go`
- Create: `internal/jellyfin/sweep_test.go`

**Step 1: Write failing tests**

Tests:

- Row one hour old with target path is swept when lookback is 24h.
- Row older than lookback is skipped by normal sweep.
- Unresolved row older than TTL is labeled `FAIL`.
- Pagination follows `TotalRecordCount` and `StartIndex`.
- HTTP/API errors return errors and do not mark rows `DRIFT`/`FAIL` incorrectly.

Run:

```bash
go test ./internal/jellyfin -run 'TestSweep_' -v
```

Expected: FAIL.

**Step 2: Add client page helper**

In `internal/jellyfin/items.go`, add a package method on existing `Client`:

```go
func (c *Client) ListItemsPage(startIndex, limit int) (*ItemsResponse, error)
```

Use `/Items` with:

- `Recursive=true`
- `IncludeItemTypes=Episode,Movie`
- `Fields=Path,ProviderIds`
- `StartIndex`
- `Limit`

**Step 3: Implement sweeper**

Create:

```go
type Sweeper struct {
    client *Client
    db     *database.MediaDB
}

func NewSweeper(client *Client, db *database.MediaDB) *Sweeper
func (s *Sweeper) RunOnce(lookback, ttl time.Duration) error
```

Query normal sweep rows with:

```go
database.QueryFilter{
    JellyfinUnresolved: true,
    TargetPathNotEmpty: true,
    EventAfter: &since,
    Limit: 500,
}
```

TTL rows use:

```go
database.QueryFilter{
    JellyfinUnresolved: true,
    TargetPathNotEmpty: true,
    EventBefore: &ttlCutoff,
    AutoLabelIsNull: true,
    Limit: 1000,
}
```

**Step 4: Run tests and commit**

```bash
go test ./internal/jellyfin -run 'TestSweep_' -v
git add internal/jellyfin/items.go internal/jellyfin/sweep.go internal/jellyfin/sweep_test.go
git commit -m "feat(jellyfin): sweep unresolved parse decisions"
```

### Task 4.4: Wire Sweeper in Daemon Main

**Files:**
- Modify: `cmd/jellywatchd/main.go`

**Step 1: Wire only when client is live**

Start sweeper only when:

```go
jellyfinClient != nil && db != nil
```

Do not gate only on config. Current daemon disables `jellyfinClient` after ping failure; respect that.

Run the first sweep after a short delay and then every 6h. Use the existing `ctx`.

**Step 2: Build**

Run:

```bash
go build ./cmd/jellywatchd
```

Expected: PASS.

**Step 3: Commit**

```bash
git add cmd/jellywatchd/main.go
git commit -m "feat(daemon): start Jellyfin parse-decision sweeper"
```

---

## Chunk 5: Auto-Labeling

### Task 5.1: Fuzzy Title Compare Without Naive Substrings

**Files:**
- Create: `internal/labeling/fuzzy.go`
- Create: `internal/labeling/fuzzy_test.go`

**Step 1: Write tests**

Required cases:

```go
{"Tracker", "tracker", true}
{"The Daily Show with Trevor Noah", "The Daily Show", true}
{"Outcome AAC5 1", "Outcome", false}
{"the dreadful aac5 1 bz", "The Dreadful", false}
{"Marvel's Daredevil", "Marvels Daredevil", true}
{"X-Men", "x men", true}
```

Run:

```bash
go test ./internal/labeling -run TestFuzzyTitleEqual -v
```

Expected: FAIL.

**Step 2: Implement**

Rules:

- Normalize case and punctuation into tokens.
- Exact token equality returns true.
- Allow one title to extend the other only when the extra token sequence starts with `with`; this handles canonical title variants like `The Daily Show with Trevor Noah`.
- Do not use arbitrary substring matching.

**Step 3: Run tests and commit**

```bash
go test ./internal/labeling -run TestFuzzyTitleEqual -v
git add internal/labeling/fuzzy.go internal/labeling/fuzzy_test.go
git commit -m "feat(labeling): add conservative fuzzy title compare"
```

### Task 5.2: Derive Labels

**Files:**
- Create: `internal/labeling/labeler.go`
- Create: `internal/labeling/labeler_test.go`

**Step 1: Write tests**

Test:

- Provider ID plus fuzzy title match -> `PASS`.
- Provider ID plus fuzzy title mismatch -> `DRIFT`.
- No provider ID beyond TTL -> `FAIL`.
- No provider ID inside TTL -> `""`.
- Provider ID with empty Jellyfin name -> `""` rather than `DRIFT`.

**Step 2: Implement**

Create:

```go
func DeriveLabel(dec database.ParseDecision, jellyfinName string, ttl time.Duration) string
```

Only derive `DRIFT` when a provider ID exists and `jellyfinName` is non-empty.

**Step 3: Run tests and commit**

```bash
go test ./internal/labeling -run TestDeriveLabel -v
git add internal/labeling/labeler.go internal/labeling/labeler_test.go
git commit -m "feat(labeling): derive parse decision labels"
```

### Task 5.3: Background Labeler Runner

**Files:**
- Create: `internal/labeling/runner.go`
- Create: `internal/labeling/runner_test.go`
- Modify: `cmd/jellywatchd/main.go`

**Step 1: Write tests**

Test:

- Runner queries only `AutoLabelIsNull` rows.
- Runner labels all unlabeled rows even when there are more than 1000 labeled newer rows.
- `getName` error skips that row and returns/logs an error; it must not write `DRIFT`.
- `UpdateAutoLabel` errors are returned.

**Step 2: Implement runner**

Create:

```go
type JellyfinNameFetcher func(itemID string) (string, error)
type Runner struct { ... }
func NewRunner(db *database.MediaDB, getName JellyfinNameFetcher) *Runner
func (r *Runner) RunOnce() error
```

Use:

```go
database.QueryFilter{AutoLabelIsNull: true, Limit: 1000}
```

Loop until a page returns fewer than the limit, or add offset support if needed. Do not skip labeled rows in Go as the primary filter.

**Step 3: Wire daemon ticker**

Only start when `jellyfinClient != nil && db != nil`.

Fetcher:

```go
item, err := jellyfinClient.GetItem(itemID)
return item.Name, err
```

Run every 15m after an initial short delay.

**Step 4: Run tests and commit**

```bash
go test ./internal/labeling -run TestRunner -v
go build ./cmd/jellywatchd
git add internal/labeling/runner.go internal/labeling/runner_test.go cmd/jellywatchd/main.go
git commit -m "feat(daemon): label parse decisions in background"
```

---

## Chunk 6: CLI and Corpus Harness

### Task 6.1: Add `jellywatch parses` Command

**Files:**
- Create: `cmd/jellywatch/parses_cmd.go`
- Create: `cmd/jellywatch/parses_cmd_test.go`
- Modify: `cmd/jellywatch/main.go`

**Step 1: Implement testable command constructor**

Create:

```go
func newParsesCmd() *cobra.Command
func newParsesCmdWithDeps(openDB func() (*database.MediaDB, error), stdout, stderr io.Writer) *cobra.Command
```

`newParsesCmd` must use:

```go
dbPath, err := paths.DatabasePath()
db, err := database.OpenPath(dbPath)
```

Register inside `main()` beside the other `rootCmd.AddCommand(...)` calls.

**Step 2: Write CLI tests**

Tests must execute the Cobra command with temp DB and captured output:

- `--failures` returns only `FAIL`.
- `--drift` returns only `DRIFT`.
- `--source-contains` filters filenames.
- `--override <id> --label wrong` updates `human_label_override`.
- Invalid label returns error.

Do not use `setupTestDB` from `internal/database`; it is package-local. Open a temp DB with `database.OpenPath`.

**Step 3: Run tests and commit**

```bash
go test ./cmd/jellywatch -run TestParsesCmd -v
go build ./cmd/jellywatch
git add cmd/jellywatch/parses_cmd.go cmd/jellywatch/parses_cmd_test.go cmd/jellywatch/main.go
git commit -m "feat(cli): add parse decision query command"
```

### Task 6.2: Add Deterministic Corpus Harness

**Files:**
- Create: `internal/naming/corpus_test.go`
- Create: `internal/naming/testdata/parse_decisions_corpus.jsonl`

**Step 1: Add anonymized fixture**

Create a small hand-authored fixture under package testdata. Include synthetic names only:

```jsonl
{"source_path":"/watch/tv/Tracker.2024.S02E19.1080p.mkv","source_filename":"Tracker.2024.S02E19.1080p.mkv","media_type":"tv","parsed_title":"Tracker","parsed_year":2024,"parsed_season":2,"parsed_episode":19}
{"source_path":"/watch/movies/Example.Movie.2024.1080p.mkv","source_filename":"Example.Movie.2024.1080p.mkv","media_type":"movie","parsed_title":"Example Movie","parsed_year":2024}
```

Do not commit raw live DB exports. If a larger corpus is desired later, add an anonymizer script and deterministic `ORDER BY`.

**Step 2: Write harness**

Create `internal/naming/corpus_test.go` in package `naming`.

Use `ParseTVShowFromPath` / `ParseMovieFromPath` so path-aware parsing is exercised. Compare title, year, season, and episode.

**Step 3: Run tests and commit**

```bash
go test ./internal/naming -run TestParserCorpusRegression -v
git add internal/naming/corpus_test.go internal/naming/testdata/parse_decisions_corpus.jsonl
git commit -m "test(naming): add deterministic parser corpus harness"
```

---

## Verification

Run targeted tests after each task. Before claiming the branch is complete, run:

```bash
go test ./internal/database ./internal/naming ./internal/daemon ./internal/organizer ./internal/jellyfin ./internal/labeling ./internal/api ./cmd/jellywatch
go build ./cmd/jellywatch
go build ./cmd/jellywatchd
```

Do not run the real daemon against the default user config as a migration smoke test. Use package tests and temp DBs only.

## Execution Handoff

Plan rewritten and saved to `docs/superpowers/plans/2026-04-26-parse-decisions-pipeline.md`.

Execution options:

1. **Subagent-driven in this session:** dispatch one worker per chunk with disjoint ownership, then review between chunks.
2. **Sequential executing-plans session:** implement chunk-by-chunk with checkpoints.

Recommended: option 1 for Chunks 2/3 in parallel with Chunk 1 DB work; keep Chunks 4-6 sequential after Chunk 1 lands.
