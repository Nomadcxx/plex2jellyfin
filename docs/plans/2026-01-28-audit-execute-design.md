# Audit --execute Actions Design

**Date:** 2026-01-28
**Goal:** Implement audit command --execute with rename and delete actions

## Overview

The audit command identifies files with low parse confidence and uses AI to suggest corrections. The --execute flag applies these corrections to both the filesystem and database.

## Architecture

**File Organization:**
- `internal/plans/plans.go` - Add `ExecuteAuditAction()` and helpers
- `internal/plans/plans_test.go` - Add tests for new functions
- `cmd/jellywatch/audit_cmd.go` - Wire up execution flow

## Data Flow

**Rename Action:**
1. Load `AuditAction` from plan
2. Build new filename from `NewTitle`, `NewYear`, `NewSeason`, `NewEpisode`
3. Call `transfer.Move(oldPath, newPath)` to move file on disk
4. If move succeeds, update `media_files` table:
   - `path` → new path
   - `normalized_title` → `NewTitle`
   - `year` → `NewYear`
   - `season` → `NewSeason`
   - `episode` → `NewEpisode`
   - `confidence` → AI confidence
   - `parse_method` → "ai"
   - `needs_review` → false
5. If move fails, return error (database unchanged)

**Delete Action:**
1. Load `AuditAction` from plan
2. Call `os.Remove(path)` to delete file
3. If delete succeeds, delete from `media_files` table
4. If delete fails, return error (database unchanged)

**File-First Principle:**
The database is source of truth. Filesystem changes happen first.
If file operation fails, database remains unchanged.
If file succeeds but DB update fails, log warning and continue.

## Functions

```go
// ExecuteAuditAction executes a single audit action
func ExecuteAuditAction(db *database.MediaDB, action AuditAction) error

// buildNewFilename constructs filename from AI suggestion
func buildNewFilename(action AuditAction, oldPath string) string

// executeRename handles rename action
func executeRename(db *database.MediaDB, action AuditAction) error

// executeDelete handles delete action
func executeDelete(db *database.MediaDB, action AuditAction) error
```

## Error Handling

- **File not found:** Log warning, mark as skipped, continue
- **File move fails:** Return error, don't update DB
- **DB update fails:** Log warning, file already moved (inconsistent state)
- **Permission denied:** Log error, continue to next action

## Confidence Threshold

Only execute actions with `Confidence >= 0.8` as specified in confidence-system.md.

## Dry-Run Mode

Same execution flow but skip actual file operations.
Print preview of what would happen.

## Testing Strategy

1. **Success path:** Mock file move succeeds, DB update succeeds
2. **File move fails:** Mock file move fails, verify DB unchanged
3. **DB update fails:** Mock file move succeeds, DB update fails
4. **Invalid paths:** Handle non-existent files gracefully
5. **TV episodes:** Verify season/episode extraction
6. **Movies:** Verify title/year extraction

## Dependencies

- `github.com/Nomadcxx/jellywatch/internal/database`
- `github.com/Nomadcxx/jellywatch/internal/transfer`
- `github.com/Nomadcxx/jellywatch/internal/plans` (existing)

## Success Criteria

- [ ] `jellywatch audit --execute` applies AI suggestions
- [ ] Files are renamed on disk
- [ ] Database is updated with new metadata
- [ ] Actions with confidence < 0.8 are skipped
- [ ] Dry-run mode shows preview
- [ ] Tests cover success and failure paths
- [ ] All existing tests still pass
