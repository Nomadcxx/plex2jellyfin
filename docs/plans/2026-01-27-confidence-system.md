# Confidence System Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add regex confidence scoring to detect low-quality parses, auto-trigger AI for poor parses, and provide an audit command for manual review.

**Architecture:** Calculate confidence (0.0-1.0) during filename parsing based on title quality indicators. Store confidence in database. Auto-trigger AI when confidence < 0.6 (if enabled). Provide `jellywatch audit` command with `--generate/--dry-run/--execute` for manual review of files with confidence < 0.8.

**Tech Stack:** Go, SQLite (schema migration), existing naming package, existing AI matcher, JSON plan storage

---

### Task 1: Create confidence.go with CalculateTitleConfidence

**Files:**
- Create: `internal/naming/confidence.go`
- Test: `internal/naming/confidence_test.go`

**Step 1: Write the failing test**

```go
// internal/naming/confidence_test.go
package naming

import "testing"

func TestCalculateTitleConfidence(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		original string
		wantMin  float64
		wantMax  float64
	}{
		{
			name:     "clean title with year",
			title:    "The Matrix",
			original: "The.Matrix.1999.1080p.BluRay.mkv",
			wantMin:  0.9,
			wantMax:  1.0,
		},
		{
			name:     "garbage title (release group)",
			title:    "RARBG",
			original: "RARBG.mkv",
			wantMin:  0.0,
			wantMax:  0.3,
		},
		{
			name:     "very short title",
			title:    "IT",
			original: "IT.2017.mkv",
			wantMin:  0.4,
			wantMax:  0.7,
		},
		{
			name:     "single word no spaces",
			title:    "Interstellar",
			original: "Interstellar.2014.mkv",
			wantMin:  0.5,
			wantMax:  0.9,
		},
		{
			name:     "duplicate year pattern",
			title:    "Matrix (2001)",
			original: "Matrix (2001) (2001).mkv",
			wantMin:  0.0,
			wantMax:  0.6,
		},
		{
			name:     "codec in title",
			title:    "Movie x264",
			original: "Movie.x264.mkv",
			wantMin:  0.0,
			wantMax:  0.5,
		},
		{
			name:     "resolution in title",
			title:    "Movie 1080p",
			original: "Movie.1080p.mkv",
			wantMin:  0.0,
			wantMax:  0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateTitleConfidence(tt.title, tt.original)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("CalculateTitleConfidence(%q, %q) = %v, want between %v and %v",
					tt.title, tt.original, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestHasDuplicateYear(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"Matrix (2001) (2001)", true},
		{"Matrix (2001)", false},
		{"2001 A Space Odyssey (2001)", false}, // Title year != release year is OK
		{"Movie 2020 2020", true},
		{"Movie (2020) 2020", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := hasDuplicateYear(tt.input)
			if got != tt.want {
				t.Errorf("hasDuplicateYear(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/naming -run TestCalculateTitleConfidence -v`
Expected: FAIL - undefined: CalculateTitleConfidence

**Step 3: Write minimal implementation**

```go
// internal/naming/confidence.go
package naming

import (
	"regexp"
	"strings"
)

var (
	// Matches codec patterns at end of title
	codecSuffixRegex = regexp.MustCompile(`(?i)\b(x264|x265|h264|h265|hevc|avc|av1|xvid|divx)$`)
	// Matches resolution patterns at end of title
	resolutionSuffixRegex = regexp.MustCompile(`(?i)\b(2160p|1080p|720p|480p|4k|uhd)$`)
	// Matches duplicate year pattern
	duplicateYearRegex = regexp.MustCompile(`\((\d{4})\).*\(?\1\)?|\b(\d{4})\b.*\b\2\b`)
)

// CalculateTitleConfidence calculates a confidence score (0.0-1.0) for a parsed title.
// Higher scores indicate cleaner, more reliable parses.
// Uses the existing blacklist.go for release group detection.
func CalculateTitleConfidence(title, originalFilename string) float64 {
	confidence := 1.0

	// Major penalties
	if IsGarbageTitle(title) {
		confidence -= 0.8
	}
	if IsObfuscatedFilename(originalFilename) {
		confidence -= 0.9
	}
	if hasDuplicateYear(originalFilename) {
		confidence -= 0.5
	}

	// Moderate penalties
	if len(title) < 3 {
		confidence -= 0.5
	}
	if endsWithCodecOrSource(title) {
		confidence -= 0.4
	}
	if hasResolutionInTitle(title) {
		confidence -= 0.4
	}
	if !strings.Contains(title, " ") && len(title) > 3 {
		confidence -= 0.3 // Single word (but not short abbreviations)
	}
	if hasReleaseMarkers(originalFilename) {
		confidence -= 0.1
	}

	// Bonuses
	if HasYearInParentheses(originalFilename) {
		confidence += 0.1
	}

	return clamp(confidence, 0.0, 1.0)
}

// hasDuplicateYear detects patterns like "Matrix (2001) (2001)" or "Movie 2020 2020"
func hasDuplicateYear(s string) bool {
	// Find all 4-digit years
	yearRegex := regexp.MustCompile(`\b(19|20)\d{2}\b`)
	matches := yearRegex.FindAllString(s, -1)

	if len(matches) < 2 {
		return false
	}

	// Check if any year appears more than once
	seen := make(map[string]int)
	for _, year := range matches {
		seen[year]++
		if seen[year] > 1 {
			return true
		}
	}
	return false
}

// endsWithCodecOrSource checks if title ends with codec/source markers
func endsWithCodecOrSource(title string) bool {
	return codecSuffixRegex.MatchString(title)
}

// hasResolutionInTitle checks if title contains resolution markers
func hasResolutionInTitle(title string) bool {
	return resolutionSuffixRegex.MatchString(title)
}

// hasReleaseMarkers checks if original filename has release group markers
func hasReleaseMarkers(filename string) bool {
	// Check for common release markers in filename
	markers := []string{
		"RARBG", "YTS", "YIFY", "SPARKS", "FGT", "NTb", "FLUX",
		"BluRay", "WEB-DL", "WEBRip", "HDTV", "REMUX",
	}
	upper := strings.ToUpper(filename)
	for _, m := range markers {
		if strings.Contains(upper, m) {
			return true
		}
	}
	return false
}

// clamp restricts value to [min, max] range
func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/naming -run TestCalculateTitleConfidence -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/naming/confidence.go internal/naming/confidence_test.go
git commit -m "$(cat <<'EOF'
feat(naming): add confidence scoring for parsed titles

Calculates 0.0-1.0 confidence based on title quality indicators:
- Garbage titles (release groups) → -0.8
- Obfuscated filenames → -0.9
- Duplicate year patterns → -0.5
- Short titles, single words, codec/resolution artifacts

Uses existing blacklist.go for release group detection.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Add database schema migration for confidence columns

**Files:**
- Modify: `internal/database/schema.go:426-433`
- Test: Manual verification via `jellywatch sync`

**Step 1: Write the migration SQL**

Add migration 10 to the migrations slice in schema.go:

```go
	{
		version: 10,
		up: []string{
			// Add confidence and parsing metadata to media_files
			`ALTER TABLE media_files ADD COLUMN confidence REAL DEFAULT 1.0`,
			`ALTER TABLE media_files ADD COLUMN parse_method TEXT DEFAULT 'regex'`,
			`ALTER TABLE media_files ADD COLUMN needs_review INTEGER DEFAULT 0`,

			// Index for efficient audit queries
			`CREATE INDEX idx_media_files_confidence ON media_files(confidence)`,
			`CREATE INDEX idx_media_files_needs_review ON media_files(needs_review)`,

			// Update schema version
			`INSERT INTO schema_version (version) VALUES (10)`,
		},
	},
```

**Step 2: Update currentSchemaVersion**

Change line 6 from:
```go
const currentSchemaVersion = 9
```
to:
```go
const currentSchemaVersion = 10
```

**Step 3: Test migration applies**

Run: `go build ./cmd/jellywatch && ./jellywatch sync --dry-run`
Expected: No errors, migration applies silently

**Step 4: Verify columns exist**

Run: `sqlite3 ~/.config/jellywatch/jellywatch.db ".schema media_files" | grep -E "(confidence|parse_method|needs_review)"`
Expected: Shows the three new columns

**Step 5: Commit**

```bash
git add internal/database/schema.go
git commit -m "$(cat <<'EOF'
feat(db): add confidence tracking columns to media_files

Migration 10 adds:
- confidence: Parse quality score (0.0-1.0)
- parse_method: 'regex', 'ai', 'manual', or 'folder'
- needs_review: Flag for audit command

Includes indexes for efficient audit queries.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Update MediaFile struct and database operations

**Files:**
- Modify: `internal/database/media_file.go` (find via grep)
- Test: Existing tests should still pass

**Step 1: Find the MediaFile struct location**

Run: `grep -n "type MediaFile struct" internal/database/*.go`

**Step 2: Add fields to MediaFile struct**

Add these fields to the MediaFile struct:

```go
	// Confidence tracking
	Confidence   float64 `json:"confidence"`
	ParseMethod  string  `json:"parse_method"`
	NeedsReview  bool    `json:"needs_review"`
```

**Step 3: Update UpsertMediaFile to include new columns**

Find the INSERT statement and add the new columns:

```go
// In the INSERT statement, add:
// confidence, parse_method, needs_review

// In the VALUES clause, add:
// ?, ?, ?

// In the args slice, add:
// file.Confidence, file.ParseMethod, boolToInt(file.NeedsReview)

// In the ON CONFLICT UPDATE clause, add:
// confidence = excluded.confidence,
// parse_method = excluded.parse_method,
// needs_review = excluded.needs_review,
```

**Step 4: Update scan functions to read new columns**

Find scanMediaFile and add scanning for new columns.

**Step 5: Run existing tests**

Run: `go test ./internal/database/... -v`
Expected: PASS (existing tests should still work)

**Step 6: Commit**

```bash
git add internal/database/
git commit -m "$(cat <<'EOF'
feat(db): add confidence fields to MediaFile operations

Updates UpsertMediaFile and scan functions to handle:
- confidence (float64)
- parse_method (string)
- needs_review (bool)

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Integrate confidence calculation into scanner

**Files:**
- Modify: `internal/scanner/scanner.go:262-341` (processFile function)
- Test: `internal/scanner/scanner_test.go`

**Step 1: Write the failing test**

Add to scanner_test.go:

```go
func TestProcessFileCalculatesConfidence(t *testing.T) {
	// Create temp DB
	db, cleanup := setupTestDB(t)
	defer cleanup()

	scanner := NewFileScanner(db)

	// Create a test file with clean name
	tmpDir := t.TempDir()
	cleanFile := filepath.Join(tmpDir, "The.Matrix.1999.1080p.BluRay.mkv")
	if err := os.WriteFile(cleanFile, make([]byte, 100*1024*1024), 0644); err != nil {
		t.Fatal(err)
	}

	info, _ := os.Stat(cleanFile)
	err := scanner.processFile(cleanFile, info, tmpDir, "movie")
	if err != nil {
		t.Fatalf("processFile failed: %v", err)
	}

	// Verify confidence was calculated
	file, err := db.GetMediaFileByPath(cleanFile)
	if err != nil {
		t.Fatalf("GetMediaFileByPath failed: %v", err)
	}

	if file.Confidence < 0.8 {
		t.Errorf("Expected high confidence for clean filename, got %v", file.Confidence)
	}
	if file.ParseMethod != "regex" {
		t.Errorf("Expected parse_method='regex', got %q", file.ParseMethod)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/scanner -run TestProcessFileCalculatesConfidence -v`
Expected: FAIL (confidence not being set)

**Step 3: Update processFile to calculate confidence**

In `internal/scanner/scanner.go`, update the processFile function around line 316:

```go
	// Calculate confidence for the parse
	confidence := naming.CalculateTitleConfidence(normalizedTitle, filename)
	needsReview := confidence < 0.8

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
		// New confidence fields
		Confidence:  confidence,
		ParseMethod: "regex",
		NeedsReview: needsReview,
	}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/scanner -run TestProcessFileCalculatesConfidence -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/scanner/scanner.go internal/scanner/scanner_test.go
git commit -m "$(cat <<'EOF'
feat(scanner): calculate confidence during file scanning

processFile now:
- Calculates confidence using naming.CalculateTitleConfidence
- Sets parse_method to 'regex'
- Flags needs_review when confidence < 0.8

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Add database query for low-confidence files

**Files:**
- Modify: `internal/database/media_file.go`
- Test: `internal/database/media_file_test.go`

**Step 1: Write the failing test**

```go
func TestGetLowConfidenceFiles(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Insert test files with varying confidence
	files := []*MediaFile{
		{Path: "/test/high.mkv", Confidence: 0.95, ParseMethod: "regex", NeedsReview: false, NormalizedTitle: "high", MediaType: "movie", Size: 1000},
		{Path: "/test/medium.mkv", Confidence: 0.75, ParseMethod: "regex", NeedsReview: true, NormalizedTitle: "medium", MediaType: "movie", Size: 1000},
		{Path: "/test/low.mkv", Confidence: 0.4, ParseMethod: "regex", NeedsReview: true, NormalizedTitle: "low", MediaType: "movie", Size: 1000},
	}

	for _, f := range files {
		if err := db.UpsertMediaFile(f); err != nil {
			t.Fatalf("UpsertMediaFile failed: %v", err)
		}
	}

	// Query files below threshold
	lowFiles, err := db.GetLowConfidenceFiles(0.8)
	if err != nil {
		t.Fatalf("GetLowConfidenceFiles failed: %v", err)
	}

	if len(lowFiles) != 2 {
		t.Errorf("Expected 2 low confidence files, got %d", len(lowFiles))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/database -run TestGetLowConfidenceFiles -v`
Expected: FAIL - undefined: GetLowConfidenceFiles

**Step 3: Implement GetLowConfidenceFiles**

```go
// GetLowConfidenceFiles returns all files with confidence below threshold
func (db *MediaDB) GetLowConfidenceFiles(threshold float64) ([]*MediaFile, error) {
	query := `
		SELECT id, path, size, modified_at, media_type,
		       parent_movie_id, parent_series_id, parent_episode_id,
		       normalized_title, year, season, episode,
		       resolution, source_type, codec, audio_format, quality_score,
		       is_jellyfin_compliant, compliance_issues,
		       source, source_priority, library_root,
		       confidence, parse_method, needs_review,
		       created_at, updated_at
		FROM media_files
		WHERE confidence < ?
		ORDER BY confidence ASC
	`

	rows, err := db.db.Query(query, threshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*MediaFile
	for rows.Next() {
		file, err := scanMediaFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	return files, rows.Err()
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/database -run TestGetLowConfidenceFiles -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/database/media_file.go internal/database/media_file_test.go
git commit -m "$(cat <<'EOF'
feat(db): add GetLowConfidenceFiles query

Returns all media files with confidence below threshold,
ordered by confidence ascending (worst first).

Used by audit command to find files needing review.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Create audit plan types in plans package

**Files:**
- Modify: `internal/plans/plans.go`
- Test: `internal/plans/plans_test.go`

**Step 1: Add audit plan types to plans.go**

```go
// AuditFile represents a file needing audit review
type AuditFile struct {
	ID           int64   `json:"id"`
	Path         string  `json:"path"`
	CurrentTitle string  `json:"current_title"`
	CurrentYear  *int    `json:"current_year,omitempty"`
	Confidence   float64 `json:"confidence"`
	ParseMethod  string  `json:"parse_method"`
	Issues       []string `json:"issues"`
	// AI suggestion (populated during --dry-run or --execute)
	AISuggestion *AISuggestion `json:"ai_suggestion,omitempty"`
}

// AISuggestion represents AI's recommended fix
type AISuggestion struct {
	Title      string  `json:"title"`
	Year       *int    `json:"year,omitempty"`
	Confidence float64 `json:"confidence"`
}

// AuditSummary contains stats for audit plans
type AuditSummary struct {
	TotalFiles     int     `json:"total_files"`
	BelowThreshold int     `json:"below_threshold"`
	AvgConfidence  float64 `json:"avg_confidence"`
}

// AuditPlan represents files flagged for audit review
type AuditPlan struct {
	CreatedAt time.Time    `json:"created_at"`
	Command   string       `json:"command"`
	Threshold float64      `json:"threshold"`
	Summary   AuditSummary `json:"summary"`
	Files     []AuditFile  `json:"files"`
}

// getAuditPlansPath returns the path to audit.json
func getAuditPlansPath() (string, error) {
	dir, err := GetPlansDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "audit.json"), nil
}

// SaveAuditPlans saves an audit plan to JSON file
func SaveAuditPlans(plan *AuditPlan) error {
	path, err := getAuditPlansPath()
	if err != nil {
		return err
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create plans directory: %w", err)
	}

	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal plan: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write plan file: %w", err)
	}

	return nil
}

// LoadAuditPlans loads an audit plan from JSON file
func LoadAuditPlans() (*AuditPlan, error) {
	path, err := getAuditPlansPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read plans file: %w", err)
	}

	var plan AuditPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("failed to parse plans file: %w", err)
	}

	return &plan, nil
}

// DeleteAuditPlans removes the audit plans file
func DeleteAuditPlans() error {
	path, err := getAuditPlansPath()
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete plan file: %w", err)
	}

	return nil
}

// ArchiveAuditPlans renames audit.json to audit.json.old
func ArchiveAuditPlans() error {
	path, err := getAuditPlansPath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	oldPath := path + ".old"
	os.Remove(oldPath)

	if err := os.Rename(path, oldPath); err != nil {
		return fmt.Errorf("failed to archive plan file: %w", err)
	}

	return nil
}
```

**Step 2: Run existing tests**

Run: `go test ./internal/plans/... -v`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/plans/plans.go
git commit -m "$(cat <<'EOF'
feat(plans): add audit plan types and persistence

Adds AuditPlan, AuditFile, AISuggestion types for confidence
audit workflow. Includes Save/Load/Delete/Archive functions
following the same pattern as duplicates and consolidate plans.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: Create audit_cmd.go with --generate flag

**Files:**
- Create: `cmd/jellywatch/audit_cmd.go`
- Modify: `cmd/jellywatch/main.go` (add command registration)

**Step 1: Create the audit command file**

```go
// cmd/jellywatch/audit_cmd.go
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/naming"
	"github.com/Nomadcxx/jellywatch/internal/plans"
	"github.com/spf13/cobra"
)

const defaultAuditThreshold = 0.8

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Audit low-confidence file parses",
	Long: `Review and fix files with low parse confidence.

Workflow:
  1. jellywatch audit --generate    # Find low-confidence files
  2. jellywatch audit --dry-run     # Preview AI fixes (optional)
  3. jellywatch audit --execute     # Apply fixes

The audit command finds files where the regex parser produced
uncertain results (confidence < 0.8) and can use AI to suggest
better titles.`,
	RunE: runAuditStatus,
}

var (
	auditGenerate  bool
	auditDryRun    bool
	auditExecute   bool
	auditThreshold float64
)

func init() {
	auditCmd.Flags().BoolVar(&auditGenerate, "generate", false, "Generate audit plan for low-confidence files")
	auditCmd.Flags().BoolVar(&auditDryRun, "dry-run", false, "Preview AI suggestions without applying")
	auditCmd.Flags().BoolVar(&auditExecute, "execute", false, "Apply AI-suggested fixes")
	auditCmd.Flags().Float64Var(&auditThreshold, "threshold", defaultAuditThreshold, "Confidence threshold (files below this are audited)")
}

func runAuditStatus(cmd *cobra.Command, args []string) error {
	db, err := database.Open()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	switch {
	case auditGenerate:
		return runAuditGenerate(db)
	case auditDryRun:
		return runAuditDryRun(db)
	case auditExecute:
		return runAuditExecute(db)
	default:
		return runAuditShowStatus(db)
	}
}

func runAuditShowStatus(db *database.MediaDB) error {
	plan, err := plans.LoadAuditPlans()
	if err != nil {
		return fmt.Errorf("failed to load plans: %w", err)
	}

	if plan == nil {
		fmt.Println("No pending audit plan.")
		fmt.Println()
		fmt.Println("Run 'jellywatch audit --generate' to find low-confidence files.")
		return nil
	}

	fmt.Printf("=== Pending Audit Plan ===\n")
	fmt.Printf("Generated: %s\n", plan.CreatedAt.Format(time.RFC3339))
	fmt.Printf("Threshold: %.2f\n", plan.Threshold)
	fmt.Printf("Files:     %d\n", plan.Summary.TotalFiles)
	fmt.Printf("Avg conf:  %.2f\n", plan.Summary.AvgConfidence)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  jellywatch audit --dry-run   # Preview AI fixes")
	fmt.Println("  jellywatch audit --execute   # Apply fixes")

	return nil
}

func runAuditGenerate(db *database.MediaDB) error {
	fmt.Printf("Scanning for files with confidence < %.2f...\n", auditThreshold)

	files, err := db.GetLowConfidenceFiles(auditThreshold)
	if err != nil {
		return fmt.Errorf("failed to query files: %w", err)
	}

	if len(files) == 0 {
		fmt.Println("No low-confidence files found.")
		return nil
	}

	// Build audit plan
	auditFiles := make([]plans.AuditFile, 0, len(files))
	var totalConfidence float64

	for _, f := range files {
		issues := detectIssues(f.NormalizedTitle, f.Path)

		auditFile := plans.AuditFile{
			ID:           f.ID,
			Path:         f.Path,
			CurrentTitle: f.NormalizedTitle,
			CurrentYear:  f.Year,
			Confidence:   f.Confidence,
			ParseMethod:  f.ParseMethod,
			Issues:       issues,
		}
		auditFiles = append(auditFiles, auditFile)
		totalConfidence += f.Confidence
	}

	plan := &plans.AuditPlan{
		CreatedAt: time.Now(),
		Command:   "jellywatch audit --generate",
		Threshold: auditThreshold,
		Summary: plans.AuditSummary{
			TotalFiles:     len(auditFiles),
			BelowThreshold: len(auditFiles),
			AvgConfidence:  totalConfidence / float64(len(auditFiles)),
		},
		Files: auditFiles,
	}

	if err := plans.SaveAuditPlans(plan); err != nil {
		return fmt.Errorf("failed to save plan: %w", err)
	}

	fmt.Printf("\n=== Audit Plan Generated ===\n")
	fmt.Printf("Files flagged: %d\n", plan.Summary.TotalFiles)
	fmt.Printf("Avg confidence: %.2f\n", plan.Summary.AvgConfidence)
	fmt.Println()

	// Show first few files
	limit := 10
	if len(auditFiles) < limit {
		limit = len(auditFiles)
	}
	fmt.Printf("Top %d lowest confidence files:\n", limit)
	for i := 0; i < limit; i++ {
		f := auditFiles[i]
		fmt.Printf("  [%.2f] %s\n", f.Confidence, f.Path)
		if len(f.Issues) > 0 {
			fmt.Printf("         Issues: %v\n", f.Issues)
		}
	}

	if len(auditFiles) > limit {
		fmt.Printf("  ... and %d more\n", len(auditFiles)-limit)
	}

	fmt.Println()
	fmt.Println("Next: Run 'jellywatch audit --dry-run' to preview AI suggestions")

	return nil
}

func runAuditDryRun(db *database.MediaDB) error {
	fmt.Println("AI dry-run not yet implemented.")
	fmt.Println("This will call the AI matcher to suggest fixes without applying them.")
	return nil
}

func runAuditExecute(db *database.MediaDB) error {
	fmt.Println("AI execute not yet implemented.")
	fmt.Println("This will apply AI-suggested fixes to the database.")
	return nil
}

// detectIssues identifies specific problems with a parsed title
func detectIssues(title, path string) []string {
	var issues []string

	if naming.IsGarbageTitle(title) {
		issues = append(issues, "garbage_title")
	}
	if naming.IsObfuscatedFilename(path) {
		issues = append(issues, "obfuscated_filename")
	}
	if len(title) < 3 {
		issues = append(issues, "very_short_title")
	}
	// Check for duplicate year pattern in path
	if hasDuplicateYearInPath(path) {
		issues = append(issues, "duplicate_year")
	}

	return issues
}

func hasDuplicateYearInPath(path string) bool {
	// Simple check for patterns like (2001) (2001)
	return naming.CalculateTitleConfidence("", path) < 0.5
}
```

**Step 2: Register command in main.go**

Find where commands are registered and add:
```go
rootCmd.AddCommand(auditCmd)
```

**Step 3: Build and test**

Run: `go build ./cmd/jellywatch && ./jellywatch audit --help`
Expected: Shows audit command help

Run: `./jellywatch audit --generate --threshold 0.8`
Expected: Either generates plan or reports no files found

**Step 4: Commit**

```bash
git add cmd/jellywatch/audit_cmd.go cmd/jellywatch/main.go
git commit -m "$(cat <<'EOF'
feat(cli): add audit command for low-confidence files

Implements jellywatch audit with:
- --generate: Find files with confidence < threshold
- --dry-run: Preview AI suggestions (stub)
- --execute: Apply fixes (stub)
- --threshold: Configure confidence threshold (default 0.8)

AI integration to be added in follow-up task.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 8: Wire up AI matcher for --dry-run and --execute

**Files:**
- Modify: `cmd/jellywatch/audit_cmd.go`
- Uses: `internal/ai/matcher.go` (existing)

**Step 1: Update imports in audit_cmd.go**

```go
import (
	"context"
	// ... existing imports
	"github.com/Nomadcxx/jellywatch/internal/ai"
	"github.com/Nomadcxx/jellywatch/internal/config"
)
```

**Step 2: Implement runAuditDryRun**

```go
func runAuditDryRun(db *database.MediaDB) error {
	plan, err := plans.LoadAuditPlans()
	if err != nil {
		return fmt.Errorf("failed to load plans: %w", err)
	}

	if plan == nil {
		fmt.Println("No pending audit plan.")
		fmt.Println("Run 'jellywatch audit --generate' first.")
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if !cfg.AI.Enabled {
		fmt.Println("AI is not enabled in config.")
		fmt.Println("Enable AI in ~/.config/jellywatch/config.yaml to use dry-run.")
		return nil
	}

	matcher, err := ai.NewMatcher(cfg.AI)
	if err != nil {
		return fmt.Errorf("failed to create AI matcher: %w", err)
	}

	ctx := context.Background()
	if !matcher.IsAvailable(ctx) {
		fmt.Println("Ollama is not running or not reachable.")
		fmt.Printf("Start Ollama or check endpoint: %s\n", cfg.AI.OllamaEndpoint)
		return nil
	}

	fmt.Printf("Requesting AI suggestions for %d files...\n\n", len(plan.Files))

	for i := range plan.Files {
		f := &plan.Files[i]
		filename := filepath.Base(f.Path)

		fmt.Printf("[%d/%d] %s\n", i+1, len(plan.Files), filename)
		fmt.Printf("  Current: %s", f.CurrentTitle)
		if f.CurrentYear != nil {
			fmt.Printf(" (%d)", *f.CurrentYear)
		}
		fmt.Printf(" [confidence: %.2f]\n", f.Confidence)

		result, err := matcher.ParseWithRetry(ctx, filename)
		if err != nil {
			fmt.Printf("  AI Error: %v\n", err)
			continue
		}

		f.AISuggestion = &plans.AISuggestion{
			Title:      result.Title,
			Confidence: result.Confidence,
		}
		if result.Year != nil {
			f.AISuggestion.Year = result.Year
		}

		fmt.Printf("  AI suggestion: %s", result.Title)
		if result.Year != nil {
			fmt.Printf(" (%d)", *result.Year)
		}
		fmt.Printf(" [confidence: %.2f]\n", result.Confidence)
		fmt.Println()
	}

	// Save updated plan with AI suggestions
	if err := plans.SaveAuditPlans(plan); err != nil {
		return fmt.Errorf("failed to save plan: %w", err)
	}

	fmt.Println("=== Dry Run Complete ===")
	fmt.Println("AI suggestions saved to plan.")
	fmt.Println()
	fmt.Println("Next: Run 'jellywatch audit --execute' to apply fixes")

	return nil
}
```

**Step 3: Implement runAuditExecute**

```go
func runAuditExecute(db *database.MediaDB) error {
	plan, err := plans.LoadAuditPlans()
	if err != nil {
		return fmt.Errorf("failed to load plans: %w", err)
	}

	if plan == nil {
		fmt.Println("No pending audit plan.")
		fmt.Println("Run 'jellywatch audit --generate' first.")
		return nil
	}

	// Count files with AI suggestions
	withSuggestions := 0
	for _, f := range plan.Files {
		if f.AISuggestion != nil && f.AISuggestion.Confidence >= 0.8 {
			withSuggestions++
		}
	}

	if withSuggestions == 0 {
		fmt.Println("No AI suggestions available.")
		fmt.Println("Run 'jellywatch audit --dry-run' first to get AI suggestions.")
		return nil
	}

	fmt.Printf("Will update %d files with high-confidence AI suggestions.\n", withSuggestions)
	fmt.Print("Continue? [y/N]: ")

	var response string
	fmt.Scanln(&response)
	if response != "y" && response != "Y" {
		fmt.Println("Cancelled.")
		return nil
	}

	updated := 0
	skipped := 0

	for _, f := range plan.Files {
		if f.AISuggestion == nil || f.AISuggestion.Confidence < 0.8 {
			skipped++
			continue
		}

		// Update database with AI suggestion
		err := db.UpdateMediaFileTitle(f.ID, f.AISuggestion.Title, f.AISuggestion.Year, f.AISuggestion.Confidence, "ai")
		if err != nil {
			fmt.Printf("Failed to update %s: %v\n", f.Path, err)
			continue
		}

		fmt.Printf("Updated: %s -> %s", f.CurrentTitle, f.AISuggestion.Title)
		if f.AISuggestion.Year != nil {
			fmt.Printf(" (%d)", *f.AISuggestion.Year)
		}
		fmt.Println()
		updated++
	}

	// Clean up plan file
	if err := plans.DeleteAuditPlans(); err != nil {
		fmt.Printf("Warning: failed to delete plan file: %v\n", err)
	}

	fmt.Printf("\n=== Execution Complete ===\n")
	fmt.Printf("Updated: %d files\n", updated)
	fmt.Printf("Skipped: %d files (low AI confidence)\n", skipped)

	return nil
}
```

**Step 4: Add UpdateMediaFileTitle to database**

In `internal/database/media_file.go`:

```go
// UpdateMediaFileTitle updates a file's parsed title with new values
func (db *MediaDB) UpdateMediaFileTitle(id int64, title string, year *int, confidence float64, parseMethod string) error {
	query := `
		UPDATE media_files
		SET normalized_title = ?,
		    year = ?,
		    confidence = ?,
		    parse_method = ?,
		    needs_review = 0,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	_, err := db.db.Exec(query, title, year, confidence, parseMethod, id)
	return err
}
```

**Step 5: Build and test**

Run: `go build ./cmd/jellywatch`
Expected: Builds successfully

Run: `./jellywatch audit --dry-run` (with Ollama running)
Expected: Calls AI for each file in plan

**Step 6: Commit**

```bash
git add cmd/jellywatch/audit_cmd.go internal/database/media_file.go
git commit -m "$(cat <<'EOF'
feat(cli): implement audit --dry-run and --execute with AI

--dry-run: Calls AI matcher for each low-confidence file,
stores suggestions in the plan file.

--execute: Applies AI suggestions with confidence >= 0.8
to the database, updating normalized_title, year, and
setting parse_method to 'ai'.

Requires AI to be enabled in config and Ollama running.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 9: Add de-obfuscation integration to scanner

**Files:**
- Modify: `internal/scanner/scanner.go:262-341`
- Uses: `internal/naming/deobfuscate.go` (existing)

**Step 1: Update processFile to use de-obfuscation**

In processFile, after determining mediaType but before parsing, add:

```go
	// Check for obfuscated filename and try to parse from folder
	if naming.IsObfuscatedFilename(filename) {
		if isEpisode {
			if tvInfo, err := naming.ParseTVShowFromPath(filePath); err == nil {
				normalizedTitle = database.NormalizeTitle(tvInfo.Title)
				if tvInfo.Year != "" {
					if yearInt, err := parseInt(tvInfo.Year); err == nil {
						year = &yearInt
					}
				}
				season = &tvInfo.Season
				episode = &tvInfo.Episode
				// Set parse method to 'folder' since we used folder name
				parseMethod = "folder"
			}
		} else {
			if movieInfo, err := naming.ParseMovieFromPath(filePath); err == nil {
				normalizedTitle = database.NormalizeTitle(movieInfo.Title)
				if movieInfo.Year != "" {
					if yearInt, err := parseInt(movieInfo.Year); err == nil {
						year = &yearInt
					}
				}
				parseMethod = "folder"
			}
		}
	}
```

**Step 2: Update the file creation to use parseMethod variable**

Change the MediaFile creation to use a `parseMethod` variable instead of hardcoded "regex":

```go
	// Default parse method
	parseMethod := "regex"

	// ... obfuscation handling sets parseMethod = "folder" ...

	// ... existing parsing logic ...

	// Create MediaFile record
	file := &database.MediaFile{
		// ... other fields ...
		ParseMethod: parseMethod,
	}
```

**Step 3: Build and test**

Run: `go build ./cmd/jellywatch`
Expected: Builds successfully

**Step 4: Commit**

```bash
git add internal/scanner/scanner.go
git commit -m "$(cat <<'EOF'
feat(scanner): integrate de-obfuscation for hash-named files

When IsObfuscatedFilename returns true, scanner now:
- Calls ParseTVShowFromPath or ParseMovieFromPath
- Extracts title/year from parent folder names
- Sets parse_method to 'folder'

This handles files from SABnzbd's obfuscation feature.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Summary

This plan implements:

1. **Confidence scoring** (`internal/naming/confidence.go`) - Calculates 0.0-1.0 score based on title quality
2. **Database schema** (migration 10) - Adds confidence, parse_method, needs_review columns
3. **Scanner integration** - Calculates confidence during file scanning
4. **Audit command** - `jellywatch audit --generate/--dry-run/--execute`
5. **AI integration** - Uses existing AI matcher for low-confidence fixes
6. **De-obfuscation** - Integrates existing ParseTVShowFromPath/ParseMovieFromPath

All code leverages existing functionality:
- `blacklist.go` with 10,992 release groups
- `deobfuscate.go` for hash-named files
- `ai/matcher.go` for title parsing
- JSON plans pattern from duplicates/consolidate
