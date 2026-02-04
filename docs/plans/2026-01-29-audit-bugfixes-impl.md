# Audit Command Bug Fixes Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix 3 bugs in audit command (dead code, hardcoded summary counts, no progress) and add comprehensive media type validation with statistics tracking

**Architecture:** Modify generateAudit() function in cmd/jellywatch/audit_cmd.go to add progress tracking, type validation layer (library_root + Sonarr/Radarr APIs), track skip reasons, and calculate accurate summary statistics. Enhance AuditPlan structure with SkipReason and expanded statistics fields.

**Tech Stack:** Go 1.21+, existing AI matcher, Sonarr/Radarr API clients, json.Marshal for plan persistence

**Design Document:** `docs/plans/2026-01-29-audit-bugfixes-design.md`

---

## Prerequisites

Before starting implementation, verify:

1. Current codebase compiles: `go build ./cmd/jellywatch`
2. Tests pass: `go test ./...`
3. Understand current audit_cmd.go structure (362 lines)

**Key Files to Understand:**
- `cmd/jellywatch/audit_cmd.go` - Main audit command (bugs here)
- `internal/plans/plans.go` - AuditPlan, AuditItem, AuditSummary structs
- `internal/ai/matcher.go` - AI Parse() interface and Result struct
- `internal/database/media_files.go` - MediaFile struct with LibraryRoot field
- `internal/sonarr/series.go` - FindSeriesByTitle() method
- `internal/config/config.go` - Config struct with Sonarr/Radarr settings

---

## Task 1: Update AuditItem Struct with SkipReason Field

**Files:**
- Modify: `internal/plans/plans.go:284-296`

**Why:** Need to track why items were skipped (type mismatch, confidence too low, title unchanged) for user feedback and debugging.

**Step 1.1: Read current struct definition**

Read `internal/plans/plans.go` lines 284-296 to see current AuditItem struct.

**Step 1.2: Add SkipReason field**

Find this code block:
```go
type AuditItem struct {
	ID           int64   `json:"id"`
	Path         string  `json:"path"`
	Size         int64   `json:"size"`
	MediaType   string   `json:"media_type"`
	Title        string   `json:"title"`
	Year         *int     `json:"year"`
	Season       *int     `json:"season,omitempty"`
	Episode      *int     `json:"episode,omitempty"`
	Confidence  float64  `json:"confidence"`
	Resolution   string    `json:"resolution,omitempty"`
	SourceType   string    `json:"source_type,omitempty"`
}
```

Replace with:
```go
type AuditItem struct {
	ID           int64   `json:"id"`
	Path         string  `json:"path"`
	Size         int64   `json:"size"`
	MediaType   string   `json:"media_type"`
	Title        string   `json:"title"`
	Year         *int     `json:"year"`
	Season       *int     `json:"season,omitempty"`
	Episode      *int     `json:"episode,omitempty"`
	Confidence  float64  `json:"confidence"`
	Resolution   string    `json:"resolution,omitempty"`
	SourceType   string    `json:"source_type,omitempty"`
	SkipReason   string    `json:"skip_reason,omitempty"`
}
```

**Step 1.3: Verify**

Run: `go build ./internal/plans`
Expected: Success, no errors

**Step 1.4: Commit**

```bash
git add internal/plans/plans.go
git commit -m "feat(plans): add SkipReason field to AuditItem

- Tracks why items were skipped during audit generation
- Optional field (omitempty) for backward compatibility
- Enables detailed user feedback on skipped files"
```

---

## Task 2: Expand AuditSummary Struct with AI Statistics

**Files:**
- Modify: `internal/plans/plans.go:310-317`

**Why:** Need comprehensive statistics: AI calls, success rate, breakdown of skip reasons.

**Step 2.1: Read current struct definition**

Read `internal/plans/plans.go` lines 310-317 to see current AuditSummary struct.

**Step 2.2: Expand with new fields**

Find this code block:
```go
type AuditSummary struct {
	TotalFiles      int     `json:"total_files"`
	FilesToRename   int     `json:"files_to_rename"`
	FilesToDelete   int     `json:"files_to_delete"`
	FilesToSkip    int     `json:"files_to_skip"`
	AvgConfidence   float64 `json:"avg_confidence"`
}
```

Replace with:
```go
type AuditSummary struct {
	TotalFiles      int     `json:"total_files"`
	FilesToRename   int     `json:"files_to_rename"`
	FilesToDelete   int     `json:"files_to_delete"`
	FilesToSkip     int     `json:"files_to_skip"`
	AvgConfidence   float64 `json:"avg_confidence"`

	// AI processing statistics
	AITotalCalls          int     `json:"ai_total_calls"`
	AISuccessCount        int     `json:"ai_success_count"`
	AIErrorCount          int     `json:"ai_error_count"`
	TypeMismatchesSkipped int     `json:"type_mismatches_skipped"`
	ConfidenceTooLow      int     `json:"confidence_too_low"`
	TitleUnchanged        int     `json:"title_unchanged"`
}
```

**Step 2.3: Verify**

Run: `go build ./internal/plans`
Expected: Success, no errors

Run: `go test ./internal/plans/...`
Expected: All tests pass (struct expansion is backward compatible)

**Step 2.4: Commit**

```bash
git add internal/plans/plans.go
git commit -m "feat(plans): expand AuditSummary with AI statistics

- Add AITotalCalls, AISuccessCount, AIErrorCount
- Add TypeMismatchesSkipped, ConfidenceTooLow, TitleUnchanged
- Provides detailed breakdown of audit generation results"
```

---

## Task 3: Create Type Validation Test File

**Files:**
- Create: `cmd/jellywatch/audit_validation_test.go`

**Why:** TDD approach - write failing tests first for inferTypeFromLibraryRoot and validateMediaType functions.

**Step 3.1: Create test file with comprehensive test cases**

Create new file `cmd/jellywatch/audit_validation_test.go`:

```go
package main

import (
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/ai"
	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/database"
)

func TestInferTypeFromLibraryRoot(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		// TV paths - should return "episode"
		{"uppercase TVSHOWS", "/mnt/STORAGE1/TVSHOWS", "episode"},
		{"lowercase tv", "/data/tv/Breaking Bad", "episode"},
		{"TV Shows with space", "/media/TV Shows/Friends", "episode"},
		{"series keyword", "/storage/Series/The Office", "episode"},
		{"shows keyword", "/nas/Shows/Game of Thrones", "episode"},

		// Movie paths - should return "movie"
		{"uppercase Movies", "/mnt/STORAGE2/Movies", "movie"},
		{"lowercase movies", "/data/movies/The Matrix", "movie"},
		{"film keyword", "/media/Films/Inception", "movie"},
		{"films plural", "/nas/films/Interstellar", "movie"},

		// Unknown paths - should return "unknown"
		{"downloads folder", "/downloads/completed", "unknown"},
		{"generic media", "/media/content", "unknown"},
		{"numbered storage", "/mnt/storage1/data", "unknown"},
		{"empty string", "", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := inferTypeFromLibraryRoot(tt.path)
			if result != tt.expected {
				t.Errorf("inferTypeFromLibraryRoot(%q) = %q, want %q",
					tt.path, result, tt.expected)
			}
		})
	}
}

func TestValidateMediaType(t *testing.T) {
	// Config with no APIs enabled (simpler tests)
	cfg := &config.Config{}

	tests := []struct {
		name         string
		file         *database.MediaFile
		aiResult     *ai.Result
		expectValid  bool
		expectReason string // substring to check for in reason
	}{
		{
			name: "TV folder with AI type tv - valid",
			file: &database.MediaFile{
				LibraryRoot: "/mnt/TVSHOWS",
				MediaType:   "episode",
			},
			aiResult:     &ai.Result{Type: "tv", Title: "Breaking Bad"},
			expectValid:  true,
			expectReason: "",
		},
		{
			name: "TV folder with AI type movie - invalid",
			file: &database.MediaFile{
				LibraryRoot: "/mnt/TVSHOWS",
				MediaType:   "episode",
			},
			aiResult:     &ai.Result{Type: "movie", Title: "The Matrix"},
			expectValid:  false,
			expectReason: "AI suggests movie but file is in episode library",
		},
		{
			name: "Movies folder with AI type movie - valid",
			file: &database.MediaFile{
				LibraryRoot: "/mnt/Movies",
				MediaType:   "movie",
			},
			aiResult:     &ai.Result{Type: "movie", Title: "Inception"},
			expectValid:  true,
			expectReason: "",
		},
		{
			name: "Movies folder with AI type tv - invalid",
			file: &database.MediaFile{
				LibraryRoot: "/mnt/Movies",
				MediaType:   "movie",
			},
			aiResult:     &ai.Result{Type: "tv", Title: "Friends"},
			expectValid:  false,
			expectReason: "AI suggests episode but file is in movie library",
		},
		{
			name: "Unknown folder with AI type tv - valid (no validation)",
			file: &database.MediaFile{
				LibraryRoot: "/downloads/completed",
				MediaType:   "episode",
			},
			aiResult:     &ai.Result{Type: "tv", Title: "The Office"},
			expectValid:  true,
			expectReason: "",
		},
		{
			name: "Unknown folder with AI type movie - valid (no validation)",
			file: &database.MediaFile{
				LibraryRoot: "/downloads/completed",
				MediaType:   "movie",
			},
			aiResult:     &ai.Result{Type: "movie", Title: "Avatar"},
			expectValid:  true,
			expectReason: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, reason := validateMediaType(tt.file, tt.aiResult, cfg)
			if valid != tt.expectValid {
				t.Errorf("validateMediaType() valid = %v, want %v; reason = %q",
					valid, tt.expectValid, reason)
			}
			if tt.expectReason != "" && reason != tt.expectReason {
				t.Errorf("validateMediaType() reason = %q, want %q",
					reason, tt.expectReason)
			}
		})
	}
}
```

**Step 3.2: Run tests to verify they fail**

Run: `go test ./cmd/jellywatch -run "TestInferTypeFromLibraryRoot|TestValidateMediaType" -v`
Expected: FAIL with "undefined: inferTypeFromLibraryRoot" and "undefined: validateMediaType"

**Step 3.3: Commit**

```bash
git add cmd/jellywatch/audit_validation_test.go
git commit -m "test(audit): add failing tests for type validation functions

- TestInferTypeFromLibraryRoot: 13 test cases for path analysis
- TestValidateMediaType: 6 test cases for validation logic
- TDD approach: tests written before implementation"
```

---

## Task 4: Implement Type Validation Functions

**Files:**
- Create: `cmd/jellywatch/audit_validation.go`

**Why:** Implement inferTypeFromLibraryRoot and validateMediaType to make tests pass.

**Step 4.1: Create validation file with complete implementation**

Create new file `cmd/jellywatch/audit_validation.go`:

```go
package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/ai"
	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/radarr"
	"github.com/Nomadcxx/jellywatch/internal/sonarr"
)

// inferTypeFromLibraryRoot analyzes library path to infer expected media type.
// Returns "episode" for TV paths, "movie" for movie paths, "unknown" otherwise.
func inferTypeFromLibraryRoot(libraryRoot string) string {
	if libraryRoot == "" {
		return "unknown"
	}

	lower := strings.ToLower(libraryRoot)

	// Check for TV indicators (check these first as they're more specific)
	tvKeywords := []string{"tvshow", "tv show", "tv_show", "tv-show", "tvseries",
		"tv series", "tv_series", "tv-series", "/tv/", "/tv", "shows", "series"}
	for _, keyword := range tvKeywords {
		if strings.Contains(lower, keyword) {
			return "episode"
		}
	}

	// Check for movie indicators
	movieKeywords := []string{"movie", "film", "/mv/", "/mv"}
	for _, keyword := range movieKeywords {
		if strings.Contains(lower, keyword) {
			return "movie"
		}
	}

	return "unknown"
}

// mapAITypeToMediaType converts AI Result.Type ("tv", "movie") to MediaFile.MediaType ("episode", "movie")
func mapAITypeToMediaType(aiType string) string {
	switch aiType {
	case "tv":
		return "episode"
	case "movie":
		return "movie"
	default:
		return "unknown"
	}
}

// validateMediaType checks if AI-suggested type matches library context.
// Returns (valid, reason) where reason explains why validation failed.
//
// Validation layers:
// 1. Primary: library_root path analysis (if path indicates TV or Movies)
// 2. Secondary: Sonarr/Radarr API lookup (if APIs are configured and enabled)
//
// If library_root and API disagree, returns invalid with explanation.
func validateMediaType(file *database.MediaFile, aiResult *ai.Result, cfg *config.Config) (valid bool, reason string) {
	// Map AI type to media type for comparison
	aiMediaType := mapAITypeToMediaType(aiResult.Type)

	// Primary validation: Check library_root path
	libraryType := inferTypeFromLibraryRoot(file.LibraryRoot)

	if libraryType != "unknown" {
		// Library path clearly indicates type - validate against it
		if libraryType != aiMediaType {
			return false, fmt.Sprintf("AI suggests %s but file is in %s library", aiMediaType, libraryType)
		}
		// Library type matches AI type - valid
		return true, ""
	}

	// Secondary validation: Check Sonarr/Radarr APIs if configured
	sonarrMatch := false
	radarrMatch := false

	// Check Sonarr for TV series match
	if cfg.Sonarr.Enabled && cfg.Sonarr.URL != "" && cfg.Sonarr.APIKey != "" {
		sonarrClient := sonarr.NewClient(sonarr.Config{
			URL:     cfg.Sonarr.URL,
			APIKey:  cfg.Sonarr.APIKey,
			Timeout: 10 * time.Second,
		})

		series, err := sonarrClient.FindSeriesByTitle(aiResult.Title)
		if err == nil && len(series) > 0 {
			sonarrMatch = true
		}
		// Ignore API errors - fall through to trust AI
	}

	// Check Radarr for movie match
	if cfg.Radarr.Enabled && cfg.Radarr.URL != "" && cfg.Radarr.APIKey != "" {
		radarrClient := radarr.NewClient(radarr.Config{
			URL:     cfg.Radarr.URL,
			APIKey:  cfg.Radarr.APIKey,
			Timeout: 10 * time.Second,
		})

		// Note: Using GetAllMovies and filtering is expensive
		// TODO: Add FindMoviesByTitle to radarr client if needed
		movies, err := radarrClient.GetAllMovies()
		if err == nil {
			titleLower := strings.ToLower(aiResult.Title)
			for _, m := range movies {
				if strings.ToLower(m.Title) == titleLower {
					radarrMatch = true
					break
				}
			}
		}
		// Ignore API errors - fall through to trust AI
	}

	// Conflict resolution: both APIs found matches
	if sonarrMatch && radarrMatch {
		return false, fmt.Sprintf("Ambiguous: '%s' found in both Sonarr (TV) and Radarr (Movies)", aiResult.Title)
	}

	// API confirms type mismatch
	if sonarrMatch && aiMediaType != "episode" {
		return false, fmt.Sprintf("Sonarr confirms '%s' is a TV series, but AI suggests movie", aiResult.Title)
	}
	if radarrMatch && aiMediaType != "movie" {
		return false, fmt.Sprintf("Radarr confirms '%s' is a movie, but AI suggests episode", aiResult.Title)
	}

	// No contradiction found - trust AI
	return true, ""
}
```

**Step 4.2: Run tests to verify they pass**

Run: `go test ./cmd/jellywatch -run "TestInferTypeFromLibraryRoot|TestValidateMediaType" -v`
Expected: All tests pass

**Step 4.3: Verify build**

Run: `go build ./cmd/jellywatch`
Expected: Success

**Step 4.4: Commit**

```bash
git add cmd/jellywatch/audit_validation.go
git commit -m "feat(audit): implement type validation functions

- inferTypeFromLibraryRoot: analyzes path for TV/movie keywords
- validateMediaType: validates AI type against library + APIs
- Primary: library_root path analysis
- Secondary: Sonarr/Radarr API lookup when configured
- Handles conflicts between library and API results"
```

---

## Task 5: Create Progress Bar Helper

**Files:**
- Modify: `cmd/jellywatch/audit_validation.go` (add to existing file)

**Why:** Need reusable progress bar rendering function for consistent display.

**Step 5.1: Add progress bar types and functions**

Append to `cmd/jellywatch/audit_validation.go`:

```go
// ProgressBar tracks and displays audit generation progress
type ProgressBar struct {
	total          int
	current        int
	actionsCreated int
	errorsCount    int
	updateInterval int // Update display every N items
}

// NewProgressBar creates a new progress bar for audit generation
func NewProgressBar(total int) *ProgressBar {
	return &ProgressBar{
		total:          total,
		current:        0,
		actionsCreated: 0,
		errorsCount:    0,
		updateInterval: 10,
	}
}

// Update increments the progress counter and optionally updates display
func (p *ProgressBar) Update(actionsCreated, errorsCount int) {
	p.current++
	p.actionsCreated = actionsCreated
	p.errorsCount = errorsCount

	// Update display every N items or at completion
	if p.current%p.updateInterval == 0 || p.current == p.total {
		p.render()
	}
}

// render draws the progress bar to stdout
func (p *ProgressBar) render() {
	if p.total == 0 {
		return
	}

	percentage := float64(p.current) / float64(p.total) * 100
	barWidth := 20
	filled := int(float64(barWidth) * float64(p.current) / float64(p.total))

	// Build progress bar string
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	// Use carriage return to overwrite previous line
	fmt.Printf("\r%s %3.0f%% (%d/%d files) | %d actions | %d errors",
		bar, percentage, p.current, p.total, p.actionsCreated, p.errorsCount)
}

// Finish completes the progress bar and moves to next line
func (p *ProgressBar) Finish() {
	p.render() // Final update
	fmt.Println() // Move to next line
}
```

**Step 5.2: Add test for progress bar**

Append to `cmd/jellywatch/audit_validation_test.go`:

```go
func TestProgressBar(t *testing.T) {
	t.Run("tracks progress correctly", func(t *testing.T) {
		pb := NewProgressBar(100)

		if pb.total != 100 {
			t.Errorf("total = %d, want 100", pb.total)
		}
		if pb.current != 0 {
			t.Errorf("current = %d, want 0", pb.current)
		}

		pb.Update(5, 1)
		if pb.current != 1 {
			t.Errorf("after Update, current = %d, want 1", pb.current)
		}
		if pb.actionsCreated != 5 {
			t.Errorf("actionsCreated = %d, want 5", pb.actionsCreated)
		}
	})

	t.Run("handles zero total", func(t *testing.T) {
		pb := NewProgressBar(0)
		pb.Update(0, 0) // Should not panic
		pb.Finish()     // Should not panic
	})
}
```

**Step 5.3: Run tests**

Run: `go test ./cmd/jellywatch -run TestProgressBar -v`
Expected: All tests pass

**Step 5.4: Commit**

```bash
git add cmd/jellywatch/audit_validation.go cmd/jellywatch/audit_validation_test.go
git commit -m "feat(audit): add ProgressBar helper for audit generation

- Tracks current/total, actions created, errors
- Updates display every 10 items (configurable)
- Uses carriage return for in-place updates
- Handles edge cases (zero total)"
```

---

## Task 6: Create Statistics Tracker

**Files:**
- Modify: `cmd/jellywatch/audit_validation.go` (add to existing file)

**Why:** Need a struct to track all skip reasons and AI statistics during processing.

**Step 6.1: Add statistics tracker**

Append to `cmd/jellywatch/audit_validation.go`:

```go
// AuditStats tracks statistics during audit generation
type AuditStats struct {
	AITotalCalls     int
	AISuccessCount   int
	AIErrorCount     int
	TypeMismatches   int
	ConfidenceTooLow int
	TitleUnchanged   int
	ActionsCreated   int
}

// NewAuditStats creates a new statistics tracker
func NewAuditStats() *AuditStats {
	return &AuditStats{}
}

// RecordAICall records an AI call result
func (s *AuditStats) RecordAICall(success bool) {
	s.AITotalCalls++
	if success {
		s.AISuccessCount++
	} else {
		s.AIErrorCount++
	}
}

// RecordSkip records a skipped file with reason
func (s *AuditStats) RecordSkip(reason string) {
	switch {
	case strings.Contains(reason, "type") || strings.Contains(reason, "library"):
		s.TypeMismatches++
	case strings.Contains(reason, "confidence"):
		s.ConfidenceTooLow++
	case strings.Contains(reason, "unchanged") || strings.Contains(reason, "same"):
		s.TitleUnchanged++
	}
}

// RecordAction records a created action
func (s *AuditStats) RecordAction() {
	s.ActionsCreated++
}

// ToSummary converts stats to AuditSummary fields
func (s *AuditStats) ToSummary(totalFiles int) (aiTotal, aiSuccess, aiError, typeMismatch, confLow, titleUnch int) {
	return s.AITotalCalls, s.AISuccessCount, s.AIErrorCount,
		s.TypeMismatches, s.ConfidenceTooLow, s.TitleUnchanged
}
```

**Step 6.2: Add test for stats tracker**

Append to `cmd/jellywatch/audit_validation_test.go`:

```go
func TestAuditStats(t *testing.T) {
	stats := NewAuditStats()

	// Record some AI calls
	stats.RecordAICall(true)
	stats.RecordAICall(true)
	stats.RecordAICall(false)

	if stats.AITotalCalls != 3 {
		t.Errorf("AITotalCalls = %d, want 3", stats.AITotalCalls)
	}
	if stats.AISuccessCount != 2 {
		t.Errorf("AISuccessCount = %d, want 2", stats.AISuccessCount)
	}
	if stats.AIErrorCount != 1 {
		t.Errorf("AIErrorCount = %d, want 1", stats.AIErrorCount)
	}

	// Record skips
	stats.RecordSkip("AI suggests movie but file is in episode library")
	stats.RecordSkip("confidence too low")
	stats.RecordSkip("title unchanged")

	if stats.TypeMismatches != 1 {
		t.Errorf("TypeMismatches = %d, want 1", stats.TypeMismatches)
	}
	if stats.ConfidenceTooLow != 1 {
		t.Errorf("ConfidenceTooLow = %d, want 1", stats.ConfidenceTooLow)
	}
	if stats.TitleUnchanged != 1 {
		t.Errorf("TitleUnchanged = %d, want 1", stats.TitleUnchanged)
	}

	// Record action
	stats.RecordAction()
	if stats.ActionsCreated != 1 {
		t.Errorf("ActionsCreated = %d, want 1", stats.ActionsCreated)
	}
}
```

**Step 6.3: Run tests**

Run: `go test ./cmd/jellywatch -run TestAuditStats -v`
Expected: All tests pass

**Step 6.4: Commit**

```bash
git add cmd/jellywatch/audit_validation.go cmd/jellywatch/audit_validation_test.go
git commit -m "feat(audit): add AuditStats tracker for statistics

- Tracks AI calls (total, success, error)
- Categorizes skip reasons automatically
- Integrates with AuditSummary struct"
```

---

## Task 7: Remove Dead Code (Bug #1)

**Files:**
- Modify: `cmd/jellywatch/audit_cmd.go:147-150`

**Why:** Lines 147-150 contain unused mediaType variable with backwards logic.

**Step 7.1: Read current code**

Read `cmd/jellywatch/audit_cmd.go` lines 144-155 to see current loop start.

**Step 7.2: Remove dead code**

Find this code block (lines 144-152):
```go
	// Generate AI suggestions for each file
	actions := make([]plans.AuditAction, 0, len(files))
	for _, file := range files {
		mediaType := file.MediaType
		if mediaType == "movie" {
			mediaType = "tv" // AI expects "tv" for episodes
		}

		ctx := context.Background()
```

Replace with (removing lines 147-150):
```go
	// Generate AI suggestions for each file
	actions := make([]plans.AuditAction, 0, len(files))
	for _, file := range files {
		ctx := context.Background()
```

**Step 7.3: Verify**

Run: `go build ./cmd/jellywatch`
Expected: Success, no "declared and not used" error

Run: `go vet ./cmd/jellywatch`
Expected: No warnings

**Step 7.4: Commit**

```bash
git add cmd/jellywatch/audit_cmd.go
git commit -m "fix(audit): remove dead code from generateAudit

- Remove unused mediaType variable (lines 147-150)
- Fix backwards logic that incorrectly swapped movie->tv
- Resolves LSP error: declared and not used

Bug #1: dead code removal"
```

---

## Task 8: Integrate Progress Bar and Statistics into generateAudit

**Files:**
- Modify: `cmd/jellywatch/audit_cmd.go:123-210`

**Why:** This is the main integration task - rewrites generateAudit to use progress bar, statistics tracking, and type validation.

**Step 8.1: Read current generateAudit function**

Read `cmd/jellywatch/audit_cmd.go` lines 123-210 to understand current structure.

**Step 8.2: Rewrite generateAudit function**

Replace the entire generateAudit function (lines 123-210) with:

```go
func generateAudit(db *database.MediaDB, cfg *config.Config, threshold float64, limit int) error {
	fmt.Printf("🔍 Scanning for files with confidence < %.2f\n", threshold)

	files, err := db.GetLowConfidenceFiles(threshold, limit)
	if err != nil {
		return fmt.Errorf("failed to query low-confidence files: %w", err)
	}

	if len(files) == 0 {
		fmt.Println("✓ No low-confidence files found")
		return nil
	}

	fmt.Printf("Found %d low-confidence files\n", len(files))

	// Initialize AI matcher
	matcher, err := ai.NewMatcher(cfg.AI)
	if err != nil {
		return fmt.Errorf("failed to initialize AI matcher: %w", err)
	}

	// Initialize tracking
	actions := make([]plans.AuditAction, 0, len(files))
	items := make([]plans.AuditItem, len(files))
	stats := NewAuditStats()
	progress := NewProgressBar(len(files))

	fmt.Printf("\nProcessing with AI...\n")

	// Process each file
	for i, file := range files {
		// Convert to AuditItem first
		items[i] = plans.AuditItem{
			ID:         file.ID,
			Path:       file.Path,
			Size:       file.Size,
			MediaType:  file.MediaType,
			Title:      file.NormalizedTitle,
			Year:       file.Year,
			Season:     file.Season,
			Episode:    file.Episode,
			Confidence: file.Confidence,
			Resolution: file.Resolution,
			SourceType: file.SourceType,
		}

		// Call AI matcher
		ctx := context.Background()
		aiResult, err := matcher.Parse(ctx, filepath.Base(file.Path))

		if err != nil {
			stats.RecordAICall(false)
			items[i].SkipReason = fmt.Sprintf("AI error: %v", err)
			progress.Update(len(actions), stats.AIErrorCount)
			continue
		}

		stats.RecordAICall(true)

		// Check confidence threshold
		if aiResult.Confidence < cfg.AI.ConfidenceThreshold {
			stats.RecordSkip("confidence too low")
			items[i].SkipReason = fmt.Sprintf("AI confidence %.2f below threshold %.2f",
				aiResult.Confidence, cfg.AI.ConfidenceThreshold)
			progress.Update(len(actions), stats.AIErrorCount)
			continue
		}

		// Check if title actually changed
		if aiResult.Title == file.NormalizedTitle {
			stats.RecordSkip("title unchanged")
			items[i].SkipReason = "Title unchanged after AI analysis"
			progress.Update(len(actions), stats.AIErrorCount)
			continue
		}

		// Validate media type
		valid, reason := validateMediaType(file, aiResult, cfg)
		if !valid {
			stats.RecordSkip(reason)
			items[i].SkipReason = reason
			progress.Update(len(actions), stats.AIErrorCount)
			continue
		}

		// Build action
		var newSeason, newEpisode *int
		if aiResult.Season != nil {
			newSeason = aiResult.Season
		} else {
			newSeason = file.Season
		}

		if len(aiResult.Episodes) > 0 {
			newEpisode = &aiResult.Episodes[0]
		} else {
			newEpisode = file.Episode
		}

		action := plans.AuditAction{
			Action:     "rename",
			NewTitle:   aiResult.Title,
			NewYear:    aiResult.Year,
			NewSeason:  newSeason,
			NewEpisode: newEpisode,
			NewPath:    buildCorrectPath(file.Path, aiResult.Title, aiResult.Year, newSeason, newEpisode),
			Reasoning:  fmt.Sprintf("AI suggested: %s (confidence: %.2f)", aiResult.Title, aiResult.Confidence),
			Confidence: aiResult.Confidence,
		}

		actions = append(actions, action)
		stats.RecordAction()
		progress.Update(len(actions), stats.AIErrorCount)
	}

	progress.Finish()

	// Build plan with correct statistics
	plan := &plans.AuditPlan{
		CreatedAt: time.Now(),
		Command:   "audit",
		Summary: plans.AuditSummary{
			TotalFiles:            len(files),
			FilesToRename:         len(actions),
			FilesToDelete:         0,
			FilesToSkip:           len(files) - len(actions),
			AvgConfidence:         calculateAvgConfidence(files),
			AITotalCalls:          stats.AITotalCalls,
			AISuccessCount:        stats.AISuccessCount,
			AIErrorCount:          stats.AIErrorCount,
			TypeMismatchesSkipped: stats.TypeMismatches,
			ConfidenceTooLow:      stats.ConfidenceTooLow,
			TitleUnchanged:        stats.TitleUnchanged,
		},
		Items:   items,
		Actions: actions,
	}

	// Save plan
	err = plans.SaveAuditPlans(plan)
	if err != nil {
		return fmt.Errorf("failed to save audit plans: %w", err)
	}

	// Print summary
	printAuditSummary(plan)

	return nil
}

// printAuditSummary displays the audit generation results
func printAuditSummary(plan *plans.AuditPlan) {
	fmt.Printf("\n✓ Processing complete\n\n")
	fmt.Printf("Summary:\n")
	fmt.Printf("  Total files analyzed: %d\n", plan.Summary.TotalFiles)
	fmt.Printf("  AI calls: %d (%d successful, %d errors)\n",
		plan.Summary.AITotalCalls, plan.Summary.AISuccessCount, plan.Summary.AIErrorCount)
	fmt.Printf("  Actions created: %d (renames)\n", plan.Summary.FilesToRename)
	fmt.Printf("  Skipped: %d\n", plan.Summary.FilesToSkip)

	if plan.Summary.FilesToSkip > 0 {
		if plan.Summary.ConfidenceTooLow > 0 {
			fmt.Printf("    - AI confidence too low: %d\n", plan.Summary.ConfidenceTooLow)
		}
		if plan.Summary.TypeMismatchesSkipped > 0 {
			fmt.Printf("    - Type validation failed: %d\n", plan.Summary.TypeMismatchesSkipped)
		}
		if plan.Summary.TitleUnchanged > 0 {
			fmt.Printf("    - Title unchanged: %d\n", plan.Summary.TitleUnchanged)
		}
		otherSkips := plan.Summary.FilesToSkip - plan.Summary.ConfidenceTooLow -
			plan.Summary.TypeMismatchesSkipped - plan.Summary.TitleUnchanged
		if otherSkips > 0 {
			fmt.Printf("    - Other (AI errors, etc.): %d\n", otherSkips)
		}
	}

	fmt.Printf("\n📁 Plan saved to: %s\n", getAuditPlansPath())
	if plan.Summary.FilesToRename > 0 {
		fmt.Printf("💡 Run 'jellywatch audit --dry-run' to preview changes\n")
	}
}
```

**Step 8.3: Verify build and tests**

Run: `go build ./cmd/jellywatch`
Expected: Success

Run: `go test ./cmd/jellywatch/... -v`
Expected: All tests pass

**Step 8.4: Commit**

```bash
git add cmd/jellywatch/audit_cmd.go
git commit -m "feat(audit): integrate progress bar, stats, and type validation

- Add progress bar with batch updates every 10 files
- Track AI statistics (calls, success, errors)
- Integrate validateMediaType for type checking
- Populate Actions field in AuditPlan
- Fix FilesToRename/FilesToSkip calculation (Bug #2)
- Add detailed summary output with skip breakdown

Bug #2: hardcoded summary counts fixed
Bug #3: progress indication added"
```

---

## Task 9: Update filesToAuditItems Function

**Files:**
- Modify: `cmd/jellywatch/audit_cmd.go:319-337`

**Why:** The filesToAuditItems function is now redundant since we build items in the main loop. We should either remove it or keep it for compatibility.

**Step 9.1: Check if function is used elsewhere**

Run: `grep -r "filesToAuditItems" ./cmd/jellywatch/`
Expected: Only in audit_cmd.go

**Step 9.2: Decision point**

If only used in generateAudit (which now builds items inline), mark as deprecated or remove.

Since we now build items inline with SkipReason, we can remove this function.

Find and remove:
```go
func filesToAuditItems(files []*database.MediaFile) []plans.AuditItem {
	items := make([]plans.AuditItem, len(files))
	for i, file := range files {
		items[i] = plans.AuditItem{
			ID:         file.ID,
			Path:       file.Path,
			Size:       file.Size,
			MediaType:   file.MediaType,
			Title:      file.NormalizedTitle,
			Year:       file.Year,
			Season:     file.Season,
			Episode:    file.Episode,
			Confidence:  file.Confidence,
			Resolution: file.Resolution,
			SourceType: file.SourceType,
		}
	}
	return items
}
```

**Step 9.3: Verify**

Run: `go build ./cmd/jellywatch`
Expected: Success (function not used)

**Step 9.4: Commit**

```bash
git add cmd/jellywatch/audit_cmd.go
git commit -m "refactor(audit): remove unused filesToAuditItems function

- Items now built inline with SkipReason support
- Reduces code duplication"
```

---

## Task 10: Integration Testing

**Files:**
- Testing only (no code changes)

**Why:** Verify the complete implementation works end-to-end.

**Step 10.1: Build fresh binary**

Run: `go build -o /tmp/jellywatch ./cmd/jellywatch`
Expected: Success

**Step 10.2: Test audit generate with small limit**

Run: `/tmp/jellywatch audit --generate --limit 5`

Expected output pattern:
```
🔍 Scanning for files with confidence < 0.80
Found 5 low-confidence files

Processing with AI...
████████████████████ 100% (5/5 files) | 2 actions | 0 errors

✓ Processing complete

Summary:
  Total files analyzed: 5
  AI calls: 5 (5 successful, 0 errors)
  Actions created: 2 (renames)
  Skipped: 3
    - AI confidence too low: 1
    - Type validation failed: 1
    - Title unchanged: 1

📁 Plan saved to: /home/user/.config/jellywatch/plans/audit.json
💡 Run 'jellywatch audit --dry-run' to preview changes
```

**Step 10.3: Verify plan JSON structure**

Run: `cat ~/.config/jellywatch/plans/audit.json | jq '.summary'`

Expected fields present:
- `ai_total_calls`
- `ai_success_count`
- `ai_error_count`
- `type_mismatches_skipped`
- `confidence_too_low`
- `title_unchanged`

Run: `cat ~/.config/jellywatch/plans/audit.json | jq '.actions | length'`
Expected: Should match "Actions created" from summary

Run: `cat ~/.config/jellywatch/plans/audit.json | jq '.items[] | select(.skip_reason != null) | .skip_reason'`
Expected: Shows skip reasons for skipped items

**Step 10.4: Test audit dry-run**

Run: `/tmp/jellywatch audit --dry-run`
Expected: Displays plan summary and actions correctly

**Step 10.5: Test with larger limit**

Run: `/tmp/jellywatch audit --generate --limit 50`
Expected: Progress bar updates smoothly every 10 files

**Step 10.6: Document results**

Create test results file (optional):
```bash
echo "Integration test results - $(date)" > /tmp/audit-test-results.txt
echo "Build: SUCCESS" >> /tmp/audit-test-results.txt
echo "Generate with limit 5: SUCCESS" >> /tmp/audit-test-results.txt
echo "Plan JSON structure: VALID" >> /tmp/audit-test-results.txt
echo "Dry-run display: SUCCESS" >> /tmp/audit-test-results.txt
```

---

## Task 11: Run Full Test Suite

**Files:**
- Testing only

**Why:** Ensure no regressions in other parts of the codebase.

**Step 11.1: Run all tests**

Run: `go test ./... -v`
Expected: All tests pass

**Step 11.2: Run with race detector**

Run: `go test -race ./...`
Expected: No race conditions detected

**Step 11.3: Run linter (if available)**

Run: `golangci-lint run ./...` (if installed)
Expected: No critical issues

**Step 11.4: Final commit**

```bash
git add -A
git commit -m "test(audit): verify all tests pass after bug fixes

- All unit tests pass
- Integration tests pass
- No race conditions
- Build successful"
```

---

## Summary Checklist

| Task | Description | Status |
|------|-------------|--------|
| 1 | Add SkipReason to AuditItem | Pending |
| 2 | Expand AuditSummary with stats | Pending |
| 3 | Create type validation tests | Pending |
| 4 | Implement type validation | Pending |
| 5 | Create ProgressBar helper | Pending |
| 6 | Create AuditStats tracker | Pending |
| 7 | Remove dead code (Bug #1) | Pending |
| 8 | Integrate all into generateAudit | Pending |
| 9 | Remove unused filesToAuditItems | Pending |
| 10 | Integration testing | Pending |
| 11 | Full test suite | Pending |

---

## Rollback Plan

If implementation causes issues:

1. Revert all commits: `git revert --no-commit HEAD~N..HEAD` (where N = number of commits)
2. Or checkout specific files: `git checkout HEAD~N -- cmd/jellywatch/audit_cmd.go`
3. Rebuild and verify: `go build ./cmd/jellywatch && go test ./...`
