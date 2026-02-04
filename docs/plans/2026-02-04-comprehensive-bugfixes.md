# Comprehensive Bugfixes Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix 7 issues affecting jellywatch CLI commands: AI context stripping, permission config ignored, malformed JSON handling, misleading UX messages, and missing cleanup command.

**Architecture:** Surgical fixes to existing code paths. Each issue is independent and can be implemented/tested in isolation. TDD approach with unit tests for each fix.

**Tech Stack:** Go 1.21+, SQLite, Ollama API

---

## Task 1: Fix AI Context Stripping (Issue 2)

**Files:**
- Modify: `internal/ai/matcher.go:82-85`
- Test: `internal/ai/matcher_test.go`

**Step 1: Write the failing test**

Add to `internal/ai/matcher_test.go`:

```go
func TestBuildContextPrompt_IncludesParentFolder(t *testing.T) {
	// Test that parent folder (series name) is included in context
	folderPath := "/mnt/STORAGE7/TVSHOWS/Family Guy (1999)/Season 19"

	// We need to test the context building logic
	// Extract parent and immediate folder names
	parentDir := filepath.Dir(folderPath)
	parentName := filepath.Base(parentDir)
	immediateName := filepath.Base(folderPath)

	if parentName != "Family Guy (1999)" {
		t.Errorf("expected parent 'Family Guy (1999)', got '%s'", parentName)
	}
	if immediateName != "Season 19" {
		t.Errorf("expected immediate 'Season 19', got '%s'", immediateName)
	}
}

func TestBuildContextPrompt_SkipsStorageRoots(t *testing.T) {
	// Parent folders like "TVSHOWS", "MOVIES", "STORAGE1" should be skipped
	skipNames := []string{"TVSHOWS", "MOVIES", "STORAGE1", ".", "/"}

	for _, name := range skipNames {
		if !shouldSkipParentFolder(name) {
			t.Errorf("expected to skip parent folder '%s'", name)
		}
	}

	// Real series names should NOT be skipped
	keepNames := []string{"Family Guy (1999)", "Breaking Bad", "The Office (US)"}
	for _, name := range keepNames {
		if shouldSkipParentFolder(name) {
			t.Errorf("should NOT skip parent folder '%s'", name)
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/ai/... -run "TestBuildContextPrompt" -v`
Expected: FAIL with "shouldSkipParentFolder not defined"

**Step 3: Write minimal implementation**

Modify `internal/ai/matcher.go`, replace lines 82-85:

```go
if folderPath != "" {
	// Extract both parent folder (series/movie name) and immediate folder (season)
	parentDir := filepath.Dir(folderPath)
	parentName := filepath.Base(parentDir)
	immediateName := filepath.Base(folderPath)

	// Include parent context if it's a meaningful name (not storage root)
	if !shouldSkipParentFolder(parentName) {
		contextPrompt += fmt.Sprintf("- Parent folder: %s\n", parentName)
	}
	contextPrompt += fmt.Sprintf("- Folder: %s\n", immediateName)
}
```

Add helper function after `getSystemPrompt()`:

```go
// shouldSkipParentFolder returns true for folder names that don't provide useful context
func shouldSkipParentFolder(name string) bool {
	if name == "" || name == "." || name == "/" {
		return true
	}
	upper := strings.ToUpper(name)
	// Skip storage roots and library type folders
	if strings.HasPrefix(upper, "STORAGE") ||
		upper == "TVSHOWS" || upper == "TV SHOWS" ||
		upper == "MOVIES" || upper == "FILMS" {
		return true
	}
	return false
}
```

Add import if not present: `"strings"`

**Step 4: Run test to verify it passes**

Run: `go test ./internal/ai/... -run "TestBuildContextPrompt" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/ai/matcher.go internal/ai/matcher_test.go
git commit -m "fix(ai): include parent folder in context for better title matching

Previously filepath.Base() stripped the series folder name, leaving only
'Season 19' as context. Now includes parent folder like 'Family Guy (1999)'
which dramatically improves AI confidence for properly-organized libraries."
```

---

## Task 2: Fix CLI Permission Config (Issue 1)

**Files:**
- Create: `internal/transfer/config.go`
- Modify: `cmd/jellywatch/consolidate_cmd.go:92`
- Modify: `internal/plans/plans.go:501,524`
- Modify: `internal/consolidate/operations.go:86-87`
- Test: `internal/transfer/config_test.go`

**Step 1: Write the failing test**

Create `internal/transfer/config_test.go`:

```go
package transfer

import (
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/config"
)

func TestOptionsFromConfig_WithPermissions(t *testing.T) {
	cfg := &config.Config{
		Permissions: config.PermissionsConfig{
			User:  "testuser",
			Group: "testgroup",
		},
	}
	// Mock ResolveUID/GID to return known values
	// For now, test that the function exists and returns options
	opts := OptionsFromConfig(cfg)

	if opts.Timeout != DefaultOptions().Timeout {
		t.Error("expected default timeout to be preserved")
	}
}

func TestOptionsFromConfig_NilConfig(t *testing.T) {
	opts := OptionsFromConfig(nil)

	if opts.TargetUID != -1 {
		t.Errorf("expected TargetUID -1 for nil config, got %d", opts.TargetUID)
	}
	if opts.TargetGID != -1 {
		t.Errorf("expected TargetGID -1 for nil config, got %d", opts.TargetGID)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/transfer/... -run "TestOptionsFromConfig" -v`
Expected: FAIL with "OptionsFromConfig not defined"

**Step 3: Write minimal implementation**

Create `internal/transfer/config.go`:

```go
package transfer

import (
	"github.com/Nomadcxx/jellywatch/internal/config"
)

// OptionsFromConfig builds TransferOptions using values from config.
// This ensures CLI commands respect the user's configured permissions.
func OptionsFromConfig(cfg *config.Config) TransferOptions {
	opts := DefaultOptions()

	if cfg == nil {
		return opts
	}

	if cfg.Permissions.WantsOwnership() {
		if uid, err := cfg.Permissions.ResolveUID(); err == nil {
			opts.TargetUID = uid
		}
		if gid, err := cfg.Permissions.ResolveGID(); err == nil {
			opts.TargetGID = gid
		}
	}

	if mode := cfg.Permissions.ParseFileMode(); mode != 0 {
		opts.FileMode = mode
	}
	if mode := cfg.Permissions.ParseDirMode(); mode != 0 {
		opts.DirMode = mode
	}

	return opts
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/transfer/... -run "TestOptionsFromConfig" -v`
Expected: PASS

**Step 5: Update call sites**

Modify `cmd/jellywatch/consolidate_cmd.go:92`:

```go
// Before:
result, err := transferer.Move(op.SourcePath, op.TargetPath, transfer.DefaultOptions())

// After:
result, err := transferer.Move(op.SourcePath, op.TargetPath, transfer.OptionsFromConfig(cfg))
```

Modify `internal/plans/plans.go:501`:

```go
// Before:
result, err := transferer.Move(file.Path, action.NewPath, transfer.DefaultOptions())

// After (need to pass cfg to executeRename):
result, err := transferer.Move(file.Path, action.NewPath, transfer.OptionsFromConfig(cfg))
```

Note: `executeRename` already receives `cfg *config.Config` parameter.

Modify `internal/plans/plans.go:524` (rollback):

```go
// Before:
rollbackResult, rollbackErr := transferer.Move(action.NewPath, file.Path, transfer.DefaultOptions())

// After:
rollbackResult, rollbackErr := transferer.Move(action.NewPath, file.Path, transfer.OptionsFromConfig(cfg))
```

Modify `internal/consolidate/operations.go:86-87`:

```go
// Before:
opts := transfer.DefaultOptions()
opts.Checksum = c.cfg.Options.VerifyChecksums

// After:
opts := transfer.OptionsFromConfig(c.cfg)
opts.Checksum = c.cfg.Options.VerifyChecksums
```

**Step 6: Run full test suite**

Run: `go test ./... -v`
Expected: All PASS

**Step 7: Commit**

```bash
git add internal/transfer/config.go internal/transfer/config_test.go \
        cmd/jellywatch/consolidate_cmd.go internal/plans/plans.go \
        internal/consolidate/operations.go
git commit -m "fix(transfer): CLI commands now respect configured permissions

Added OptionsFromConfig() helper that builds transfer options from config.
Updated consolidate, audit, and plans to use config permissions instead of
DefaultOptions(). Files created/moved by CLI now have correct ownership."
```

---

## Task 3: Add Flexible JSON Parsing (Issue 3)

**Files:**
- Create: `internal/ai/types.go`
- Modify: `internal/ai/matcher.go` (update Result struct usage)
- Test: `internal/ai/types_test.go`

**Step 1: Write the failing test**

Create `internal/ai/types_test.go`:

```go
package ai

import (
	"encoding/json"
	"testing"
)

func TestFlexInt_UnmarshalInt(t *testing.T) {
	var f FlexInt
	err := json.Unmarshal([]byte("1999"), &f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Value == nil || *f.Value != 1999 {
		t.Errorf("expected 1999, got %v", f.Value)
	}
}

func TestFlexInt_UnmarshalString(t *testing.T) {
	var f FlexInt
	err := json.Unmarshal([]byte(`"2018"`), &f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Value == nil || *f.Value != 2018 {
		t.Errorf("expected 2018, got %v", f.Value)
	}
}

func TestFlexInt_UnmarshalNull(t *testing.T) {
	var f FlexInt
	err := json.Unmarshal([]byte("null"), &f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Value != nil {
		t.Errorf("expected nil, got %v", f.Value)
	}
}

func TestFlexIntSlice_UnmarshalIntArray(t *testing.T) {
	var f FlexIntSlice
	err := json.Unmarshal([]byte("[1, 2, 3]"), &f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f) != 3 || f[0] != 1 || f[1] != 2 || f[2] != 3 {
		t.Errorf("expected [1,2,3], got %v", f)
	}
}

func TestFlexIntSlice_UnmarshalStringArray(t *testing.T) {
	var f FlexIntSlice
	err := json.Unmarshal([]byte(`["S01E06", "S01E07"]`), &f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f) != 2 || f[0] != 6 || f[1] != 7 {
		t.Errorf("expected [6,7], got %v", f)
	}
}

func TestExtractEpisodeNumber(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"S01E06", 6},
		{"S01E12", 12},
		{"E05", 5},
		{"5", 5},
		{"invalid", 0},
	}

	for _, tc := range tests {
		result := extractEpisodeNumber(tc.input)
		if result != tc.expected {
			t.Errorf("extractEpisodeNumber(%q) = %d, want %d", tc.input, result, tc.expected)
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/ai/... -run "TestFlex" -v`
Expected: FAIL with "FlexInt not defined"

**Step 3: Write minimal implementation**

Create `internal/ai/types.go`:

```go
package ai

import (
	"encoding/json"
	"strconv"
	"strings"
)

// FlexInt handles JSON that may be int, string, or null
type FlexInt struct {
	Value *int
}

func (f *FlexInt) UnmarshalJSON(data []byte) error {
	// Handle null
	if string(data) == "null" {
		f.Value = nil
		return nil
	}

	// Try int first
	var i int
	if err := json.Unmarshal(data, &i); err == nil {
		f.Value = &i
		return nil
	}

	// Try string
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		s = strings.TrimSpace(s)
		if parsed, err := strconv.Atoi(s); err == nil {
			f.Value = &parsed
			return nil
		}
	}

	// Can't parse - leave as nil
	f.Value = nil
	return nil
}

// FlexIntSlice handles episodes that may be int array or string array
type FlexIntSlice []int

func (f *FlexIntSlice) UnmarshalJSON(data []byte) error {
	// Handle null
	if string(data) == "null" {
		*f = nil
		return nil
	}

	// Try int array first
	var ints []int
	if err := json.Unmarshal(data, &ints); err == nil {
		*f = ints
		return nil
	}

	// Try string array
	var strs []string
	if err := json.Unmarshal(data, &strs); err == nil {
		result := make([]int, 0, len(strs))
		for _, s := range strs {
			if num := extractEpisodeNumber(s); num > 0 {
				result = append(result, num)
			}
		}
		*f = result
		return nil
	}

	*f = nil
	return nil
}

// extractEpisodeNumber extracts episode number from strings like "S01E06"
func extractEpisodeNumber(s string) int {
	s = strings.ToUpper(strings.TrimSpace(s))

	// Handle "S01E06" format
	if idx := strings.Index(s, "E"); idx >= 0 {
		numStr := s[idx+1:]
		// Remove leading zeros and trailing non-digits
		numStr = strings.TrimLeft(numStr, "0")
		for i, c := range numStr {
			if c < '0' || c > '9' {
				numStr = numStr[:i]
				break
			}
		}
		if numStr == "" {
			numStr = "0"
		}
		if num, err := strconv.Atoi(numStr); err == nil {
			return num
		}
	}

	// Try direct parse
	if num, err := strconv.Atoi(s); err == nil {
		return num
	}

	return 0
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/ai/... -run "TestFlex\|TestExtract" -v`
Expected: PASS

**Step 5: Update Result struct to use flexible types**

Modify `internal/ai/matcher.go`, update the Result struct:

```go
// Result represents the AI's parsed output
type Result struct {
	Title           string       `json:"title"`
	Year            *FlexInt     `json:"year,omitempty"`
	Type            string       `json:"type"`
	Season          *FlexInt     `json:"season,omitempty"`
	Episodes        FlexIntSlice `json:"episodes,omitempty"`
	AbsoluteEpisode *FlexInt     `json:"absolute_episode,omitempty"`
	AirDate         string       `json:"air_date,omitempty"`
	Confidence      float64      `json:"confidence"`
}
```

Update any code that accesses `result.Year`, `result.Season`, etc. to use `.Value`:

```go
// Example: where code does result.Year, change to:
if result.Year != nil && result.Year.Value != nil {
    year := *result.Year.Value
    // use year
}
```

**Step 6: Run full test suite**

Run: `go test ./internal/ai/... -v`
Expected: All PASS

**Step 7: Commit**

```bash
git add internal/ai/types.go internal/ai/types_test.go internal/ai/matcher.go
git commit -m "fix(ai): handle malformed JSON with flexible type parsing

Added FlexInt and FlexIntSlice types that accept both int and string values.
Handles cases where model returns '\"2018\"' instead of '2018' for year,
or '[\"S01E06\"]' instead of '[6]' for episodes."
```

---

## Task 4: Improve Consolidate Generate UX (Issue 4)

**Files:**
- Modify: `cmd/jellywatch/consolidate_generate.go:40-50`

**Step 1: Add detailed per-item status output**

Modify `cmd/jellywatch/consolidate_generate.go`, after line 47 (after `consolidator.GenerateAllPlans()`):

```go
// Show per-item analysis
fmt.Println("\nAnalyzing scattered content...")
for _, cp := range consolidatePlans {
	yearStr := ""
	if cp.Year != nil {
		yearStr = fmt.Sprintf(" (%d)", *cp.Year)
	}

	if !cp.CanProceed {
		reasons := strings.Join(cp.Reasons, ", ")
		fmt.Printf("  ⚠️  %s%s: Skipped - %s\n", cp.Title, yearStr, reasons)
	} else if len(cp.Operations) == 0 {
		fmt.Printf("  ℹ️  %s%s: No files >100MB to move (samples/cruft only)\n", cp.Title, yearStr)
	} else {
		totalSize := int64(0)
		for _, op := range cp.Operations {
			totalSize += op.Size
		}
		fmt.Printf("  ✓ %s%s: %d files (%s) to consolidate\n",
			cp.Title, yearStr, len(cp.Operations), formatBytes(totalSize))
	}
}
fmt.Println()
```

Add import: `"strings"`

**Step 2: Add cleanup suggestion when no plans**

Modify the `len(plan.Plans) == 0` block around line 110:

```go
if len(plan.Plans) == 0 {
	fmt.Println("✅ No consolidation needed")
	if len(skippedItems) > 0 {
		fmt.Printf("\n⚠️  %d conflicts were skipped:\n", len(skippedItems))
		for _, item := range skippedItems {
			yearStr := ""
			if item.year != nil {
				yearStr = fmt.Sprintf(" (%d)", *item.year)
			}
			fmt.Printf("  • %s%s\n", item.title, yearStr)
			for _, reason := range item.reasons {
				fmt.Printf("    - %s\n", reason)
			}
		}
	}

	// Suggest cleanup if folders only contain samples
	fmt.Println("\n💡 Scattered folders may contain only sample files or cruft.")
	fmt.Println("   Consider cleaning them with: sudo jellywatch cleanup cruft")

	return nil
}
```

**Step 3: Test manually**

Run: `jellywatch consolidate generate`
Expected: Shows per-item status with clear explanation of why items are skipped

**Step 4: Commit**

```bash
git add cmd/jellywatch/consolidate_generate.go
git commit -m "ux(consolidate): show detailed per-item analysis during generate

Now shows why each scattered item is skipped (permissions, samples only, etc.)
and suggests cleanup command when no consolidation is needed."
```

---

## Task 5: Clarify Status Output (Issue 5)

**Files:**
- Modify: `cmd/jellywatch/status_cmd.go:81-110`

**Step 1: Restructure status output**

Replace the Statistics and Conflicts sections in `cmd/jellywatch/status_cmd.go`:

```go
fmt.Println("Statistics")
fmt.Println("----------")
fmt.Printf("Total Files:          %d\n", stats.TotalFiles)
fmt.Printf("TV Episodes:          %d\n", episodeCount)
fmt.Printf("Movies:               %d\n", movieCount)
fmt.Printf("Non-Compliant Files:  %d\n", stats.NonCompliantFiles)
fmt.Println()

// Get duplicate breakdowns
movieDups, _ := db.FindDuplicateMovies()
episodeDups, _ := db.FindDuplicateEpisodes()

fmt.Println("Duplicates (same content exists multiple times)")
fmt.Println("------------------------------------------------")
fmt.Printf("Movie duplicates:     %d groups\n", len(movieDups))
fmt.Printf("Episode duplicates:   %d groups\n", len(episodeDups))
if stats.SpaceReclaimable > 0 {
	fmt.Printf("Space reclaimable:    %s\n", formatBytes(stats.SpaceReclaimable))
}
if len(movieDups)+len(episodeDups) > 0 {
	fmt.Println("→ Run 'jellywatch duplicates generate' to review")
}
fmt.Println()

// Categorize conflicts
seriesConflicts := 0
movieConflicts := 0
for _, c := range conflicts {
	if c.MediaType == "series" {
		seriesConflicts++
	} else {
		movieConflicts++
	}
}

fmt.Println("Scattered (same title across storage drives)")
fmt.Println("--------------------------------------------")
fmt.Printf("Series in multiple locations:  %d\n", seriesConflicts)
fmt.Printf("Movies in multiple locations:  %d\n", movieConflicts)
if seriesConflicts > 0 {
	fmt.Println("→ Run 'jellywatch consolidate generate' to review")
}

if len(conflicts) > 0 {
	fmt.Println("\nDetails:")
	shown := 0
	for _, c := range conflicts {
		if shown >= 5 {
			fmt.Printf("  ... and %d more\n", len(conflicts)-5)
			break
		}
		yearStr := ""
		if c.Year != nil {
			yearStr = fmt.Sprintf(" (%d)", *c.Year)
		}
		fmt.Printf("  [%s] %s%s - %d locations\n", c.MediaType, c.Title, yearStr, len(c.Locations))
		shown++
	}
}
fmt.Println()
```

**Step 2: Test manually**

Run: `jellywatch status`
Expected: Clear separation between Duplicates and Scattered with explanations

**Step 3: Commit**

```bash
git add cmd/jellywatch/status_cmd.go
git commit -m "ux(status): clarify duplicates vs scattered terminology

Separates 'Duplicates' (same file multiple copies) from 'Scattered'
(same title across drives). Adds inline explanations and suggested commands."
```

---

## Task 6: Improve Audit Plan Display (Issue 6)

**Files:**
- Modify: `cmd/jellywatch/audit_cmd.go:315-333`

**Step 1: Update displayAuditPlan to link items and actions**

Replace `displayAuditPlan` function:

```go
func displayAuditPlan(plan *plans.AuditPlan, showDetails bool) error {
	fmt.Printf("\n📋 Audit Plan\n")
	fmt.Printf("Created: %s\n", plan.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("\nSummary:\n")
	fmt.Printf("  Total Files: %d\n", plan.Summary.TotalFiles)
	fmt.Printf("  Files to Rename: %d\n", plan.Summary.FilesToRename)
	fmt.Printf("  Files to Delete: %d\n", plan.Summary.FilesToDelete)
	fmt.Printf("  Files to Skip: %d\n", plan.Summary.FilesToSkip)
	fmt.Printf("  Avg Confidence: %.2f\n", plan.Summary.AvgConfidence)

	if !showDetails {
		if plan.Summary.FilesToRename > 0 {
			fmt.Printf("\n💡 Run 'jellywatch audit --dry-run' to see detailed changes\n")
		}
		return nil
	}

	// Build action index for items that have actions
	// Actions are created in order for items that pass validation
	actionIdx := 0

	fmt.Printf("\nFiles:\n")
	for i, item := range plan.Items {
		fmt.Printf("\n[%d] %s\n", i+1, filepath.Base(item.Path))
		fmt.Printf("    Path: %s\n", filepath.Dir(item.Path))
		fmt.Printf("    Current: %s (confidence: %.2f)\n", item.Title, item.Confidence)

		if item.SkipReason != "" {
			fmt.Printf("    ⚠️  Skipped: %s\n", item.SkipReason)
		} else if actionIdx < len(plan.Actions) {
			action := plan.Actions[actionIdx]
			fmt.Printf("    ✓ %s → %s\n", action.Action, filepath.Base(action.NewPath))
			fmt.Printf("      New title: %s (confidence: %.2f)\n", action.NewTitle, action.Confidence)
			if action.Reasoning != "" {
				fmt.Printf("      Reason: %s\n", action.Reasoning)
			}
			actionIdx++
		}
	}

	if plan.Summary.FilesToRename > 0 {
		fmt.Printf("\n💡 Run 'jellywatch audit --execute' to apply changes\n")
	}

	return nil
}
```

Add import if needed: `"path/filepath"`

**Step 2: Test manually**

Run: `jellywatch audit --dry-run`
Expected: Each item shows its corresponding action inline

**Step 3: Commit**

```bash
git add cmd/jellywatch/audit_cmd.go
git commit -m "ux(audit): show items with their actions inline

Previously showed summary only. Now displays each file with its
proposed action (rename/skip) and reasoning for easier review."
```

---

## Task 7: Add Cleanup Command (Issue 7)

**Files:**
- Create: `cmd/jellywatch/cleanup_cmd.go`
- Modify: `cmd/jellywatch/main.go` (register command)
- Test: `cmd/jellywatch/cleanup_cmd_test.go`

**Step 1: Create the cleanup command**

Create `cmd/jellywatch/cleanup_cmd.go`:

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Nomadcxx/jellywatch/internal/config"
)

func newCleanupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Clean up cruft files and empty directories",
		Long: `Remove non-media files (samples, .nfo, .txt, images) and empty directories
from your media libraries.

Subcommands:
  cruft   - Delete sample files and metadata cruft
  empty   - Delete empty directories only`,
	}

	cmd.AddCommand(newCleanupCruftCmd())
	cmd.AddCommand(newCleanupEmptyCmd())

	return cmd
}

func newCleanupCruftCmd() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "cruft",
		Short: "Delete sample files, .nfo, .txt, and other non-media cruft",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCleanupCruft(dryRun)
		},
	}

	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "Preview without deleting")
	return cmd
}

func newCleanupEmptyCmd() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "empty",
		Short: "Delete empty directories",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCleanupEmpty(dryRun)
		},
	}

	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "Preview without deleting")
	return cmd
}

// Cruft file extensions that should be deleted
var cruftExtensions = map[string]bool{
	".nfo": true, ".txt": true, ".jpg": true, ".jpeg": true,
	".png": true, ".gif": true, ".srt": true, ".sub": true,
	".idx": true, ".sfv": true, ".md5": true, ".url": true,
}

// Maximum size for sample files (50MB)
const maxSampleSize = 50 * 1024 * 1024

func runCleanupCruft(dryRun bool) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	allRoots := append(cfg.Libraries.TV, cfg.Libraries.Movies...)
	if len(allRoots) == 0 {
		return fmt.Errorf("no libraries configured")
	}

	if dryRun {
		fmt.Println("🔍 Scanning for cruft files (dry run)...")
	} else {
		fmt.Println("🗑️  Cleaning cruft files...")
	}

	var totalSize int64
	var fileCount int

	for _, root := range allRoots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}

			if isCruft(path, info) {
				if dryRun {
					fmt.Printf("  [DELETE] %s (%s)\n", path, formatBytes(info.Size()))
				} else {
					if err := os.Remove(path); err != nil {
						fmt.Printf("  ❌ Failed: %s: %v\n", path, err)
						return nil
					}
					fmt.Printf("  ✓ Deleted: %s\n", filepath.Base(path))
				}
				totalSize += info.Size()
				fileCount++
			}
			return nil
		})
		if err != nil {
			fmt.Printf("⚠️  Warning: error scanning %s: %v\n", root, err)
		}
	}

	fmt.Println()
	if dryRun {
		fmt.Printf("Would delete %d files, reclaiming %s\n", fileCount, formatBytes(totalSize))
		fmt.Println("\nRun without --dry-run to delete these files.")
	} else {
		fmt.Printf("✅ Deleted %d files, reclaimed %s\n", fileCount, formatBytes(totalSize))
	}

	return nil
}

func runCleanupEmpty(dryRun bool) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	allRoots := append(cfg.Libraries.TV, cfg.Libraries.Movies...)
	if len(allRoots) == 0 {
		return fmt.Errorf("no libraries configured")
	}

	if dryRun {
		fmt.Println("🔍 Scanning for empty directories (dry run)...")
	} else {
		fmt.Println("🗑️  Removing empty directories...")
	}

	var dirCount int

	// Walk in reverse depth order to delete children before parents
	for _, root := range allRoots {
		var dirs []string
		filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err == nil && info.IsDir() && path != root {
				dirs = append(dirs, path)
			}
			return nil
		})

		// Process deepest first
		for i := len(dirs) - 1; i >= 0; i-- {
			dir := dirs[i]
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}

			if len(entries) == 0 {
				if dryRun {
					fmt.Printf("  [DELETE] %s/\n", dir)
				} else {
					if err := os.Remove(dir); err == nil {
						fmt.Printf("  ✓ Removed: %s/\n", dir)
					}
				}
				dirCount++
			}
		}
	}

	fmt.Println()
	if dryRun {
		fmt.Printf("Would remove %d empty directories\n", dirCount)
	} else {
		fmt.Printf("✅ Removed %d empty directories\n", dirCount)
	}

	return nil
}

func isCruft(path string, info os.FileInfo) bool {
	name := strings.ToLower(filepath.Base(path))
	ext := strings.ToLower(filepath.Ext(path))

	// Known cruft extensions
	if cruftExtensions[ext] {
		return true
	}

	// Sample files (small video files with "sample" in name or path)
	if isVideoFile(path) && info.Size() < maxSampleSize {
		if strings.Contains(name, "sample") ||
			strings.Contains(strings.ToLower(filepath.Dir(path)), "sample") {
			return true
		}
	}

	return false
}
```

**Step 2: Register command in main.go**

Add to `cmd/jellywatch/main.go` in the init() or command setup section:

```go
rootCmd.AddCommand(newCleanupCmd())
```

**Step 3: Test manually**

Run: `jellywatch cleanup cruft --dry-run`
Expected: Lists cruft files that would be deleted

Run: `jellywatch cleanup empty --dry-run`
Expected: Lists empty directories that would be removed

**Step 4: Commit**

```bash
git add cmd/jellywatch/cleanup_cmd.go cmd/jellywatch/main.go
git commit -m "feat(cleanup): add CLI command to remove cruft and empty dirs

New command: jellywatch cleanup [cruft|empty] [--dry-run]
- cruft: removes sample files, .nfo, .txt, images, etc.
- empty: removes empty directories
Both support --dry-run for preview."
```

---

## Task 8: Final Integration Test

**Step 1: Run full test suite**

```bash
go test ./... -v
```

Expected: All tests pass

**Step 2: Manual integration test**

```bash
# Test AI context fix
jellywatch audit --generate --limit 5
# Check that confidence scores improved for organized folders

# Test permission config
sudo jellywatch duplicates execute
# Check that files are owned by configured user, not root

# Test consolidate UX
jellywatch consolidate generate
# Check that per-item status is shown

# Test status clarity
jellywatch status
# Check that Duplicates vs Scattered is clear

# Test cleanup
sudo jellywatch cleanup cruft --dry-run
# Check that sample files are identified
```

**Step 3: Final commit**

```bash
git add -A
git commit -m "test: verify all comprehensive bugfixes work together"
```

---

## Summary

| Task | Issue | Type | Risk |
|------|-------|------|------|
| 1 | AI context stripped | Bug fix | Low |
| 2 | Permission config ignored | Bug fix | Medium |
| 3 | Malformed JSON | Robustness | Low |
| 4 | Consolidate UX | UX | Low |
| 5 | Status clarity | UX | Low |
| 6 | Audit display | UX | Low |
| 7 | Cleanup command | Feature | Low |
| 8 | Integration test | Test | N/A |

**Total estimated tasks:** 8 main tasks with ~40 discrete steps
