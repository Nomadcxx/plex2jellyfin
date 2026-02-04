# Consolidation TV-Only Design

**Date:** 2026-01-24
**Status:** Design Complete

## Problem

The `jellywatch consolidate` command currently processes ALL conflicts detected by `DetectConflicts()`, including both TV show conflicts AND movie conflicts. However, consolidation (cross-drive merging of spread media) should only apply to TV series. Movies should be handled by the `jellywatch duplicates` command for quality-based deletion.

**User Intent:** Duplicate movies should be DELETED (higher quality wins), not MOVED (cross-drive consolidation). Only TV series spread across drives should be consolidated to a single location.

## Current Architecture

```
DetectConflicts() → Returns [TV conflicts + Movie conflicts]
                 ↓
Consolidator.GenerateAllPlans()
                 ↓
Generates MOVE plans for ALL conflicts (bug: includes movies)
```

## Solution Design

### Core Logic Change

**Filter movie conflicts before generating consolidation plans:**

```go
func (c *Consolidator) GenerateAllPlans() ([]*Plan, error) {
    conflicts, err := c.db.DetectConflicts()
    // ...

    for _, conflict := range conflicts {
        // Skip movie conflicts - consolidation only applies to TV series
        if conflict.MediaType != "series" {
            c.stats.SkippedConflicts++
            continue
        }
        // Generate MOVE plan for TV series
        plan, err := c.GeneratePlan(&conflict)
        // ...
    }
    return plans, nil
}
```

### Stats Tracking

Add `SkippedConflicts` field to `Stats` struct:

```go
type Stats struct {
    ConflictsFound   int
    SkippedConflicts int // Conflicts skipped (e.g., movies in consolidation mode)
    PlansGenerated   int
    FilesMoved       int
    BytesMoved       int64
    StartTime        time.Time
    EndTime          time.Time
}
```

### CLI Messaging

**Before:**
```
Conflicts analyzed: 46
Consolidation opportunities: 46
Move operations: 336
Space to reclaim: 331.8 GB
```

**After:**
```
=== TV Show Consolidation Plan ===

Conflicts analyzed: 46
├─ TV show conflicts: 34
└─ Movie conflicts skipped: 12 (use 'jellywatch duplicates' for quality conflicts)

Consolidation opportunities: 34
Move operations: 336
Space to reclaim: 331.8 GB

Generated 34 consolidation plans for TV series across multiple drives
```

## Files to Modify

1. **`internal/consolidate/consolidate.go`**
   - Add `SkippedConflicts int` to `Stats` struct

2. **`internal/consolidate/operations.go`**
   - Add filter in `GenerateAllPlans()` to skip `MediaType != "series"`
   - Increment `c.stats.SkippedConflicts` for skipped conflicts

3. **`cmd/jellywatch/consolidate_cmd.go`**
   - Update `runGeneratePlans()` to show TV/movie breakdown
   - Add helpful message directing users to `duplicates` command for movies
   - Update `runExecutePlans()` for clearer execution feedback

4. **`README.md`**
   - Clarify consolidation applies to TV series only
   - Update command descriptions to reflect proper usage

## No Changes Required

- Database layer (`conflicts.go`, `DetectConflicts()`) - keep it working correctly
- Existing test files - add new tests, don't modify existing ones

## Testing Strategy

### Unit Tests

1. **TestConsolidator_GenerateAllPlans_ExcludesMovies**
   - Mock: 5 conflicts (3 TV, 2 movies)
   - Verify: 3 plans generated, SkippedConflicts = 2

2. **TestConsolidator_GenerateAllPlans_AllMovies**
   - Mock: 5 movie conflicts, 0 TV
   - Verify: 0 plans, SkippedConflicts = 5

3. **TestConsolidator_GenerateAllPlans_NoConflicts**
   - Mock: empty conflicts list
   - Verify: 0 plans, 0 skipped

4. **TestStats_SkippedConflicts_Field**
   - Verify Stats struct includes SkippedConflicts field

### Integration Tests

- Run `go test ./...` after all changes
- Verify no existing tests break

### Manual Testing

- Run `jellywatch consolidate --generate` on library with mixed conflicts
- Verify movies excluded, TV shows processed
- Verify CLI messaging shows correct counts

## Edge Cases

1. **No TV conflicts detected**
   - Output: "No TV show conflicts found. All 46 conflicts are movies. Use 'jellywatch duplicates' for quality conflict resolution."

2. **All conflicts filtered out**
   - Output: "0 consolidation plans generated (all conflicts were movies)"

3. **TV conflicts with no eligible files**
   - Existing `CanProceed` logic handles this
   - Output: "Show Name (Year): Skipped - no files >100MB to move"

## Implementation Order

**Phase 1: Core Logic**
1. Add `SkippedConflicts` to Stats struct
2. Add filter in `GenerateAllPlans()`
3. Run existing tests (verify no regressions)

**Phase 2: CLI Messaging**
4. Update `runGeneratePlans()` messaging
5. Update `runExecutePlans()` feedback
6. Manual CLI testing

**Phase 3: Documentation**
7. Update README
8. Commit all changes

## Design Principles Applied

- **Minimal changes** - Single filter, no schema changes
- **Clear separation** - Consolidation (TV) vs Duplicates (movies + TV)
- **Transparent feedback** - Users see what's processed vs skipped
- **No breaking changes** - Database layer unchanged
- **YAGNI** - Only add what's needed to fix the bug

## Success Criteria

- [x] Consolidation command only processes TV series
- [x] CLI messaging clearly shows TV vs movie breakdown
- [x] Users directed to `duplicates` command for movies
- [x] All existing tests pass
- [x] New unit tests cover filtering behavior
- [x] README updated with accurate usage
