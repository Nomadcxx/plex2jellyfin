# AI Auto-Trigger During Scanning - Design Document

## Overview

This design adds automatic AI title parsing during scanning when regex confidence is low. It integrates with the existing confidence system and audit command.

## Goals

1. Automatically improve parse quality during scanning without user intervention
2. Minimize AI/token usage through smart triggering and caching
3. Flag unresolved files for manual review via audit system
4. Handle Ollama unavailability gracefully

## Configuration

Add `auto_trigger_threshold` to `AIConfig`:

```go
type AIConfig struct {
    Enabled              bool    `mapstructure:"enabled"`
    AutoTriggerThreshold float64 `mapstructure:"auto_trigger_threshold"` // NEW: default 0.6
    ConfidenceThreshold  float64 `mapstructure:"confidence_threshold"`   // existing: default 0.8
    // ... rest unchanged
}
```

**Two thresholds:**
- `auto_trigger_threshold` (0.6): Regex confidence below this triggers AI call
- `confidence_threshold` (0.8): Minimum AI confidence to accept result

**Config example:**
```toml
[ai]
enabled = true
auto_trigger_threshold = 0.6
confidence_threshold = 0.8
```

## Scanner Flow

Order of operations: **Regex → De-obfuscation → AI**

```
processFile(filePath, info, libraryRoot, mediaType)
│
├─► 1. REGEX PARSE
│   - Parse filename with naming.ParseTVShowName/ParseMovieName
│   - Calculate regexConfidence using naming.CalculateTitleConfidence
│   - Set parseMethod = "regex"
│
├─► 2. DE-OBFUSCATION (if obfuscated filename detected)
│   - If naming.IsObfuscatedFilename(filename):
│     - Try naming.ParseTVShowFromPath / ParseMovieFromPath
│     - Calculate folderConfidence
│     - If folderConfidence > regexConfidence:
│       - Use folder result, parseMethod = "folder"
│
├─► 3. AI TRIGGER (if low confidence + AI enabled)
│   - If bestConfidence < autoTriggerThreshold (0.6):
│     - If AI enabled AND circuit breaker closed:
│       - Check cache first (ai_parse_cache table)
│       - If cache miss: call AI matcher
│       - If aiConfidence >= confidenceThreshold (0.8):
│         - Use AI result, parseMethod = "ai"
│       - Else if aiConfidence > bestConfidence:
│         - Use AI result anyway (still better)
│
└─► 4. SAVE TO DATABASE
    - Set needs_review = (bestConfidence < 0.8)
    - UpsertMediaFile with confidence, parseMethod, needs_review
```

## AIHelper Component

New file: `internal/scanner/ai_helper.go`

```go
type AIHelper struct {
    matcher     *ai.Matcher
    db          *database.MediaDB
    cfg         config.AIConfig

    // Circuit breaker state
    failures    int
    lastFailure time.Time
    circuitOpen bool
    mu          sync.Mutex
}

func NewAIHelper(cfg config.AIConfig, db *database.MediaDB) (*AIHelper, error)

func (h *AIHelper) TryParse(ctx context.Context, filename string) (*ai.Result, bool, error)
// Returns: result, fromCache, error

func (h *AIHelper) isCircuitOpen() bool
func (h *AIHelper) recordFailure()
func (h *AIHelper) resetFailures()
```

**Circuit breaker behavior:**
- Opens after 5 failures within 2 minutes
- Stays open for 30 seconds (cooldown)
- Auto-closes after cooldown

**Cache behavior:**
- Uses existing `ai_parse_cache` table
- Lookup by normalized filename + media type
- Updates `last_used_at` and `usage_count` on cache hits

## Scan Result Stats

Additions to `ScanResult`:

```go
type ScanResult struct {
    // ... existing fields ...

    // AI stats
    AITriggered  int  // Files where AI was called
    AICacheHits  int  // Files served from cache
    AISucceeded  int  // AI calls that improved confidence
    AIFailed     int  // AI calls that failed/timed out
    NeedsReview  int  // Files flagged for audit
}
```

## Logging Output

**During scan (INFO level):**
```
Scanning TV libraries...
  [AI] Parsing: 30e2dc4173fc4798bbe5fd40137ed621.mkv
  [AI] Cache hit: The.Matrix.1999.1080p.BluRay.mkv
```

**Summary at end:**
```
=== Scan Complete ===
Files scanned:    1,247
Files added:      23
Duration:         45s

AI Summary:
  Triggered:      15
  Cache hits:     8
  Improved:       12
  Failed:         3
  Needs review:   6 (run 'jellywatch audit' to review)
```

## Audit System Connection

Files with `needs_review=true` are picked up by the audit command:

**Flagged when:**
- Best confidence < 0.8 after all parsing attempts
- AI unavailable (circuit open, Ollama down)
- AI returned low confidence result

**Audit workflow:**
```bash
jellywatch audit --generate   # Find needs_review files
jellywatch audit --dry-run    # Preview AI fixes
jellywatch audit --execute    # Apply fixes
```

## Error Handling

| Scenario | Behavior |
|----------|----------|
| AI disabled | Skip AI, flag if confidence < 0.8 |
| Ollama unreachable | Circuit breaker opens, flag for review |
| AI timeout | Record failure, use best regex/folder result |
| AI low confidence | Compare with regex, keep better one |
| Cache miss | Call AI, cache result for future |

## Files to Modify

1. `internal/config/config.go` - Add AutoTriggerThreshold
2. `internal/scanner/ai_helper.go` - NEW: AI helper with circuit breaker
3. `internal/scanner/scanner.go` - Integrate AI into processFile
4. `internal/scanner/types.go` - Add AI stats to ScanResult
5. `internal/database/ai_cache.go` - Cache read/write helpers (may exist)

## Integration with Confidence System Plan

This design extends the confidence system plan (`2026-01-27-confidence-system.md`):

- **Confidence plan Task 1-4**: Creates confidence calculation (prerequisite)
- **This design**: Adds auto-trigger during scanning
- **Confidence plan Task 7-8**: Audit command (works with needs_review flag)

The confidence system plan should be implemented first, then this auto-trigger feature.
