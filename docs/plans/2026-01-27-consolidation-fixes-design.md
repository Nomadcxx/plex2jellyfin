# Consolidation Fixes & Scan Improvements

**Date:** 2026-01-27
**Status:** Approved

## Overview

Fixes discovered during a real consolidation job, plus a scan coverage gap for nested files.

## Problems

1. **Empty folders left behind** - After moving files, source directories remain empty with cruft files
2. **Silent database failures** - If a moved file isn't in the DB, nothing happens
3. **Stale plans persist** - Plan JSON remains after partial execution
4. **Nested files not scanned** - Hash-named files in subdirectories (from sabnzbd) aren't picked up

## Solutions

### 1. Smart Cruft Cleanup

After moving a video file, check what remains in the source directory:
- If **only cruft files** remain (no video files), delete cruft then directory
- If **other video files** remain, leave everything alone
- Walk up directory tree, repeating until hitting library root

**Cruft files (safe to delete):**
- `.nfo`, `.txt`, `.srt`, `.sub`, `.idx`
- `.jpg`, `.jpeg`, `.png`, `.gif`
- `.nzb`, `.sfv`, `.md5`

**Protected files (never auto-delete):**
- `.mkv`, `.mp4`, `.avi`, `.m4v`, `.ts`, `.wmv`, `.mov`

**Safety layers:**
1. Library root protection - Never delete at or above library root
2. Empty/cruft check before delete - Re-verify immediately before removal
3. Only cleanup source paths - Never touch target directories
4. Path validation - Ensure path is within a known library
5. `os.Remove()` for directories - Fails if not truly empty (final safety net)

```go
var videoExtensions = map[string]bool{
    ".mkv": true, ".mp4": true, ".avi": true,
    ".m4v": true, ".ts": true, ".wmv": true, ".mov": true,
}

func cleanupSourceDir(dir string, libraryRoots []string) error {
    // Safety checks
    if !isInsideLibrary(dir, libraryRoots) || isLibraryRoot(dir, libraryRoots) {
        return nil
    }

    entries, err := os.ReadDir(dir)
    if err != nil {
        return nil
    }

    // Check if any video files remain
    for _, entry := range entries {
        if !entry.IsDir() {
            ext := strings.ToLower(filepath.Ext(entry.Name()))
            if videoExtensions[ext] {
                return nil  // Video file exists, leave everything
            }
        }
    }

    // No video files - delete all cruft
    for _, entry := range entries {
        path := filepath.Join(dir, entry.Name())
        os.RemoveAll(path)
    }

    // Directory now empty, remove it
    os.Remove(dir)

    // Continue up the tree (recursive call with parent)
    return cleanupSourceDir(filepath.Dir(dir), libraryRoots)
}
```

### 2. Auto-Insert Moved Files

If a file moves successfully but isn't in the database, create a new entry by parsing metadata from the target path.

```go
// After successful move
file, err := db.GetMediaFile(op.SourcePath)
if err != nil || file == nil {
    // File not in DB - create entry from target path
    fmt.Printf("  ℹ️  File not tracked, adding to database\n")

    newFile, err := createMediaFileFromPath(op.TargetPath, op.Size)
    if err != nil {
        fmt.Printf("  ⚠️  Moved but failed to parse: %v\n", err)
    } else {
        if err := db.UpsertMediaFile(newFile); err != nil {
            fmt.Printf("  ⚠️  Moved but failed to insert: %v\n", err)
        }
    }
} else {
    // Existing flow: delete old entry, update path, upsert
    db.DeleteMediaFile(op.SourcePath)
    file.Path = op.TargetPath
    db.UpsertMediaFile(file)
}
```

The `createMediaFileFromPath()` function reuses existing parsing logic from the filesystem scanner.

### 3. Plan Lifecycle & Failure Handling

Track operation outcomes granularly and always handle the plan file after execution.

```go
movedCount := 0
alreadyGoneCount := 0  // Files that don't exist (already moved)
failedCount := 0       // Actual failures (permission errors, etc.)

for _, op := range group.Operations {
    // Check if source exists BEFORE attempting move
    if _, err := os.Stat(op.SourcePath); os.IsNotExist(err) {
        fmt.Printf("  ⏭️  Already moved: %s\n", filepath.Base(op.SourcePath))
        alreadyGoneCount++
        continue
    }

    // Attempt move...
}

// After execution - always handle plan file
if failedCount == 0 {
    plans.DeleteConsolidatePlans()
    fmt.Println("✅ Plan completed and removed")
} else {
    plans.ArchiveConsolidatePlans()  // Renames to .old
    fmt.Println("⚠️  Plan archived due to failures")
}
```

New functions in `internal/plans/`:
- `ArchiveConsolidatePlans()` - Renames to `consolidate.json.old`
- `ArchiveDuplicatePlans()` - Renames to `duplicates.json.old`

### 4. Recursive Video Scanning

Scan into subdirectories to find nested files, filtering by video extension.

```go
func scanDirectory(root string) ([]*MediaFile, error) {
    var files []*MediaFile

    err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
        if err != nil {
            return nil  // Skip unreadable dirs
        }

        if d.IsDir() {
            return nil  // Continue into subdirectories
        }

        // Only process video files
        ext := strings.ToLower(filepath.Ext(path))
        if !videoExtensions[ext] {
            return nil
        }

        mediaFile, err := parseMediaFile(path)
        if err == nil {
            files = append(files, mediaFile)
        }

        return nil
    })

    return files, err
}
```

## Files to Change

| File | Changes |
|------|---------|
| `cmd/jellywatch/consolidate_cmd.go` | Empty folder cleanup, cruft deletion, auto-insert, plan lifecycle |
| `internal/plans/plans.go` | Add `ArchiveConsolidatePlans()` and `ArchiveDuplicatePlans()` |
| `internal/sync/filesystem.go` | Recursive scanning with video extension filter |

## New Helper Functions

- `cleanupSourceDir()` - Smart cruft cleanup that climbs directory tree
- `isInsideLibrary()` - Safety check for path validation
- `createMediaFileFromPath()` - Create DB entry from path when file not tracked
- `ArchiveConsolidatePlans()` / `ArchiveDuplicatePlans()` - Rename to `.old`

## Behavior Changes

1. After move: Delete cruft files, remove empty dirs up to library root
2. If moved file not in DB: Parse metadata and insert automatically
3. If source file already gone: Skip with "Already moved" (not an error)
4. After execution: Delete plan on success, archive to `.old` on failure

## No Changes To

- Database schema
- Plan JSON structure
- CLI flags or workflow
