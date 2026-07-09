# Daemon AI Enhancement — Design Spec

**Date:** 2026-03-16
**Status:** Approved
**Problem:** The daemon (`plex2jellyfin-daemon`) organizes files using regex-only parsing, which produces data quality issues: missing apostrophes, missing years, title disambiguation failures. The AI infrastructure (`ai.Integrator`, `ai.Matcher`, Ollama Cloud) is fully built but only wired into manual CLI commands (`plex2jellyfin scan`, `plex2jellyfin audit`), not the live daemon path.

**Goal:** Wire AI into the daemon's file organization pipeline to automatically fix low-confidence regex parses, while being conservative with cloud API calls and maintaining daemon reliability.

---

## Design Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Sync vs async | Async — organize immediately, AI in background | Daemon must rename/move in real time |
| Correction strategy | Don't organize until confident | Avoids risky rename-in-place logic |
| AI backend | Ollama Cloud (configured during install) | Cloud-based — real API costs, needs rate limiting |
| Rate limiting | Per-hour burst (10) + daily cap (50) | Belt and suspenders for cloud costs |
| Safety valve | Hybrid: change category + confidence threshold | Predictable auto-apply rules, risky changes flagged |
| Logging | Dedicated JSONL file | Easy to inspect, grep, tail |
| Review workflow | JSONL log + `plex2jellyfin review` CLI | Web UI can come later |

---

## Architecture

### 1. Unified Parse Path

**Problem:** Handler calls `naming.ParseTVShowName()` for logging, then organizer calls `naming.ParseTVShowFromPath()` again internally. Two parses, potential disagreement.

**Solution:** Add new organizer methods that accept the existing `naming.TVShowInfo` / `naming.MovieInfo` structs directly, rather than introducing a new type:

```go
// internal/organizer/organizer.go

// Reuses existing naming.TVShowInfo and naming.MovieInfo — no new types needed.
// The handler's single parse result flows through to the organizer unchanged.

func (o *Organizer) OrganizeTVWithParsed(sourcePath string, info *naming.TVShowInfo, sizeFunc func(string) (int64, error)) (*OrganizationResult, error)
func (o *Organizer) OrganizeMovieWithParsed(sourcePath string, info *naming.MovieInfo, targetLib string) (*OrganizationResult, error)
```

These methods skip the internal re-parse and use the provided info directly for folder creation, filename formatting, and library selection. They return `*OrganizationResult` (same as existing methods) so the handler can use the result for stats, logging, and notifications. Existing `OrganizeTVEpisodeAuto` / `OrganizeMovie` methods remain unchanged for backward compatibility (CLI commands still use them).

The `sizeFunc` parameter matches the existing closure pattern used by `OrganizeTVEpisodeAuto` — `func(string) (int64, error)`. `OrganizeTVWithParsed` still performs library selection internally via `selector.SelectTVShowLibrary(info.Title, info.Year, size)` — it only skips the re-parse, not the library selection. For movies, the handler pre-selects the library (same as current behavior) and passes it as `targetLib`.

**Handler parse functions:** The handler must use path-aware parsers to preserve deobfuscation fallback:
- TV: `naming.ParseTVShowName(filepath.Base(path))` (filename-based, same as current)
- Movies: `naming.ParseMovieFromPath(path)` (path-based, falls back to folder name for obfuscated filenames) — this replaces the current `naming.ParseMovieName()` call in the handler to match the organizer's existing behavior

### 2. Two-Lane Processing

The handler gains a ticker goroutine for processing the slow lane. This is a single `time.Ticker` started in `cmd/plex2jellyfin-daemon/main.go` alongside the existing watcher and periodic scanner goroutines — same pattern, same lifecycle management.

**Fast lane** (confidence >= `AutoTriggerThreshold`, default 0.6):
- Handler parses with `naming.ParseTVShowName()` (TV) / `naming.ParseMovieFromPath()` (movies)
- Computes `naming.CalculateTitleConfidence(title, filename)`
- Confidence is adequate — calls `OrganizeTVWithParsed()` / `OrganizeMovieWithParsed()` immediately
- Logs to JSONL: `{"action": "fast_lane", "confidence": 0.85, ...}`
- Identical speed to current behavior

**Slow lane** (confidence < 0.6):
- Handler parses filename — confidence too low
- File stays in watch directory, untouched
- Added to `pendingAI` map on the handler struct: `map[string]PendingItem`
- Logs to JSONL: `{"action": "queued_for_ai", "confidence": 0.42, ...}`

```go
// Added to handler.go

type PendingItem struct {
    Path       string
    Filename   string
    TVInfo     *naming.TVShowInfo  // set if TV episode
    MovieInfo  *naming.MovieInfo   // set if movie
    MediaType  string              // "tv" or "movie"
    Confidence float64
    QueuedAt   time.Time
    TargetLib  string              // for movies: which library to use
}
```

**Pending map cap:** Maximum 100 items. If the cap is reached, new low-confidence files are organized immediately with regex (fast-lane fallback) and logged with `"action": "pending_cap_reached"`. This prevents unbounded memory growth during bulk imports of poorly-named files.

**Ticker** (runs every 30s, configurable via `[ai].enhancement_interval_seconds`):
- Iterates `pendingAI`, respecting rate limits
- For each item: calls `ai.Matcher.ParseWithRetry()` (NOT `EnhanceTitle`, because we need the full `*ai.Result` including `Confidence`, `Year`, `Season`, `Episodes`)
- Classifies the change via `ClassifyChange()`
- If safe + confident enough: organizes file with AI metadata via `*WithParsed()` methods
- If risky or low confidence: logs as flagged for review, removes from pending
- If rate limit hit: leaves item in pending for next tick
- If source file no longer exists (user deleted it): removes from pending, logs

**Pending item expiry:** Items older than 24 hours are removed from pending and logged as expired. Prevents stale items accumulating if Ollama Cloud is down.

**Restart recovery:** On daemon startup, the handler scans the JSONL log for unresolved `queued_for_ai` entries (no corresponding `ai_enhanced`, `flagged_for_review`, or `expired` entry). For each, it checks if the source file still exists in the watch directory. If so, it re-queues the item. If not (file was manually moved or deleted), it appends an `"action": "lost_on_restart"` entry. This prevents orphaned log entries.

### 3. Safety Valve — Change Classification

A pure function that compares regex and AI results:

```go
// internal/daemon/classify.go

type ChangeCategory string

const (
    ChangePunctuation   ChangeCategory = "punctuation"
    ChangeCasing        ChangeCategory = "casing"
    ChangeYearAdded     ChangeCategory = "year_added"
    ChangeYearCorrected ChangeCategory = "year_corrected"
    ChangeTitleDifferent ChangeCategory = "different_title"
    ChangeTypeDifferent ChangeCategory = "type_change"
    ChangeStructural    ChangeCategory = "structural"  // season/episode change
)

type ChangeClassification struct {
    Category      ChangeCategory
    Safe          bool
    MinConfidence float64
}

func ClassifyChange(regexTitle, aiTitle, regexYear, aiYear string, regexMediaType, aiMediaType string) ChangeClassification
```

**Auto-apply thresholds (safe changes):**

| Category | Min AI Confidence |
|---|---|
| Punctuation fix | 0.80 |
| Casing correction | 0.80 |
| Year addition | 0.85 |
| Year correction | 0.90 |

**Always flag for review (risky changes):**
- Different title entirely (`ChangeTitleDifferent`)
- Type change movie <-> tv (`ChangeTypeDifferent`)
- Season/episode change (`ChangeStructural`)

**"Different title" detection:** Uses Jaccard similarity on word sets. Both titles are lowercased and de-punctuated, split into word sets. If `|intersection| / |union| < 0.70`, it's classified as a different title. Example: "Freddys Nightmares" vs "Freddy's Nightmares" → {"freddys","nightmares"} vs {"freddys","nightmares"} (after de-punctuation) → Jaccard 1.0 → punctuation change. "Weird" vs "Something Completely Different" → Jaccard 0.0 → different title.

### 4. Rate Limiting

```go
// internal/daemon/ratelimit.go

type AIRateLimiter struct {
    hourlyCap    int
    dailyCap     int
    hourlyUsed   int
    dailyUsed    int
    hourlyReset  time.Time
    dailyReset   time.Time
    mu           sync.Mutex
}

func NewAIRateLimiter(hourlyCap, dailyCap int) *AIRateLimiter
func (r *AIRateLimiter) Allow() bool    // checks both caps, auto-resets on window rollover
func (r *AIRateLimiter) Record()        // increments counters after a successful AI call
func (r *AIRateLimiter) Stats() (hourlyUsed, dailyUsed int)  // for logging
```

**Config:**

```toml
[ai]
enabled = true
hourly_limit = 10        # max AI calls per hour
daily_limit = 50          # max AI calls per day
enhancement_interval_seconds = 30  # ticker interval for processing pending items
```

**Defaults:** 10/hour, 50/day. Conservative for cloud API. User can increase if their Ollama Cloud plan allows it.

When limits are hit, pending items stay queued. The ticker logs a rate limit event and moves on.

### 5. JSONL Enhancement Log

**Location:** `~/.config/plex2jellyfin/ai-enhancements.jsonl`

**Schema:** Single JSON object per line, differentiated by `action` field:

```json
{"ts":"2026-03-16T14:30:00Z","action":"fast_lane","file":"Show.S01E05.1080p.mkv","regex_title":"Show","confidence":0.85}
{"ts":"2026-03-16T14:30:01Z","action":"queued_for_ai","file":"freddys.nightmares.s01e01.mkv","regex_title":"freddys nightmares","confidence":0.38}
{"ts":"2026-03-16T14:30:31Z","action":"ai_enhanced","file":"freddys.nightmares.s01e01.mkv","regex_title":"freddys nightmares","ai_title":"Freddy's Nightmares","ai_confidence":0.92,"category":"punctuation","auto_applied":true}
{"ts":"2026-03-16T14:30:32Z","action":"flagged_for_review","file":"weird.file.mkv","regex_title":"Weird","ai_title":"Something Completely Different","ai_confidence":0.88,"category":"different_title","reason":"risky change"}
{"ts":"2026-03-16T14:31:00Z","action":"rate_limited","pending_count":3,"hourly_used":10,"daily_used":34}
{"ts":"2026-03-16T14:31:30Z","action":"expired","file":"old.file.mkv","reason":"pending > 24h"}
{"ts":"2026-03-16T15:00:00Z","action":"review_approved","file":"weird.file.mkv","applied_title":"Something Completely Different"}
{"ts":"2026-03-16T15:00:01Z","action":"review_rejected","file":"other.mkv","reason":"user rejected"}
```

**Log writer:**

```go
// internal/daemon/enhancelog.go

type EnhanceLogger struct {
    path       string
    maxSize    int64  // 10MB default
    maxBackups int    // 3 default
}

func NewEnhanceLogger(dir string) *EnhanceLogger
func (l *EnhanceLogger) Log(entry EnhanceLogEntry) error
```

**Rotation:** On each write, checks file size. If > 10MB, renames current file to `.1` (shifting existing backups), creates new file. Keeps last 3 backups. No external dependencies.

### 6. `plex2jellyfin review` Command

```
$ plex2jellyfin review
3 items flagged for review:

1. weird.file.mkv
   Regex: "Weird" (confidence: 0.35)
   AI suggests: "Something Completely Different" (confidence: 0.88)
   Category: different_title
   [a]pprove  [r]eject  [s]kip

2. another.file.mkv
   Regex: "Another" (confidence: 0.42)
   AI suggests: "Another Show (2024)" (confidence: 0.91)
   Category: year_added
   [a]pprove  [r]eject  [s]kip
```

- Reads `ai-enhancements.jsonl`, filters `action == "flagged_for_review"` entries that don't have a corresponding `review_approved` or `review_rejected` entry
- **Conflict detection:** Before applying, checks if the source file still exists in the watch directory. If it's gone (already organized by a re-download, or manually moved), shows status and skips
- On approve: calls `OrganizeTVWithParsed` / `OrganizeMovieWithParsed` with the AI metadata, appends `review_approved` to log
- On reject: appends `review_rejected` to log
- Non-interactive mode: `plex2jellyfin review --list` (just prints, no prompts)
- **Organizer initialization:** The review command constructs an `Organizer` using the same config-based setup as the daemon (library paths, transfer backend, etc.), loaded from `config.toml`

---

## Files Changed

| File | Change |
|---|---|
| `internal/organizer/organizer.go` | Add `OrganizeTVWithParsed()`, `OrganizeMovieWithParsed()` accepting `naming.*Info` types |
| `internal/daemon/handler.go` | Add `pendingAI` map (cap 100), confidence gating in `processFile()`, AI-related fields to `MediaHandlerConfig` |
| `internal/daemon/ratelimit.go` | **New file.** `AIRateLimiter` struct |
| `internal/daemon/classify.go` | **New file.** `ClassifyChange()` function, change category types |
| `internal/daemon/enhancelog.go` | **New file.** JSONL writer with rotation |
| `cmd/plex2jellyfin/review_cmd.go` | **New file.** `plex2jellyfin review` command |
| `cmd/plex2jellyfin-daemon/main.go` | Wire `ai.Matcher` into `MediaHandlerConfig`, start ticker goroutine |
| `internal/config/config.go` | Add `HourlyLimit`, `DailyLimit`, `EnhancementIntervalSeconds` to `AIConfig` struct (with `mapstructure` tags), update `ToTOML()` serialization |

## Files NOT Changed

- `internal/naming/*` — Regex parsing stays as-is
- `internal/ai/*` — Matcher, Cache, Circuit Breaker all reused without modification (ticker calls `Matcher.ParseWithRetry()` directly, wrapped with the existing `scanner.AIHelper` pattern for circuit breaking and caching)
- `internal/scanner/*` — Scanner's AI integration untouched
- Existing organizer methods — Backward compatible, CLI commands unaffected

---

## Config Additions

Three new fields added to the existing `AIConfig` struct with `mapstructure` tags:

```go
// internal/config/config.go — added to AIConfig struct
HourlyLimit                int `mapstructure:"hourly_limit"`
DailyLimit                 int `mapstructure:"daily_limit"`
EnhancementIntervalSeconds int `mapstructure:"enhancement_interval_seconds"`
```

TOML representation:

```toml
[ai]
# Existing fields unchanged
enabled = true
ollama_endpoint = "https://..."
model = "qwen2.5vl:7b"
confidence_threshold = 0.8
auto_trigger_threshold = 0.6

# New fields for daemon enhancement
hourly_limit = 10
daily_limit = 50
enhancement_interval_seconds = 30
```

The `ToTOML()` method and `defaultConfig()` / `testConfig()` functions must also be updated to include these three fields.

---

## Error Handling

- **Ollama Cloud down:** Circuit breaker trips after N failures (existing `CircuitBreakerConfig`). Pending items stay queued. Fast-lane files are unaffected.
- **Pending queue overflow:** Items expire after 24 hours. Logged as `"action": "expired"`.
- **File deleted while pending:** Detected when ticker processes item (`os.Stat` check). Removed from pending, logged.
- **Rate limit exhausted:** Items stay queued for next window. Logged.
- **AI returns garbage:** Confidence will be below threshold, result rejected. Existing Integrator already falls back to regex title when confidence < `ConfidenceThreshold`.

---

## Testing Strategy

- `classify_test.go` — Unit tests for `ClassifyChange()` with every category combination
- `ratelimit_test.go` — Unit tests for `AIRateLimiter` including window rollover
- `enhancelog_test.go` — Unit tests for JSONL writing and rotation
- `handler_test.go` — Integration tests for two-lane routing (mock AI, verify fast/slow lane behavior)
- `organizer_test.go` — Tests for `*WithParsed()` methods ensuring they skip internal re-parse
- Existing tests remain untouched
