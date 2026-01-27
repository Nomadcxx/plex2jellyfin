# AI Auto-Trigger During Scanning - Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Automatically trigger AI parsing during scanning when regex confidence is low, with circuit breaker and caching.

**Architecture:** Add `auto_trigger_threshold` to config. Create AIHelper with circuit breaker. Integrate into scanner's processFile to call AI when confidence < 0.6. Use existing ai/cache.go for caching. Flag unresolved files with `needs_review=true` for audit system.

**Tech Stack:** Go, SQLite, existing ai/matcher.go, existing ai/cache.go

**Prerequisite:** The confidence system plan (`2026-01-27-confidence-system.md`) Tasks 1-4 must be implemented first (confidence calculation, database columns, scanner integration).

---

### Task 1: Add AutoTriggerThreshold to AIConfig

**Files:**
- Modify: `internal/config/config.go:131-143`
- Test: Manual verification

**Step 1: Add the new field to AIConfig struct**

In `internal/config/config.go`, find the AIConfig struct (around line 131) and add the new field:

```go
// AIConfig contains AI title matching configuration
type AIConfig struct {
	Enabled              bool                 `mapstructure:"enabled"`
	OllamaEndpoint       string               `mapstructure:"ollama_endpoint"`
	Model                string               `mapstructure:"model"`
	ConfidenceThreshold  float64              `mapstructure:"confidence_threshold"`
	AutoTriggerThreshold float64              `mapstructure:"auto_trigger_threshold"` // NEW
	TimeoutSeconds       int                  `mapstructure:"timeout_seconds"`
	CacheEnabled         bool                 `mapstructure:"cache_enabled"`
	CloudModel           string               `mapstructure:"cloud_model"`
	AutoResolveRisky     bool                 `mapstructure:"auto_resolve_risky"`
	CircuitBreaker       CircuitBreakerConfig `mapstructure:"circuit_breaker"`
	Keepalive            KeepaliveConfig      `mapstructure:"keepalive"`
}
```

**Step 2: Update DefaultConfig**

In `DefaultConfig()` (around line 218), add the default value:

```go
AI: AIConfig{
	Enabled:              false,
	OllamaEndpoint:       "http://localhost:11434",
	Model:                "qwen2.5vl:7b",
	ConfidenceThreshold:  0.8,
	AutoTriggerThreshold: 0.6,  // NEW: trigger AI when regex confidence < this
	TimeoutSeconds:       5,
	CacheEnabled:         true,
	// ... rest unchanged
},
```

**Step 3: Update DefaultAIConfig helper**

In `DefaultAIConfig()` (around line 473), add:

```go
func DefaultAIConfig() AIConfig {
	return AIConfig{
		Enabled:              false,
		OllamaEndpoint:       "http://localhost:11434",
		Model:                "qwen2.5vl:7b",
		ConfidenceThreshold:  0.8,
		AutoTriggerThreshold: 0.6,  // NEW
		TimeoutSeconds:       5,
		CacheEnabled:         true,
		// ... rest unchanged
	}
}
```

**Step 4: Update ToTOML method**

In `ToTOML()` (around line 382-389), add the new field to the AI section:

```go
[ai]
enabled = %v
ollama_endpoint = "%s"
model = "%s"
confidence_threshold = %.2f
auto_trigger_threshold = %.2f
timeout_seconds = %d
```

And update the format arguments accordingly.

**Step 5: Build and verify**

Run: `go build ./cmd/jellywatch`
Expected: Builds successfully

**Step 6: Commit**

```bash
git add internal/config/config.go
git commit -m "feat(config): add auto_trigger_threshold for AI during scanning"
```

---

### Task 2: Create AIHelper with Circuit Breaker

**Files:**
- Create: `internal/scanner/ai_helper.go`
- Test: `internal/scanner/ai_helper_test.go`

**Step 1: Write the failing test**

```go
// internal/scanner/ai_helper_test.go
package scanner

import (
	"context"
	"testing"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/config"
)

func TestAIHelper_CircuitBreaker(t *testing.T) {
	cfg := config.AIConfig{
		Enabled:              true,
		AutoTriggerThreshold: 0.6,
		ConfidenceThreshold:  0.8,
		CircuitBreaker: config.CircuitBreakerConfig{
			FailureThreshold:     3,
			FailureWindowSeconds: 60,
			CooldownSeconds:      1,
		},
	}

	helper := NewAIHelper(cfg, nil, nil) // nil db and matcher for unit test

	// Record failures to open circuit
	for i := 0; i < 3; i++ {
		helper.RecordFailure()
	}

	if !helper.IsCircuitOpen() {
		t.Error("Circuit should be open after 3 failures")
	}

	// Wait for cooldown
	time.Sleep(1100 * time.Millisecond)

	if helper.IsCircuitOpen() {
		t.Error("Circuit should be closed after cooldown")
	}
}

func TestAIHelper_ResetFailures(t *testing.T) {
	cfg := config.AIConfig{
		CircuitBreaker: config.CircuitBreakerConfig{
			FailureThreshold:     3,
			FailureWindowSeconds: 60,
			CooldownSeconds:      5,
		},
	}

	helper := NewAIHelper(cfg, nil, nil)

	helper.RecordFailure()
	helper.RecordFailure()
	helper.ResetFailures()

	if helper.IsCircuitOpen() {
		t.Error("Circuit should be closed after reset")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/scanner -run TestAIHelper -v`
Expected: FAIL - undefined: NewAIHelper

**Step 3: Write the AIHelper implementation**

```go
// internal/scanner/ai_helper.go
package scanner

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/ai"
	"github.com/Nomadcxx/jellywatch/internal/config"
)

var ErrCircuitOpen = errors.New("circuit breaker is open")

// AIHelper manages AI parsing with circuit breaker and caching
type AIHelper struct {
	matcher *ai.Matcher
	cache   *ai.Cache
	cfg     config.AIConfig

	// Circuit breaker state
	mu           sync.Mutex
	failures     int
	firstFailure time.Time
	circuitOpen  bool
	openedAt     time.Time
}

// NewAIHelper creates a new AI helper
func NewAIHelper(cfg config.AIConfig, db *sql.DB, matcher *ai.Matcher) *AIHelper {
	var cache *ai.Cache
	if db != nil && cfg.CacheEnabled {
		cache = ai.NewCache(db)
	}

	return &AIHelper{
		matcher: matcher,
		cache:   cache,
		cfg:     cfg,
	}
}

// TryParse attempts to parse filename with AI, respecting circuit breaker and cache
// Returns: result, fromCache, error
func (h *AIHelper) TryParse(ctx context.Context, filename, mediaType string) (*ai.Result, bool, error) {
	// Check circuit breaker
	if h.IsCircuitOpen() {
		return nil, false, ErrCircuitOpen
	}

	// Check cache first
	if h.cache != nil {
		normalized := ai.NormalizeInput(filename)
		cached, err := h.cache.Get(normalized, mediaType, h.cfg.Model)
		if err == nil && cached != nil {
			return cached, true, nil
		}
	}

	// Call AI matcher
	if h.matcher == nil {
		return nil, false, errors.New("AI matcher not initialized")
	}

	result, err := h.matcher.ParseWithRetry(ctx, filename)
	if err != nil {
		h.RecordFailure()
		return nil, false, err
	}

	// Cache successful result
	if h.cache != nil {
		normalized := ai.NormalizeInput(filename)
		h.cache.Put(normalized, mediaType, h.cfg.Model, result, 0) // latency tracked elsewhere
	}

	h.ResetFailures()
	return result, false, nil
}

// IsCircuitOpen checks if the circuit breaker is open
func (h *AIHelper) IsCircuitOpen() bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.circuitOpen {
		return false
	}

	// Check if cooldown has passed
	cooldown := time.Duration(h.cfg.CircuitBreaker.CooldownSeconds) * time.Second
	if time.Since(h.openedAt) >= cooldown {
		h.circuitOpen = false
		h.failures = 0
		return false
	}

	return true
}

// RecordFailure records an AI failure for circuit breaker
func (h *AIHelper) RecordFailure() {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	window := time.Duration(h.cfg.CircuitBreaker.FailureWindowSeconds) * time.Second

	// Reset if outside failure window
	if h.failures > 0 && now.Sub(h.firstFailure) > window {
		h.failures = 0
	}

	if h.failures == 0 {
		h.firstFailure = now
	}

	h.failures++

	// Open circuit if threshold exceeded
	if h.failures >= h.cfg.CircuitBreaker.FailureThreshold {
		h.circuitOpen = true
		h.openedAt = now
	}
}

// ResetFailures resets the failure counter (call after successful AI call)
func (h *AIHelper) ResetFailures() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.failures = 0
}

// IsEnabled returns true if AI is enabled and matcher is available
func (h *AIHelper) IsEnabled() bool {
	return h.cfg.Enabled && h.matcher != nil
}

// GetAutoTriggerThreshold returns the threshold for auto-triggering AI
func (h *AIHelper) GetAutoTriggerThreshold() float64 {
	return h.cfg.AutoTriggerThreshold
}

// GetConfidenceThreshold returns the minimum AI confidence to accept
func (h *AIHelper) GetConfidenceThreshold() float64 {
	return h.cfg.ConfidenceThreshold
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/scanner -run TestAIHelper -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/scanner/ai_helper.go internal/scanner/ai_helper_test.go
git commit -m "feat(scanner): add AIHelper with circuit breaker and caching"
```

---

### Task 3: Add AI Stats to ScanResult

**Files:**
- Modify: `internal/scanner/types.go:20-27`

**Step 1: Add AI stats fields to ScanResult**

In `internal/scanner/types.go`, update the ScanResult struct:

```go
// ScanResult contains statistics from a scan operation
type ScanResult struct {
	FilesScanned int
	FilesAdded   int
	FilesUpdated int
	FilesSkipped int
	FilesRemoved int // Files in DB but not on disk
	Duration     time.Duration
	Errors       []error

	// AI stats
	AITriggered  int // Files where AI was called
	AICacheHits  int // Files served from cache
	AISucceeded  int // AI calls that improved confidence
	AIFailed     int // AI calls that failed/timed out
	NeedsReview  int // Files flagged for audit
}
```

**Step 2: Build and verify**

Run: `go build ./cmd/jellywatch`
Expected: Builds successfully

**Step 3: Commit**

```bash
git add internal/scanner/types.go
git commit -m "feat(scanner): add AI stats to ScanResult"
```

---

### Task 4: Update FileScanner to Accept AIHelper

**Files:**
- Modify: `internal/scanner/scanner.go:16-23` (FileScanner struct)
- Modify: `internal/scanner/scanner.go:56-69` (NewFileScanner)

**Step 1: Add AIHelper field to FileScanner**

```go
// FileScanner scans libraries and populates the media_files database
type FileScanner struct {
	db             *database.MediaDB
	aiHelper       *AIHelper  // NEW: optional AI helper
	minMovieSize   int64
	minEpisodeSize int64
	skipPatterns   []string
}
```

**Step 2: Create NewFileScannerWithAI constructor**

Add this function after NewFileScanner:

```go
// NewFileScannerWithAI creates a file scanner with AI support
func NewFileScannerWithAI(db *database.MediaDB, aiHelper *AIHelper) *FileScanner {
	scanner := NewFileScanner(db)
	scanner.aiHelper = aiHelper
	return scanner
}
```

**Step 3: Build and verify**

Run: `go build ./cmd/jellywatch`
Expected: Builds successfully

**Step 4: Commit**

```bash
git add internal/scanner/scanner.go
git commit -m "feat(scanner): add AIHelper support to FileScanner"
```

---

### Task 5: Integrate AI into processFile

**Files:**
- Modify: `internal/scanner/scanner.go:262-341` (processFile function)

**Step 1: Update processFile to use AI**

Replace the processFile function with this updated version that includes the full flow:
Regex → De-obfuscation → AI

```go
// processFile extracts metadata and stores file in database
func (s *FileScanner) processFile(filePath string, info os.FileInfo, libraryRoot string, mediaType string, result *ScanResult) error {
	filename := filepath.Base(filePath)

	// Determine media type if not specified
	if mediaType == "" {
		if naming.IsTVEpisodeFilename(filename) {
			mediaType = "episode"
		} else if naming.IsMovieFilename(filename) {
			mediaType = "movie"
		} else {
			return fmt.Errorf("unable to determine media type")
		}
	}

	isEpisode := mediaType == "episode"

	// === STEP 1: REGEX PARSE ===
	var normalizedTitle string
	var year *int
	var season, episode *int
	parseMethod := "regex"

	if isEpisode {
		tv, err := naming.ParseTVShowName(filename)
		if err != nil {
			return fmt.Errorf("parse TV show: %w", err)
		}
		normalizedTitle = database.NormalizeTitle(tv.Title)
		if tv.Year != "" {
			if yearInt, err := parseInt(tv.Year); err == nil {
				year = &yearInt
			}
		}
		season = &tv.Season
		episode = &tv.Episode
	} else {
		movie, err := naming.ParseMovieName(filename)
		if err != nil {
			return fmt.Errorf("parse movie: %w", err)
		}
		normalizedTitle = database.NormalizeTitle(movie.Title)
		if movie.Year != "" {
			if yearInt, err := parseInt(movie.Year); err == nil {
				year = &yearInt
			}
		}
	}

	// Calculate regex confidence
	bestConfidence := naming.CalculateTitleConfidence(normalizedTitle, filename)

	// === STEP 2: DE-OBFUSCATION (if obfuscated filename) ===
	if naming.IsObfuscatedFilename(filename) {
		if isEpisode {
			if tvInfo, err := naming.ParseTVShowFromPath(filePath); err == nil {
				folderTitle := database.NormalizeTitle(tvInfo.Title)
				folderConfidence := naming.CalculateTitleConfidence(folderTitle, filepath.Base(filepath.Dir(filePath)))
				if folderConfidence > bestConfidence {
					normalizedTitle = folderTitle
					if tvInfo.Year != "" {
						if yearInt, err := parseInt(tvInfo.Year); err == nil {
							year = &yearInt
						}
					}
					season = &tvInfo.Season
					episode = &tvInfo.Episode
					bestConfidence = folderConfidence
					parseMethod = "folder"
				}
			}
		} else {
			if movieInfo, err := naming.ParseMovieFromPath(filePath); err == nil {
				folderTitle := database.NormalizeTitle(movieInfo.Title)
				folderConfidence := naming.CalculateTitleConfidence(folderTitle, filepath.Base(filepath.Dir(filePath)))
				if folderConfidence > bestConfidence {
					normalizedTitle = folderTitle
					if movieInfo.Year != "" {
						if yearInt, err := parseInt(movieInfo.Year); err == nil {
							year = &yearInt
						}
					}
					bestConfidence = folderConfidence
					parseMethod = "folder"
				}
			}
		}
	}

	// === STEP 3: AI TRIGGER (if low confidence + AI enabled) ===
	if s.aiHelper != nil && s.aiHelper.IsEnabled() {
		if bestConfidence < s.aiHelper.GetAutoTriggerThreshold() {
			ctx := context.Background()
			aiResult, fromCache, err := s.aiHelper.TryParse(ctx, filename, mediaType)

			if fromCache {
				result.AICacheHits++
			}

			if err == nil && aiResult != nil {
				result.AITriggered++

				// Check if AI result is better
				if aiResult.Confidence >= s.aiHelper.GetConfidenceThreshold() {
					// AI is confident - use it
					normalizedTitle = database.NormalizeTitle(aiResult.Title)
					year = aiResult.Year
					if isEpisode && aiResult.Season != nil {
						season = aiResult.Season
						if len(aiResult.Episodes) > 0 {
							ep := aiResult.Episodes[0]
							episode = &ep
						}
					}
					bestConfidence = aiResult.Confidence
					parseMethod = "ai"
					result.AISucceeded++
				} else if aiResult.Confidence > bestConfidence {
					// AI not confident but still better than regex
					normalizedTitle = database.NormalizeTitle(aiResult.Title)
					year = aiResult.Year
					if isEpisode && aiResult.Season != nil {
						season = aiResult.Season
						if len(aiResult.Episodes) > 0 {
							ep := aiResult.Episodes[0]
							episode = &ep
						}
					}
					bestConfidence = aiResult.Confidence
					parseMethod = "ai"
					result.AISucceeded++
				}
			} else if err != nil {
				result.AIFailed++
			}
		}
	}

	// === STEP 4: SAVE TO DATABASE ===
	needsReview := bestConfidence < 0.8
	if needsReview {
		result.NeedsReview++
	}

	// Extract quality metadata
	qualityMeta := quality.ExtractMetadata(filePath, info.Size(), isEpisode)

	// Check compliance
	isCompliant, issues := database.CheckCompliance(filePath, libraryRoot)

	// Create MediaFile record
	file := &database.MediaFile{
		Path:                filePath,
		Size:                info.Size(),
		ModifiedAt:          info.ModTime(),
		MediaType:           mediaType,
		NormalizedTitle:     normalizedTitle,
		Year:                year,
		Season:              season,
		Episode:             episode,
		Resolution:          qualityMeta.Resolution,
		SourceType:          qualityMeta.SourceType,
		Codec:               qualityMeta.Codec,
		AudioFormat:         qualityMeta.AudioFormat,
		QualityScore:        qualityMeta.QualityScore,
		IsJellyfinCompliant: isCompliant,
		ComplianceIssues:    issues,
		Source:              "filesystem",
		SourcePriority:      50,
		LibraryRoot:         libraryRoot,
		// Confidence fields (from confidence system plan)
		Confidence:  bestConfidence,
		ParseMethod: parseMethod,
		NeedsReview: needsReview,
	}

	// Upsert to database
	return s.db.UpsertMediaFile(file)
}
```

**Step 2: Update scanPath to pass result**

Update the scanPath function signature and calls to pass the result pointer for AI stats:

```go
// scanPath is the internal recursive scanner
func (s *FileScanner) scanPath(ctx context.Context, path string, mediaType string, result *ScanResult) error {
	return filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		// ... existing checks ...

		// Process the file - pass result for AI stats
		if err := s.processFile(filePath, info, path, mediaType, result); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("process file %s: %w", filePath, err))
			return nil
		}

		result.FilesAdded++
		return nil
	})
}
```

**Step 3: Build and verify**

Run: `go build ./cmd/jellywatch`
Expected: Builds successfully

**Step 4: Commit**

```bash
git add internal/scanner/scanner.go
git commit -m "feat(scanner): integrate AI auto-trigger into processFile

Flow: Regex → De-obfuscation → AI
- Triggers AI when confidence < auto_trigger_threshold
- Uses cache when available
- Compares results and keeps best confidence
- Flags needs_review when best confidence < 0.8"
```

---

### Task 6: Add Logging for AI Activity

**Files:**
- Modify: `internal/scanner/scanner.go` (processFile and summary)

**Step 1: Add logging import if not present**

```go
import (
	// ... existing imports
	"log"
)
```

**Step 2: Add INFO log when AI is triggered**

In processFile, after the AI call:

```go
if err == nil && aiResult != nil {
	result.AITriggered++
	if fromCache {
		log.Printf("[AI] Cache hit: %s", filename)
	} else {
		log.Printf("[AI] Parsing: %s", filename)
	}
	// ... rest of AI handling
}
```

**Step 3: Build and verify**

Run: `go build ./cmd/jellywatch`
Expected: Builds successfully

**Step 4: Commit**

```bash
git add internal/scanner/scanner.go
git commit -m "feat(scanner): add INFO logging for AI activity"
```

---

### Task 7: Update Scan Command to Show AI Summary

**Files:**
- Modify: `cmd/jellywatch/scan_cmd.go` (find the scan output section)

**Step 1: Find the scan command**

Run: `grep -n "Scan Complete\|FilesScanned" cmd/jellywatch/scan_cmd.go`

**Step 2: Add AI summary output**

After the existing scan summary, add:

```go
// Show AI summary if AI was used
if result.AITriggered > 0 || result.AICacheHits > 0 || result.NeedsReview > 0 {
	fmt.Println()
	fmt.Println("AI Summary:")
	fmt.Printf("  Triggered:    %d\n", result.AITriggered)
	fmt.Printf("  Cache hits:   %d\n", result.AICacheHits)
	fmt.Printf("  Improved:     %d\n", result.AISucceeded)
	fmt.Printf("  Failed:       %d\n", result.AIFailed)
	if result.NeedsReview > 0 {
		fmt.Printf("  Needs review: %d (run 'jellywatch audit' to review)\n", result.NeedsReview)
	}
}
```

**Step 3: Build and verify**

Run: `go build ./cmd/jellywatch`
Expected: Builds successfully

**Step 4: Commit**

```bash
git add cmd/jellywatch/scan_cmd.go
git commit -m "feat(cli): show AI summary in scan output"
```

---

### Task 8: Wire Up AIHelper in Scan Command

**Files:**
- Modify: `cmd/jellywatch/scan_cmd.go` (initialization section)

**Step 1: Initialize AIHelper if AI is enabled**

Find where the FileScanner is created and update:

```go
// Load config
cfg, err := config.Load()
if err != nil {
	return fmt.Errorf("failed to load config: %w", err)
}

// Create scanner with optional AI support
var fileScanner *scanner.FileScanner

if cfg.AI.Enabled {
	// Initialize AI matcher
	matcher, err := ai.NewMatcher(cfg.AI)
	if err != nil {
		log.Printf("Warning: Failed to initialize AI matcher: %v", err)
		fileScanner = scanner.NewFileScanner(db)
	} else {
		// Check if Ollama is available
		ctx := context.Background()
		if matcher.IsAvailable(ctx) {
			aiHelper := scanner.NewAIHelper(cfg.AI, db.DB(), matcher)
			fileScanner = scanner.NewFileScannerWithAI(db, aiHelper)
			log.Printf("AI scanning enabled (model: %s)", cfg.AI.Model)
		} else {
			log.Printf("Warning: Ollama not available, AI scanning disabled")
			fileScanner = scanner.NewFileScanner(db)
		}
	}
} else {
	fileScanner = scanner.NewFileScanner(db)
}
```

**Step 2: Add required import**

```go
import (
	// ... existing imports
	"github.com/Nomadcxx/jellywatch/internal/ai"
)
```

**Step 3: Build and verify**

Run: `go build ./cmd/jellywatch`
Expected: Builds successfully

**Step 4: Test with AI disabled**

Run: `./jellywatch scan --dry-run`
Expected: Scans without AI (AI Summary section not shown or shows zeros)

**Step 5: Commit**

```bash
git add cmd/jellywatch/scan_cmd.go
git commit -m "feat(cli): wire up AIHelper in scan command

Initializes AI matcher and helper when:
- AI is enabled in config
- Ollama is available

Falls back to regex-only scanning otherwise."
```

---

## Summary

This plan implements AI auto-trigger during scanning:

1. **Task 1**: Add `auto_trigger_threshold` config option (0.6 default)
2. **Task 2**: Create AIHelper with circuit breaker and cache integration
3. **Task 3**: Add AI stats to ScanResult
4. **Task 4**: Update FileScanner to accept AIHelper
5. **Task 5**: Integrate full flow into processFile (Regex → De-obfuscation → AI)
6. **Task 6**: Add INFO logging for AI activity
7. **Task 7**: Show AI summary in scan output
8. **Task 8**: Wire up AIHelper in scan command

**Dependencies:**
- Requires confidence system plan Tasks 1-4 (confidence calculation, database columns)
- Uses existing `internal/ai/cache.go` for caching
- Uses existing `internal/ai/matcher.go` for AI calls

**Testing:**
- Unit tests for circuit breaker in Task 2
- Manual testing with Ollama running
- Test fallback when Ollama unavailable
