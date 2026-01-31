# Implementation Summary: Critical Audit System Bugs Fixed

**Date:** 2026-01-31  
**Plan:** `docs/plans/2026-01-31-fix-audit-critical-bugs.md`  
**Status:** ✅ All 5 tasks completed successfully

---

## Overview

Fixed two critical bugs in JellyWatch's audit system:
1. **Cross-Device Link Errors** - File operations failing when moving between different filesystems
2. **AI Hallucinations** - AI making incorrect title guesses due to lack of context

---

## Changes Made

### Bug Fix 1: Cross-Device Support (Tasks 1-2)

**Problem:** `os.Rename()` fails with "invalid cross-device link" when moving files between different mount points (e.g., `/mnt/STORAGE5` → `/mnt/STORAGE1`).

**Solution:** Replace `os.Rename()` with `transfer.Move()` which handles cross-filesystem moves via copy-then-delete with retry/timeout support.

**Files Modified:**
- `internal/plans/plans.go:464` - Audit rename execution
- `internal/plans/plans.go:483` - Rollback logic
- `internal/consolidate/executor.go:256` - Consolidation rename
- `internal/plans/plans_test.go` - New test (75 lines)
- `internal/consolidate/executor_test.go` - New test (57 lines)

**Impact:**
- ✅ Audit `--execute` now works across mount points
- ✅ Consolidate operations handle scattered series on different drives
- ✅ Automatic fallback to rsync/native backends with timeout protection

---

### Bug Fix 2: AI Context Enhancement (Task 3)

**Problem:** AI received only filename basename (e.g., "pb.s04e15.mkv") with zero context, causing hallucinations:
- "pb" → "History's Greatest Mysteries" ❌ (should be "Prison Break")
- Files in MOVIES folder suggested as TV episodes
- Generic abbreviations matched to wrong shows

**Solution:** Enhanced AI prompt to include:
1. **Library Type** ("movie library" vs "TV show library")
2. **Folder Path** (parent directory name)
3. **Current Metadata** (existing parse + confidence score)

**Files Modified:**
- `internal/ai/matcher.go` - Added `ParseWithContext()` method (32 lines)
- `internal/ai/matcher.go` - Updated system prompt with context usage guide
- `cmd/jellywatch/audit_cmd.go:170` - Pass library context to AI
- `internal/ai/matcher_test.go` - Updated 29 tests to use new method

**Impact:**
- ✅ AI now considers folder structure (e.g., "Prison Break/Season 4/")
- ✅ Library type prevents cross-contamination (movies vs TV)
- ✅ Existing metadata used as hints for disambiguation
- ✅ Backward compatible - old `Parse()` method still available

---

### Testing (Task 4)

**New Integration Tests:**
- `tests/integration/cross_device_test.go` (92 lines)
  - `TestAuditMove_CrossDevice` - Full audit rename flow
  - `TestTransfer_BackendAvailability` - Backend verification

**Test Results:**
```
✅ internal/plans          - 10 tests pass
✅ internal/consolidate    - 8 tests pass  
✅ internal/ai             - 29 tests pass
✅ tests/integration       - 2 tests pass
```

---

### Documentation (Task 5)

**New Documentation:**
- `docs/ai-context.md` (84 lines) - Complete AI context guide
  - Before/after examples showing hallucination fixes
  - Configuration guide
  - Troubleshooting section

**Updated Documentation:**
- `README.md` - Added "AI Context" section explaining audit command enhancements

---

## Commits

```
660a218 docs: document AI context enhancement and cross-device fixes
c9044fe test: add integration tests for cross-device file moves
de76e27 feat(ai): enhance prompt with library context to reduce hallucinations
c63ca41 fix(consolidate): use transfer.Move() for cross-device support
97aa565 fix(plans): use transfer.Move() for cross-device support in audit rename
```

**Total Changes:**
- 10 files modified
- +572 lines added
- -65 lines removed
- 5 semantic commits

---

## Remaining Issues (Out of Scope)

During implementation, we identified additional issues not addressed in this plan:

### Issue 3: Permission-Denied Deletions

**Problem:** When JellyWatch runs as user A but files are owned by user B (e.g., jellyfin), `os.Remove()` fails with "permission denied".

**Location:**
- `cmd/jellywatch/duplicates_cmd.go:64` - Duplicate deletion
- `internal/plans/plans.go:435` - Audit file deletion

**Symptoms:**
- User had to manually `chown` files before duplicate deletion worked
- Error: "Failed to delete /mnt/STORAGE5/MOVIES/...mkv: permission denied"

**Impact:** Moderate - Workaround exists (manual chown) but poor UX

**Recommendation:** Separate fix required - complex because:
- Permissions are per-installation configured
- Can't assume sudo/chown access
- Need to check config `[permissions]` section and apply before delete
- Or provide better error messages guiding user to fix permissions

### Issue 4: Database Inconsistency on Delete Failure

**Problem:** `duplicates_cmd.go` doesn't remove from database when filesystem delete fails, leaving ghost entries.

**Location:** `cmd/jellywatch/duplicates_cmd.go:64-71`

**Impact:** Low - Database becomes inconsistent but rescan fixes it

**Recommendation:** Add database cleanup on delete failure or mark as "delete failed" status

---

## Verification Checklist

- [x] All unit tests pass
- [x] Integration tests pass
- [x] Code uses transfer.Move() in both locations (plans.go, consolidate/executor.go)
- [x] AI prompt includes library context
- [x] Documentation updated (README.md, ai-context.md)
- [x] No new lint errors
- [x] All 5 tasks committed with semantic messages

---

## Success Criteria Met

✅ Cross-device rename errors eliminated  
✅ AI hallucinations reduced via context  
✅ Comprehensive tests added  
✅ Documentation complete  
✅ All tests passing  

**Implementation Plan:** docs/plans/2026-01-31-fix-audit-critical-bugs.md  
**Execution Time:** ~18 minutes (5 tasks via subagent-driven development)
