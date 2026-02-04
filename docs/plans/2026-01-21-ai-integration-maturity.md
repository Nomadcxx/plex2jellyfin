# AI Integration Maturity

**Date:** 2026-01-21
**Status:** Design Complete

## Overview

Mature the existing AI integration from experimental prototype to production-ready feature. Focus on reliability, observability, and code cleanliness while expanding AI to high-value use cases where regex genuinely struggles.

## Design Philosophy

**AI is a scalpel, not a sledgehammer.**

- AI is triggered only when heuristics have low confidence
- Never block the main pipeline waiting for AI
- Conservative thresholds to avoid false positives
- Background automation over CLI proliferation

## Current State Problems

1. **Dead code**: `internal/naming/ai_naming.go` has unused `ParseMovieNameWithAI()` and `ParseTVShowNameWithAI()`
2. **Duplicate integration paths**: Two conflicting ways to integrate AI
3. **Config mismatch**: AI uses JSON while everything else uses TOML
4. **No retry logic**: AI failures are immediate and final
5. **No circuit breaker**: Failing AI can pound the pipeline
6. **Silent degradation**: Users don't know when AI is unavailable
7. **Blocking calls**: Slow AI blocks file organization
8. **Limited use cases**: AI only used for title parsing

## Architecture Changes

### 1. Background Enhancement Pipeline

Files are never blocked waiting for AI. Organization happens immediately with regex, AI improves asynchronously.

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│ File arrives │────▶│ Regex parse │────▶│ Organize    │
└─────────────┘     └─────────────┘     │ immediately │
                                        └──────┬──────┘
                                               │
                    ┌──────────────────────────┘
                    ▼
              ┌───────────┐     ┌─────────────┐     ┌──────────────┐
              │ Queue for │────▶│ AI process  │────▶│ Update cache │
              │ AI async  │     │ (background)│     └──────┬───────┘
              └───────────┘     └─────────────┘            │
                                                           ▼
                                               ┌─────────────────────┐
                                               │ Significant         │
                                               │ improvement?        │
                                               └──────────┬──────────┘
                                                          │
                                    ┌─────────────────────┴─────────────────────┐
                                    ▼                                           ▼
                              ┌───────────┐                              ┌─────────────┐
                              │ No: Done  │                              │ Yes: Queue  │
                              └───────────┘                              │ for review  │
                                                                         └─────────────┘
```

### 2. Circuit Breaker

Prevents hammering a failing Ollama instance.

```go
type CircuitBreaker struct {
    state           CircuitState  // closed, open, half-open
    failureCount    int
    lastFailure     time.Time
    openedAt        time.Time

    // Configuration
    failureThreshold int           // 5 failures
    failureWindow    time.Duration // 2 minutes
    cooldownPeriod   time.Duration // 30 seconds
}

type CircuitState int

const (
    CircuitClosed   CircuitState = iota  // Normal operation
    CircuitOpen                          // Rejecting requests
    CircuitHalfOpen                      // Testing recovery
)
```

**State transitions:**
- **Closed → Open**: 5 failures within 2 minutes
- **Open → Half-Open**: After 30 second cooldown
- **Half-Open → Closed**: Test request succeeds
- **Half-Open → Open**: Test request fails

### 3. Response Recovery Chain

Handle malformed AI responses gracefully.

```
┌────────────────┐
│ AI Response    │
└───────┬────────┘
        ▼
┌────────────────┐     Success
│ 1. Parse JSON  │─────────────▶ Use result
└───────┬────────┘
        │ Failure
        ▼
┌────────────────┐     Success
│ 2. Extract     │─────────────▶ Use partial
│    partial     │               (title only OK)
└───────┬────────┘
        │ Failure
        ▼
┌────────────────┐     Success
│ 3. Retry with  │─────────────▶ Use result
│    nudge       │
└───────┬────────┘
        │ Failure
        ▼
┌────────────────┐
│ 4. Fallback    │─────────────▶ Use regex result
│    to regex    │
└────────────────┘
```

**Extractive parsing** for broken JSON:
```go
func extractPartialResult(response string) (*PartialResult, bool) {
    // Try to extract title even from malformed JSON
    titleMatch := regexp.MustCompile(`"title"\s*:\s*"([^"]+)"`).FindStringSubmatch(response)
    if titleMatch == nil {
        return nil, false
    }

    result := &PartialResult{Title: titleMatch[1]}

    // Try to extract other fields
    if yearMatch := regexp.MustCompile(`"year"\s*:\s*(\d{4})`).FindStringSubmatch(response); yearMatch != nil {
        year, _ := strconv.Atoi(yearMatch[1])
        result.Year = &year
    }

    // Partial results get lower confidence
    result.Confidence = 0.7
    return result, true
}
```

**Retry nudge prompt:**
```
Your previous response was not valid JSON. Return ONLY valid JSON with no markdown, no explanation, just the JSON object:
```

### 4. Keepalive for Cold Start Prevention

Prevent Ollama model unloading during active use.

```go
type Keepalive struct {
    interval     time.Duration // 5 minutes
    idleTimeout  time.Duration // 30 minutes since last real request
    lastRequest  time.Time
    ticker       *time.Ticker
    stopCh       chan struct{}
}

func (k *Keepalive) Start(matcher *Matcher) {
    k.ticker = time.NewTicker(k.interval)
    go func() {
        for {
            select {
            case <-k.ticker.C:
                if time.Since(k.lastRequest) < k.idleTimeout {
                    // Keep model warm with minimal request
                    matcher.Ping(context.Background())
                }
            case <-k.stopCh:
                return
            }
        }
    }()
}
```

Only active when:
- AI is enabled
- Circuit is closed
- jellywatchd is running (not one-off CLI)
- Recent AI activity (within 30 minutes)

### 5. AI Improvements Review Queue

New database table for tracking AI-suggested improvements.

```sql
CREATE TABLE ai_improvements (
    id INTEGER PRIMARY KEY,
    file_path TEXT NOT NULL,
    current_title TEXT NOT NULL,
    suggested_title TEXT NOT NULL,
    current_year TEXT,
    suggested_year TEXT,
    media_type TEXT NOT NULL,  -- 'movie' or 'tv'
    regex_confidence REAL NOT NULL,
    ai_confidence REAL NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    status TEXT DEFAULT 'pending',  -- pending, approved, rejected, ignored
    reviewed_at TIMESTAMP,
    UNIQUE(file_path)
);
```

**Integration with existing `jellywatch review` command:**

The existing review command shows skipped items. AI improvements appear in the same flow:

```
$ jellywatch review

Pending Items (3 skipped, 2 AI improvements)

SKIPPED ITEMS:
  1. Unknown.File.2024.mkv
     Reason: Could not determine media type

AI IMPROVEMENTS:
  2. Movies/Halo (2024)/Halo (2024).mkv
     Current:   Halo (2024)
     Suggested: Halo (2022) - TV Series
     Confidence: 0.94 (was 0.45)

  3. TV Shows/The Office/Season 01/The Office S01E01.mkv
     Current:   The Office
     Suggested: The Office (US)
     Confidence: 0.91 (was 0.62)

[number] to act, [s]kip all, [q]uit:
```

## Extended AI Use Cases

### Use Case 1: Title Parsing (existing, improved)

**Trigger:** Regex confidence < 0.8
**Threshold:** AI confidence >= 0.8
**Action:** Use AI title, cache result

### Use Case 2: Media Type Detection

**Trigger:** No clear S01E01 pattern AND no clear year AND ambiguous
**Threshold:** AI confidence >= 0.85
**Action:** Use AI determination for organization path

```go
func (i *Integrator) DetectMediaType(filename string) (string, float64, error) {
    // Only invoke if heuristics are uncertain
    if hasEpisodePattern(filename) {
        return "tv", 1.0, nil
    }
    if hasMovieYear(filename) && !hasSeasonIndicator(filename) {
        return "movie", 0.95, nil
    }

    // Ambiguous - ask AI
    result, err := i.matcher.ParseMediaType(ctx, filename)
    if err != nil {
        return "", 0, err
    }
    return result.Type, result.Confidence, nil
}
```

### Use Case 3: Series Matching

**Trigger:** HOLDEN miss AND Sonarr miss AND parseable filename
**Threshold:** AI confidence >= 0.8
**Action:** Suggest matching series from library

```go
type SeriesMatchRequest struct {
    Filename    string
    ParsedTitle string
    Libraries   []string  // Existing series in libraries
}

type SeriesMatchResult struct {
    MatchedSeries string  // Existing series name
    Confidence    float64
    Reasoning     string  // "Title similarity", "Same year", etc.
}
```

**Prompt structure:**
```
Given filename: "GOT.S08E05.720p.WEB.mkv"
Parsed title: "GOT"

Which of these existing series does it belong to?
- Game of Thrones (2011)
- Ghosts of Tsushima (2024)
- None of the above

Return JSON: {"matched_series": "...", "confidence": 0.95}
```

### Use Case 4: Sample/Junk Detection (Conservative)

**Trigger:** File < 500MB AND not in expected location AND filename ambiguous
**Threshold:** AI confidence >= 0.95 (conservative - never lose real content)
**Action:** Mark as junk only if very confident

```go
type JunkDetectionResult struct {
    IsJunk     bool
    Confidence float64
    Reason     string  // "sample", "promo", "trailer", "extra", etc.
}

func (i *Integrator) DetectJunk(filename string, fileSize int64) (*JunkDetectionResult, error) {
    // Fast path: obvious patterns
    if containsSamplePattern(filename) {
        return &JunkDetectionResult{IsJunk: true, Confidence: 0.99, Reason: "sample"}, nil
    }
    if fileSize > 1*GB {
        return &JunkDetectionResult{IsJunk: false, Confidence: 0.99}, nil
    }

    // Ambiguous small file - ask AI
    result, err := i.matcher.DetectJunk(ctx, filename, fileSize)
    if err != nil {
        return &JunkDetectionResult{IsJunk: false}, nil  // Conservative: assume real
    }

    // Only mark as junk if VERY confident
    if result.Confidence < 0.95 {
        return &JunkDetectionResult{IsJunk: false}, nil
    }

    return result, nil
}
```

## Observability

### Status Display

Minimal, focused status in `jellywatch status`:

**Healthy:**
```
AI Integration
  Status:       enabled (circuit closed)
  Model:        llama3.2 (warm)
  Last success: 2m ago
  Cache:        1,247 entries (78% hit rate)
  Pending:      3 improvements awaiting review
```

**Degraded:**
```
AI Integration
  Status:       degraded (circuit open)
  Reason:       5 failures in 2m (connection refused)
  Recovery:     18s remaining
  Fallback:     using regex parsing
```

**Disabled:**
```
AI Integration
  Status:       disabled
  Enable:       set ai.enabled = true in config.toml
```

### CLI Inline Warnings

During organize when AI unavailable:
```
Organizing: The.Matrix.1999.2160p.UHD.BluRay.x265-GROUP.mkv
  ⚠ AI unavailable (circuit open) - using regex
  → Movies/The Matrix (1999)/The Matrix (1999).mkv
```

### Daemon Logging

Structured logs for jellywatchd:
```
level=warn msg="AI circuit breaker opened" failures=5 window=2m reason="connection refused"
level=info msg="AI circuit breaker recovering" state=half-open
level=info msg="AI circuit breaker closed" test_latency=245ms
```

### Metrics Tracked

| Metric | Type | Purpose |
|--------|------|---------|
| `circuit_state` | enum | Current circuit breaker state |
| `circuit_opened_at` | timestamp | Calculate recovery time |
| `last_failure_reason` | string | Display to user |
| `last_success_at` | timestamp | Health indicator |
| `cache_entries` | gauge | Cache size |
| `cache_hits` | counter | Hit rate numerator |
| `cache_misses` | counter | Hit rate denominator |
| `pending_improvements` | gauge | Review queue size |
| `model_state` | enum | warm/cold |

## Code Cleanup

### Remove Dead Code

Delete `internal/naming/ai_naming.go`:
- `ParseMovieNameWithAI()` - never called
- `ParseTVShowNameWithAI()` - never called
- `OptionalMatcher` struct - unused
- `NewAIMatcher()` - unused
- `aiResultToMovieInfo()` - unused
- `aiResultToTVShowInfo()` - unused

### Unify Configuration

Move AI config from separate JSON to main TOML config.

**Before (ai.json):**
```json
{
  "enabled": true,
  "ollama_endpoint": "http://localhost:11434",
  "model": "llama3.2"
}
```

**After (config.toml):**
```toml
[ai]
enabled = true
ollama_endpoint = "http://localhost:11434"
model = "llama3.2"
confidence_threshold = 0.8
timeout = "5s"
cache_enabled = true

[ai.circuit_breaker]
failure_threshold = 5
failure_window = "2m"
cooldown_period = "30s"

[ai.keepalive]
enabled = true
interval = "5m"
idle_timeout = "30m"
```

### Single Integration Path

All AI integration flows through `internal/ai/integrator.go`:

```go
type Integrator struct {
    // Core
    enabled    bool
    matcher    *Matcher
    cache      *Cache

    // Reliability
    circuit    *CircuitBreaker
    keepalive  *Keepalive

    // Background processing
    queue      chan *EnhancementRequest

    // Metrics
    metrics    *Metrics
}

// Public API
func (i *Integrator) EnhanceTitle(regexTitle, filename, mediaType string) (string, ParseSource, error)
func (i *Integrator) DetectMediaType(filename string) (string, float64, error)
func (i *Integrator) MatchSeries(filename, parsedTitle string, existing []string) (*SeriesMatch, error)
func (i *Integrator) DetectJunk(filename string, fileSize int64) (*JunkResult, error)
func (i *Integrator) Status() *AIStatus
func (i *Integrator) Close() error
```

## File Structure

```
internal/ai/
├── integrator.go        # Main integration point (expanded)
├── matcher.go           # Ollama API client
├── cache.go             # SQLite caching
├── circuit_breaker.go   # NEW: Circuit breaker implementation
├── keepalive.go         # NEW: Model warmth maintenance
├── recovery.go          # NEW: Response recovery chain
├── background.go        # NEW: Async enhancement queue
├── improvements.go      # NEW: AI improvements review queue
├── prompts.go           # NEW: All prompts in one place
├── media_type.go        # NEW: Media type detection
├── series_match.go      # NEW: Series matching
├── junk_detect.go       # NEW: Sample/junk detection
├── config.go            # Updated: TOML integration
├── types.go             # Result types
├── metrics.go           # NEW: Observability
├── normalize.go         # Input normalization
├── confidence.go        # Regex confidence scoring
└── data/
    ├── release_groups.txt
    └── known_titles.txt

# DELETED:
internal/naming/ai_naming.go
```

## Migration

1. **Config migration**: On startup, if old `ai.json` exists:
   - Read values
   - Write to `[ai]` section in `config.toml`
   - Rename `ai.json` to `ai.json.migrated`
   - Log: "AI config migrated to config.toml"

2. **Database migration**: Add `ai_improvements` table in schema migration

## Testing Strategy

### Unit Tests
- Circuit breaker state transitions
- Response recovery chain (valid JSON, broken JSON, extraction, retry)
- Keepalive timing logic
- Junk detection thresholds

### Integration Tests
- Full background enhancement flow
- Circuit breaker with real timeouts
- Improvements queue and review

### Manual Testing
- Kill Ollama mid-request → verify circuit opens
- Return malformed JSON → verify recovery
- Organize with AI disabled → verify no blocking
