# Consolidate MOVE Execution Design

**Date:** 2026-01-22
**Status:** Approved

## Overview

Complete the consolidation feature by implementing proper MOVE plan execution. This allows `jellywatch consolidate --execute` to move files across drives, merging scattered content into single locations.

## Requirements

| Requirement | Decision |
|-------------|----------|
| Move backend | Use `internal/transfer` (rsync), add `pv` fallback for progress |
| Untracked files | Smart filter: move files >100MB, skip samples |
| Size threshold | 100MB |
| Empty directories | Delete after move if empty |
| Dry-run output | Summary per conflict (not per file) |
| Target selection | Location with most files (existing behavior) |

## Implementation Sections

### 1. Move Execution Backend

**Files:** `internal/transfer/pv.go` (new), `internal/transfer/transfer.go`

- Add `pv` detection via `exec.LookPath("pv")` at init
- Add `BackendPV` option that pipes through `pv` for progress display
- Command: `pv source_file | dd of=destination bs=1M`
- Falls back to rsync if `pv` not available
- Backend selection: if user requests progress AND `pv` available, use it; otherwise rsync

### 2. Smart File Filter (100MB Threshold)

**Files:** `internal/consolidate/consolidate.go`, `internal/consolidate/operations.go`

- Define constant: `const MinConsolidationFileSize = 100 * 1024 * 1024`
- In `getFilesToMove()`: skip files under 100MB
- In `StorePlan()`: if file not in database but >100MB on disk, still create MOVE plan
- Make `source_file_id` nullable in schema for untracked files

**Schema change:**
```sql
ALTER TABLE consolidation_plans 
ALTER COLUMN source_file_id DROP NOT NULL;
```

### 3. Empty Directory Cleanup

**Files:** `internal/consolidate/operations.go`

Add function:
```go
func cleanupEmptyDir(dir string) error
```

Behavior:
- Check if directory is empty (no files, no subdirs)
- If empty, remove with `os.Remove(dir)`
- Call after each successful move in `executeOperation()`
- Safety: never delete library root directories
- Dry-run: log "Would delete empty directory: X"

### 4. Dry-Run Output Format

**Files:** `internal/consolidate/operations.go`

Output format:
```
=== Consolidation Preview ===

Fallout (2024): 5 files -> /mnt/STORAGE5/TVSHOWS/Fallout (2024)/
The Rookie (2018): 12 files -> /mnt/STORAGE1/TVSHOWS/The Rookie (2018)/
Emily In Paris (2020): 3 files -> /mnt/STORAGE7/TVSHOWS/Emily In Paris (2020)/

Total: 20 files across 3 conflicts
Estimated data: 45.2 GiB

Skipped 5 conflicts (no files >100MB to move)
```

Keep detailed output via `--verbose` flag.

### 5. Target Location Selection

**Files:** `internal/consolidate/consolidate.go`

No changes needed - existing `chooseTargetPath()` already picks location with most files.

Add debug logging: `Selected target: /path/ (8 files vs 3 files)`

### 6. Executor Integration

**Files:** `internal/consolidate/executor.go`

Add/verify MOVE handling in `ExecutePlans()`:

1. Verify source file exists
2. Create destination directory if needed
3. Call `transfer.Move()` with pv backend if available
4. Verify destination file exists and size matches
5. Call `cleanupEmptyDir()` on source parent
6. Mark plan as completed in database

Error handling:
- Mark failed plans as `failed` with error message
- Continue with remaining plans (don't abort all)
- Summary shows success/failure counts

Post-execution:
- Mark conflict as `resolved` in conflicts table
- Update `media_files` path for moved files (if tracked)

## Files to Modify

| File | Changes |
|------|---------|
| `internal/transfer/pv.go` | New file: pv backend implementation |
| `internal/transfer/transfer.go` | Add pv detection |
| `internal/consolidate/consolidate.go` | Add MinConsolidationFileSize constant, size filter |
| `internal/consolidate/operations.go` | StorePlan nullable file_id, cleanupEmptyDir, DryRun format |
| `internal/consolidate/executor.go` | Add MOVE handling |
| `internal/database/migrations.go` | Make source_file_id nullable |

## Testing

1. `jellywatch consolidate --generate` - verify plans created with size filter
2. `jellywatch consolidate --dry-run` - verify summary format
3. `jellywatch consolidate --execute` on test data - verify moves, cleanup, conflict resolution
