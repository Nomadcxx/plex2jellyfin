# AI Audit Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement `jellywatch audit` command that scans the media library, uses AI to analyze files, and provides corrections with user approval.

**Architecture:** New CLI command with three modes: scan-only (`jellywatch audit`), interactive review (`--review`), and apply corrections (`--fix`). Uses existing AI infrastructure (matcher.go, integrator.go) with confidence threshold of 0.6 for CLI mode. **IMPORTANT:** Database does NOT have confidence field in media_files table - AI analysis must be run for each file to determine if correction is needed.

**Tech Stack:** Go, Cobra CLI, existing AI/Matcher infrastructure, Sonarr/Radarr API clients, SQLite database.

---

### Task 1: Create Audit Command File

**Files:**
- Create: `cmd/jellywatch/audit_cmd.go`

**Step 1: Create command scaffold with exact design signature**

```go
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/ai"
	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/radarr"
	"github.com/Nomadcxx/jellywatch/internal/sonarr"
	"github.com/spf13/cobra"
)

func newAuditCmd() *cobra.Command {
	var (
		scan      bool
		review    bool
		fix       bool
		verbose   bool
		threshold float64
	)

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Audit library with AI-powered title correction",
		Long: `Scan media files and use AI to correct low-confidence titles.
Compares against Sonarr/Radarr metadata for accuracy.
Reports findings and optionally applies corrections.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAudit(scan, review, fix, verbose, threshold)
		},
	}

	cmd.Flags().BoolVarP(&scan, "scan", "s", false, "scan all files (default: non-compliant only)")
	cmd.Flags().BoolVarP(&review, "review", "r", false, "interactive mode: review each suggestion")
	cmd.Flags().BoolVarP(&fix, "fix", "f", false, "apply corrections without review (dangerous)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "detailed output")
	cmd.Flags().Float64Var(&threshold, "threshold", 0.6, "confidence threshold (0.0-1.0)")

	return cmd
}
```

**Step 2: Run test to verify it fails**

Run: `go build ./cmd/jellywatch`
Expected: BUILD FAILS because `runAudit` function not defined

**Step 3: Write minimal implementation skeleton**

```go
func runAudit(scan, review, fix, verbose bool, threshold float64) error {
	fmt.Println("AI Audit feature not yet implemented")
	return nil
}
```

**Step 4: Run test to verify it builds**

Run: `go build ./cmd/jellywatch`
Expected: BUILD SUCCESS

**Step 5: Add to main.go**

Edit `cmd/jellywatch/main.go` around line 80:

```go
rootCmd.AddCommand(newAuditCmd())
```

**Step 6: Commit**

```bash
git add cmd/jellywatch/audit_cmd.go cmd/jellywatch/main.go
git commit -m "feat: scaffold audit command"
```

---

### Task 2: Create Audit Engine Structure

**Files:**
- Modify: `cmd/jellywatch/audit_cmd.go`

**Step 1: Define audit structures matching design**

```go
type AuditEngine struct {
	db        *database.MediaDB
	sonarr    *sonarr.Client
	radarr    *radarr.Client
	aiMatcher *ai.Integrator
	config    config.AIConfig
}

type FileCandidate struct {
	ID           int64
	Filename     string
	LibraryPath  string
	MediaType    string
	NormalizedTitle string
	Year         *int
	Season       *int
	Episode      *int
	IsJellyfinCompliant bool
}

type Correction struct {
	ID           int64
	Filename     string
	Current      string
	Suggested    string
	Source       string
	Confidence   float64
	Action       string
	Reason       string
}

type Progress interface {
	Update(current, total int, message string)
}
```

**Step 2: Add progress implementation**

```go
type ConsoleProgress struct{}

func (p *ConsoleProgress) Update(current, total int, message string) {
	if total > 0 {
		fmt.Printf("\r[%d/%d] %s", current, total, message)
	} else {
		fmt.Printf("\r%s", message)
	}
}
```

**Step 3: Add getFileCandidates method (NOT using confidence)**

```go
func (e *AuditEngine) getFileCandidates(scanAll bool) ([]*FileCandidate, error) {
	var query string
	var args []interface{}

	if scanAll {
		// Scan ALL files for audit
		query = `
			SELECT id, filename, library_root, media_type, normalized_title,
			       year, season, episode, is_jellyfin_compliant
			FROM media_files
			ORDER BY created_at DESC
			LIMIT 500`
	} else {
		// Scan only non-compliant files
		query = `
			SELECT id, filename, library_root, media_type, normalized_title,
			       year, season, episode, is_jellyfin_compliant
			FROM media_files
			WHERE is_jellyfin_compliant = 0
			ORDER BY created_at DESC
			LIMIT 500`
	}

	rows, err := e.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*FileCandidate
	for rows.Next() {
		var f FileCandidate
		err := rows.Scan(
			&f.ID, &f.Filename, &f.LibraryPath, &f.MediaType, &f.NormalizedTitle,
			&f.Year, &f.Season, &f.Episode, &f.IsJellyfinCompliant,
		)
		if err != nil {
			return nil, err
		}
		files = append(files, &f)
	}

	return files, rows.Err()
}
```

**Step 4: Test database query**

Run: `go test ./internal/database -run TestMediaFiles`
Expected: Tests pass (verify query doesn't break existing tests)

**Step 5: Commit**

```bash
git add cmd/jellywatch/audit_cmd.go
git commit -m "feat: add audit engine structure and file candidate detection"
```

---

### Task 3: Update AI Integrator with Exact Design Signatures

**Files:**
- Modify: `internal/ai/integrator.go`

**Step 1: Add CLIContext enum matching design**

```go
type CLIContext int

const (
	CLIContext         CLIContext = iota // Direct CLI invocation
	DaemonBackground                     // Daemon background processing
	ManualScan                          // Manual scan triggered by user
)

type ParseSource int

const (
	SourceRegex ParseSource = iota
	SourceAI
	SourceSonarr
	SourceRadarr
)
```

**Step 2: Add CLI-specific enhance method with EXACT signature from design**

```go
func (i *Integrator) EnhanceTitleCLI(
	ctx context.Context,
	regexTitle, filename, mediaType string,
	requestSource CLIContext,
) (string, ParseSource, error) {
	// Parse with regex first to get baseline
	regexResult, regexConf := i.parseWithRegex(filename)

	// For CLI mode, use hardcoded threshold of 0.6
	cliThreshold := 0.6

	if regexConf >= cliThreshold {
		// High confidence, use regex
		return regexResult.Title, SourceRegex, nil
	}

	// Low confidence, use AI
	aiResult, err := i.matcher.ParseWithRetry(ctx, filename)
	if err != nil {
		return "", SourceRegex, err
	}

	// Verify AI confidence
	if aiResult.Confidence >= cliThreshold {
		return aiResult.Title, SourceAI, nil
	}

	// Return AI result even if below threshold (user will review)
	return aiResult.Title, SourceAI, nil
}
```

**Step 3: Run test to verify compilation**

Run: `go build ./internal/ai`
Expected: BUILD SUCCESS

**Step 4: Test enhancement logic**

Add test to `internal/ai/integrator_test.go`:

```go
func TestEnhanceTitleCLI(t *testing.T) {
	cfg := config.AIConfig{
		Enabled:             false, // Don't actually call AI in tests
		ConfidenceThreshold: 0.8,
	}

	matcher, _ := NewMatcher(cfg)
	integrator := NewIntegrator(matcher, cfg)

	// Test with high-confidence filename (should use regex)
	title, source, err := integrator.EnhanceTitleCLI(
		context.Background(),
		"Test Movie 2020",
		"Test.Movie.2020.1080p",
		"movie",
		CLIContext,
	)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if source != SourceRegex {
		t.Errorf("Expected SourceRegex, got %v", source)
	}
	if title != "Test Movie" {
		t.Errorf("Expected 'Test Movie', got %s", title)
	}
}
```

**Step 5: Run tests**

Run: `go test ./internal/ai -run TestEnhanceTitleCLI`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/ai/integrator.go internal/ai/integrator_test.go
git commit -m "feat: add CLI mode to AI integrator with exact design signatures"
```

---

### Task 4: Add Sonarr/Radarr Metadata Comparison

**Files:**
- Modify: `cmd/jellywatch/audit_cmd.go`

**Step 1: Add metadata fetch methods**

```go
func (e *AuditEngine) getSonarrMetadata(ctx context.Context, title string, year *int) (*sonarr.Series, error) {
	if e.sonarr == nil {
		return nil, nil
	}

	// Search Sonarr for matching series
	series, err := e.sonarr.SearchSeries(ctx, title)
	if err != nil {
		return nil, err
	}

	// Find best match
	for _, s := range series {
		if s.Title == title && (year == nil || s.Year == *year) {
			return &s, nil
		}
	}

	return nil, nil
}

func (e *AuditEngine) getRadarrMetadata(ctx context.Context, title string, year *int) (*radarr.Movie, error) {
	if e.radarr == nil {
		return nil, nil
	}

	// Search Radarr for matching movie
	movies, err := e.radarr.SearchMovies(ctx, title)
	if err != nil {
		return nil, err
	}

	// Find best match
	for _, m := range movies {
		if m.Title == title && (year == nil || m.Year == *year) {
			return &m, nil
		}
	}

	return nil, nil
}
```

**Step 2: Run test to verify compilation**

Run: `go build ./cmd/jellywatch`
Expected: BUILD SUCCESS

**Step 3: Add comparison logic**

```go
func (e *AuditEngine) compareWithMetadata(
	ctx context.Context,
	current *FileCandidate,
	aiTitle string,
	aiConfidence float64,
) *Correction {
	var correction Correction
	correction.ID = current.ID
	correction.Filename = current.Filename
	correction.Current = current.NormalizedTitle
	correction.Suggested = aiTitle
	correction.Confidence = aiConfidence
	correction.Source = "ai"

	// Compare with Sonarr/Radarr for validation
	if current.MediaType == "episode" {
		if series, err := e.getSonarrMetadata(ctx, aiTitle, current.Year); err == nil && series != nil {
			if series.Title == aiTitle {
				correction.Source = "sonarr+ai"
				correction.Confidence = math.Max(correction.Confidence, 0.95)
				correction.Reason = fmt.Sprintf("Validated against Sonarr: %s", series.Title)
			}
		}
	} else if current.MediaType == "movie" {
		if movie, err := e.getRadarrMetadata(ctx, aiTitle, current.Year); err == nil && movie != nil {
			if movie.Title == aiTitle {
				correction.Source = "radarr+ai"
				correction.Confidence = math.Max(correction.Confidence, 0.95)
				correction.Reason = fmt.Sprintf("Validated against Radarr: %s", movie.Title)
			}
		}
	}

	return &correction
}
```

**Step 4: Test metadata integration**

Run: `go test ./internal/sonarr ./internal/radarr`
Expected: Existing tests pass

**Step 5: Commit**

```bash
git add cmd/jellywatch/audit_cmd.go
git commit -m "feat: add Sonarr/Radarr metadata comparison for audit"
```

---

### Task 5: Implement Interactive Review Mode

**Files:**
- Modify: `cmd/jellywatch/audit_cmd.go`

**Step 1: Add review prompt method**

```go
func askUser(correction *Correction) (string, error) {
	fmt.Printf("\n[%d] %s\n", correction.ID, correction.Filename)
	fmt.Printf("    Current:  %s\n", correction.Current)
	fmt.Printf("    Suggests: %s [Confidence: %.2f%%]\n", correction.Suggested, correction.Confidence*100)
	fmt.Printf("    Source:    %s\n", correction.Source)
	if correction.Reason != "" {
		fmt.Printf("    Reason:    %s\n", correction.Reason)
	}

	fmt.Print("\nAction: [A]ccept, [S]kip, [M]anual edit, [Q]uit: ")

	var input string
	fmt.Scanln(&input)
	input = strings.ToLower(strings.TrimSpace(input))

	switch input {
	case "a", "accept":
		return "accept", nil
	case "s", "skip":
		return "skip", nil
	case "m", "manual":
		return "manual", nil
	case "q", "quit":
		return "quit", nil
	default:
		return "skip", nil
	}
}
```

**Step 2: Run test to verify compilation**

Run: `go build ./cmd/jellywatch`
Expected: BUILD SUCCESS

**Step 3: Add manual edit method**

```go
func manualEdit(current string) (string, error) {
	fmt.Print("Enter new title: ")
	var newTitle string
	fmt.Scanln(&newTitle)
	newTitle = strings.TrimSpace(newTitle)

	if newTitle == "" {
		return current, fmt.Errorf("title cannot be empty")
	}

	return newTitle, nil
}
```

**Step 4: Add apply correction method with security checks**

```go
func (e *AuditEngine) applyCorrection(correction *Correction) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Update media_files table
	_, err := e.db.ExecContext(ctx, `
		UPDATE media_files
		SET normalized_title = ?, updated_at = ?
		WHERE id = ?`,
		correction.Suggested,
		time.Now(),
		correction.ID,
	)

	if err != nil {
		return fmt.Errorf("database update failed: %w", err)
	}

	// Record in ai_improvements table for audit trail
	aiImp := &database.AIImprovement{
		RequestID:    fmt.Sprintf("audit-%d-%s", correction.ID, time.Now().Format("20060102-150405")),
		Filename:     correction.Filename,
		UserTitle:    correction.Current,
		UserType:     "correction",
		UserYear:     nil, // Year not extracted from normalized_title
		AITitle:      &correction.Suggested,
		AIType:       &correction.Source,
		AIYear:       nil,
		AIConfidence: &correction.Confidence,
		Status:       "completed",
		Model:        &e.config.Model,
	}

	return e.db.UpsertAIImprovement(aiImp)
}
```

**Step 5: Test database operations**

Run: `go test ./internal/database -run TestAIImprovements`
Expected: Tests pass

**Step 6: Commit**

```bash
git add cmd/jellywatch/audit_cmd.go
git commit -m "feat: add interactive review mode with manual edit support"
```

---

### Task 6: Implement Main Audit Logic with Progress

**Files:**
- Modify: `cmd/jellywatch/audit_cmd.go`

**Step 1: Complete runAudit function with security and progress**

```go
func runAudit(scan, review, fix, verbose bool, threshold float64) error {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Security: Verify Ollama endpoint is localhost or LAN
	if cfg.AI.Enabled {
		if err := verifyOllamaEndpoint(cfg.AI.OllamaEndpoint); err != nil {
			return fmt.Errorf("security: %w", err)
		}
	}

	// Open database
	dbPath := config.GetDatabasePath()
	db, err := database.OpenPath(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Create AI matcher with 30s timeout
	cfg.AI.TimeoutSeconds = 30
	aiMatcher, err := ai.NewMatcher(cfg.AI)
	if err != nil {
		return fmt.Errorf("failed to create AI matcher: %w", err)
	}

	// Create integrator
	integrator := ai.NewIntegrator(aiMatcher, cfg.AI)

	// Create Sonarr/Radarr clients if configured
	var sonarrClient *sonarr.Client
	var radarrClient *radarr.Client

	if cfg.Sonarr.Enabled {
		sonarrClient = sonarr.NewClient(sonarr.Config{
			URL:    cfg.Sonarr.URL,
			APIKey: cfg.Sonarr.APIKey,
		})
	}

	if cfg.Radarr.Enabled {
		radarrClient = radarr.NewClient(radarr.Config{
			URL:    cfg.Radarr.URL,
			APIKey: cfg.Radarr.APIKey,
		})
	}

	// Create audit engine
	engine := &AuditEngine{
		db:        db,
		sonarr:    sonarrClient,
		radarr:    radarrClient,
		aiMatcher: integrator,
		config:    cfg.AI,
	}

	// Get file candidates (NOT filtering by confidence)
	files, err := engine.getFileCandidates(scan)
	if err != nil {
		return fmt.Errorf("failed to get file candidates: %w", err)
	}

	if len(files) == 0 {
		if scan {
			fmt.Println("No files found to audit")
		} else {
			fmt.Println("No non-compliant files found")
		}
		return nil
	}

	fmt.Printf("Scanning %d files for corrections...\n\n", len(files))

	// Process each file with progress
	ctx := context.Background()
	progress := &ConsoleProgress{}
	var corrections []*Correction

	for i, file := range files {
		progress.Update(i+1, len(files), fmt.Sprintf("Analyzing: %s", file.Filename))

		// Get AI enhancement
		aiTitle, source, err := engine.aiMatcher.EnhanceTitleCLI(
			ctx,
			file.NormalizedTitle,
			file.Filename,
			file.MediaType,
			CLIContext,
		)

		if err != nil {
			if verbose {
				fmt.Printf("\n✗ %s: AI error: %v\n", file.Filename, err)
			}
			continue
		}

		// Skip if AI returns same title
		if aiTitle == file.NormalizedTitle {
			continue
		}

		// Compare with metadata
		correction := engine.compareWithMetadata(ctx, file, aiTitle, threshold)
		corrections = append(corrections, correction)
	}

	// Clear progress line
	fmt.Println()

	if len(corrections) == 0 {
		fmt.Println("No corrections suggested")
		return nil
	}

	// Display format from design
	fmt.Printf("=== Title Audit Report ===\n\n")
	fmt.Printf("Files Scanned:  %d\n", len(files))
	fmt.Printf("Corrections:    %d\n\n", len(corrections))

	// Process corrections
	var processed, accepted, skipped, errors int

	for _, correction := range corrections {
		// Determine action
		var action string
		if fix {
			action = "accept"
		} else if review {
			action, err = askUser(correction)
			if err != nil || action == "quit" {
				break
			}
		} else {
			// Scan mode - just report
			fmt.Printf("[%d] %s\n", correction.ID, correction.Filename)
			fmt.Printf("    Current:  %s\n", correction.Current)
			fmt.Printf("    Suggests: %s [%.2f%%]\n", correction.Suggested, correction.Confidence*100)
			if correction.Reason != "" {
				fmt.Printf("    Reason:   %s\n", correction.Reason)
			}
			action = "skip"
		}

		// Apply action
		switch action {
		case "accept":
			if err := engine.applyCorrection(correction); err != nil {
				fmt.Printf("✗ Failed to apply correction: %v\n", err)
				errors++
			} else {
				accepted++
				if verbose {
					fmt.Printf("✓ Applied: %s → %s\n", correction.Current, correction.Suggested)
				}
			}
		case "skip":
			skipped++
			if verbose {
				fmt.Printf("⊘ Skipped: %s\n", correction.Filename)
			}
		case "manual":
			newTitle, err := manualEdit(correction.Current)
			if err != nil {
				fmt.Printf("✗ Manual edit failed: %v\n", err)
				errors++
				skipped++
			} else {
				// Create correction with manual title
				manualCorrection := *correction
				manualCorrection.Suggested = newTitle
				manualCorrection.Source = "manual"
				if err := engine.applyCorrection(&manualCorrection); err != nil {
					fmt.Printf("✗ Failed to apply manual correction: %v\n", err)
					errors++
				} else {
					accepted++
					if verbose {
						fmt.Printf("✓ Applied manual: %s → %s\n", correction.Current, newTitle)
					}
				}
			}
		}

		processed++
	}

	// Report summary from design
	fmt.Printf("\nSummary:\n")
	fmt.Printf("%d corrections processed\n", processed)
	fmt.Printf("%d accepted, %d skipped, %d errors\n", accepted, skipped, errors)

	return nil
}
```

**Step 2: Add security verification function**

```go
func verifyOllamaEndpoint(endpoint string) error {
	// Security: Ensure endpoint is localhost or LAN only
	if strings.HasPrefix(endpoint, "http://") {
		endpoint = endpoint[7:]
	} else if strings.HasPrefix(endpoint, "https://") {
		endpoint = endpoint[8:]
	}

	host := strings.Split(endpoint, ":")[0]

	// Allow localhost variants
	localhostVariants := []string{"localhost", "127.0.0.1", "0.0.0.0", "::1", ""}
	for _, variant := range localhostVariants {
		if host == variant {
			return nil
		}
	}

	// Allow private IP ranges
	if strings.HasPrefix(host, "192.168.") ||
		strings.HasPrefix(host, "10.") ||
		strings.HasPrefix(host, "172.") {
		return nil
	}

	return fmt.Errorf("Ollama endpoint must be localhost or LAN only, got: %s", host)
}
```

**Step 3: Run integration test**

Run: `go build ./cmd/jellywatch && ./jellywatch audit --help`
Expected: Shows help for audit command

**Step 4: Commit**

```bash
git add cmd/jellywatch/audit_cmd.go
git commit -m "feat: complete audit command with progress and security"
```

---

### Task 7: Add Tests for Audit Functionality

**Files:**
- Create: `cmd/jellywatch/audit_test.go`

**Step 1: Test command creation**

```go
package main

import (
	"testing"
)

func TestAuditCommand(t *testing.T) {
	cmd := newAuditCmd()
	if cmd == nil {
		t.Fatal("Expected non-nil command")
	}

	if cmd.Use != "audit" {
		t.Errorf("Expected command use 'audit', got %s", cmd.Use)
	}

	flags := cmd.Flags()
	scanFlag := flags.Lookup("scan")
	if scanFlag == nil {
		t.Error("Expected 'scan' flag")
	}

	reviewFlag := flags.Lookup("review")
	if reviewFlag == nil {
		t.Error("Expected 'review' flag")
	}
}
```

**Step 2: Run audit command test**

Run: `go test ./cmd/jellywatch -run TestAuditCommand`
Expected: PASS

**Step 3: Test security verification**

```go
func TestVerifyOllamaEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		wantErr  bool
	}{
		{"localhost", "http://localhost:11434", false},
		{"127.0.0.1", "http://127.0.0.1:11434", false},
		{"LAN IP", "http://192.168.1.100:11434", false},
		{"private IP", "http://10.0.0.5:11434", false},
		{"public IP", "http://8.8.8.8:11434", true},
		{"public domain", "http://example.com:11434", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyOllamaEndpoint(tt.endpoint)
			if (err != nil) != tt.wantErr {
				t.Errorf("verifyOllamaEndpoint() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
```

**Step 4: Run security tests**

Run: `go test ./cmd/jellywatch -run TestVerifyOllamaEndpoint`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/jellywatch/audit_test.go
git commit -m "test: add tests for audit functionality and security"
```

---

### Task 8: Update Documentation

**Files:**
- Modify: `README.md`

**Step 1: Add audit command to README**

Add to README.md Usage section:

```markdown
### AI-Powered Title Audit

```bash
# Audit non-compliant files only
jellywatch audit

# Scan ALL files (not just non-compliant)
jellywatch audit --scan

# Interactive review mode
jellywatch audit --review

# Apply corrections (use with caution)
jellywatch audit --fix

# Verbose output
jellywatch audit --verbose

# Adjust confidence threshold (default: 0.6)
jellywatch audit --threshold 0.7
```

The audit command scans files and uses AI to suggest title corrections. Compare against Sonarr/Radarr metadata for validation.

**Features:**
- Analyzes non-compliant files by default (use `--scan` for all files)
- AI corrections with confidence scoring
- Sonarr/Radarr metadata validation
- Interactive review before applying changes
- Manual edit support
- Audit trail stored in database
- Security: Validates Ollama endpoint (localhost/LAN only)
```

**Step 2: Add example output to README**

Add example from design:

```markdown
**Example:**
```bash
$ jellywatch audit --review
=== Title Audit Report ===

Files Scanned:  500
Corrections:    15

[1] The.Matrix.1999.1080p.Bluray.x264-GROUP
    Current:  the matrix 1999
    Suggests: The Matrix [Confidence: 98%]
    Source:    sonarr+ai
    Reason:    Validated against Sonarr: The Matrix

Action: [A]ccept, [S]kip, [M]anual edit, [Q]uit: a
✓ Applied: the matrix 1999 → The Matrix

[2] Star Trek (2009) Remastered Edition
    Current:  star trek 2009 remastered
    Suggests: Star Trek (2009) [Confidence: 97%]
    Source:    radarr+ai
    Reason:    Validated against Radarr: Star Trek (2009)

Action: [A]ccept, [S]kip, [M]anual edit, [Q]uit: s

Summary:
15 corrections processed
12 accepted, 3 skipped, 0 errors
```

**Step 3: Commit**

```bash
git add README.md
git commit -m "docs: add audit command documentation with examples"
```

---

## Final Verification Checklist

After completing all tasks:

- [ ] Command builds successfully: `go build ./cmd/jellywatch`
- [ ] Help command works: `./jellywatch audit --help`
- [ ] Security tests pass: `go test ./cmd/jellywatch -run TestVerifyOllamaEndpoint`
- [ ] Integration tests pass: `go test ./internal/ai -run TestEnhanceTitleCLI`
- [ ] Database queries work: Verify no errors in audit mode
- [ ] Output format matches design specification
- [ ] All security checks implemented:
  - [ ] Ollama endpoint validation
  - [ ] No credentials in logs
  - [ ] Request timeout (30s)
  - [ ] Database context with timeout
- [ ] Progress bar displays during audit
- [ ] Audit trail in ai_improvements table

---

## Execution Options

**Plan complete and saved to `docs/plans/2026-01-27-ai-audit-implementation.md`. Two execution options:**

**1. Subagent-Driven (this session)** - I dispatch fresh subagent per task, review between tasks, fast iteration

**2. Parallel Session (separate)** - Open new session with executing-plans, batch execution with checkpoints

**Which approach?**

**If Subagent-Driven chosen:**
- **REQUIRED SUB-SKILL:** Use superpowers:subagent-driven-development
- Stay in this session
- Fresh subagent per task + code review

**If Parallel Session chosen:**
- Guide them to open new session in worktree
- **REQUIRED SUB-SKILL:** New session uses superpowers:executing-plans
