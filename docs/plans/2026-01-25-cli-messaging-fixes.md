# CLI Messaging Fixes Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix CLI messaging confusion and add delete capability to duplicates command

**Architecture:** Update command output strings and add new command flag with proper validation

**Tech Stack:** Go, Cobra CLI framework

---

## Task Structure

### Task 1: Fix consolidate --generate "Space to reclaim" messaging

**Files:**
- Modify: `cmd/jellywatch/consolidate_cmd.go:32`

**Step 1: Locate the issue**

Line 32 shows: `fmt.Printf("  Space to reclaim:          %s\n", formatBytes(spaceToReclaim))`

**Step 2: Change to "Files to relocate"**

Change from:
```go
fmt.Printf("  Space to reclaim:          %s\n", formatBytes(spaceToReclaim))
```

To:
```go
fmt.Printf("  Files to relocate:         %s\n", formatBytes(spaceToReclaim))
```

**Step 3: Verify change matches line alignment**

The format strings use %-15s padding, ensure "Files to relocate" aligns with other fields

**Step 4: Run build**

```bash
go build -o build/jellywatch ./cmd/jellywatch
```

Expected: SUCCESS

**Step 5: Test consolidate --generate output**

```bash
./build/jellywatch consolidate --generate
```

Verify output shows "Files to relocate:" not "Space to reclaim:"

**Step 6: Commit**

```bash
git add cmd/jellywatch/consolidate_cmd.go
git commit -m "fix(cli): consolidate shows 'Files to relocate' not 'Space to reclaim'"
```

---

### Task 2: Add --execute flag to jellywatch duplicates command

**Files:**
- Modify: `cmd/jellywatch/duplicates_cmd.go`

**Step 1: Add execute flag variable**

After existing flags (line 13-16), add:
```go
var (
    moviesOnly bool
    tvOnly     bool
    showFilter string
    execute    bool  // NEW
)
```

**Step 2: Register the flag**

After line 40, add:
```go
cmd.Flags().BoolVar(&execute, "execute", false, "Execute removal of duplicate files")
```

**Step 3: Update runDuplicates signature**

Change line 45 from:
```go
func runDuplicates(moviesOnly, tvOnly bool, showFilter string) error {
```

To:
```go
func runDuplicates(moviesOnly, tvOnly bool, showFilter string, execute bool) error {
```

**Step 4: Update RunE function to pass execute flag**

Change line 34 from:
```go
RunE: func(cmd *cobra.Command, args []string) error {
    return runDuplicates(moviesOnly, tvOnly, showFilter)
},
```

To:
```go
RunE: func(cmd *cobra.Command, args []string) error {
    return runDuplicates(moviesOnly, tvOnly, showFilter, execute)
},
```

**Step 5: Implement delete logic in runDuplicates**

Before the summary section (line 169), add execution logic:

```go
// Execute deletions if requested
if execute {
    fmt.Println("⚠️  Deleting duplicate files...")
    fmt.Println()

    deleteCount := 0
    deletedSize := int64(0)

    // Delete movie duplicates
    if !tvOnly {
        movieGroups, err := db.FindDuplicateMovies()
        if err != nil {
            return fmt.Errorf("failed to find duplicate movies: %w", err)
        }

        for _, group := range movieGroups {
            if len(group.Files) < 2 {
                continue
            }

            for _, file := range group.Files {
                if group.BestFile != nil && file.ID == group.BestFile.ID {
                    continue  // Keep best file
                }

                // Delete file
                err := os.Remove(file.Path)
                if err != nil {
                    fmt.Printf("Error deleting %s: %v\n", file.Path, err)
                    continue
                }

                // Remove from database
                _, err = db.DB().Exec("DELETE FROM media_files WHERE id = ?", file.ID)
                if err != nil {
                    fmt.Printf("Error removing from database: %v\n", err)
                }

                deleteCount++
                deletedSize += file.Size

                fmt.Printf("✓ Deleted: %s\n", file.Path)
            }
        }
    }

    // Delete TV episode duplicates
    if !moviesOnly {
        episodeGroups, err := db.FindDuplicateEpisodes()
        if err != nil {
            return fmt.Errorf("failed to find duplicate episodes: %w", err)
        }

        for _, group := range episodeGroups {
            if len(group.Files) < 2 {
                continue
            }

            for _, file := range group.Files {
                if group.BestFile != nil && file.ID == group.BestFile.ID {
                    continue  // Keep best file
                }

                // Delete file
                err := os.Remove(file.Path)
                if err != nil {
                    fmt.Printf("Error deleting %s: %v\n", file.Path, err)
                    continue
                }

                // Remove from database
                _, err = db.DB().Exec("DELETE FROM media_files WHERE id = ?", file.ID)
                if err != nil {
                    fmt.Printf("Error removing from database: %v\n", err)
                }

                deleteCount++
                deletedSize += file.Size

                fmt.Printf("✓ Deleted: %s\n", file.Path)
            }
        }
    }

    fmt.Println()
    fmt.Printf("=== Deletion Complete ===")
    fmt.Printf("Files deleted:     %d\n", deleteCount)
    fmt.Printf("Space reclaimed:    %s\n", formatBytes(deletedSize))
    fmt.Println()

    if deleteCount == 0 {
        fmt.Println("No duplicates were deleted.")
    }

    return nil
}
```

**Step 6: Remove consolidate suggestions from duplicates output**

Replace lines 77-80:
```go
fmt.Println("\nTo remove duplicates:")
fmt.Println("  jellywatch consolidate --generate  # Generate cleanup plans")
fmt.Println("  jellywatch consolidate --dry-run   # Preview actions")
fmt.Println("  jellywatch consolidate --execute   # Execute cleanup")
```

With:
```go
fmt.Println("\nTo remove duplicates:")
fmt.Println("  jellywatch duplicates --execute")
```

**Step 7: Add os import**

Add to imports (line 3-9):
```go
import (
    "fmt"
    "os"  // NEW
    "strings"

    "github.com/Nomadcxx/jellywatch/internal/database"
    "github.com/spf13/cobra"
)
```

**Step 8: Update help text**

Add --execute flag documentation in Long description (around line 31):

Change from:
```go
Examples:
  jellywatch duplicates              # List all duplicates
  jellywatch duplicates --movies     # Only movies
  jellywatch duplicates --tv         # Only TV episodes
  jellywatch duplicates --show=Silo  # Duplicates for specific show
```

To:
```go
Examples:
  jellywatch duplicates              # List all duplicates
  jellywatch duplicates --execute     # Remove duplicate files (keeps best quality)
  jellywatch duplicates --movies     # Only movies
  jellywatch duplicates --tv         # Only TV episodes
  jellywatch duplicates --show=Silo  # Duplicates for specific show
```

**Step 9: Run build**

```bash
go build -o build/jellywatch ./cmd/jellywatch
```

Expected: SUCCESS

**Step 10: Test duplicates command without --execute**

```bash
./build/jellywatch duplicates --help
```

Verify --execute flag is shown in help.

**Step 11: Test duplicates list (no execution)**

```bash
./build/jellywatch duplicates
```

Expected: Lists duplicates, shows "To remove duplicates: jellywatch duplicates --execute"

**Step 12: Test duplicates --execute (dry run on test files first)**

Note: This will delete actual files, so be careful in testing.

**Step 13: Commit**

```bash
git add cmd/jellywatch/duplicates_cmd.go
git commit -m "feat(duplicates): add --execute flag to delete duplicate files"
```

---

### Task 3: Verify consolidate cannot delete files

**Files:**
- Review: `internal/consolidate/*`
- Search: executor.go, operations.go

**Step 1: Search for file deletion in consolidate package**

```bash
grep -r "os.Remove\|io.Remove\|Delete" internal/consolidate/
```

Expected: No matches (consolidate should only use file moves/renames)

**Step 2: Review executor.ExecutePlans implementation**

Check in `internal/consolidate/executor.go`:
- Look for any code that deletes files (not in consolidation_plans table)
- Verify only move/rename operations are performed

**Step 3: Verify no DELETE action types exist**

Check consolidation_plans schema for action types:

```sql
SELECT DISTINCT action FROM consolidation_plans;
```

Expected values should be: "move", "rename" only - NOT "delete"

**Step 4: Document findings**

If any deletion found in consolidate, report back to user. Otherwise, confirm safe.

---

## Verification

### After all tasks complete:

**1. Build binaries:**
```bash
go build -o build/jellywatch ./cmd/jellywatch
```

Expected: SUCCESS

**2. Test consolidate --generate output:**

```bash
./build/jellywatch consolidate --generate
```

Verify:
- Shows "Files to relocate:" not "Space to reclaim:"

**3. Test duplicates --help:**

```bash
./build/jellywatch duplicates --help
```

Verify:
- Shows --execute flag in help
- Examples include --execute usage

**4. Test duplicates list:**

```bash
./build/jellywatch duplicates
```

Verify:
- Lists duplicates correctly
- Shows "To remove duplicates: jellywatch duplicates --execute" (not consolidate suggestion)

**5. Test all package tests:**

```bash
go test ./...
```

Expected: All PASS

---

## Notes

- Consolidate command is strictly for moving/reorganizing files
- Duplicates command will handle deletion explicitly with --execute flag
- File deletion only happens from duplicates --execute command
- Quality scoring in duplicates command ensures best version is kept
