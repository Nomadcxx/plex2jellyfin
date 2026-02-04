# CLI Messaging Fixes Design

> **Created:** 2026-01-25
> **Status:** Design Phase

## Architecture Overview

**Clean Command Separation:**
- `jellywatch consolidate` → **Reorganization only** (moves/renames files)
- `jellywatch duplicates` → **Analysis + Deletion** (finds and removes duplicate files)

**No Overlapping Responsibilities:**
- Consolidate generates plans for structural changes (moving files to correct locations)
- Duplicates handles duplicate file cleanup with direct deletion
- Each command operates independently without cross-suggestions

## Problem Statement

Four CLI messaging and architectural issues exist:

1. **`jellywatch consolidate --generate` terminology:**
   - Currently shows "Space to reclaim: 265.5 GB"
   - Consolidation moves files, doesn't reclaim disk space
   - Users are confused about what happens to their disk

2. **`jellywatch consolidate` has dual responsibility:**
   - Currently handles both reorganization (moves) AND deletion (duplicates)
   - Creates architectural confusion about command purposes
   - Should focus purely on reorganization

3. **`jellywatch duplicates` missing functionality:**
   - Cannot delete duplicates directly
   - Only lists duplicates and suggests using consolidate
   - Users must manually run consolidate to delete
   - Should have `--execute` flag for direct deletion

4. **Workflow confusion:**
   - Commands cross-suggest each other's functions
   - Unclear separation between reorganization vs cleanup operations
   - Need clear architectural boundaries

## Design Principles

1. **Architectural Clarity** - Each command has one clear responsibility
2. **Clean Separation** - Reorganization ≠ Deletion operations
3. **Clear Terminology** - "reclaim" for deletions, "relocate" for reorganization
4. **Safe Operations** - Explicit flags prevent accidental data loss
5. **Independent Commands** - No cross-suggestions or dependencies

## Architecture

### Component: Consolidation System (Reorganization Only)

**Location:** `cmd/jellywatch/consolidate_cmd.go`

**Responsibility:** Library structure reorganization via file moves/renames

**Flow:**
```
consolidate --generate
  ↓
Analyze non-compliant filenames and locations
  ↓
Generate reorganization plans (move/rename only)
  ↓
Output summary:
  - "Files to relocate: X"
  - "Data to relocate: Y GB"
  - "Move operations: N"
```

**Key Constraint:** **NEVER generates delete plans for duplicates**

### Component: Duplicates System (Analysis + Deletion)

**Location:** `cmd/jellywatch/duplicates_cmd.go`

**Responsibility:** Duplicate file detection and cleanup

**Flow:**
```
duplicates --list (default)
  ↓
Find duplicate groups (movies/TV episodes)
  ↓
Display analysis with [KEEP]/[DELETE] markers
  ↓
Suggest: --execute to delete marked files

duplicates --execute
  ↓
Find duplicate groups
  ↓
For each group:
  - Identify best file (highest quality score)
  - Delete non-best files (os.Remove)
  - Remove from database (DELETE FROM media_files)
  ↓
Report: Files deleted, space reclaimed
```

**Error Handling:**
- File not found: Log warning, continue
- Permission denied: Log error, fail operation
- Database failure: Log error (partial state OK)

## Data Flow

### Duplicates Command (Analysis + Deletion)

**List Mode (default):**
```
duplicates [--movies|--tv|--show=NAME]
  ↓
Query database for duplicate groups
  ↓
Display formatted table per group:
  - Quality Score | Size | Resolution | Source | Path [KEEP/DELETE]
  - Per-group space reclaimable
  ↓
Summary: groups, files, total space reclaimable
  ↓
Suggest: --execute to delete marked files
```

**Execute Mode:**
```
duplicates --execute [--movies|--tv|--show=NAME]
  ↓
Query database for duplicate groups
  ↓
For each group:
  1. Validate group.BestFile exists
  2. For each file where file.ID != bestFile.ID:
     - Delete from filesystem (os.Remove)
     - Delete from media_files table
     - Track success/failure counts
  ↓
Output results:
  - "Files deleted: N"
  - "Space reclaimed: X GB"
  - "Errors: N" (if any)
```

### Consolidation Command (Reorganization Only)

**Generate Mode:**
```
consolidate --generate
  ↓
Generate reorganization plans (moves/renames only)
  ↓
Calculate totals:
  - movePlans = count of move operations
  - relocateBytes = sum of file sizes being moved
  ↓
Output summary:
  - "Files to relocate: N"
  - "Data to relocate: X GB"
  - "Move operations: N"
  ↓
Next steps: --dry-run, --execute
```

**Key Difference:** No delete plans generated - consolidation focuses purely on file placement

## Command Separation

| Command | Purpose | Operations | Flags |
|---------|-----------|------------|-------|
| `jellywatch consolidate` | **Reorganize library structure** | Move, Rename | `--generate`, `--dry-run`, `--execute` |
| `jellywatch duplicates` | **Find and remove duplicates** | Analyze, Delete | `--list` (default), `--execute` (NEW), `--movies`, `--tv`, `--show` |

### Architectural Boundaries

**Consolidation Command:**
- Focuses on **structural organization** (moving files to correct Jellyfin locations)
- Generates plans for **reorganization only**
- **Never** handles duplicate deletion
- Uses rsync/file operations for safe moves

**Duplicates Command:**
- Focuses on **duplicate cleanup** (finding and removing inferior copies)
- **Never** handles reorganization
- Provides both analysis (`--list`) and execution (`--execute`) modes
- Uses direct deletion (os.Remove) for cleanup

**Key Constraints:**
- Commands operate independently - no cross-suggestions
- Consolidation never generates delete plans
- Duplicates never suggests reorganization commands
- Each command has one clear responsibility

## Implementation Approach

**Clean Architecture Refactoring: Separate reorganization from deletion**

**Core strategy:**
- Remove delete functionality from consolidate command
- Make consolidate focus purely on reorganization (moves/renames)
- Add direct deletion to duplicates command
- Update terminology and messaging for clarity
- Remove all cross-command suggestions

**Changes breakdown:**

1. **Consolidation Refactor** (MODIFY: `internal/consolidate/`)
   - Remove duplicate deletion logic from planner.go
   - Update executor.go to reject delete plans
   - Change generate logic to focus on reorganization only
   - Update terminology: "Space to reclaim" → "Data to relocate"

2. **Duplicates Enhancement** (MODIFY: `cmd/jellywatch/duplicates_cmd.go`)
   - Add `--execute` flag for direct deletion
   - Add `--list` flag (default behavior, explicit)
   - Implement deletion logic with os.Remove() + DB cleanup
   - Remove consolidate suggestions from output
   - Add clear separation between analysis and execution

3. **Shared Utilities** (NEW: `cmd/jellywatch/formatter.go`)
   - `formatBytes()` - Move from main.go for reuse
   - `printSummaryField()` - Consistent summary formatting
   - `printCommandStep()` - Consistent command guidance

4. **Command Descriptions** (MODIFY: both command files)
   - Update help text to reflect clean separation
   - Remove cross-suggestions from all output
   - Make responsibilities crystal clear

## Migration Strategy

**Breaking Change Notice:**
This update introduces architectural clarity by separating reorganization from deletion operations.

### Transition Plan

**Phase 1: Code Changes**
- Remove delete plan generation from consolidate
- Add --execute flag to duplicates
- Update terminology and messaging
- Add migration logic to handle existing plans

**Phase 2: User Migration**
- Existing consolidate plans containing delete operations become invalid
- System detects and clears obsolete plans on first run
- Users see clear messaging about architectural change
- Guides users to use appropriate command for their needs

### Migration Logic

**On First Run After Update:**
```
Detect existing consolidate plans
  ↓
If plans contain delete operations:
  - Log: "Consolidate command focus changed to reorganization only"
  - Clear all pending plans
  - Suggest: "Run 'consolidate --generate' for reorganization plans"
  - Suggest: "Run 'duplicates --execute' for duplicate cleanup"
  ↓
Continue normal operation
```

**User Communication:**
```
⚠️  Consolidate command architecture updated!

The consolidate command now focuses purely on reorganization (moving/renaming files).
Duplicate deletion has moved to the duplicates command.

Your existing consolidation plans have been cleared.
To reorganize your library: jellywatch consolidate --generate
To clean up duplicates:     jellywatch duplicates --execute
```

**Benefits:**
- ✅ **Architectural Clarity** - Each command has one responsibility
- ✅ **No Overlapping Concerns** - Reorganization vs deletion cleanly separated
- ✅ **User Experience** - Clear, non-confusing command purposes
- ✅ **Maintainable** - Single responsibility principle
- ✅ **Testable** - Independent command behaviors
- ✅ **Future-Proof** - Clear boundaries for future features

## Edge Cases

### Duplicates Command (--execute)

**File System Errors:**
1. **File deleted but DB entry exists:**
   - os.Remove() succeeds, DB DELETE fails
   - Handle: Log warning, count as error, continue

2. **DB entry deleted but file doesn't exist:**
   - os.Remove() fails "no such file", DB DELETE succeeds
   - Handle: Log info, count as success (file already gone)

3. **Permission denied:**
   - os.Remove() fails "permission denied"
   - Handle: Log error, count as failure, continue with other files

4. **Best file is NULL:**
   - group.BestFile == nil
   - Handle: Skip group, log warning, don't delete anything

**Data Validation:**
5. **Duplicate group has < 2 files:**
   - Not actually a duplicate
   - Handle: Skip group (no deletion needed)

6. **Database connection fails mid-operation:**
   - Some files deleted, DB operations fail
   - Handle: Log detailed error, report partial success

### Consolidation Command (--generate)

**After Architectural Change:**
7. **Existing delete plans become obsolete:**
   - Old consolidate plans contained delete operations
   - Handle: Clear all pending plans, require regeneration

8. **No reorganization opportunities found:**
   - All files already in correct locations
   - Handle: Clear messaging that library is well-organized

## Testing Strategy

### Unit Tests
1. **Duplicates deletion logic:**
   - Test --execute with mock database
   - Test movie/episode deletion logic
   - Test file system error handling
   - Test database error handling

2. **Consolidation reorganization:**
   - Test --generate creates move plans only
   - Test terminology shows "relocate" not "reclaim"
   - Test no delete plans generated

3. **Shared utilities:**
   - Test formatter functions
   - Test summary field formatting

### Integration Tests
1. **End-to-end workflows:**
   - Run consolidate --generate → verify move-only plans
   - Run duplicates --execute → verify files deleted
   - Verify database consistency after operations

2. **Error scenarios:**
   - Test permission denied scenarios
   - Test partial failure handling
   - Test database connection issues

3. **Migration testing:**
   - Test behavior with existing consolidate plans
   - Test plan regeneration after architectural change

## Success Criteria

### Functional Requirements
1. ✅ `jellywatch consolidate --generate` shows "Data to relocate" (not "Space to reclaim")
2. ✅ `jellywatch consolidate` generates **only move/rename plans** (no deletes)
3. ✅ `jellywatch duplicates` has `--execute` flag for direct deletion
4. ✅ `jellywatch duplicates --execute` deletes duplicate files from filesystem + database
5. ✅ Commands operate independently with no cross-suggestions

### User Experience
6. ✅ Clear command purposes (reorganization vs cleanup)
7. ✅ Safe operations with explicit flags
8. ✅ Informative error handling and reporting
9. ✅ Consistent terminology across commands

### Technical Quality
10. ✅ All unit tests pass
11. ✅ Integration tests pass
12. ✅ Build succeeds
13. ✅ Clean architectural separation maintained
14. ✅ Migration from old consolidate behavior handled gracefully

## Summary

**Architectural Clarity Achieved:**

**Before:** Confusing overlap
- `consolidate` did moves + deletes
- `duplicates` only analyzed, suggested consolidate
- Cross-command suggestions created workflow confusion
- Terminology mixed "reclaim" vs "relocate"

**After:** Clean separation
- `consolidate` → **Reorganization only** (moves/renames files)
- `duplicates` → **Analysis + Deletion** (finds/removes duplicates)
- Independent operation with clear responsibilities
- Consistent terminology ("relocate" for moves, "reclaim" for deletions)

**Benefits:**
- **User Clarity:** No more confusion about what each command does
- **Architectural Integrity:** Single responsibility principle
- **Maintenance:** Easier to modify and test individual features
- **Future-Proof:** Clear boundaries for new features

**Migration Impact:**
- One-time breaking change requires plan regeneration
- Clear user communication about architectural shift
- Improved long-term user experience
