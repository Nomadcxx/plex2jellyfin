# Comprehensive Bugfixes Design

**Date:** 2026-02-04
**Status:** Ready for Implementation

## Overview

This design addresses 7 issues identified through systematic debugging of jellywatch CLI commands (`audit`, `duplicates`, `consolidate`, `status`).

## Issues Summary

| # | Issue | Root Cause | Fix Type |
|---|-------|------------|----------|
| 1 | Permission config ignored in CLI | CLI uses `DefaultOptions()`, doesn't pass config uid/gid | Bug fix |
| 2 | AI audit returns garbage confidence | `filepath.Base()` strips series folder context | Bug fix |
| 3 | AI returns malformed JSON | Model returns strings for ints, prompt not explicit | Robustness |
| 4 | Consolidate "no plan" misleading | Sample-only folders detected but filtered, no explanation | UX |
| 5 | Status output confusing | Duplicates/conflicts/scattered terminology unclear | UX |
| 6 | Audit display doesn't link items/actions | Display logic shows them separately | UX |
| 7 | No cleanup command for cruft | Helper functions exist but no CLI exposure | Feature |

---

## Issue 1: CLI Commands Ignore Permission Config

### Problem
CLI commands (`audit --execute`, `duplicates execute`, `consolidate execute`) use `transfer.DefaultOptions()` which sets `TargetUID: -1, TargetGID: -1`, ignoring the user's configured permissions.

When running as sudo, new/moved files get created with root ownership instead of the configured user.

### Affected Files
- `cmd/jellywatch/consolidate_cmd.go:92`
- `internal/plans/plans.go:501, 524`
- `internal/consolidate/operations.go:86`

### Fix
Create a helper function that builds transfer options from config:

```go
// internal/transfer/config.go (new file)
func OptionsFromConfig(cfg *config.Config) TransferOptions {
    opts := DefaultOptions()
    if cfg != nil && cfg.Permissions.WantsOwnership() {
        uid, _ := cfg.Permissions.ResolveUID()
        gid, _ := cfg.Permissions.ResolveGID()
        opts.TargetUID = uid
        opts.TargetGID = gid
    }
    if cfg != nil && cfg.Permissions.FileMode != "" {
        opts.FileMode = cfg.Permissions.ParseFileMode()
    }
    if cfg != nil && cfg.Permissions.DirMode != "" {
        opts.DirMode = cfg.Permissions.ParseDirMode()
    }
    return opts
}
```

Update all CLI command call sites to use `transfer.OptionsFromConfig(cfg)` instead of `transfer.DefaultOptions()`.

### Documentation
Add to README/docs:
```
For file operations that modify ownership, run with sudo:
  sudo jellywatch duplicates execute
  sudo jellywatch consolidate execute
  sudo jellywatch audit --execute

Configure target ownership in config.toml:
  [permissions]
  user = "nomadx"
  group = "media"
```

---

## Issue 2: AI Context Stripped by filepath.Base()

### Problem
`internal/ai/matcher.go:83` uses `filepath.Base(folderPath)` which extracts only the immediate parent folder.

For `/mnt/STORAGE7/TVSHOWS/Family Guy (1999)/Season 19/file.mkv`:
- **Current:** AI sees `Folder name: Season 19` (useless)
- **Expected:** AI sees `Parent folder: Family Guy (1999)` + `Folder: Season 19`

### Fix
Replace lines 82-85 in `internal/ai/matcher.go`:

```go
if folderPath != "" {
    // Extract both parent folder (series/movie name) and immediate folder (season)
    parentDir := filepath.Dir(folderPath)
    parentName := filepath.Base(parentDir)
    immediateName := filepath.Base(folderPath)

    // Provide parent context if meaningful (not root or dot)
    if parentName != "" && parentName != "." && parentName != "/" &&
       !strings.HasPrefix(parentName, "STORAGE") && parentName != "TVSHOWS" && parentName != "MOVIES" {
        contextPrompt += fmt.Sprintf("- Parent folder: %s\n", parentName)
    }
    contextPrompt += fmt.Sprintf("- Folder: %s\n", immediateName)
}
```

### Expected Impact
- Files in properly-organized folders should see dramatic confidence improvement
- "Family Guy S19E12.mkv" in "Family Guy (1999)/Season 19/" should score 0.95+ instead of 0.00

---

## Issue 3: AI Malformed JSON Handling

### Problem
Model sometimes returns:
- `"year": "2018"` (string instead of int)
- `"episodes": ["S01E06"]` (string array instead of int array)
- `"title": []` (empty array instead of string)

### Fix
Add flexible JSON unmarshaling in `internal/ai/types.go` (new file):

```go
package ai

import (
    "encoding/json"
    "strconv"
    "strings"
)

// FlexInt handles JSON that may be int or string
type FlexInt struct {
    Value *int
}

func (f *FlexInt) UnmarshalJSON(data []byte) error {
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
    return nil // Leave as nil if unparseable
}

// FlexIntSlice handles episodes that may be int array or string array
type FlexIntSlice []int

func (f *FlexIntSlice) UnmarshalJSON(data []byte) error {
    // Try int array first
    var ints []int
    if err := json.Unmarshal(data, &ints); err == nil {
        *f = ints
        return nil
    }
    // Try string array
    var strs []string
    if err := json.Unmarshal(data, &strs); err == nil {
        for _, s := range strs {
            // Extract number from strings like "S01E06"
            if num := extractEpisodeNumber(s); num > 0 {
                *f = append(*f, num)
            }
        }
        return nil
    }
    return nil
}

func extractEpisodeNumber(s string) int {
    // Handle "S01E06" format
    s = strings.ToUpper(s)
    if idx := strings.Index(s, "E"); idx >= 0 {
        numStr := strings.TrimLeft(s[idx+1:], "0")
        if num, err := strconv.Atoi(numStr); err == nil {
            return num
        }
    }
    // Try direct parse
    if num, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
        return num
    }
    return 0
}
```

Update `Result` struct to use flexible types.

### Prompt Enhancement
Add explicit type warnings to system prompt:

```
IMPORTANT: Return exact types as shown:
- "year": 1999        // integer, NOT "1999"
- "episodes": [1, 2]  // integers, NOT ["S01E01"]
- "title": "Name"     // string, NOT [] or null
```

---

## Issue 4: Consolidate Generate Misleading Output

### Problem
`jellywatch scan` reports "4 scattered series" but `consolidate generate` says "No consolidation needed" with no explanation. The folders contain only sample files (<100MB).

### Fix
Enhance `consolidate generate` output in `cmd/jellywatch/consolidate_generate.go`:

```go
// After analyzing, show per-item status
fmt.Println("\nAnalyzing scattered content...")
for _, conflict := range conflicts {
    if conflict.MediaType != "series" {
        continue
    }

    plan, _ := consolidator.GeneratePlan(&conflict)

    yearStr := ""
    if conflict.Year != nil {
        yearStr = fmt.Sprintf(" (%d)", *conflict.Year)
    }

    if !plan.CanProceed {
        fmt.Printf("  - %s%s: Skipped (%s)\n", conflict.Title, yearStr, strings.Join(plan.Reasons, ", "))
    } else if len(plan.Operations) == 0 {
        fmt.Printf("  - %s%s: No files >100MB to move (samples only)\n", conflict.Title, yearStr)
    } else {
        fmt.Printf("  + %s%s: %d files to move\n", conflict.Title, yearStr, len(plan.Operations))
    }
}

// If no operations, suggest cleanup
if len(plan.Plans) == 0 && len(skippedItems) > 0 {
    fmt.Println("\n💡 These folders contain only sample files or cruft.")
    fmt.Println("   Consider cleaning them with: jellywatch cleanup cruft")
}
```

---

## Issue 5: Status Output Clarification

### Problem
Status shows "Duplicate Groups: 17" and "Unresolved: 20" without explaining:
- Duplicates = same file multiple copies (delete inferior)
- Conflicts = same title in multiple storage locations (consolidate)

### Fix
Restructure `cmd/jellywatch/status_cmd.go` output:

```go
fmt.Println("Statistics")
fmt.Println("----------")
fmt.Printf("Total Files:          %d\n", stats.TotalFiles)
fmt.Printf("TV Episodes:          %d\n", episodeCount)
fmt.Printf("Movies:               %d\n", movieCount)
fmt.Printf("Non-Compliant Files:  %d\n", stats.NonCompliantFiles)
fmt.Println()

// Duplicates section
movieDups, _ := db.FindDuplicateMovies()
episodeDups, _ := db.FindDuplicateEpisodes()
fmt.Println("Duplicates (same content, keep best quality)")
fmt.Println("---------------------------------------------")
fmt.Printf("Movie duplicates:    %d groups\n", len(movieDups))
fmt.Printf("Episode duplicates:  %d groups\n", len(episodeDups))
if len(movieDups)+len(episodeDups) > 0 {
    fmt.Printf("→ Run 'jellywatch duplicates generate' to review\n")
}
fmt.Println()

// Scattered section
fmt.Println("Scattered (same title across storage drives)")
fmt.Println("---------------------------------------------")
seriesConflicts := 0
movieConflicts := 0
for _, c := range conflicts {
    if c.MediaType == "series" {
        seriesConflicts++
    } else {
        movieConflicts++
    }
}
fmt.Printf("Series in multiple locations:  %d\n", seriesConflicts)
fmt.Printf("Movies in multiple locations:  %d\n", movieConflicts)
if seriesConflicts > 0 {
    fmt.Printf("→ Run 'jellywatch consolidate generate' to review\n")
}
```

---

## Issue 6: Audit Display Links Items to Actions

### Problem
`displayAuditPlan()` shows summary stats but doesn't show which items have which actions.

### Fix
Update `cmd/jellywatch/audit_cmd.go` `displayAuditPlan()`:

```go
func displayAuditPlan(plan *plans.AuditPlan, showActions bool) error {
    // ... existing summary code ...

    if showActions {
        fmt.Printf("\nItems:\n")

        // Build action lookup by matching original path
        actionByPath := make(map[string]*plans.AuditAction)
        for i := range plan.Actions {
            // Actions are in same order as items that have actions
            // Find corresponding item
            if i < len(plan.Items) {
                actionByPath[plan.Items[i].Path] = &plan.Actions[i]
            }
        }

        for i, item := range plan.Items {
            fmt.Printf("\n[%d] %s\n", i+1, filepath.Base(item.Path))
            fmt.Printf("    Path: %s\n", item.Path)
            fmt.Printf("    Current: %s (confidence: %.2f)\n", item.Title, item.Confidence)

            if item.SkipReason != "" {
                fmt.Printf("    ⚠️  Skipped: %s\n", item.SkipReason)
            } else if action, ok := actionByPath[item.Path]; ok {
                fmt.Printf("    → %s to: %s (confidence: %.2f)\n",
                    action.Action, filepath.Base(action.NewPath), action.Confidence)
            }
        }
    }

    return nil
}
```

---

## Issue 7: Add Cleanup Command

### Problem
Helper functions exist in `cleanup.go` but no CLI command is exposed.

### Fix
Add new command in `cmd/jellywatch/cleanup_cmd.go`:

```go
package main

import (
    "fmt"
    "os"
    "path/filepath"

    "github.com/spf13/cobra"
    "github.com/Nomadcxx/jellywatch/internal/config"
)

func newCleanupCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "cleanup",
        Short: "Clean up cruft files and empty directories",
    }

    cmd.AddCommand(newCleanupCruftCmd())
    cmd.AddCommand(newCleanupEmptyCmd())

    return cmd
}

func newCleanupCruftCmd() *cobra.Command {
    var dryRun bool

    cmd := &cobra.Command{
        Use:   "cruft",
        Short: "Delete sample files, .nfo, .txt, and other non-media files",
        RunE: func(cmd *cobra.Command, args []string) error {
            return runCleanupCruft(dryRun)
        },
    }

    cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without deleting")
    return cmd
}

func runCleanupCruft(dryRun bool) error {
    cfg, err := config.Load()
    if err != nil {
        return err
    }

    allRoots := append(cfg.Libraries.TV, cfg.Libraries.Movies...)

    cruftPatterns := []string{
        "*.nfo", "*.txt", "*.jpg", "*.png", "*.srt", "*.sub",
        "*-sample.*", "*sample.*", "Sample/*",
    }

    var totalDeleted int64
    var filesDeleted int

    for _, root := range allRoots {
        err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
            if err != nil || info.IsDir() {
                return nil
            }

            // Check if matches cruft pattern
            if isCruftFile(path, info, cruftPatterns) {
                if dryRun {
                    fmt.Printf("[DRY RUN] Would delete: %s (%s)\n", path, formatBytes(info.Size()))
                } else {
                    if err := os.Remove(path); err == nil {
                        filesDeleted++
                        totalDeleted += info.Size()
                    }
                }
            }
            return nil
        })
        if err != nil {
            fmt.Printf("Warning: error walking %s: %v\n", root, err)
        }
    }

    if dryRun {
        fmt.Printf("\nWould delete %d files, reclaiming %s\n", filesDeleted, formatBytes(totalDeleted))
    } else {
        fmt.Printf("\nDeleted %d files, reclaimed %s\n", filesDeleted, formatBytes(totalDeleted))
    }

    return nil
}

func isCruftFile(path string, info os.FileInfo, patterns []string) bool {
    name := filepath.Base(path)

    // Sample files under 50MB
    if info.Size() < 50*1024*1024 &&
       (strings.Contains(strings.ToLower(name), "sample") ||
        strings.Contains(strings.ToLower(filepath.Dir(path)), "sample")) {
        return true
    }

    // Known cruft extensions
    ext := strings.ToLower(filepath.Ext(name))
    cruftExts := map[string]bool{
        ".nfo": true, ".txt": true, ".jpg": true, ".png": true,
        ".srt": true, ".sub": true, ".idx": true,
    }
    return cruftExts[ext]
}
```

Register in `main.go`:
```go
rootCmd.AddCommand(newCleanupCmd())
```

---

## Implementation Order

1. **Issue 2** (AI context) - One-line fix, highest impact
2. **Issue 1** (permissions) - Bug fix, enables sudo workflow
3. **Issue 3** (JSON parsing) - Robustness improvement
4. **Issue 4** (consolidate UX) - Better feedback
5. **Issue 5** (status UX) - Clarity improvement
6. **Issue 6** (audit display) - UX improvement
7. **Issue 7** (cleanup command) - New feature

## Testing

Each fix should include:
- Unit test for the changed function
- Manual verification with real media library

## Rollout

1. Implement fixes on feature branch
2. Run full test suite
3. Manual testing with user's library
4. Merge to main
