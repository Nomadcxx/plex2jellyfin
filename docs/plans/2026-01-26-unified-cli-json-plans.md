# Unified CLI JSON Plans Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Unify `duplicates` and `consolidate` commands to use identical JSON-based workflow.

**Architecture:** Create `internal/plans/` package for JSON read/write. Update both commands to use `--generate` → `--dry-run` → `--execute` workflow with JSON files stored in `~/.config/jellywatch/plans/`.

**Tech Stack:** Go, encoding/json, os (file operations)

---

## Task 1: Create plans package with types

**Files:**
- Create: `internal/plans/plans.go`
- Create: `internal/plans/plans_test.go`

**Step 1: Create the plans package with types**

Create `internal/plans/plans.go`:

```go
package plans

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// FileInfo represents a media file in a plan
type FileInfo struct {
	ID           int64  `json:"id"`
	Path         string `json:"path"`
	Size         int64  `json:"size"`
	QualityScore int    `json:"quality_score"`
	Resolution   string `json:"resolution"`
	SourceType   string `json:"source_type"`
}

// DuplicateGroup represents a group of duplicate files
type DuplicateGroup struct {
	GroupID   string    `json:"group_id"`
	Title     string    `json:"title"`
	Year      *int      `json:"year"`
	MediaType string    `json:"media_type"`
	Season    *int      `json:"season,omitempty"`
	Episode   *int      `json:"episode,omitempty"`
	Keep      FileInfo  `json:"keep"`
	Delete    FileInfo  `json:"delete"`
}

// DuplicateSummary contains summary stats for duplicate plans
type DuplicateSummary struct {
	TotalGroups      int   `json:"total_groups"`
	FilesToDelete    int   `json:"files_to_delete"`
	SpaceReclaimable int64 `json:"space_reclaimable"`
}

// DuplicatePlan represents the full duplicate deletion plan
type DuplicatePlan struct {
	CreatedAt time.Time        `json:"created_at"`
	Command   string           `json:"command"`
	Summary   DuplicateSummary `json:"summary"`
	Plans     []DuplicateGroup `json:"plans"`
}

// MoveOperation represents a single file move
type MoveOperation struct {
	Action     string `json:"action"`
	SourcePath string `json:"source_path"`
	TargetPath string `json:"target_path"`
	Size       int64  `json:"size"`
}

// ConsolidateGroup represents files to consolidate for one title
type ConsolidateGroup struct {
	ConflictID     int64           `json:"conflict_id"`
	Title          string          `json:"title"`
	Year           *int            `json:"year"`
	MediaType      string          `json:"media_type"`
	TargetLocation string          `json:"target_location"`
	Operations     []MoveOperation `json:"operations"`
}

// ConsolidateSummary contains summary stats for consolidate plans
type ConsolidateSummary struct {
	TotalConflicts int   `json:"total_conflicts"`
	TotalMoves     int   `json:"total_moves"`
	TotalBytes     int64 `json:"total_bytes"`
}

// ConsolidatePlan represents the full consolidation plan
type ConsolidatePlan struct {
	CreatedAt time.Time          `json:"created_at"`
	Command   string             `json:"command"`
	Summary   ConsolidateSummary `json:"summary"`
	Plans     []ConsolidateGroup `json:"plans"`
}

// GetPlansDir returns the directory for plan files
func GetPlansDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".config", "jellywatch", "plans"), nil
}

// ensurePlansDir creates the plans directory if it doesn't exist
func ensurePlansDir() error {
	dir, err := GetPlansDir()
	if err != nil {
		return err
	}
	return os.MkdirAll(dir, 0755)
}

// getDuplicatePlansPath returns the path to duplicates.json
func getDuplicatePlansPath() (string, error) {
	dir, err := GetPlansDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "duplicates.json"), nil
}

// getConsolidatePlansPath returns the path to consolidate.json
func getConsolidatePlansPath() (string, error) {
	dir, err := GetPlansDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "consolidate.json"), nil
}
```

**Step 2: Run build to verify no syntax errors**

Run: `go build ./internal/plans/...`
Expected: No errors

**Step 3: Commit**

```bash
git add internal/plans/plans.go
git commit -m "feat(plans): add types for JSON-based plans"
```

---

## Task 2: Add plans package read/write functions

**Files:**
- Modify: `internal/plans/plans.go`
- Create: `internal/plans/plans_test.go`

**Step 1: Add read/write functions to plans.go**

Add to `internal/plans/plans.go`:

```go
// SaveDuplicatePlans writes the duplicate plan to JSON file
func SaveDuplicatePlans(plan *DuplicatePlan) error {
	if err := ensurePlansDir(); err != nil {
		return fmt.Errorf("failed to create plans directory: %w", err)
	}

	path, err := getDuplicatePlansPath()
	if err != nil {
		return err
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

// LoadDuplicatePlans reads the duplicate plan from JSON file
func LoadDuplicatePlans() (*DuplicatePlan, error) {
	path, err := getDuplicatePlansPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No plans file exists
		}
		return nil, fmt.Errorf("failed to read plan file: %w", err)
	}

	var plan DuplicatePlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("failed to parse plan file: %w", err)
	}

	return &plan, nil
}

// DeleteDuplicatePlans removes the duplicate plans file
func DeleteDuplicatePlans() error {
	path, err := getDuplicatePlansPath()
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete plan file: %w", err)
	}

	return nil
}

// SaveConsolidatePlans writes the consolidate plan to JSON file
func SaveConsolidatePlans(plan *ConsolidatePlan) error {
	if err := ensurePlansDir(); err != nil {
		return fmt.Errorf("failed to create plans directory: %w", err)
	}

	path, err := getConsolidatePlansPath()
	if err != nil {
		return err
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

// LoadConsolidatePlans reads the consolidate plan from JSON file
func LoadConsolidatePlans() (*ConsolidatePlan, error) {
	path, err := getConsolidatePlansPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No plans file exists
		}
		return nil, fmt.Errorf("failed to read plan file: %w", err)
	}

	var plan ConsolidatePlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("failed to parse plan file: %w", err)
	}

	return &plan, nil
}

// DeleteConsolidatePlans removes the consolidate plans file
func DeleteConsolidatePlans() error {
	path, err := getConsolidatePlansPath()
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete plan file: %w", err)
	}

	return nil
}
```

**Step 2: Create test file**

Create `internal/plans/plans_test.go`:

```go
package plans

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadDuplicatePlans(t *testing.T) {
	// Use temp directory
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	year := 2005
	plan := &DuplicatePlan{
		CreatedAt: time.Now(),
		Command:   "duplicates",
		Summary: DuplicateSummary{
			TotalGroups:      1,
			FilesToDelete:    1,
			SpaceReclaimable: 4400000000,
		},
		Plans: []DuplicateGroup{
			{
				GroupID:   "abc123",
				Title:     "Robots",
				Year:      &year,
				MediaType: "movie",
				Keep: FileInfo{
					ID:           1,
					Path:         "/storage1/Robots.mkv",
					Size:         4300000000,
					QualityScore: 284,
				},
				Delete: FileInfo{
					ID:           2,
					Path:         "/storage2/Robots.mkv",
					Size:         4400000000,
					QualityScore: 84,
				},
			},
		},
	}

	// Save
	err := SaveDuplicatePlans(plan)
	if err != nil {
		t.Fatalf("SaveDuplicatePlans failed: %v", err)
	}

	// Verify file exists
	path, _ := getDuplicatePlansPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("Plan file was not created")
	}

	// Load
	loaded, err := LoadDuplicatePlans()
	if err != nil {
		t.Fatalf("LoadDuplicatePlans failed: %v", err)
	}

	if loaded == nil {
		t.Fatal("Loaded plan is nil")
	}

	if loaded.Summary.TotalGroups != 1 {
		t.Errorf("Expected 1 group, got %d", loaded.Summary.TotalGroups)
	}

	if len(loaded.Plans) != 1 {
		t.Fatalf("Expected 1 plan, got %d", len(loaded.Plans))
	}

	if loaded.Plans[0].Title != "Robots" {
		t.Errorf("Expected title 'Robots', got '%s'", loaded.Plans[0].Title)
	}

	// Delete
	err = DeleteDuplicatePlans()
	if err != nil {
		t.Fatalf("DeleteDuplicatePlans failed: %v", err)
	}

	// Verify file is gone
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("Plan file was not deleted")
	}
}

func TestLoadDuplicatePlans_NoFile(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	plan, err := LoadDuplicatePlans()
	if err != nil {
		t.Fatalf("LoadDuplicatePlans should not error for missing file: %v", err)
	}

	if plan != nil {
		t.Fatal("Expected nil plan for missing file")
	}
}

func TestSaveAndLoadConsolidatePlans(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	year := 2005
	plan := &ConsolidatePlan{
		CreatedAt: time.Now(),
		Command:   "consolidate",
		Summary: ConsolidateSummary{
			TotalConflicts: 1,
			TotalMoves:     2,
			TotalBytes:     1000000000,
		},
		Plans: []ConsolidateGroup{
			{
				ConflictID:     1,
				Title:          "American Dad",
				Year:           &year,
				MediaType:      "series",
				TargetLocation: "/storage1/TV/American Dad (2005)",
				Operations: []MoveOperation{
					{
						Action:     "move",
						SourcePath: "/storage2/TV/American Dad/S01E01.mkv",
						TargetPath: "/storage1/TV/American Dad (2005)/S01E01.mkv",
						Size:       500000000,
					},
				},
			},
		},
	}

	err := SaveConsolidatePlans(plan)
	if err != nil {
		t.Fatalf("SaveConsolidatePlans failed: %v", err)
	}

	loaded, err := LoadConsolidatePlans()
	if err != nil {
		t.Fatalf("LoadConsolidatePlans failed: %v", err)
	}

	if loaded.Summary.TotalConflicts != 1 {
		t.Errorf("Expected 1 conflict, got %d", loaded.Summary.TotalConflicts)
	}

	err = DeleteConsolidatePlans()
	if err != nil {
		t.Fatalf("DeleteConsolidatePlans failed: %v", err)
	}
}

func TestGetPlansDir(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	dir, err := GetPlansDir()
	if err != nil {
		t.Fatalf("GetPlansDir failed: %v", err)
	}

	expected := filepath.Join(tempDir, ".config", "jellywatch", "plans")
	if dir != expected {
		t.Errorf("Expected %s, got %s", expected, dir)
	}
}
```

**Step 3: Run tests**

Run: `go test ./internal/plans/... -v`
Expected: All tests PASS

**Step 4: Commit**

```bash
git add internal/plans/
git commit -m "feat(plans): add JSON read/write functions with tests"
```

---

## Task 3: Update duplicates command with --generate flag

**Files:**
- Modify: `cmd/jellywatch/duplicates_cmd.go`

**Step 1: Rewrite duplicates command**

Replace `cmd/jellywatch/duplicates_cmd.go` entirely:

```go
package main

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/plans"
	"github.com/spf13/cobra"
)

func newDuplicatesCmd() *cobra.Command {
	var (
		moviesOnly bool
		tvOnly     bool
		showFilter string
		generate   bool
		dryRun     bool
		execute    bool
	)

	cmd := &cobra.Command{
		Use:   "duplicates [flags]",
		Short: "Find and remove duplicate media files",
		Long: `Find and optionally remove duplicate media files from your library.

Workflow:
  1. jellywatch duplicates              # Show duplicate analysis
  2. jellywatch duplicates --generate   # Generate deletion plans
  3. jellywatch duplicates --dry-run    # Preview pending plans
  4. jellywatch duplicates --execute    # Execute plans (delete files)

Examples:
  jellywatch duplicates              # List all duplicates
  jellywatch duplicates --movies     # Only movies
  jellywatch duplicates --tv         # Only TV episodes
  jellywatch duplicates --generate   # Generate deletion plans
  jellywatch duplicates --dry-run    # Preview what would be deleted
  jellywatch duplicates --execute    # DELETE inferior duplicates
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDuplicates(moviesOnly, tvOnly, showFilter, generate, dryRun, execute)
		},
	}

	cmd.Flags().BoolVar(&moviesOnly, "movies", false, "Show only movie duplicates")
	cmd.Flags().BoolVar(&tvOnly, "tv", false, "Show only TV episode duplicates")
	cmd.Flags().StringVar(&showFilter, "show", "", "Filter by show name")
	cmd.Flags().BoolVar(&generate, "generate", false, "Generate deletion plans")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview pending plans")
	cmd.Flags().BoolVar(&execute, "execute", false, "Execute pending plans (delete files)")

	return cmd
}

func runDuplicates(moviesOnly, tvOnly bool, showFilter string, generate, dryRun, execute bool) error {
	db, err := database.Open()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Generate plans
	if generate {
		return runDuplicatesGenerate(db, moviesOnly, tvOnly, showFilter)
	}

	// Preview plans
	if dryRun {
		return runDuplicatesDryRun()
	}

	// Execute plans
	if execute {
		return runDuplicatesExecute(db)
	}

	// Default: show analysis
	return runDuplicatesAnalysis(db, moviesOnly, tvOnly, showFilter)
}

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

func runDuplicatesAnalysis(db *database.MediaDB, moviesOnly, tvOnly bool, showFilter string) error {
	totalGroups := 0
	totalReclaimable := int64(0)

	// Movie duplicates
	if !tvOnly {
		movieGroups, err := db.FindDuplicateMovies()
		if err != nil {
			return fmt.Errorf("failed to find duplicate movies: %w", err)
		}

		if len(movieGroups) > 0 {
			fmt.Println("=== Duplicate Movies ===")
			for _, group := range movieGroups {
				if len(group.Files) < 2 {
					continue
				}
				totalGroups++
				totalReclaimable += group.SpaceReclaimable
				printDuplicateGroup(group.NormalizedTitle, group.Year, nil, nil, group.Files, group.BestFile)
			}
		}
	}

	// TV duplicates
	if !moviesOnly {
		episodeGroups, err := db.FindDuplicateEpisodes()
		if err != nil {
			return fmt.Errorf("failed to find duplicate episodes: %w", err)
		}

		if len(episodeGroups) > 0 {
			fmt.Println("=== Duplicate TV Episodes ===")
			for _, group := range episodeGroups {
				if len(group.Files) < 2 {
					continue
				}
				if showFilter != "" && !strings.Contains(strings.ToLower(group.NormalizedTitle), strings.ToLower(showFilter)) {
					continue
				}
				totalGroups++
				totalReclaimable += group.SpaceReclaimable
				printDuplicateGroup(group.NormalizedTitle, group.Year, group.Season, group.Episode, group.Files, group.BestFile)
			}
		}
	}

	if totalGroups == 0 {
		fmt.Println("✅ No duplicates found!")
		return nil
	}

	fmt.Println("=== Summary ===")
	fmt.Printf("Duplicate groups:   %d\n", totalGroups)
	fmt.Printf("Space reclaimable: %s\n", formatBytes(totalReclaimable))
	fmt.Println("\nNext steps:")
	fmt.Println("  jellywatch duplicates --generate   # Generate deletion plans")
	fmt.Println("  jellywatch duplicates --dry-run    # Preview plans")
	fmt.Println("  jellywatch duplicates --execute    # Execute plans")

	return nil
}

func printDuplicateGroup(title string, year, season, episode *int, files []*database.MediaFile, bestFile *database.MediaFile) {
	yearStr := ""
	if year != nil {
		yearStr = fmt.Sprintf(" (%d)", *year)
	}
	episodeStr := ""
	if season != nil && episode != nil {
		episodeStr = fmt.Sprintf(" S%02dE%02d", *season, *episode)
	}

	fmt.Printf("=== %s%s%s ===\n", title, yearStr, episodeStr)
	fmt.Printf("%-15s | %-8s | %-12s | %-8s | %s\n", "Quality Score", "Size", "Resolution", "Source", "Path")
	fmt.Println(strings.Repeat("-", 120))

	for _, file := range files {
		marker := "[DELETE]"
		if bestFile != nil && file.ID == bestFile.ID {
			marker = "[KEEP]"
		}
		fmt.Printf("%-15d | %-8s | %-12s | %-8s | %s %s\n",
			file.QualityScore, formatBytes(file.Size), file.Resolution, file.SourceType, file.Path, marker)
	}
	fmt.Println()
}

func runDuplicatesGenerate(db *database.MediaDB, moviesOnly, tvOnly bool, showFilter string) error {
	fmt.Println("🔍 Analyzing duplicates...")

	plan := &plans.DuplicatePlan{
		CreatedAt: time.Now(),
		Command:   "duplicates",
		Summary:   plans.DuplicateSummary{},
		Plans:     []plans.DuplicateGroup{},
	}

	// Movie duplicates
	if !tvOnly {
		movieGroups, err := db.FindDuplicateMovies()
		if err != nil {
			return fmt.Errorf("failed to find duplicate movies: %w", err)
		}

		for _, group := range movieGroups {
			if len(group.Files) < 2 || group.BestFile == nil {
				continue
			}

			for _, file := range group.Files {
				if file.ID == group.BestFile.ID {
					continue
				}

				plan.Plans = append(plan.Plans, plans.DuplicateGroup{
					GroupID:   generateGroupID(group.NormalizedTitle, group.Year, nil, nil),
					Title:     group.NormalizedTitle,
					Year:      group.Year,
					MediaType: "movie",
					Keep: plans.FileInfo{
						ID:           group.BestFile.ID,
						Path:         group.BestFile.Path,
						Size:         group.BestFile.Size,
						QualityScore: group.BestFile.QualityScore,
						Resolution:   group.BestFile.Resolution,
						SourceType:   group.BestFile.SourceType,
					},
					Delete: plans.FileInfo{
						ID:           file.ID,
						Path:         file.Path,
						Size:         file.Size,
						QualityScore: file.QualityScore,
						Resolution:   file.Resolution,
						SourceType:   file.SourceType,
					},
				})

				plan.Summary.FilesToDelete++
				plan.Summary.SpaceReclaimable += file.Size
			}
			plan.Summary.TotalGroups++
		}
	}

	// TV duplicates
	if !moviesOnly {
		episodeGroups, err := db.FindDuplicateEpisodes()
		if err != nil {
			return fmt.Errorf("failed to find duplicate episodes: %w", err)
		}

		for _, group := range episodeGroups {
			if len(group.Files) < 2 || group.BestFile == nil {
				continue
			}

			if showFilter != "" && !strings.Contains(strings.ToLower(group.NormalizedTitle), strings.ToLower(showFilter)) {
				continue
			}

			for _, file := range group.Files {
				if file.ID == group.BestFile.ID {
					continue
				}

				plan.Plans = append(plan.Plans, plans.DuplicateGroup{
					GroupID:   generateGroupID(group.NormalizedTitle, group.Year, group.Season, group.Episode),
					Title:     group.NormalizedTitle,
					Year:      group.Year,
					MediaType: "series",
					Season:    group.Season,
					Episode:   group.Episode,
					Keep: plans.FileInfo{
						ID:           group.BestFile.ID,
						Path:         group.BestFile.Path,
						Size:         group.BestFile.Size,
						QualityScore: group.BestFile.QualityScore,
						Resolution:   group.BestFile.Resolution,
						SourceType:   group.BestFile.SourceType,
					},
					Delete: plans.FileInfo{
						ID:           file.ID,
						Path:         file.Path,
						Size:         file.Size,
						QualityScore: file.QualityScore,
						Resolution:   file.Resolution,
						SourceType:   file.SourceType,
					},
				})

				plan.Summary.FilesToDelete++
				plan.Summary.SpaceReclaimable += file.Size
			}
			plan.Summary.TotalGroups++
		}
	}

	if len(plan.Plans) == 0 {
		fmt.Println("✅ No duplicates found!")
		return nil
	}

	if err := plans.SaveDuplicatePlans(plan); err != nil {
		return fmt.Errorf("failed to save plans: %w", err)
	}

	fmt.Println("\n✅ Plans generated successfully!")
	fmt.Printf("\nDuplicate Summary:\n")
	fmt.Printf("  Duplicate groups:   %d\n", plan.Summary.TotalGroups)
	fmt.Printf("  Files to delete:    %d\n", plan.Summary.FilesToDelete)
	fmt.Printf("  Space reclaimable: %s\n", formatBytes(plan.Summary.SpaceReclaimable))

	plansDir, _ := plans.GetPlansDir()
	fmt.Printf("\nPlan saved to: %s/duplicates.json\n", plansDir)

	fmt.Println("\nNext steps:")
	fmt.Println("  jellywatch duplicates --dry-run    # Preview what will be deleted")
	fmt.Println("  jellywatch duplicates --execute    # Execute the plans")

	return nil
}

func runDuplicatesDryRun() error {
	plan, err := plans.LoadDuplicatePlans()
	if err != nil {
		return fmt.Errorf("failed to load plans: %w", err)
	}

	if plan == nil {
		fmt.Println("No pending plans found.")
		fmt.Println("Run 'jellywatch duplicates --generate' first to create plans.")
		return nil
	}

	fmt.Println("🔍 DRY RUN - No changes will be made")
	fmt.Printf("\nPlan created: %s\n", plan.CreatedAt.Format(time.RFC3339))
	fmt.Printf("Files to delete: %d\n", plan.Summary.FilesToDelete)
	fmt.Printf("Space to reclaim: %s\n\n", formatBytes(plan.Summary.SpaceReclaimable))

	for i, p := range plan.Plans {
		yearStr := ""
		if p.Year != nil {
			yearStr = fmt.Sprintf(" (%d)", *p.Year)
		}
		episodeStr := ""
		if p.Season != nil && p.Episode != nil {
			episodeStr = fmt.Sprintf(" S%02dE%02d", *p.Season, *p.Episode)
		}

		fmt.Printf("[%d] %s%s%s\n", i+1, p.Title, yearStr, episodeStr)
		fmt.Printf("    KEEP:   %s (%s, score %d)\n", p.Keep.Path, formatBytes(p.Keep.Size), p.Keep.QualityScore)
		fmt.Printf("    DELETE: %s (%s, score %d)\n\n", p.Delete.Path, formatBytes(p.Delete.Size), p.Delete.QualityScore)
	}

	fmt.Println("To execute these plans, run:")
	fmt.Println("  jellywatch duplicates --execute")

	return nil
}

func runDuplicatesExecute(db *database.MediaDB) error {
	plan, err := plans.LoadDuplicatePlans()
	if err != nil {
		return fmt.Errorf("failed to load plans: %w", err)
	}

	if plan == nil {
		fmt.Println("No pending plans found.")
		fmt.Println("Run 'jellywatch duplicates --generate' first to create plans.")
		return nil
	}

	fmt.Printf("⚠️  WARNING: This will permanently DELETE %d files.\n", plan.Summary.FilesToDelete)
	fmt.Printf("Space to reclaim: %s\n", formatBytes(plan.Summary.SpaceReclaimable))
	fmt.Println("This action CANNOT be undone!")
	fmt.Print("\nContinue? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read confirmation: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response != "y" && response != "yes" {
		fmt.Println("❌ Execution cancelled.")
		return nil
	}

	fmt.Println("\n🗑️  Deleting duplicate files...")

	deletedCount := 0
	failedCount := 0
	reclaimedBytes := int64(0)

	for i, p := range plan.Plans {
		fmt.Printf("[%d/%d] Deleting: %s\n", i+1, len(plan.Plans), p.Delete.Path)

		fileInfo, err := os.Stat(p.Delete.Path)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("  ⚠️  File already gone, skipping")
				continue
			}
			fmt.Printf("  ❌ Failed to stat: %v\n", err)
			failedCount++
			continue
		}
		fileSize := fileInfo.Size()

		if err := os.Remove(p.Delete.Path); err != nil {
			fmt.Printf("  ❌ Failed to delete: %v\n", err)
			failedCount++
			continue
		}

		if err := db.DeleteMediaFile(p.Delete.Path); err != nil {
			fmt.Printf("  ⚠️  Deleted but failed to update database: %v\n", err)
		}

		deletedCount++
		reclaimedBytes += fileSize
		fmt.Printf("  ✅ Deleted (%s reclaimed)\n", formatBytes(fileSize))
	}

	// Delete plans file on success
	if failedCount == 0 {
		if err := plans.DeleteDuplicatePlans(); err != nil {
			fmt.Printf("⚠️  Failed to clean up plans file: %v\n", err)
		}
	}

	fmt.Println("\n=== Execution Complete ===")
	fmt.Printf("✅ Successfully deleted: %d files\n", deletedCount)
	if failedCount > 0 {
		fmt.Printf("❌ Failed to delete:     %d files\n", failedCount)
	}
	fmt.Printf("💾 Space reclaimed:      %s\n", formatBytes(reclaimedBytes))

	return nil
}
```

**Step 2: Build and test**

Run: `go build ./cmd/jellywatch`
Expected: No errors

**Step 3: Commit**

```bash
git add cmd/jellywatch/duplicates_cmd.go
git commit -m "feat(duplicates): add --generate workflow with JSON plans"
```

---

## Task 4: Update consolidate command to use JSON plans

**Files:**
- Modify: `cmd/jellywatch/consolidate_cmd.go`

**Step 1: Rewrite consolidate command to use JSON plans**

Replace `cmd/jellywatch/consolidate_cmd.go` entirely:

```go
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/consolidate"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/plans"
	"github.com/Nomadcxx/jellywatch/internal/service"
	"github.com/Nomadcxx/jellywatch/internal/transfer"
	"github.com/spf13/cobra"
)

func newConsolidateCmd() *cobra.Command {
	var (
		generate bool
		dryRun   bool
		execute  bool
	)

	cmd := &cobra.Command{
		Use:   "consolidate [flags]",
		Short: "Consolidate scattered media files",
		Long: `Find and consolidate media files scattered across multiple locations.

Workflow:
  1. jellywatch consolidate              # Show scattered media analysis
  2. jellywatch consolidate --generate   # Generate consolidation plans
  3. jellywatch consolidate --dry-run    # Preview pending plans
  4. jellywatch consolidate --execute    # Execute plans (move files)

Examples:
  jellywatch consolidate              # Show what needs consolidation
  jellywatch consolidate --generate   # Generate/refresh plans
  jellywatch consolidate --dry-run    # Preview pending plans
  jellywatch consolidate --execute    # Execute pending plans
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConsolidate(generate, dryRun, execute)
		},
	}

	cmd.Flags().BoolVar(&generate, "generate", false, "Generate consolidation plans")
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "d", false, "Preview pending plans")
	cmd.Flags().BoolVar(&execute, "execute", false, "Execute pending plans")

	return cmd
}

func runConsolidate(generate, dryRun, execute bool) error {
	db, err := database.Open()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Generate plans
	if generate {
		return runConsolidateGenerate(db)
	}

	// Preview plans
	if dryRun {
		return runConsolidateDryRun()
	}

	// Execute plans
	if execute {
		return runConsolidateExecute(db)
	}

	// Default: show analysis
	return runConsolidateAnalysis(db)
}

func runConsolidateAnalysis(db *database.MediaDB) error {
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
	fmt.Println("  jellywatch consolidate --generate   # Generate consolidation plans")
	fmt.Println("  jellywatch consolidate --dry-run    # Preview plans")
	fmt.Println("  jellywatch consolidate --execute    # Execute plans")

	return nil
}

func runConsolidateGenerate(db *database.MediaDB) error {
	fmt.Println("🔍 Analyzing database for consolidation opportunities...")

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	consolidator := consolidate.NewConsolidator(db, cfg)
	generatedPlans, err := consolidator.GenerateAllPlans()
	if err != nil {
		return fmt.Errorf("failed to generate plans: %w", err)
	}

	if len(generatedPlans) == 0 {
		fmt.Println("✨ No consolidation needed - your library is already organized!")
		return nil
	}

	plan := &plans.ConsolidatePlan{
		CreatedAt: time.Now(),
		Command:   "consolidate",
		Summary:   plans.ConsolidateSummary{},
		Plans:     []plans.ConsolidateGroup{},
	}

	for _, gp := range generatedPlans {
		group := plans.ConsolidateGroup{
			ConflictID:     gp.ConflictID,
			Title:          gp.Title,
			Year:           gp.Year,
			MediaType:      gp.MediaType,
			TargetLocation: gp.TargetPath,
			Operations:     []plans.MoveOperation{},
		}

		for _, op := range gp.Operations {
			group.Operations = append(group.Operations, plans.MoveOperation{
				Action:     op.Action,
				SourcePath: op.SourcePath,
				TargetPath: op.TargetPath,
				Size:       op.Size,
			})
			plan.Summary.TotalMoves++
			plan.Summary.TotalBytes += op.Size
		}

		plan.Plans = append(plan.Plans, group)
		plan.Summary.TotalConflicts++
	}

	if err := plans.SaveConsolidatePlans(plan); err != nil {
		return fmt.Errorf("failed to save plans: %w", err)
	}

	fmt.Println("\n✅ Plans generated successfully!")
	fmt.Printf("\nConsolidation Summary:\n")
	fmt.Printf("  Conflicts found:    %d\n", plan.Summary.TotalConflicts)
	fmt.Printf("  Move operations:    %d\n", plan.Summary.TotalMoves)
	fmt.Printf("  Data to relocate:  %s\n", formatBytes(plan.Summary.TotalBytes))

	plansDir, _ := plans.GetPlansDir()
	fmt.Printf("\nPlan saved to: %s/consolidate.json\n", plansDir)

	fmt.Println("\nNext steps:")
	fmt.Println("  jellywatch consolidate --dry-run    # Preview what will happen")
	fmt.Println("  jellywatch consolidate --execute    # Execute the plans")

	return nil
}

func runConsolidateDryRun() error {
	plan, err := plans.LoadConsolidatePlans()
	if err != nil {
		return fmt.Errorf("failed to load plans: %w", err)
	}

	if plan == nil {
		fmt.Println("No pending plans found.")
		fmt.Println("Run 'jellywatch consolidate --generate' first to create plans.")
		return nil
	}

	fmt.Println("🔍 DRY RUN - No changes will be made")
	fmt.Printf("\nPlan created: %s\n", plan.CreatedAt.Format(time.RFC3339))
	fmt.Printf("Conflicts to resolve: %d\n", plan.Summary.TotalConflicts)
	fmt.Printf("Files to move: %d\n", plan.Summary.TotalMoves)
	fmt.Printf("Data to relocate: %s\n\n", formatBytes(plan.Summary.TotalBytes))

	for i, group := range plan.Plans {
		yearStr := ""
		if group.Year != nil {
			yearStr = fmt.Sprintf(" (%d)", *group.Year)
		}

		fmt.Printf("[%d] %s%s\n", i+1, group.Title, yearStr)
		fmt.Printf("    Target: %s\n", group.TargetLocation)

		for _, op := range group.Operations {
			fmt.Printf("    %s: %s\n", strings.ToUpper(op.Action), op.SourcePath)
			if op.Action == "move" {
				fmt.Printf("         → %s\n", op.TargetPath)
			}
		}
		fmt.Println()
	}

	fmt.Println("To execute these plans, run:")
	fmt.Println("  jellywatch consolidate --execute")

	return nil
}

func runConsolidateExecute(db *database.MediaDB) error {
	plan, err := plans.LoadConsolidatePlans()
	if err != nil {
		return fmt.Errorf("failed to load plans: %w", err)
	}

	if plan == nil {
		fmt.Println("No pending plans found.")
		fmt.Println("Run 'jellywatch consolidate --generate' first to create plans.")
		return nil
	}

	fmt.Printf("⚠️  This will move %d files (%s).\n", plan.Summary.TotalMoves, formatBytes(plan.Summary.TotalBytes))
	fmt.Print("Continue? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read confirmation: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response != "y" && response != "yes" {
		fmt.Println("❌ Execution cancelled.")
		return nil
	}

	fmt.Println("\n📦 Executing consolidation plans...")

	transferer, err := transfer.New(transfer.BackendRsync)
	if err != nil {
		return fmt.Errorf("failed to create transferer: %w", err)
	}

	movedCount := 0
	failedCount := 0
	movedBytes := int64(0)

	ctx := context.Background()

	for _, group := range plan.Plans {
		yearStr := ""
		if group.Year != nil {
			yearStr = fmt.Sprintf(" (%d)", *group.Year)
		}
		fmt.Printf("\n[%s] %s%s\n", group.MediaType, group.Title, yearStr)

		for _, op := range group.Operations {
			if op.Action != "move" {
				continue
			}

			fmt.Printf("  Moving: %s\n", op.SourcePath)

			// Ensure target directory exists
			targetDir := op.TargetPath[:strings.LastIndex(op.TargetPath, "/")]
			if err := os.MkdirAll(targetDir, 0755); err != nil {
				fmt.Printf("  ❌ Failed to create directory: %v\n", err)
				failedCount++
				continue
			}

			// Move file
			result, err := transferer.Transfer(ctx, op.SourcePath, op.TargetPath)
			if err != nil {
				fmt.Printf("  ❌ Failed to move: %v\n", err)
				failedCount++
				continue
			}

			// Update database
			if err := db.UpdateMediaFilePath(op.SourcePath, op.TargetPath); err != nil {
				fmt.Printf("  ⚠️  Moved but failed to update database: %v\n", err)
			}

			movedCount++
			movedBytes += result.BytesCopied
			fmt.Printf("  ✅ Moved (%s)\n", formatBytes(result.BytesCopied))
		}
	}

	// Delete plans file on success
	if failedCount == 0 {
		if err := plans.DeleteConsolidatePlans(); err != nil {
			fmt.Printf("⚠️  Failed to clean up plans file: %v\n", err)
		}
	}

	fmt.Println("\n=== Execution Complete ===")
	fmt.Printf("✅ Successfully moved: %d files\n", movedCount)
	if failedCount > 0 {
		fmt.Printf("❌ Failed to move:     %d files\n", failedCount)
	}
	fmt.Printf("📦 Data relocated:     %s\n", formatBytes(movedBytes))

	return nil
}
```

**Step 2: Build and test**

Run: `go build ./cmd/jellywatch`
Expected: No errors

**Step 3: Commit**

```bash
git add cmd/jellywatch/consolidate_cmd.go
git commit -m "feat(consolidate): switch to JSON plans, remove database table usage"
```

---

## Task 5: Remove migration 9 (duplicate_plans table)

**Files:**
- Modify: `internal/database/schema.go`

**Step 1: Remove the duplicate_plans migration we added earlier**

Remove the version 9 migration block from `internal/database/schema.go` (lines 426-472).

**Step 2: Build and test**

Run: `go build ./... && go test ./...`
Expected: All pass

**Step 3: Commit**

```bash
git add internal/database/schema.go
git commit -m "chore: remove unused duplicate_plans migration"
```

---

## Task 6: Update scan command guidance

**Files:**
- Modify: `cmd/jellywatch/scan_cmd.go`

**Step 1: Update the guidance section in scan output**

Find and update the guidance section to use the new workflow:

```go
// Show guidance
if hasDuplicates || hasScattered {
	fmt.Println("\n=== What's Next ===")

	if hasDuplicates {
		fmt.Println("\n1. Handle duplicates first (recommended):")
		fmt.Println("   jellywatch duplicates --generate   # Generate deletion plans")
		fmt.Println("   jellywatch duplicates --dry-run    # Preview plans")
		fmt.Println("   jellywatch duplicates --execute    # Execute plans")
	}

	if hasScattered {
		step := "1"
		if hasDuplicates {
			step = "2"
		}
		fmt.Printf("\n%s. Then consolidate scattered media:\n", step)
		fmt.Println("   jellywatch consolidate --generate  # Generate move plans")
		fmt.Println("   jellywatch consolidate --dry-run   # Preview plans")
		fmt.Println("   jellywatch consolidate --execute   # Execute plans")
	}

	fmt.Println("\nOr use the interactive wizard:")
	fmt.Println("   jellywatch fix                     # Guided cleanup")
}
```

**Step 2: Build**

Run: `go build ./cmd/jellywatch`
Expected: No errors

**Step 3: Commit**

```bash
git add cmd/jellywatch/scan_cmd.go
git commit -m "feat(scan): update guidance for unified workflow"
```

---

## Summary

| Task | Description | Files |
|------|-------------|-------|
| 1 | Create plans package types | `internal/plans/plans.go` |
| 2 | Add read/write functions | `internal/plans/plans.go`, `plans_test.go` |
| 3 | Update duplicates command | `cmd/jellywatch/duplicates_cmd.go` |
| 4 | Update consolidate command | `cmd/jellywatch/consolidate_cmd.go` |
| 5 | Remove unused migration | `internal/database/schema.go` |
| 6 | Update scan guidance | `cmd/jellywatch/scan_cmd.go` |

**Total: 6 tasks, 6 commits**
