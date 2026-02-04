# Consolidation Fixes Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix consolidation bugs: empty folder cleanup, auto-insert moved files, and proper plan lifecycle management.

**Architecture:** Add helper functions to consolidate command for cruft cleanup and auto-insert. Add archive functions to plans package. Modify execute flow to track "already moved" separately from failures.

**Tech Stack:** Go, filepath.Walk, os package

---

## Task 1: Add Archive Functions to Plans Package

**Files:**
- Modify: `internal/plans/plans.go:223-235` (after DeleteConsolidatePlans)
- Test: `internal/plans/plans_test.go`

**Step 1: Write the failing test for ArchiveConsolidatePlans**

Add to `internal/plans/plans_test.go`:

```go
func TestArchiveConsolidatePlans(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	// Create a plan first
	plan := &ConsolidatePlan{
		CreatedAt: time.Now(),
		Command:   "consolidate",
		Summary:   ConsolidateSummary{TotalConflicts: 1},
		Plans:     []ConsolidateGroup{},
	}

	err := SaveConsolidatePlans(plan)
	if err != nil {
		t.Fatalf("SaveConsolidatePlans failed: %v", err)
	}

	// Archive it
	err = ArchiveConsolidatePlans()
	if err != nil {
		t.Fatalf("ArchiveConsolidatePlans failed: %v", err)
	}

	// Original file should be gone
	path, _ := getConsolidatePlansPath()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("Original plan file should be gone after archive")
	}

	// .old file should exist
	oldPath := path + ".old"
	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		t.Fatal("Archived .old file should exist")
	}
}

func TestArchiveDuplicatePlans(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	year := 2005
	plan := &DuplicatePlan{
		CreatedAt: time.Now(),
		Command:   "duplicates",
		Summary:   DuplicateSummary{TotalGroups: 1},
		Plans: []DuplicateGroup{
			{GroupID: "test", Title: "Test", Year: &year, MediaType: "movie"},
		},
	}

	err := SaveDuplicatePlans(plan)
	if err != nil {
		t.Fatalf("SaveDuplicatePlans failed: %v", err)
	}

	err = ArchiveDuplicatePlans()
	if err != nil {
		t.Fatalf("ArchiveDuplicatePlans failed: %v", err)
	}

	path, _ := getDuplicatePlansPath()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("Original plan file should be gone")
	}

	oldPath := path + ".old"
	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		t.Fatal("Archived .old file should exist")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/plans/... -v -run "TestArchive"`
Expected: FAIL with "undefined: ArchiveConsolidatePlans"

**Step 3: Implement the archive functions**

Add to `internal/plans/plans.go` after `DeleteConsolidatePlans`:

```go
// ArchiveConsolidatePlans renames consolidate.json to consolidate.json.old
func ArchiveConsolidatePlans() error {
	path, err := getConsolidatePlansPath()
	if err != nil {
		return err
	}

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil // Nothing to archive
	}

	oldPath := path + ".old"

	// Remove old archive if exists
	os.Remove(oldPath)

	// Rename current to .old
	if err := os.Rename(path, oldPath); err != nil {
		return fmt.Errorf("failed to archive plan file: %w", err)
	}

	return nil
}

// ArchiveDuplicatePlans renames duplicates.json to duplicates.json.old
func ArchiveDuplicatePlans() error {
	path, err := getDuplicatePlansPath()
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

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/plans/... -v -run "TestArchive"`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/plans/plans.go internal/plans/plans_test.go
git commit -m "feat(plans): add archive functions for plan lifecycle"
```

---

## Task 2: Add Video Extensions Helper

**Files:**
- Create: `cmd/jellywatch/cleanup.go`
- Test: `cmd/jellywatch/cleanup_test.go`

**Step 1: Write the failing test**

Create `cmd/jellywatch/cleanup_test.go`:

```go
package main

import (
	"testing"
)

func TestIsVideoFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/path/to/movie.mkv", true},
		{"/path/to/movie.mp4", true},
		{"/path/to/movie.avi", true},
		{"/path/to/movie.MKV", true}, // Case insensitive
		{"/path/to/movie.nfo", false},
		{"/path/to/movie.jpg", false},
		{"/path/to/movie.srt", false},
		{"/path/to/movie.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := isVideoFile(tt.path)
			if result != tt.expected {
				t.Errorf("isVideoFile(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./cmd/jellywatch/... -v -run "TestIsVideoFile"`
Expected: FAIL with "undefined: isVideoFile"

**Step 3: Implement the helper**

Create `cmd/jellywatch/cleanup.go`:

```go
package main

import (
	"path/filepath"
	"strings"
)

// videoExtensions defines video file extensions
var videoExtensions = map[string]bool{
	".mkv":  true,
	".mp4":  true,
	".avi":  true,
	".m4v":  true,
	".ts":   true,
	".wmv":  true,
	".mov":  true,
	".m2ts": true,
	".webm": true,
}

// isVideoFile checks if a path is a video file by extension
func isVideoFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return videoExtensions[ext]
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./cmd/jellywatch/... -v -run "TestIsVideoFile"`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/jellywatch/cleanup.go cmd/jellywatch/cleanup_test.go
git commit -m "feat(cleanup): add video extension helper"
```

---

## Task 3: Add Library Root Check Helpers

**Files:**
- Modify: `cmd/jellywatch/cleanup.go`
- Modify: `cmd/jellywatch/cleanup_test.go`

**Step 1: Write the failing tests**

Add to `cmd/jellywatch/cleanup_test.go`:

```go
func TestIsLibraryRoot(t *testing.T) {
	roots := []string{
		"/mnt/STORAGE1/TVSHOWS",
		"/mnt/STORAGE2/MOVIES",
	}

	tests := []struct {
		path     string
		expected bool
	}{
		{"/mnt/STORAGE1/TVSHOWS", true},
		{"/mnt/STORAGE2/MOVIES", true},
		{"/mnt/STORAGE1/TVSHOWS/Show Name", false},
		{"/mnt/STORAGE3/OTHER", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := isLibraryRoot(tt.path, roots)
			if result != tt.expected {
				t.Errorf("isLibraryRoot(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestIsInsideLibrary(t *testing.T) {
	roots := []string{
		"/mnt/STORAGE1/TVSHOWS",
		"/mnt/STORAGE2/MOVIES",
	}

	tests := []struct {
		path     string
		expected bool
	}{
		{"/mnt/STORAGE1/TVSHOWS/Show/Season 01", true},
		{"/mnt/STORAGE2/MOVIES/Movie (2020)", true},
		{"/mnt/STORAGE1/TVSHOWS", true}, // Root itself is inside
		{"/mnt/STORAGE3/OTHER/file.mkv", false},
		{"/home/user/file.mkv", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := isInsideLibrary(tt.path, roots)
			if result != tt.expected {
				t.Errorf("isInsideLibrary(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./cmd/jellywatch/... -v -run "TestIsLibraryRoot|TestIsInsideLibrary"`
Expected: FAIL with "undefined"

**Step 3: Implement the helpers**

Add to `cmd/jellywatch/cleanup.go`:

```go
// isLibraryRoot checks if path is exactly a library root
func isLibraryRoot(path string, libraryRoots []string) bool {
	cleanPath := filepath.Clean(path)
	for _, root := range libraryRoots {
		if filepath.Clean(root) == cleanPath {
			return true
		}
	}
	return false
}

// isInsideLibrary checks if path is inside any library root
func isInsideLibrary(path string, libraryRoots []string) bool {
	cleanPath := filepath.Clean(path)
	for _, root := range libraryRoots {
		cleanRoot := filepath.Clean(root)
		if strings.HasPrefix(cleanPath, cleanRoot) {
			return true
		}
	}
	return false
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./cmd/jellywatch/... -v -run "TestIsLibraryRoot|TestIsInsideLibrary"`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/jellywatch/cleanup.go cmd/jellywatch/cleanup_test.go
git commit -m "feat(cleanup): add library root validation helpers"
```

---

## Task 4: Add Cruft Cleanup Function

**Files:**
- Modify: `cmd/jellywatch/cleanup.go`
- Modify: `cmd/jellywatch/cleanup_test.go`

**Step 1: Write the failing test**

Add to `cmd/jellywatch/cleanup_test.go`:

```go
import (
	"os"
	"path/filepath"
)

func TestCleanupSourceDir_DeletesCruftAndEmptyDirs(t *testing.T) {
	// Create temp directory structure
	tempDir := t.TempDir()
	libraryRoot := filepath.Join(tempDir, "TVSHOWS")
	showDir := filepath.Join(libraryRoot, "Show (2020)")
	seasonDir := filepath.Join(showDir, "Season 01")
	episodeDir := filepath.Join(seasonDir, "Episode.Folder")

	os.MkdirAll(episodeDir, 0755)

	// Create cruft files (no video files)
	os.WriteFile(filepath.Join(episodeDir, "movie.nfo"), []byte("nfo"), 0644)
	os.WriteFile(filepath.Join(episodeDir, "cover.jpg"), []byte("jpg"), 0644)

	libraryRoots := []string{libraryRoot}

	// Cleanup should delete cruft and empty dirs
	err := cleanupSourceDir(episodeDir, libraryRoots)
	if err != nil {
		t.Fatalf("cleanupSourceDir failed: %v", err)
	}

	// Episode dir should be gone
	if _, err := os.Stat(episodeDir); !os.IsNotExist(err) {
		t.Error("Episode dir should be deleted")
	}

	// Season dir should be gone (was empty after episode dir deleted)
	if _, err := os.Stat(seasonDir); !os.IsNotExist(err) {
		t.Error("Season dir should be deleted")
	}

	// Show dir should be gone
	if _, err := os.Stat(showDir); !os.IsNotExist(err) {
		t.Error("Show dir should be deleted")
	}

	// Library root should still exist
	if _, err := os.Stat(libraryRoot); os.IsNotExist(err) {
		t.Error("Library root should NOT be deleted")
	}
}

func TestCleanupSourceDir_PreservesVideoFiles(t *testing.T) {
	tempDir := t.TempDir()
	libraryRoot := filepath.Join(tempDir, "TVSHOWS")
	showDir := filepath.Join(libraryRoot, "Show (2020)")
	seasonDir := filepath.Join(showDir, "Season 01")

	os.MkdirAll(seasonDir, 0755)

	// Create a video file that should be preserved
	os.WriteFile(filepath.Join(seasonDir, "episode.mkv"), []byte("video"), 0644)
	os.WriteFile(filepath.Join(seasonDir, "episode.nfo"), []byte("nfo"), 0644)

	libraryRoots := []string{libraryRoot}

	err := cleanupSourceDir(seasonDir, libraryRoots)
	if err != nil {
		t.Fatalf("cleanupSourceDir failed: %v", err)
	}

	// Season dir should still exist (has video file)
	if _, err := os.Stat(seasonDir); os.IsNotExist(err) {
		t.Error("Season dir should NOT be deleted - contains video file")
	}

	// Video file should still exist
	if _, err := os.Stat(filepath.Join(seasonDir, "episode.mkv")); os.IsNotExist(err) {
		t.Error("Video file should NOT be deleted")
	}

	// NFO file should still exist (we only delete cruft when NO videos remain)
	if _, err := os.Stat(filepath.Join(seasonDir, "episode.nfo")); os.IsNotExist(err) {
		t.Error("NFO should NOT be deleted when video exists")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./cmd/jellywatch/... -v -run "TestCleanupSourceDir"`
Expected: FAIL with "undefined: cleanupSourceDir"

**Step 3: Implement the function**

Add to `cmd/jellywatch/cleanup.go`:

```go
import (
	"os"
)

// cleanupSourceDir removes cruft files and empty directories after a move.
// It only deletes cruft if NO video files remain in the directory.
// It walks up the directory tree, deleting empty dirs until hitting a library root.
func cleanupSourceDir(dir string, libraryRoots []string) error {
	currentDir := filepath.Clean(dir)

	for {
		// Safety: must be inside a known library
		if !isInsideLibrary(currentDir, libraryRoots) {
			return nil
		}

		// Safety: never delete a library root
		if isLibraryRoot(currentDir, libraryRoots) {
			return nil
		}

		// Read directory contents
		entries, err := os.ReadDir(currentDir)
		if err != nil {
			return nil // Can't read, stop safely
		}

		// Check if any video files remain
		hasVideo := false
		for _, entry := range entries {
			if !entry.IsDir() && isVideoFile(entry.Name()) {
				hasVideo = true
				break
			}
		}

		// If video files exist, stop - don't delete anything
		if hasVideo {
			return nil
		}

		// No video files - delete all cruft files
		for _, entry := range entries {
			entryPath := filepath.Join(currentDir, entry.Name())
			if entry.IsDir() {
				// Recursively clean subdirectories first
				cleanupSourceDir(entryPath, libraryRoots)
			}
			// Remove the entry (file or now-empty dir)
			os.RemoveAll(entryPath)
		}

		// Now directory should be empty, remove it
		// os.Remove fails if not empty - that's our safety net
		if err := os.Remove(currentDir); err != nil {
			return nil // Not empty or error, stop
		}

		// Move up one level
		currentDir = filepath.Dir(currentDir)
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./cmd/jellywatch/... -v -run "TestCleanupSourceDir"`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/jellywatch/cleanup.go cmd/jellywatch/cleanup_test.go
git commit -m "feat(cleanup): add smart cruft cleanup function"
```

---

## Task 5: Update Consolidate Execute - Track Already Moved

**Files:**
- Modify: `cmd/jellywatch/consolidate_cmd.go:227-330` (runConsolidateExecute function)

**Step 1: Read current implementation**

Run: Review lines 227-330 of consolidate_cmd.go (already read)

**Step 2: Modify runConsolidateExecute to track already-moved files**

Replace the execution loop in `runConsolidateExecute` with:

```go
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

	// Load config to get library roots for cleanup
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	allLibraryRoots := append(cfg.Libraries.TV, cfg.Libraries.Movies...)

	transferer, err := transfer.New(transfer.BackendRsync)
	if err != nil {
		return fmt.Errorf("failed to create transferer: %w", err)
	}

	movedCount := 0
	alreadyGoneCount := 0
	failedCount := 0
	movedBytes := int64(0)

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

			// Check if source exists BEFORE attempting move
			if _, err := os.Stat(op.SourcePath); os.IsNotExist(err) {
				fmt.Printf("  ⏭️  Already moved: %s\n", filepath.Base(op.SourcePath))
				alreadyGoneCount++
				continue
			}

			fmt.Printf("  Moving: %s\n", op.SourcePath)

			// Ensure target directory exists
			targetDir := filepath.Dir(op.TargetPath)
			if err := os.MkdirAll(targetDir, 0755); err != nil {
				fmt.Printf("  ❌ Failed to create directory: %v\n", err)
				failedCount++
				continue
			}

			// Move file
			result, err := transferer.Move(op.SourcePath, op.TargetPath, transfer.DefaultOptions())
			if err != nil {
				fmt.Printf("  ❌ Failed to move: %v\n", err)
				failedCount++
				continue
			}

			// Update database
			if err := updateDatabaseAfterMove(db, op.SourcePath, op.TargetPath, op.Size); err != nil {
				fmt.Printf("  ⚠️  Moved but database update issue: %v\n", err)
			}

			// Cleanup source directory (delete cruft and empty dirs)
			sourceDir := filepath.Dir(op.SourcePath)
			if err := cleanupSourceDir(sourceDir, allLibraryRoots); err != nil {
				fmt.Printf("  ⚠️  Cleanup warning: %v\n", err)
			}

			movedCount++
			movedBytes += result.BytesCopied
			fmt.Printf("  ✅ Moved (%s)\n", formatBytes(result.BytesCopied))
		}
	}

	// Handle plan file based on results
	if failedCount == 0 {
		if err := plans.DeleteConsolidatePlans(); err != nil {
			fmt.Printf("⚠️  Failed to clean up plans file: %v\n", err)
		}
		fmt.Println("\n✅ Plan completed and removed")
	} else {
		if err := plans.ArchiveConsolidatePlans(); err != nil {
			fmt.Printf("⚠️  Failed to archive plans file: %v\n", err)
		}
		fmt.Println("\n⚠️  Plan archived to consolidate.json.old due to failures")
	}

	fmt.Println("\n=== Execution Complete ===")
	fmt.Printf("✅ Successfully moved: %d files\n", movedCount)
	if alreadyGoneCount > 0 {
		fmt.Printf("⏭️  Already moved:     %d files\n", alreadyGoneCount)
	}
	if failedCount > 0 {
		fmt.Printf("❌ Failed to move:     %d files\n", failedCount)
	}
	fmt.Printf("📦 Data relocated:     %s\n", formatBytes(movedBytes))

	return nil
}
```

**Step 3: Build to verify syntax**

Run: `go build ./cmd/jellywatch/...`
Expected: Build succeeds (may fail on missing updateDatabaseAfterMove - that's next task)

**Step 4: Commit (partial - need Task 6 first)**

Hold commit until Task 6 is complete.

---

## Task 6: Add Auto-Insert Database Helper

**Files:**
- Modify: `cmd/jellywatch/consolidate_cmd.go` (add helper function)

**Step 1: Add the updateDatabaseAfterMove function**

Add to `cmd/jellywatch/consolidate_cmd.go`:

```go
// updateDatabaseAfterMove updates or creates a database entry after a file move
func updateDatabaseAfterMove(db *database.MediaDB, sourcePath, targetPath string, size int64) error {
	// Try to get existing file entry
	file, err := db.GetMediaFile(sourcePath)
	if err != nil || file == nil {
		// File not in DB - create new entry
		fmt.Printf("  ℹ️  File not tracked, adding to database\n")
		return createMediaFileEntry(db, targetPath, size)
	}

	// Delete old entry
	if err := db.DeleteMediaFile(sourcePath); err != nil {
		return fmt.Errorf("failed to delete old entry: %w", err)
	}

	// Update path and upsert
	file.Path = targetPath
	if err := db.UpsertMediaFile(file); err != nil {
		return fmt.Errorf("failed to update entry: %w", err)
	}

	return nil
}

// createMediaFileEntry creates a minimal database entry for a moved file
func createMediaFileEntry(db *database.MediaDB, path string, size int64) error {
	// Determine media type from path
	mediaType := "movie"
	if strings.Contains(strings.ToLower(path), "tvshow") ||
	   strings.Contains(strings.ToLower(path), "tv show") ||
	   strings.Contains(path, "Season") {
		mediaType = "episode"
	}

	// Get file info
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	// Create minimal entry - will be enriched on next scan
	file := &database.MediaFile{
		Path:            path,
		Size:            info.Size(),
		ModifiedAt:      info.ModTime(),
		MediaType:       mediaType,
		NormalizedTitle: "pending-rescan", // Will be updated on next scan
		Source:          "consolidate",
		SourcePriority:  50,
	}

	return db.UpsertMediaFile(file)
}
```

**Step 2: Add required import if missing**

Ensure these imports are at the top of `consolidate_cmd.go`:

```go
import (
	"path/filepath"
	// ... existing imports
)
```

**Step 3: Build to verify everything compiles**

Run: `go build ./cmd/jellywatch/...`
Expected: PASS

**Step 4: Commit Tasks 5 and 6 together**

```bash
git add cmd/jellywatch/consolidate_cmd.go cmd/jellywatch/cleanup.go
git commit -m "feat(consolidate): add smart cleanup and auto-insert on move

- Track already-moved files separately from failures
- Auto-insert moved files into database if not tracked
- Clean up cruft files and empty directories after moves
- Archive plan to .old on partial failure, delete on success"
```

---

## Task 7: Update Duplicates Execute with Same Pattern

**Files:**
- Modify: `cmd/jellywatch/duplicates_cmd.go:362-442` (runDuplicatesExecute function)

**Step 1: Update the plan lifecycle handling**

Modify the end of `runDuplicatesExecute` to use archive on failure:

```go
// Replace lines 427-432 with:

	// Handle plan file based on results
	if failedCount == 0 {
		if err := plans.DeleteDuplicatePlans(); err != nil {
			fmt.Printf("⚠️  Failed to clean up plans file: %v\n", err)
		}
		fmt.Println("\n✅ Plan completed and removed")
	} else {
		if err := plans.ArchiveDuplicatePlans(); err != nil {
			fmt.Printf("⚠️  Failed to archive plans file: %v\n", err)
		}
		fmt.Println("\n⚠️  Plan archived to duplicates.json.old due to failures")
	}
```

**Step 2: Build to verify**

Run: `go build ./cmd/jellywatch/...`
Expected: PASS

**Step 3: Commit**

```bash
git add cmd/jellywatch/duplicates_cmd.go
git commit -m "feat(duplicates): archive plan on partial failure"
```

---

## Task 8: Integration Test

**Files:**
- None (manual testing)

**Step 1: Run all unit tests**

Run: `go test ./... -v`
Expected: All tests pass

**Step 2: Build the binary**

Run: `go build -o jellywatch ./cmd/jellywatch/`
Expected: Binary builds successfully

**Step 3: Manual test - generate and dry-run**

Run:
```bash
./jellywatch scan
./jellywatch consolidate --generate
./jellywatch consolidate --dry-run
```

Expected: Shows plan without errors

**Step 4: Commit any final fixes**

If any issues found, fix and commit.

---

## Summary of Files Changed

| File | Type | Purpose |
|------|------|---------|
| `internal/plans/plans.go` | Modify | Add ArchiveConsolidatePlans, ArchiveDuplicatePlans |
| `internal/plans/plans_test.go` | Modify | Tests for archive functions |
| `cmd/jellywatch/cleanup.go` | Create | isVideoFile, isLibraryRoot, isInsideLibrary, cleanupSourceDir |
| `cmd/jellywatch/cleanup_test.go` | Create | Tests for cleanup functions |
| `cmd/jellywatch/consolidate_cmd.go` | Modify | Already-moved tracking, auto-insert, cleanup, plan lifecycle |
| `cmd/jellywatch/duplicates_cmd.go` | Modify | Plan archive on failure |
