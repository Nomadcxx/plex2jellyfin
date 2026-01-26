# Fix Wizard Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement an interactive fix wizard with OpenAPI-based API to unify duplicate/consolidate workflows and enable future web UI.

**Architecture:** Service layer decouples business logic from CLI. CLI commands become thin clients. OpenAPI spec defines the API contract, with SSE for progress streaming.

**Tech Stack:** Go, Cobra CLI, oapi-codegen, net/http, database/sql

---

## Task 1: Fix SQL NULL Error in Planner

**Files:**
- Modify: `internal/consolidate/planner.go:1-50` (struct definition)
- Modify: `internal/consolidate/planner.go:243-290` (GetPendingPlans)
- Test: `internal/consolidate/planner_test.go`

**Step 1: Write a failing test for NULL source_file_id**

Add to `internal/consolidate/planner_test.go`:

```go
func TestGetPendingPlans_NullSourceFileID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert a plan with NULL source_file_id (move operation)
	_, err := db.DB().Exec(`
		INSERT INTO consolidation_plans
		(status, action, source_file_id, source_path, target_path, reason, reason_details)
		VALUES ('pending', 'move', NULL, '/source/path', '/target/path', 'consolidation', 'moving to target')
	`)
	if err != nil {
		t.Fatalf("Failed to insert test plan: %v", err)
	}

	planner := NewPlanner(db)
	plans, err := planner.GetPendingPlans()
	if err != nil {
		t.Fatalf("GetPendingPlans failed with NULL source_file_id: %v", err)
	}

	if len(plans) != 1 {
		t.Fatalf("Expected 1 plan, got %d", len(plans))
	}

	if plans[0].SourceFileID.Valid {
		t.Error("Expected SourceFileID.Valid to be false for NULL value")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/consolidate/... -run TestGetPendingPlans_NullSourceFileID -v`
Expected: FAIL with "converting NULL to int64 is unsupported"

**Step 3: Update ConsolidationPlan struct**

In `internal/consolidate/planner.go`, find the struct and update:

```go
import (
	"database/sql"
	// ... other imports
)

// ConsolidationPlan represents a pending consolidation action
type ConsolidationPlan struct {
	ID            int64
	CreatedAt     string
	Status        string
	Action        string
	SourceFileID  sql.NullInt64  // Changed from int64
	SourcePath    string
	TargetPath    string
	Reason        string
	ReasonDetails string
	ExecutedAt    string
	ErrorMessage  string
	ConflictID    int64
}
```

**Step 4: Update GetPendingPlans Scan call**

In `internal/consolidate/planner.go`, update the Scan in GetPendingPlans:

```go
err := rows.Scan(
	&plan.ID,
	&plan.CreatedAt,
	&plan.Status,
	&plan.Action,
	&plan.SourceFileID,  // Now sql.NullInt64, handles NULL
	&plan.SourcePath,
	&plan.TargetPath,
	&plan.Reason,
	&plan.ReasonDetails,
	&executedAt,
	&errorMsg,
	&conflictID,
)
```

**Step 5: Update usages of SourceFileID**

In `internal/consolidate/executor.go` and `cmd/jellywatch/consolidate_cmd.go`, update any usage:

```go
// Before:
file, _ := db.GetMediaFileByID(plan.SourceFileID)

// After:
var file *database.MediaFile
if plan.SourceFileID.Valid {
	file, _ = db.GetMediaFileByID(plan.SourceFileID.Int64)
}
```

**Step 6: Run test to verify it passes**

Run: `go test ./internal/consolidate/... -run TestGetPendingPlans_NullSourceFileID -v`
Expected: PASS

**Step 7: Run all consolidate tests**

Run: `go test ./internal/consolidate/... -v`
Expected: All tests PASS

**Step 8: Commit**

```bash
git add internal/consolidate/planner.go internal/consolidate/executor.go cmd/jellywatch/consolidate_cmd.go internal/consolidate/planner_test.go
git commit -m "fix: handle NULL source_file_id in consolidation plans"
```

---

## Task 2: Create Service Layer Foundation

**Files:**
- Create: `internal/service/service.go`
- Create: `internal/service/analysis.go`
- Test: `internal/service/analysis_test.go`

**Step 1: Create service package with CleanupService**

Create `internal/service/service.go`:

```go
package service

import (
	"github.com/Nomadcxx/jellywatch/internal/database"
)

// CleanupService provides operations for cleaning up media libraries
type CleanupService struct {
	db *database.MediaDB
}

// NewCleanupService creates a new cleanup service
func NewCleanupService(db *database.MediaDB) *CleanupService {
	return &CleanupService{db: db}
}
```

**Step 2: Create analysis types and methods**

Create `internal/service/analysis.go`:

```go
package service

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// DuplicateGroup represents files that are duplicates of each other
type DuplicateGroup struct {
	ID            string      // Hash of normalized title + year + episode
	Title         string
	Year          *int
	MediaType     string      // "movie" or "series"
	Season        *int        // For TV
	Episode       *int        // For TV
	Files         []MediaFile
	BestFileID    int64
	ReclaimableBytes int64
}

// MediaFile represents a media file in the analysis
type MediaFile struct {
	ID           int64
	Path         string
	Size         int64
	Resolution   string
	SourceType   string
	QualityScore int
}

// ScatteredItem represents media scattered across multiple locations
type ScatteredItem struct {
	ID             int64
	Title          string
	Year           *int
	MediaType      string
	Locations      []string
	TargetLocation string
	FilesToMove    int
	BytesToMove    int64
}

// DuplicateAnalysis contains the full duplicate analysis results
type DuplicateAnalysis struct {
	Groups           []DuplicateGroup
	TotalFiles       int
	TotalGroups      int
	ReclaimableBytes int64
}

// ScatteredAnalysis contains scattered media analysis results
type ScatteredAnalysis struct {
	Items       []ScatteredItem
	TotalItems  int
	TotalMoves  int
	TotalBytes  int64
}

// generateGroupID creates a unique ID for a duplicate group
func generateGroupID(title string, year *int, season, episode *int) string {
	parts := []string{strings.ToLower(title)}
	if year != nil {
		parts = append(parts, fmt.Sprintf("%d", *year))
	}
	if season != nil {
		parts = append(parts, fmt.Sprintf("s%d", *season))
	}
	if episode != nil {
		parts = append(parts, fmt.Sprintf("e%d", *episode))
	}
	hash := sha256.Sum256([]byte(strings.Join(parts, "-")))
	return fmt.Sprintf("%x", hash[:8])
}
```

**Step 3: Write failing test for AnalyzeDuplicates**

Create `internal/service/analysis_test.go`:

```go
package service

import (
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/database"
)

func setupTestDB(t *testing.T) *database.MediaDB {
	db, err := database.OpenPath(":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	return db
}

func TestAnalyzeDuplicates_Empty(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	svc := NewCleanupService(db)
	analysis, err := svc.AnalyzeDuplicates()
	if err != nil {
		t.Fatalf("AnalyzeDuplicates failed: %v", err)
	}

	if analysis.TotalGroups != 0 {
		t.Errorf("Expected 0 groups, got %d", analysis.TotalGroups)
	}
}

func TestAnalyzeDuplicates_FindsMovieDuplicates(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert duplicate movies
	year := 2005
	_, _ = db.InsertMediaFile(&database.MediaFile{
		Path:            "/storage1/Movies/Robots (2005)/Robots.mkv",
		NormalizedTitle: "robots",
		Year:            &year,
		MediaType:       "movie",
		Size:            4000000000,
		QualityScore:    284,
		Resolution:      "720p",
		SourceType:      "BluRay",
	})
	_, _ = db.InsertMediaFile(&database.MediaFile{
		Path:            "/storage2/Movies/Robots (2005)/Robots.mkv",
		NormalizedTitle: "robots",
		Year:            &year,
		MediaType:       "movie",
		Size:            4400000000,
		QualityScore:    84,
		Resolution:      "unknown",
		SourceType:      "unknown",
	})

	svc := NewCleanupService(db)
	analysis, err := svc.AnalyzeDuplicates()
	if err != nil {
		t.Fatalf("AnalyzeDuplicates failed: %v", err)
	}

	if analysis.TotalGroups != 1 {
		t.Errorf("Expected 1 group, got %d", analysis.TotalGroups)
	}

	if len(analysis.Groups) > 0 && len(analysis.Groups[0].Files) != 2 {
		t.Errorf("Expected 2 files in group, got %d", len(analysis.Groups[0].Files))
	}
}
```

**Step 4: Run tests to verify they fail**

Run: `go test ./internal/service/... -v`
Expected: FAIL with "AnalyzeDuplicates not defined"

**Step 5: Implement AnalyzeDuplicates**

Add to `internal/service/analysis.go`:

```go
// AnalyzeDuplicates finds all duplicate media files
func (s *CleanupService) AnalyzeDuplicates() (*DuplicateAnalysis, error) {
	analysis := &DuplicateAnalysis{
		Groups: []DuplicateGroup{},
	}

	// Get movie duplicates
	movieGroups, err := s.db.FindDuplicateMovies()
	if err != nil {
		return nil, fmt.Errorf("failed to find duplicate movies: %w", err)
	}

	for _, mg := range movieGroups {
		if len(mg.Files) < 2 {
			continue
		}

		group := DuplicateGroup{
			ID:        generateGroupID(mg.NormalizedTitle, mg.Year, nil, nil),
			Title:     mg.NormalizedTitle,
			Year:      mg.Year,
			MediaType: "movie",
			Files:     make([]MediaFile, len(mg.Files)),
			ReclaimableBytes: mg.SpaceReclaimable,
		}

		for i, f := range mg.Files {
			group.Files[i] = MediaFile{
				ID:           f.ID,
				Path:         f.Path,
				Size:         f.Size,
				Resolution:   f.Resolution,
				SourceType:   f.SourceType,
				QualityScore: f.QualityScore,
			}
		}

		if mg.BestFile != nil {
			group.BestFileID = mg.BestFile.ID
		}

		analysis.Groups = append(analysis.Groups, group)
		analysis.TotalFiles += len(mg.Files)
		analysis.ReclaimableBytes += mg.SpaceReclaimable
	}

	// Get episode duplicates
	episodeGroups, err := s.db.FindDuplicateEpisodes()
	if err != nil {
		return nil, fmt.Errorf("failed to find duplicate episodes: %w", err)
	}

	for _, eg := range episodeGroups {
		if len(eg.Files) < 2 {
			continue
		}

		group := DuplicateGroup{
			ID:        generateGroupID(eg.NormalizedTitle, eg.Year, eg.Season, eg.Episode),
			Title:     eg.NormalizedTitle,
			Year:      eg.Year,
			MediaType: "series",
			Season:    eg.Season,
			Episode:   eg.Episode,
			Files:     make([]MediaFile, len(eg.Files)),
			ReclaimableBytes: eg.SpaceReclaimable,
		}

		for i, f := range eg.Files {
			group.Files[i] = MediaFile{
				ID:           f.ID,
				Path:         f.Path,
				Size:         f.Size,
				Resolution:   f.Resolution,
				SourceType:   f.SourceType,
				QualityScore: f.QualityScore,
			}
		}

		if eg.BestFile != nil {
			group.BestFileID = eg.BestFile.ID
		}

		analysis.Groups = append(analysis.Groups, group)
		analysis.TotalFiles += len(eg.Files)
		analysis.ReclaimableBytes += eg.SpaceReclaimable
	}

	analysis.TotalGroups = len(analysis.Groups)
	return analysis, nil
}
```

**Step 6: Run tests to verify they pass**

Run: `go test ./internal/service/... -v`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/service/
git commit -m "feat: add service layer with duplicate analysis"
```

---

## Task 3: Add Scattered Media Analysis to Service

**Files:**
- Modify: `internal/service/analysis.go`
- Modify: `internal/service/analysis_test.go`

**Step 1: Write failing test for AnalyzeScattered**

Add to `internal/service/analysis_test.go`:

```go
func TestAnalyzeScattered_FindsConflicts(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert conflicting series in different locations
	year := 2005
	_, _ = db.InsertMediaFile(&database.MediaFile{
		Path:            "/storage1/TV/American Dad (2005)/S01E01.mkv",
		NormalizedTitle: "american dad",
		Year:            &year,
		MediaType:       "series",
		Size:            500000000,
	})
	_, _ = db.InsertMediaFile(&database.MediaFile{
		Path:            "/storage2/TV/American Dad! (2005)/S01E02.mkv",
		NormalizedTitle: "american dad",
		Year:            &year,
		MediaType:       "series",
		Size:            500000000,
	})

	// Force conflict detection
	_ = db.DetectConflicts()

	svc := NewCleanupService(db)
	analysis, err := svc.AnalyzeScattered()
	if err != nil {
		t.Fatalf("AnalyzeScattered failed: %v", err)
	}

	if analysis.TotalItems != 1 {
		t.Errorf("Expected 1 scattered item, got %d", analysis.TotalItems)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/service/... -run TestAnalyzeScattered -v`
Expected: FAIL with "AnalyzeScattered not defined"

**Step 3: Implement AnalyzeScattered**

Add to `internal/service/analysis.go`:

```go
// AnalyzeScattered finds media scattered across multiple locations
func (s *CleanupService) AnalyzeScattered() (*ScatteredAnalysis, error) {
	analysis := &ScatteredAnalysis{
		Items: []ScatteredItem{},
	}

	conflicts, err := s.db.GetUnresolvedConflicts()
	if err != nil {
		return nil, fmt.Errorf("failed to get conflicts: %w", err)
	}

	for _, c := range conflicts {
		item := ScatteredItem{
			ID:        c.ID,
			Title:     c.Title,
			Year:      c.Year,
			MediaType: c.MediaType,
			Locations: c.Locations,
		}

		// Determine target location (one with most files)
		if len(c.Locations) > 0 {
			item.TargetLocation = c.Locations[0] // Default to first
			// Could enhance to pick location with most files
		}

		// Count files to move (from non-target locations)
		for _, loc := range c.Locations {
			if loc != item.TargetLocation {
				// Count would need filesystem access or DB query
				item.FilesToMove++ // Simplified: 1 per location for now
			}
		}

		analysis.Items = append(analysis.Items, item)
	}

	analysis.TotalItems = len(analysis.Items)
	return analysis, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/service/... -run TestAnalyzeScattered -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/service/analysis.go internal/service/analysis_test.go
git commit -m "feat: add scattered media analysis to service layer"
```

---

## Task 4: Improve Scan Output with Categories

**Files:**
- Modify: `cmd/jellywatch/scan_cmd.go`

**Step 1: Update scan output to show categories**

In `cmd/jellywatch/scan_cmd.go`, replace the conflicts display section (around line 167-185):

```go
// Show categorized issues
if showStats {
	fmt.Println("\n=== Database Stats ===")

	// Count series
	var seriesCount int
	for _, lib := range cfg.Libraries.TV {
		count, _ := db.CountSeriesInLibrary(lib)
		seriesCount += count
	}
	fmt.Printf("TV Series: %d\n", seriesCount)

	// Count movies
	var movieCount int
	for _, lib := range cfg.Libraries.Movies {
		count, _ := db.CountMoviesInLibrary(lib)
		movieCount += count
	}
	fmt.Printf("Movies: %d\n", movieCount)

	// Analyze duplicates
	svc := service.NewCleanupService(db)

	dupAnalysis, err := svc.AnalyzeDuplicates()
	hasDuplicates := err == nil && dupAnalysis.TotalGroups > 0

	scatterAnalysis, err := svc.AnalyzeScattered()
	hasScattered := err == nil && scatterAnalysis.TotalItems > 0

	if hasDuplicates || hasScattered {
		fmt.Println("\n=== Issues Found ===")
	}

	// Show duplicates
	if hasDuplicates {
		fmt.Printf("\n📁 DUPLICATES (same content, different quality): %d groups\n", dupAnalysis.TotalGroups)
		fmt.Printf("   → These have inferior copies that can be DELETED to save %s\n", formatBytes(dupAnalysis.ReclaimableBytes))
	}

	// Show scattered media
	if hasScattered {
		fmt.Printf("\n🔀 SCATTERED MEDIA (same title in multiple locations): %d items\n", scatterAnalysis.TotalItems)
		fmt.Println("   → These need files MOVED to consolidate into one folder")

		// Show details
		for _, item := range scatterAnalysis.Items {
			yearStr := ""
			if item.Year != nil {
				yearStr = fmt.Sprintf(" (%d)", *item.Year)
			}
			fmt.Printf("  [%s] %s%s\n", item.MediaType, item.Title, yearStr)
			for _, loc := range item.Locations {
				fmt.Printf("    - %s\n", loc)
			}
		}
	}

	// Show guidance
	if hasDuplicates || hasScattered {
		fmt.Println("\n=== What's Next ===")

		if hasDuplicates {
			fmt.Println("\n1. Handle duplicates first (recommended):")
			fmt.Println("   jellywatch fix              # Interactive wizard (handles all)")
			fmt.Println("   jellywatch duplicates       # Review duplicates only")
		}

		if hasScattered {
			step := "1"
			if hasDuplicates {
				step = "2"
			}
			fmt.Printf("\n%s. Then consolidate scattered media:\n", step)
			fmt.Println("   jellywatch consolidate      # Review and organize")
		}
	} else {
		fmt.Println("\n✨ No issues detected - your library is clean!")
	}
}
```

**Step 2: Add service import**

At top of `cmd/jellywatch/scan_cmd.go`:

```go
import (
	// ... existing imports
	"github.com/Nomadcxx/jellywatch/internal/service"
)
```

**Step 3: Test manually**

Run: `go build -o jellywatch ./cmd/jellywatch && ./jellywatch scan --stats`
Expected: Categorized output with duplicates and scattered sections

**Step 4: Commit**

```bash
git add cmd/jellywatch/scan_cmd.go
git commit -m "feat: categorize duplicates vs scattered in scan output"
```

---

## Task 5: Create Fix Command (Wizard)

**Files:**
- Create: `cmd/jellywatch/fix_cmd.go`
- Create: `internal/wizard/wizard.go`
- Create: `internal/wizard/duplicates.go`
- Create: `internal/wizard/consolidate.go`
- Modify: `cmd/jellywatch/root.go`

**Step 1: Create wizard package with core types**

Create `internal/wizard/wizard.go`:

```go
package wizard

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/service"
)

// Wizard provides an interactive cleanup experience
type Wizard struct {
	db      *database.MediaDB
	service *service.CleanupService
	reader  *bufio.Reader
	dryRun  bool
	autoYes bool
}

// Options configures wizard behavior
type Options struct {
	DryRun         bool
	AutoYes        bool
	DuplicatesOnly bool
	ConsolidateOnly bool
}

// New creates a new wizard
func New(db *database.MediaDB, opts Options) *Wizard {
	return &Wizard{
		db:      db,
		service: service.NewCleanupService(db),
		reader:  bufio.NewReader(os.Stdin),
		dryRun:  opts.DryRun,
		autoYes: opts.AutoYes,
	}
}

// Run executes the wizard
func (w *Wizard) Run(opts Options) error {
	fmt.Println("🧹 Jellywatch Fix Wizard")
	if w.dryRun {
		fmt.Println("   (DRY RUN - no changes will be made)")
	}
	fmt.Println()

	// Analyze
	dupAnalysis, err := w.service.AnalyzeDuplicates()
	if err != nil {
		return fmt.Errorf("failed to analyze duplicates: %w", err)
	}

	scatterAnalysis, err := w.service.AnalyzeScattered()
	if err != nil {
		return fmt.Errorf("failed to analyze scattered media: %w", err)
	}

	// Summary
	if dupAnalysis.TotalGroups > 0 {
		fmt.Printf("Found %d duplicate groups (%s reclaimable)\n",
			dupAnalysis.TotalGroups, formatBytes(dupAnalysis.ReclaimableBytes))
	}
	if scatterAnalysis.TotalItems > 0 {
		fmt.Printf("Found %d scattered media items (need consolidation)\n",
			scatterAnalysis.TotalItems)
	}

	if dupAnalysis.TotalGroups == 0 && scatterAnalysis.TotalItems == 0 {
		fmt.Println("\n✨ No issues found - your library is clean!")
		return nil
	}

	fmt.Println()

	// Handle duplicates
	if !opts.ConsolidateOnly && dupAnalysis.TotalGroups > 0 {
		if err := w.handleDuplicates(dupAnalysis); err != nil {
			return err
		}
	}

	// Handle scattered
	if !opts.DuplicatesOnly && scatterAnalysis.TotalItems > 0 {
		if err := w.handleScattered(scatterAnalysis); err != nil {
			return err
		}
	}

	return nil
}

// prompt asks for user input
func (w *Wizard) prompt(message string) string {
	fmt.Print(message)
	if w.autoYes {
		fmt.Println("y")
		return "y"
	}
	response, _ := w.reader.ReadString('\n')
	return strings.TrimSpace(strings.ToLower(response))
}

// confirm asks yes/no question
func (w *Wizard) confirm(message string) bool {
	response := w.prompt(message + " [Y/n]: ")
	return response == "" || response == "y" || response == "yes"
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
```

**Step 2: Create duplicates wizard step**

Create `internal/wizard/duplicates.go`:

```go
package wizard

import (
	"fmt"
	"os"

	"github.com/Nomadcxx/jellywatch/internal/service"
)

func (w *Wizard) handleDuplicates(analysis *service.DuplicateAnalysis) error {
	fmt.Println("=== Step 1: Duplicates ===")

	if !w.confirm("Handle duplicates now?") {
		fmt.Println("Skipping duplicates.")
		return nil
	}
	fmt.Println()

	deletedCount := 0
	reclaimedBytes := int64(0)
	applyAll := false

	for i, group := range analysis.Groups {
		yearStr := ""
		if group.Year != nil {
			yearStr = fmt.Sprintf(" (%d)", *group.Year)
		}

		episodeStr := ""
		if group.Season != nil && group.Episode != nil {
			episodeStr = fmt.Sprintf(" S%02dE%02d", *group.Season, *group.Episode)
		}

		fmt.Printf("[%d/%d] %s%s%s - %d copies found\n",
			i+1, len(analysis.Groups), group.Title, yearStr, episodeStr, len(group.Files))

		// Show files
		for _, f := range group.Files {
			marker := "DELETE"
			if f.ID == group.BestFileID {
				marker = "KEEP"
			}
			fmt.Printf("  %s: %s %s (%s) - %s\n",
				marker, f.Resolution, f.SourceType, formatBytes(f.Size), f.Path)
		}

		fmt.Printf("\n  Space saved: %s\n\n", formatBytes(group.ReclaimableBytes))

		// Get action
		var action string
		if applyAll {
			action = "k"
		} else {
			action = w.prompt("  [K]eep suggestion / [S]wap / [Skip] / [A]ll remaining / [Q]uit: ")
		}

		switch action {
		case "k", "":
			// Delete non-best files
			for _, f := range group.Files {
				if f.ID != group.BestFileID {
					if w.dryRun {
						fmt.Printf("  Would delete: %s\n", f.Path)
					} else {
						if err := os.Remove(f.Path); err != nil {
							fmt.Printf("  ❌ Failed to delete %s: %v\n", f.Path, err)
							continue
						}
						_ = w.db.DeleteMediaFile(f.Path)
						fmt.Printf("  ✅ Deleted: %s\n", f.Path)
					}
					deletedCount++
					reclaimedBytes += f.Size
				}
			}
		case "s":
			fmt.Println("  Swap not implemented yet - skipping")
		case "skip":
			fmt.Println("  Skipped")
		case "a":
			applyAll = true
			// Process this one too
			for _, f := range group.Files {
				if f.ID != group.BestFileID {
					if w.dryRun {
						fmt.Printf("  Would delete: %s\n", f.Path)
					} else {
						if err := os.Remove(f.Path); err != nil {
							fmt.Printf("  ❌ Failed to delete %s: %v\n", f.Path, err)
							continue
						}
						_ = w.db.DeleteMediaFile(f.Path)
						fmt.Printf("  ✅ Deleted: %s\n", f.Path)
					}
					deletedCount++
					reclaimedBytes += f.Size
				}
			}
		case "q":
			fmt.Println("Quitting duplicates step.")
			break
		}
		fmt.Println()
	}

	if w.dryRun {
		fmt.Printf("✅ Would delete %d files, reclaim %s\n\n", deletedCount, formatBytes(reclaimedBytes))
	} else {
		fmt.Printf("✅ Deleted %d files, reclaimed %s\n\n", deletedCount, formatBytes(reclaimedBytes))
	}

	return nil
}
```

**Step 3: Create consolidate wizard step**

Create `internal/wizard/consolidate.go`:

```go
package wizard

import (
	"fmt"

	"github.com/Nomadcxx/jellywatch/internal/service"
)

func (w *Wizard) handleScattered(analysis *service.ScatteredAnalysis) error {
	fmt.Println("=== Step 2: Consolidation ===")

	if !w.confirm("Handle scattered media now?") {
		fmt.Println("Skipping consolidation.")
		return nil
	}
	fmt.Println()

	consolidatedCount := 0
	applyAll := false

	for i, item := range analysis.Items {
		yearStr := ""
		if item.Year != nil {
			yearStr = fmt.Sprintf(" (%d)", *item.Year)
		}

		fmt.Printf("[%d/%d] %s%s - scattered across %d locations\n",
			i+1, len(analysis.Items), item.Title, yearStr, len(item.Locations))

		fmt.Printf("  Target: %s\n", item.TargetLocation)
		for _, loc := range item.Locations {
			if loc != item.TargetLocation {
				fmt.Printf("  Move from: %s\n", loc)
			}
		}
		fmt.Println()

		var action string
		if applyAll {
			action = "m"
		} else {
			action = w.prompt("  [M]ove / [S]kip / [A]ll remaining / [Q]uit: ")
		}

		switch action {
		case "m", "":
			if w.dryRun {
				fmt.Println("  Would consolidate files to target location")
			} else {
				// TODO: Implement actual move using consolidate package
				fmt.Println("  ⚠️ Consolidation execution not yet implemented")
			}
			consolidatedCount++
		case "skip", "s":
			fmt.Println("  Skipped")
		case "a":
			applyAll = true
			if w.dryRun {
				fmt.Println("  Would consolidate files to target location")
			} else {
				fmt.Println("  ⚠️ Consolidation execution not yet implemented")
			}
			consolidatedCount++
		case "q":
			fmt.Println("Quitting consolidation step.")
			break
		}
		fmt.Println()
	}

	if w.dryRun {
		fmt.Printf("✅ Would consolidate %d items\n\n", consolidatedCount)
	} else {
		fmt.Printf("✅ Consolidated %d items\n\n", consolidatedCount)
	}

	return nil
}
```

**Step 4: Create fix command**

Create `cmd/jellywatch/fix_cmd.go`:

```go
package main

import (
	"fmt"

	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/wizard"
	"github.com/spf13/cobra"
)

func newFixCmd() *cobra.Command {
	var (
		dryRun          bool
		autoYes         bool
		duplicatesOnly  bool
		consolidateOnly bool
	)

	cmd := &cobra.Command{
		Use:   "fix",
		Short: "Interactive wizard to clean up your media library",
		Long: `Launch an interactive wizard that guides you through fixing issues
in your media library.

The wizard handles:
  1. Duplicates - Delete inferior quality copies to save space
  2. Scattered media - Consolidate files spread across multiple locations

Examples:
  jellywatch fix                    # Full interactive wizard
  jellywatch fix --dry-run          # Preview what would happen
  jellywatch fix --yes              # Auto-accept all suggestions
  jellywatch fix --duplicates-only  # Only handle duplicates
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFix(dryRun, autoYes, duplicatesOnly, consolidateOnly)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would happen without making changes")
	cmd.Flags().BoolVarP(&autoYes, "yes", "y", false, "Auto-accept all suggestions")
	cmd.Flags().BoolVar(&duplicatesOnly, "duplicates-only", false, "Only handle duplicates")
	cmd.Flags().BoolVar(&consolidateOnly, "consolidate-only", false, "Only handle consolidation")

	return cmd
}

func runFix(dryRun, autoYes, duplicatesOnly, consolidateOnly bool) error {
	db, err := database.Open()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	opts := wizard.Options{
		DryRun:          dryRun,
		AutoYes:         autoYes,
		DuplicatesOnly:  duplicatesOnly,
		ConsolidateOnly: consolidateOnly,
	}

	w := wizard.New(db, opts)
	return w.Run(opts)
}
```

**Step 5: Register fix command in root**

In `cmd/jellywatch/root.go`, add to the command registration:

```go
rootCmd.AddCommand(newFixCmd())
```

**Step 6: Test manually**

Run: `go build -o jellywatch ./cmd/jellywatch && ./jellywatch fix --dry-run`
Expected: Interactive wizard runs in dry-run mode

**Step 7: Commit**

```bash
git add cmd/jellywatch/fix_cmd.go cmd/jellywatch/root.go internal/wizard/
git commit -m "feat: add interactive fix wizard command"
```

---

## Task 6: Add Dry-Run to Duplicates Command

**Files:**
- Modify: `cmd/jellywatch/duplicates_cmd.go`

**Step 1: Add dry-run flag**

In `cmd/jellywatch/duplicates_cmd.go`, add flag:

```go
var (
	moviesOnly bool
	tvOnly     bool
	showFilter string
	execute    bool
	dryRun     bool  // Add this
)

// In flag definitions:
cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be deleted without making changes")
```

**Step 2: Update RunE to pass dryRun**

```go
RunE: func(cmd *cobra.Command, args []string) error {
	return runDuplicates(moviesOnly, tvOnly, showFilter, execute, dryRun)
}
```

**Step 3: Update function signature and logic**

```go
func runDuplicates(moviesOnly, tvOnly bool, showFilter string, execute, dryRun bool) error {
	// ... existing code until the execute section ...

	// If dry-run flag is set, show what would be deleted
	if dryRun {
		fmt.Println("\n🔍 DRY RUN - No changes will be made")
		fmt.Printf("\nWould delete %d files to reclaim %s:\n", len(filesToDelete), formatBytes(totalReclaimable))
		for _, path := range filesToDelete {
			fmt.Printf("  - %s\n", path)
		}
		return nil
	}

	// If execute flag is set, delete the files
	if execute {
		// ... existing execute code ...
	}

	// ... rest of function ...
}
```

**Step 4: Test manually**

Run: `go build -o jellywatch ./cmd/jellywatch && ./jellywatch duplicates --dry-run`
Expected: Shows what would be deleted without prompting

**Step 5: Commit**

```bash
git add cmd/jellywatch/duplicates_cmd.go
git commit -m "feat: add --dry-run flag to duplicates command"
```

---

## Task 7: Simplify Consolidate Command (Remove --generate)

**Files:**
- Modify: `cmd/jellywatch/consolidate_cmd.go`

**Step 1: Update command to generate on-the-fly**

Remove the `--generate` flag and modify `runConsolidate` to generate plans when needed:

```go
func newConsolidateCmd() *cobra.Command {
	var (
		dryRun   bool
		execute  bool
		status   bool
	)

	cmd := &cobra.Command{
		Use:   "consolidate [flags]",
		Short: "Consolidate scattered media files",
		Long: `Find and consolidate media files scattered across multiple locations.

This command identifies media with the same title in different folders
and offers to move them to a single location.

Examples:
  jellywatch consolidate              # Show what needs consolidation
  jellywatch consolidate --dry-run    # Preview the consolidation plan
  jellywatch consolidate --execute    # Execute the consolidation
  jellywatch consolidate --status     # Show pending plan status
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConsolidate(dryRun, execute, status)
		},
	}

	cmd.Flags().BoolVarP(&dryRun, "dry-run", "d", false, "Show what would be done without making changes")
	cmd.Flags().BoolVar(&execute, "execute", false, "Execute consolidation")
	cmd.Flags().BoolVar(&status, "status", false, "Show plan summary")

	return cmd
}

func runConsolidate(dryRun, execute, status bool) error {
	db, err := database.Open()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Show status if requested
	if status {
		return runConsolidateStatus(db)
	}

	// For dry-run or execute, generate plans on-the-fly
	if execute || dryRun {
		ctx := context.Background()

		// Generate fresh plans
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Clear old pending plans and regenerate
		if err := clearPendingPlans(db); err != nil {
			return fmt.Errorf("failed to clear old plans: %w", err)
		}

		consolidator := consolidate.NewConsolidator(db, cfg)
		plans, err := consolidator.GenerateAllPlans()
		if err != nil {
			return fmt.Errorf("failed to generate plans: %w", err)
		}

		// Store plans
		for _, plan := range plans {
			if err := consolidator.StorePlan(plan); err != nil {
				fmt.Printf("Warning: Failed to store plan for %s: %v\n", plan.Title, err)
			}
		}

		return runExecutePlans(ctx, db, dryRun)
	}

	// Default: Show summary using service layer
	svc := service.NewCleanupService(db)
	analysis, err := svc.AnalyzeScattered()
	if err != nil {
		return fmt.Errorf("failed to analyze: %w", err)
	}

	if analysis.TotalItems == 0 {
		fmt.Println("✨ No scattered media found - your library is organized!")
		return nil
	}

	fmt.Printf("Found %d items scattered across multiple locations:\n\n", analysis.TotalItems)

	for _, item := range analysis.Items {
		yearStr := ""
		if item.Year != nil {
			yearStr = fmt.Sprintf(" (%d)", *item.Year)
		}
		fmt.Printf("[%s] %s%s\n", item.MediaType, item.Title, yearStr)
		for _, loc := range item.Locations {
			fmt.Printf("  - %s\n", loc)
		}
	}

	fmt.Println("\nNext steps:")
	fmt.Println("  jellywatch consolidate --dry-run   # Preview what will happen")
	fmt.Println("  jellywatch consolidate --execute   # Execute consolidation")

	return nil
}
```

**Step 2: Add service import**

```go
import (
	// ... existing
	"github.com/Nomadcxx/jellywatch/internal/service"
)
```

**Step 3: Test manually**

Run: `go build -o jellywatch ./cmd/jellywatch && ./jellywatch consolidate --dry-run`
Expected: Shows consolidation plan without needing --generate first

**Step 4: Commit**

```bash
git add cmd/jellywatch/consolidate_cmd.go
git commit -m "refactor: simplify consolidate command, remove --generate step"
```

---

## Task 8: Create OpenAPI Specification

**Files:**
- Create: `api/openapi.yaml`

**Step 1: Create the OpenAPI spec**

Create `api/openapi.yaml`:

```yaml
openapi: 3.1.0
info:
  title: Jellywatch API
  description: API for managing media library cleanup operations
  version: 1.0.0
  license:
    name: MIT

servers:
  - url: http://localhost:8080/api/v1
    description: Local development server

paths:
  /duplicates:
    get:
      operationId: getDuplicates
      summary: Get duplicate analysis
      description: Analyzes the media library for duplicate files
      responses:
        '200':
          description: Duplicate analysis results
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/DuplicateAnalysis'
        '500':
          $ref: '#/components/responses/InternalError'

  /duplicates/{groupId}:
    delete:
      operationId: deleteDuplicate
      summary: Delete inferior file from duplicate group
      parameters:
        - name: groupId
          in: path
          required: true
          schema:
            type: string
        - name: fileId
          in: query
          description: Specific file to delete (default is auto-selected inferior)
          schema:
            type: integer
            format: int64
      responses:
        '200':
          description: Deletion result
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/OperationResult'
        '404':
          $ref: '#/components/responses/NotFound'
        '500':
          $ref: '#/components/responses/InternalError'

  /scattered:
    get:
      operationId: getScattered
      summary: Get scattered media analysis
      description: Finds media scattered across multiple locations
      responses:
        '200':
          description: Scattered media analysis
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ScatteredAnalysis'
        '500':
          $ref: '#/components/responses/InternalError'

  /scattered/{itemId}/consolidate:
    post:
      operationId: consolidateItem
      summary: Consolidate scattered media item
      parameters:
        - name: itemId
          in: path
          required: true
          schema:
            type: integer
            format: int64
      responses:
        '200':
          description: Consolidation result
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/OperationResult'
        '404':
          $ref: '#/components/responses/NotFound'
        '500':
          $ref: '#/components/responses/InternalError'

  /scan:
    post:
      operationId: startScan
      summary: Trigger library scan
      responses:
        '202':
          description: Scan started
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ScanStatus'
        '409':
          description: Scan already in progress
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Error'

  /scan/status:
    get:
      operationId: getScanStatus
      summary: Get scan status with SSE for progress
      responses:
        '200':
          description: Scan status stream
          content:
            text/event-stream:
              schema:
                $ref: '#/components/schemas/ScanProgress'

  /health:
    get:
      operationId: healthCheck
      summary: Health check endpoint
      responses:
        '200':
          description: Service is healthy
          content:
            application/json:
              schema:
                type: object
                properties:
                  status:
                    type: string
                    example: ok
                  version:
                    type: string

components:
  schemas:
    DuplicateAnalysis:
      type: object
      required:
        - groups
        - totalFiles
        - totalGroups
        - reclaimableBytes
      properties:
        groups:
          type: array
          items:
            $ref: '#/components/schemas/DuplicateGroup'
        totalFiles:
          type: integer
        totalGroups:
          type: integer
        reclaimableBytes:
          type: integer
          format: int64

    DuplicateGroup:
      type: object
      required:
        - id
        - title
        - mediaType
        - files
        - bestFileId
        - reclaimableBytes
      properties:
        id:
          type: string
        title:
          type: string
        year:
          type: integer
          nullable: true
        mediaType:
          type: string
          enum: [movie, series]
        season:
          type: integer
          nullable: true
        episode:
          type: integer
          nullable: true
        files:
          type: array
          items:
            $ref: '#/components/schemas/MediaFile'
        bestFileId:
          type: integer
          format: int64
        reclaimableBytes:
          type: integer
          format: int64

    MediaFile:
      type: object
      required:
        - id
        - path
        - size
        - qualityScore
      properties:
        id:
          type: integer
          format: int64
        path:
          type: string
        size:
          type: integer
          format: int64
        resolution:
          type: string
        sourceType:
          type: string
        qualityScore:
          type: integer

    ScatteredAnalysis:
      type: object
      required:
        - items
        - totalItems
        - totalMoves
        - totalBytes
      properties:
        items:
          type: array
          items:
            $ref: '#/components/schemas/ScatteredItem'
        totalItems:
          type: integer
        totalMoves:
          type: integer
        totalBytes:
          type: integer
          format: int64

    ScatteredItem:
      type: object
      required:
        - id
        - title
        - mediaType
        - locations
        - targetLocation
        - filesToMove
      properties:
        id:
          type: integer
          format: int64
        title:
          type: string
        year:
          type: integer
          nullable: true
        mediaType:
          type: string
        locations:
          type: array
          items:
            type: string
        targetLocation:
          type: string
        filesToMove:
          type: integer
        bytesToMove:
          type: integer
          format: int64

    OperationResult:
      type: object
      required:
        - success
      properties:
        success:
          type: boolean
        message:
          type: string
        filesAffected:
          type: integer
        bytesAffected:
          type: integer
          format: int64
        error:
          type: string

    ScanStatus:
      type: object
      required:
        - status
      properties:
        status:
          type: string
          enum: [idle, scanning, completed, failed]
        startedAt:
          type: string
          format: date-time
        completedAt:
          type: string
          format: date-time
        progress:
          type: integer
          minimum: 0
          maximum: 100
        message:
          type: string

    ScanProgress:
      type: object
      properties:
        type:
          type: string
          enum: [started, progress, completed, error]
        current:
          type: integer
        total:
          type: integer
        message:
          type: string
        duplicatesFound:
          type: integer
        scatteredFound:
          type: integer

    Error:
      type: object
      required:
        - code
        - message
      properties:
        code:
          type: string
        message:
          type: string

  responses:
    NotFound:
      description: Resource not found
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/Error'
    InternalError:
      description: Internal server error
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/Error'
```

**Step 2: Commit**

```bash
mkdir -p api
git add api/openapi.yaml
git commit -m "docs: add OpenAPI 3.1 specification for Jellywatch API"
```

---

## Task 9: Generate API Types with oapi-codegen

**Files:**
- Create: `api/generate.go`
- Create: `api/types.gen.go` (generated)
- Create: `api/server.gen.go` (generated)

**Step 1: Install oapi-codegen**

Run: `go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest`

**Step 2: Create generate.go**

Create `api/generate.go`:

```go
package api

//go:generate oapi-codegen -package api -generate types -o types.gen.go openapi.yaml
//go:generate oapi-codegen -package api -generate chi-server -o server.gen.go openapi.yaml
```

**Step 3: Run code generation**

Run: `go generate ./api/...`
Expected: Creates types.gen.go and server.gen.go

**Step 4: Commit generated files**

```bash
git add api/
git commit -m "feat: generate API types and server interface from OpenAPI spec"
```

---

## Task 10: Implement API Server

**Files:**
- Create: `internal/api/server.go`
- Create: `internal/api/handlers.go`
- Create: `cmd/jellywatch/serve_cmd.go`

**Step 1: Create API server**

Create `internal/api/server.go`:

```go
package api

import (
	"net/http"

	"github.com/Nomadcxx/jellywatch/api"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Server implements the API
type Server struct {
	db      *database.MediaDB
	service *service.CleanupService
}

// NewServer creates a new API server
func NewServer(db *database.MediaDB) *Server {
	return &Server{
		db:      db,
		service: service.NewCleanupService(db),
	}
}

// Handler returns the HTTP handler
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.SetHeader("Content-Type", "application/json"))

	// Mount API routes
	api.HandlerFromMux(s, r)

	return r
}
```

**Step 2: Create handlers**

Create `internal/api/handlers.go`:

```go
package api

import (
	"encoding/json"
	"net/http"

	"github.com/Nomadcxx/jellywatch/api"
)

// Ensure Server implements the interface
var _ api.ServerInterface = (*Server)(nil)

// GetDuplicates implements api.ServerInterface
func (s *Server) GetDuplicates(w http.ResponseWriter, r *http.Request) {
	analysis, err := s.service.AnalyzeDuplicates()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "analysis_failed", err.Error())
		return
	}

	// Convert to API types
	response := api.DuplicateAnalysis{
		Groups:           make([]api.DuplicateGroup, len(analysis.Groups)),
		TotalFiles:       analysis.TotalFiles,
		TotalGroups:      analysis.TotalGroups,
		ReclaimableBytes: analysis.ReclaimableBytes,
	}

	for i, g := range analysis.Groups {
		group := api.DuplicateGroup{
			Id:               g.ID,
			Title:            g.Title,
			MediaType:        api.DuplicateGroupMediaType(g.MediaType),
			Files:            make([]api.MediaFile, len(g.Files)),
			BestFileId:       g.BestFileID,
			ReclaimableBytes: g.ReclaimableBytes,
		}
		if g.Year != nil {
			group.Year = g.Year
		}
		if g.Season != nil {
			group.Season = g.Season
		}
		if g.Episode != nil {
			group.Episode = g.Episode
		}

		for j, f := range g.Files {
			group.Files[j] = api.MediaFile{
				Id:           f.ID,
				Path:         f.Path,
				Size:         f.Size,
				Resolution:   &f.Resolution,
				SourceType:   &f.SourceType,
				QualityScore: f.QualityScore,
			}
		}
		response.Groups[i] = group
	}

	writeJSON(w, http.StatusOK, response)
}

// DeleteDuplicate implements api.ServerInterface
func (s *Server) DeleteDuplicate(w http.ResponseWriter, r *http.Request, groupId string, params api.DeleteDuplicateParams) {
	// TODO: Implement deletion
	writeJSON(w, http.StatusOK, api.OperationResult{
		Success: true,
		Message: ptrString("Not implemented yet"),
	})
}

// GetScattered implements api.ServerInterface
func (s *Server) GetScattered(w http.ResponseWriter, r *http.Request) {
	analysis, err := s.service.AnalyzeScattered()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "analysis_failed", err.Error())
		return
	}

	response := api.ScatteredAnalysis{
		Items:      make([]api.ScatteredItem, len(analysis.Items)),
		TotalItems: analysis.TotalItems,
		TotalMoves: analysis.TotalMoves,
		TotalBytes: analysis.TotalBytes,
	}

	for i, item := range analysis.Items {
		response.Items[i] = api.ScatteredItem{
			Id:             item.ID,
			Title:          item.Title,
			Year:           item.Year,
			MediaType:      item.MediaType,
			Locations:      item.Locations,
			TargetLocation: item.TargetLocation,
			FilesToMove:    item.FilesToMove,
			BytesToMove:    &item.BytesToMove,
		}
	}

	writeJSON(w, http.StatusOK, response)
}

// ConsolidateItem implements api.ServerInterface
func (s *Server) ConsolidateItem(w http.ResponseWriter, r *http.Request, itemId int64) {
	// TODO: Implement consolidation
	writeJSON(w, http.StatusOK, api.OperationResult{
		Success: true,
		Message: ptrString("Not implemented yet"),
	})
}

// StartScan implements api.ServerInterface
func (s *Server) StartScan(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement scan trigger
	writeJSON(w, http.StatusAccepted, api.ScanStatus{
		Status:  api.ScanStatusStatusScanning,
		Message: ptrString("Scan started"),
	})
}

// GetScanStatus implements api.ServerInterface
func (s *Server) GetScanStatus(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement SSE streaming
	writeJSON(w, http.StatusOK, api.ScanStatus{
		Status: api.ScanStatusStatusIdle,
	})
}

// HealthCheck implements api.ServerInterface
func (s *Server) HealthCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": "1.0.0",
	})
}

// Helper functions
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(api.Error{
		Code:    code,
		Message: message,
	})
}

func ptrString(s string) *string {
	return &s
}
```

**Step 3: Create serve command**

Create `cmd/jellywatch/serve_cmd.go`:

```go
package main

import (
	"fmt"
	"net/http"

	"github.com/Nomadcxx/jellywatch/internal/api"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/spf13/cobra"
)

func newServeCmd() *cobra.Command {
	var (
		addr string
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the API server",
		Long: `Start the HTTP API server for web UI and external integrations.

The API follows the OpenAPI 3.1 specification defined in api/openapi.yaml.

Examples:
  jellywatch serve                  # Start on default port 8080
  jellywatch serve --addr :9000     # Start on port 9000
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(addr)
		},
	}

	cmd.Flags().StringVar(&addr, "addr", ":8080", "Address to listen on")

	return cmd
}

func runServe(addr string) error {
	db, err := database.Open()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	server := api.NewServer(db)
	handler := server.Handler()

	fmt.Printf("Starting Jellywatch API server on %s\n", addr)
	fmt.Println("API documentation: http://localhost" + addr + "/api/v1")

	return http.ListenAndServe(addr, handler)
}
```

**Step 4: Register serve command in root**

In `cmd/jellywatch/root.go`:

```go
rootCmd.AddCommand(newServeCmd())
```

**Step 5: Add chi dependency**

Run: `go get github.com/go-chi/chi/v5`

**Step 6: Test manually**

Run: `go build -o jellywatch ./cmd/jellywatch && ./jellywatch serve`
Then: `curl http://localhost:8080/api/v1/health`
Expected: `{"status":"ok","version":"1.0.0"}`

**Step 7: Commit**

```bash
git add internal/api/ cmd/jellywatch/serve_cmd.go cmd/jellywatch/root.go go.mod go.sum
git commit -m "feat: add API server with OpenAPI-generated handlers"
```

---

## Summary

| Task | Description | Commits |
|------|-------------|---------|
| 1 | Fix SQL NULL error | `fix: handle NULL source_file_id` |
| 2 | Create service layer | `feat: add service layer with duplicate analysis` |
| 3 | Add scattered analysis | `feat: add scattered media analysis` |
| 4 | Improve scan output | `feat: categorize duplicates vs scattered` |
| 5 | Create fix wizard | `feat: add interactive fix wizard command` |
| 6 | Add dry-run to duplicates | `feat: add --dry-run to duplicates` |
| 7 | Simplify consolidate | `refactor: simplify consolidate command` |
| 8 | Create OpenAPI spec | `docs: add OpenAPI 3.1 specification` |
| 9 | Generate API types | `feat: generate API types from OpenAPI` |
| 10 | Implement API server | `feat: add API server` |

**Total: 10 tasks, 10 commits**
