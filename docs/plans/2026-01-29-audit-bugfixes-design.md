# Audit Command Bug Fixes Design

**Date:** 2026-01-29
**Status:** Approved
**Author:** AI Assistant

## Problem Statement

The `jellywatch audit --generate` command has three critical bugs and lacks media type validation:

1. **Bug #1 - Dead Code** (lines 147-150): The `mediaType` variable is assigned but never used. The logic is also backwards (swaps movie to tv).

2. **Bug #2 - Hardcoded Summary Counts** (lines 192-195): `FilesToRename: 0` and `FilesToSkip: len(files)` are hardcoded instead of calculated from actual actions generated.

3. **Bug #3 - No Progress Output** (lines 144-186): Processing 100+ files with AI calls takes minutes but produces no progress indication.

4. **Missing Feature - Media Type Validation**: No validation that AI-suggested type matches the library context. Could create invalid paths (movie with season/episode fields).

## Design Decisions

### Media Type Validation Strategy
- **Primary**: Use `library_root` path analysis (if path contains "TV"/"shows" = episode, "Movies"/"film" = movie)
- **Secondary**: Query Sonarr/Radarr APIs when available to validate title exists
- **Conflict**: If library_root and API disagree, skip and inform user

### Progress Output
- Interactive progress bar with batch updates every 10 files
- Format: `████████░░░░░░░░░░░░ 40% (40/100 files) | 12 actions created`
- Final summary with detailed statistics

### Dead Code
- Remove completely (lines 147-150)

### Plan Structure Enhancement
- Populate existing `Actions` field
- Add `SkipReason` field to `AuditItem`
- Expand `AuditSummary` with AI statistics

## Architecture

### Components Modified

**1. generateAudit() function** (cmd/jellywatch/audit_cmd.go)
- Adds progress tracking with batch counter
- Integrates type validation before creating actions
- Populates Actions field and enhanced statistics
- Removes dead code

**2. New validation helper: validateMediaType()**
- Takes: file, aiResult, cfg (for Sonarr/Radarr access)
- Returns: validation result (valid/invalid) and reason string
- Primary check: library_root path analysis
- Secondary check: Sonarr/Radarr API query (if enabled)

**3. Enhanced AuditPlan structure** (internal/plans/plans.go)
- Actions field: populated with all generated rename actions
- AuditItem.SkipReason: new optional field for skipped items
- AuditSummary expansion: AITotalCalls, AISuccessRate, TypeMismatchesSkipped

### Data Flow

```
fetch low-confidence files (DB)
  |
initialize AI matcher
  |
for each file:
  update progress counter (every 10th)
  |
  call AI matcher Parse()
  |
  if AI error -> log and continue
  |
  if AI confidence < threshold -> skip
  |
  validateMediaType(file, aiResult, cfg)
  |
  if invalid type match -> mark item with SkipReason
  |
  if valid -> build rename action
  |
calculate summary statistics
  |
save plan with Actions populated
```

## Media Type Validation Logic

### Primary: Library Root Path Analysis

```go
func inferTypeFromLibraryRoot(libraryRoot string) string {
    lower := strings.ToLower(libraryRoot)
    // Check for TV indicators
    if strings.Contains(lower, "tv") || 
       strings.Contains(lower, "series") || 
       strings.Contains(lower, "shows") {
        return "episode"
    }
    // Check for movie indicators
    if strings.Contains(lower, "movie") || 
       strings.Contains(lower, "film") {
        return "movie"
    }
    return "unknown"  // Can't determine from path
}
```

### Secondary: Sonarr/Radarr API Check

When APIs are configured and enabled:
1. **Sonarr check**: `client.FindSeriesByTitle(title)` - returns matches if known TV series
2. **Radarr check**: Similar lookup for movies

### Conflict Resolution Matrix

| Library Root | AI Type | Sonarr Match | Radarr Match | Result |
|--------------|---------|--------------|--------------|--------|
| TV folder | episode | - | - | Valid |
| TV folder | movie | - | - | Skip: "AI suggests movie but file is in TV library" |
| Movie folder | movie | - | - | Valid |
| Movie folder | episode | - | - | Skip: "AI suggests TV but file is in Movies library" |
| Unknown | episode | Yes | No | Valid (API confirms) |
| Unknown | movie | No | Yes | Valid (API confirms) |
| Unknown | movie | Yes | Yes | Skip: "Ambiguous - found in both Sonarr and Radarr" |
| Unknown | * | No | No | Valid (trust AI, no API contradiction) |

## Progress Indication

### Display Format

```
Scanning for files with confidence < 0.80
Found 100 low-confidence files

Processing with AI...
████████░░░░░░░░░░░░ 40% (40/100 files) | 12 actions created
```

### Final Summary

```
Processing complete

Summary:
  Total files analyzed: 100
  AI calls made: 100 (98 successful, 2 errors)
  Actions created: 23 (renames)
  Skipped: 77
    - AI confidence too low: 45
    - Type validation failed: 8
    - Title unchanged: 24

Plan saved to: ~/.config/jellywatch/plans/audit.json
Run 'jellywatch audit --dry-run' to preview changes
```

## Error Handling

| Error Type | Handling |
|------------|----------|
| AI matcher init fails | Return error immediately |
| Individual AI Parse() error | Log warning, increment error count, continue |
| Sonarr/Radarr API timeout | Log warning, fall back to library_root only |
| API returns error | Log warning, fall back to library_root only |
| All files processed, 0 actions | Normal completion, inform user |

## Files Changed

### Modified

1. **cmd/jellywatch/audit_cmd.go**
   - Remove dead code (lines 147-150)
   - Add progress tracking with batch updates
   - Integrate type validation before action creation
   - Populate Actions field in plan
   - Fix summary counts (FilesToRename, FilesToSkip)
   - Add enhanced summary output

2. **internal/plans/plans.go**
   - Add `SkipReason` field to `AuditItem` struct
   - Expand `AuditSummary` struct with new fields

### New

3. **cmd/jellywatch/audit_validation.go** (or add to audit_cmd.go)
   - `inferTypeFromLibraryRoot(libraryRoot string) string`
   - `validateMediaType(file, aiResult, cfg) (bool, string)`
   - Helper for Sonarr/Radarr API checks

### Unchanged

- `internal/ai/matcher.go` - AI interface unchanged
- `internal/sonarr/` - Using existing `FindSeriesByTitle()` method
- `internal/radarr/` - Using existing lookup methods

## Testing Strategy

1. Unit test `inferTypeFromLibraryRoot()` with various path patterns
2. Unit test `validateMediaType()` with mocked API responses
3. Integration test with real DB: `jellywatch audit --generate --limit 5`
4. Verify progress bar displays correctly
5. Verify plan JSON contains Actions and enhanced summary

## Implementation Order

1. Remove dead code (lines 147-150)
2. Add progress tracking to loop
3. Implement `inferTypeFromLibraryRoot()`
4. Implement `validateMediaType()` with Sonarr/Radarr integration
5. Update AuditItem and AuditSummary structs
6. Fix summary counts and populate Actions field
7. Add enhanced summary output
8. Test with real database
