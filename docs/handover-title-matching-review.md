# Implementation Review: Title Matching & Season Normalization Fixes

## Context
This is a review of the implementation of 6 tasks to fix JellyWatch's media library organization bugs that were causing duplicate/nested folders and inconsistent season numbering.

**Original Plan:** `docs/plans/2026-03-18-title-matching-and-season-normalization.md`

## Commits to Review
Please review these 5 commits in order:
1. `c22f370` - feat: add NormalizeForMatch() for directory matching
2. `5cf548e` - fix: use NormalizeForMatch in findExistingShowDir  
3. `f77572c` - fix: remove overly-broad release pattern
4. `659cf3b` - feat: add findExistingSeasonDir to reuse existing season folders
5. `93e4cd5` - feat: add ExtractYearFlexible for bare year support

## What Was Implemented

### Task 1: NormalizeForMatch()
- **Location:** `internal/database/normalize.go:42-57`
- **Should add:** `NormalizeForMatch()` function that strips all punctuation for directory matching
- **Should create:** `internal/database/normalize_test.go` with tests

### Task 2: Fix findExistingShowDir() functions
- **Locations:** 
  - `internal/organizer/organizer.go:968-996`
  - `internal/library/selector.go:417-440`
- **Should do:** Both functions now use `database.NormalizeForMatch()` for comparison
- **Should have:** Tests in `organizer_test.go` and `selector_test.go`

### Task 3: Remove dangerous release pattern
- **Location:** `internal/naming/naming.go:54` (was line 54, now removed)
- **Should remove:** Pattern `\b[A-Z]{2,5}\d*$` from `releasePatterns`
- **Should have:** Test in `naming_test.go`

### Task 4: Add findExistingSeasonDir()
- **Location:** `internal/organizer/organizer.go` (appended at end of file)
- **Should add:** `findExistingSeasonDir()` function
- **Should wire into:** 
  - `OrganizeTVEpisode()` (line ~614)
  - `OrganizeTVWithParsed()` (line ~758)
- **Should have:** Test in `organizer_test.go`

### Task 5: Add ExtractYearFlexible()
- **Location:** `internal/database/normalize.go:72-95`
- **Should add:** `ExtractYearFlexible()` function
- **Should use in:** `internal/sync/filesystem.go:parseMediaDir()`
- **Should have:** Test in `normalize_test.go`

## Review Checklist

### Completeness
- [ ] All 6 tasks from the plan are implemented
- [ ] No missing requirements from the spec
- [ ] No extra/unneeded features added

### Code Quality
- [ ] Code follows existing patterns in the codebase
- [ ] Function names are clear and accurate
- [ ] No obvious bugs or edge cases missed
- [ ] Comments explain the "why" not just "what"

### Testing
- [ ] All new functions have tests
- [ ] Tests actually verify behavior (not just mock behavior)
- [ ] Tests cover edge cases
- [ ] All tests pass: `go test ./internal/database/... ./internal/organizer/... ./internal/library/... ./internal/naming/... ./internal/sync/... -v`

### Integration
- [ ] No import cycles introduced
- [ ] Build passes: `go build ./...`
- [ ] No regressions in existing functionality

## Known Issues (Pre-existing, NOT part of this review)
- `internal/database/media_files_low_confidence_test.go` has compilation errors (wrong arity on `GetLowConfidenceFiles`, missing `CountLowConfidenceFiles`) - these predate this work and were temporarily moved aside during testing

## Report Format
Please provide:
1. **Summary** - Overall assessment (approved / needs fixes)
2. **Issues Found** - List any problems with file:line references
3. **Strengths** - What was done well
4. **Recommendations** - Any optional improvements
