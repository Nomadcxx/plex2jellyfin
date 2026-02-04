# Consolidation TV-Only Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix consolidation command to only process TV series conflicts, excluding movie conflicts from consolidation plans.

**Architecture:** Add MediaType filter in Consolidator.GenerateAllPlans() to skip movie conflicts, track skipped conflicts in Stats struct, update CLI messaging to show TV vs movie breakdown.

**Tech Stack:** Go 1.21+, SQLite, Cobra CLI

---

## Task 1: Add SkippedConflicts Field to Stats Struct

**Files:**
- Modify: `internal/consolidate/consolidate.go:26-33`

**Step 1: Write failing test**

```go
// Add to internal/consolidate/consolidate_test.go
func TestStats_SkippedConflicts_Field(t *testing.T) {
    stats := Stats{}
    if stats.SkippedConflicts != 0 {
        t.Errorf("Expected SkippedConflicts to default to 0, got %d", stats.SkippedConflicts)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/consolidate/... -run TestStats_SkippedConflicts_Field -v`
Expected: FAIL with "unknown field SkippedConflicts" or similar

**Step 3: Write minimal implementation**

Edit `internal/consolidate/consolidate.go`, modify the Stats struct:

```go
// Stats tracks consolidation statistics
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

**Step 4: Run test to verify it passes**

Run: `go test ./internal/consolidate/... -run TestStats_SkippedConflicts_Field -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/consolidate/consolidate.go internal/consolidate/consolidate_test.go
git commit -m "feat: add SkippedConflicts field to Stats struct"
```

---

## Task 2: Add Filter to Exclude Movie Conflicts

**Files:**
- Modify: `internal/consolidate/operations.go:102-125`
- Test: `internal/consolidate/operations_test.go`

**Step 1: Write failing test**

```go
// Add to internal/consolidate/operations_test.go
func TestConsolidator_GenerateAllPlans_ExcludesMovies(t *testing.T) {
    // Setup mock database with mixed conflicts
    mockDB := &database.MediaDB{}
    // In real test, use a mock that returns 3 TV conflicts and 2 movie conflicts
    // For now, we'll add a placeholder test

    c := NewConsolidator(mockDB, &config.Config{})

    plans, err := c.GenerateAllPlans()
    if err != nil {
        t.Fatalf("GenerateAllPlans failed: %v", err)
    }

    // Should only generate plans for TV shows, not movies
    if len(plans) != 3 {
        t.Errorf("Expected 3 plans (TV shows only), got %d", len(plans))
    }

    if c.stats.SkippedConflicts != 2 {
        t.Errorf("Expected 2 skipped conflicts (movies), got %d", c.stats.SkippedConflicts)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/consolidate/... -run TestConsolidator_GenerateAllPlans_ExcludesMovies -v`
Expected: FAIL (test will fail because we haven't added the filter yet, or because mock doesn't exist)

**Step 3: Write minimal implementation**

Edit `internal/consolidate/operations.go`, modify GenerateAllPlans() method:

```go
// GenerateAllPlans generates consolidation plans for all conflicts
func (c *Consolidator) GenerateAllPlans() ([]*Plan, error) {
    // Detect conflicts first
    conflicts, err := c.db.DetectConflicts()
    if err != nil {
        return nil, fmt.Errorf("failed to detect conflicts: %w", err)
    }

    c.stats.ConflictsFound = len(conflicts)

    var plans []*Plan
    for _, conflict := range conflicts {
        // Skip movie conflicts - consolidation only applies to TV series
        if conflict.MediaType != "series" {
            c.stats.SkippedConflicts++
            continue
        }

        plan, err := c.GeneratePlan(&conflict)
        if err != nil {
            fmt.Printf("Warning: Failed to generate plan for conflict %d (%s): %v\n",
                conflict.ID, conflict.Title, err)
            continue
        }

        plans = append(plans, plan)
    }

    return plans, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/consolidate/... -run TestConsolidator_GenerateAllPlans_ExcludesMovies -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/consolidate/operations.go internal/consolidate/operations_test.go
git commit -m "feat: filter out movie conflicts from consolidation plans"
```

---

## Task 3: Test All-Movies Edge Case

**Files:**
- Test: `internal/consolidate/operations_test.go`

**Step 1: Write failing test**

```go
func TestConsolidator_GenerateAllPlans_AllMovies(t *testing.T) {
    // Setup mock database with only movie conflicts
    mockDB := &database.MediaDB{}
    // Mock returns 5 movie conflicts, 0 TV conflicts

    c := NewConsolidator(mockDB, &config.Config{})

    plans, err := c.GenerateAllPlans()
    if err != nil {
        t.Fatalf("GenerateAllPlans failed: %v", err)
    }

    if len(plans) != 0 {
        t.Errorf("Expected 0 plans (all movies), got %d", len(plans))
    }

    if c.stats.SkippedConflicts != 5 {
        t.Errorf("Expected 5 skipped conflicts (all movies), got %d", c.stats.SkippedConflicts)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/consolidate/... -run TestConsolidator_GenerateAllPlans_AllMovies -v`
Expected: FAIL (until mock is properly implemented)

**Step 3: Verify implementation handles edge case**

The filter in GenerateAllPlans() already handles this case (no code changes needed). Just ensure mock is set up correctly.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/consolidate/... -run TestConsolidator_GenerateAllPlans_AllMovies -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/consolidate/operations_test.go
git commit -m "test: add all-movies edge case test"
```

---

## Task 4: Run Existing Tests to Verify No Regressions

**Files:**
- All consolidate package tests

**Step 1: Run all consolidate tests**

Run: `go test ./internal/consolidate/... -v`
Expected: All tests PASS

**Step 2: Run all project tests**

Run: `go test ./...`
Expected: All tests PASS

**Step 3: Note any failures**

If any tests fail, debug and fix before proceeding. Document failures in commit message if they were pre-existing.

**Step 4: Commit test run verification (no code changes)**

```bash
git commit --allow-empty -m "test: verified no regressions in consolidate package"
```

---

## Task 5: Update CLI Messaging - runGeneratePlans()

**Files:**
- Modify: `cmd/jellywatch/consolidate_cmd.go`

**Step 1: Examine current runGeneratePlans() implementation**

Run: `grep -A 50 "func runGeneratePlans" cmd/jellywatch/consolidate_cmd.go`
Note the current output format and logic.

**Step 2: Update messaging to show TV vs movie breakdown**

Locate the section that outputs statistics. Replace or enhance it with:

```go
// After generating plans, update output
fmt.Printf("=== TV Show Consolidation Plan ===\n\n")
fmt.Printf("Conflicts analyzed: %d\n", stats.ConflictsFound)
fmt.Printf("├─ TV show conflicts: %d\n", len(plans))
fmt.Printf("└─ Movie conflicts skipped: %d (use 'jellywatch duplicates' for quality conflicts)\n",
    stats.SkippedConflicts)

if len(plans) > 0 {
    fmt.Printf("\nConsolidation opportunities: %d\n", len(plans))

    var totalOps int
    var totalBytes int64
    for _, plan := range plans {
        totalOps += plan.TotalFiles
        totalBytes += plan.TotalBytes
    }

    fmt.Printf("Move operations: %d\n", totalOps)
    fmt.Printf("Space to reclaim: %s\n", formatBytes(totalBytes))

    fmt.Printf("\nGenerated %d consolidation plans for TV series across multiple drives\n", len(plans))
} else {
    fmt.Printf("\nNo TV show conflicts found")
    if stats.SkippedConflicts > 0 {
        fmt.Printf(". All %d conflicts are movies. Use 'jellywatch duplicates' for quality conflict resolution.\n",
            stats.SkippedConflicts)
    } else {
        fmt.Printf(".\n")
    }
}
```

**Step 3: Test CLI output manually**

Run: `go build -o jellywatch ./cmd/jellywatch && ./jellywatch consolidate --generate`
Expected: Output shows TV vs movie breakdown clearly

**Step 4: Commit**

```bash
git add cmd/jellywatch/consolidate_cmd.go
git commit -m "feat: update CLI messaging to show TV vs movie conflict breakdown"
```

---

## Task 6: Update CLI Messaging - runExecutePlans()

**Files:**
- Modify: `cmd/jellywatch/consolidate_cmd.go`

**Step 1: Examine current runExecutePlans() implementation**

Run: `grep -A 30 "func runExecutePlans" cmd/jellywatch/consolidate_cmd.go`
Note current execution feedback format.

**Step 2: Add clearer execution feedback**

Update execution loop to show progress:

```go
// At the start of execute loop
if stats.SkippedConflicts > 0 {
    fmt.Printf("\nNote: Skipping %d movie conflicts (consolidation applies to TV series only)\n",
        stats.SkippedConflicts)
    fmt.Printf("Use 'jellywatch duplicates' for movie quality conflict resolution.\n\n")
}

// Enhance per-plan output
fmt.Printf("Processing: %s (%v)\n", plan.Title, plan.Year)
fmt.Printf("  Type: %s\n", plan.MediaType)
if !plan.CanProceed {
    fmt.Printf("  Status: Skipped - %v\n", plan.Reasons)
} else {
    fmt.Printf("  Status: Moving %d files (%s)\n", plan.TotalFiles, formatBytes(plan.TotalBytes))
}
```

**Step 3: Test CLI output manually**

Run: `./jellywatch consolidate --execute --dry-run`
Expected: Clear per-plan feedback showing type and action

**Step 4: Commit**

```bash
git add cmd/jellywatch/consolidate_cmd.go
git commit -m "feat: add clearer execution feedback in consolidate command"
```

---

## Task 7: Update README Documentation

**Files:**
- Modify: `README.md`

**Step 1: Locate consolidation command documentation**

Run: `grep -A 20 "Consolidation" README.md` or `grep -A 20 "consolidate" README.md`

**Step 2: Add clarification about consolidation scope**

Find the section describing the consolidate command. Add/update with:

```markdown
### TV Show Consolidation

The `consolidate` command merges TV series spread across multiple drives into a single location.

**Usage:**
```bash
jellywatch consolidate --generate  # Generate consolidation plans
jellywatch consolidate --execute    # Execute consolidation plans
jellywatch consolidate --dry-run    # Preview changes
```

**Important:** This command only processes **TV series conflicts**. For duplicate movies (quality conflicts), use the `duplicates` command.

**What it does:**
- Detects TV shows existing on multiple drives
- Identifies the drive with the most episodes as the target
- Creates MOVE plans to consolidate episodes from other drives
- Only processes files ≥100MB (skips samples, extras)
- Cleans up empty directories after successful moves

**Example output:**
```
=== TV Show Consolidation Plan ===

Conflicts analyzed: 46
├─ TV show conflicts: 34
└─ Movie conflicts skipped: 12 (use 'jellywatch duplicates' for quality conflicts)

Consolidation opportunities: 34
Move operations: 336
Space to reclaim: 331.8 GB
```
```

**Step 3: Verify README renders correctly**

Run: `head -100 README.md` or view in markdown previewer if available

**Step 4: Commit**

```bash
git add README.md
git commit -m "docs: clarify consolidation applies to TV series only"
```

---

## Task 8: Final Verification

**Files:**
- All modified files

**Step 1: Run all tests**

Run: `go test ./...`
Expected: All tests PASS

**Step 2: Build binaries**

Run: `go build ./cmd/jellywatch && go build ./cmd/jellywatchd`
Expected: Builds succeed with no errors

**Step 3: Check git status**

Run: `git status`
Expected: Clean working directory (all changes committed)

**Step 4: Verify git log**

Run: `git log --oneline -10`
Expected: See all commit messages from this implementation

**Step 5: Create summary commit**

```bash
git commit --allow-empty -m "feat: complete TV-only consolidation implementation

- Added SkippedConflicts field to Stats struct
- Filter out movie conflicts from consolidation plans
- Updated CLI messaging to show TV vs movie breakdown
- Added tests for filtering behavior
- Updated README to clarify consolidation scope

Fixes issue where consolidate command incorrectly attempted to process
movie conflicts. Movies should be handled by duplicates command for
quality-based deletion, not cross-drive consolidation."
```

---

## Testing Checklist

- [ ] All unit tests pass
- [ ] All integration tests pass
- [ ] `jellywatch consolidate --generate` shows correct TV/movie breakdown
- [ ] Movie conflicts are excluded from consolidation plans
- [ ] TV conflicts are still processed correctly
- [ ] CLI messaging is clear and helpful
- [ ] README documentation is accurate
- [ ] No regressions in existing functionality

---

## Dependencies

**Required Skills:**
- None (standalone implementation)

**Documentation to Reference:**
- Design document: `docs/plans/2026-01-24-consolidation-tv-only-design.md`
- Original design: `docs/plans/2025-01-24-consolidation-fix-design.md`

**Related Commands:**
- `jellywatch duplicates` - handles quality conflicts for movies and TV
- `jellywatch consolidate --generate` - generates TV series consolidation plans
- `jellywatch consolidate --execute` - executes consolidation plans

---

## Notes for Implementation

1. **Testing approach**: Use table-driven tests if multiple test cases are similar
2. **Mock database**: You may need to create a minimal mock for `database.MediaDB` to test the filtering behavior
3. **CLI formatting**: Ensure output is aligned and readable, use `fmt.Printf` with proper spacing
4. **Error messages**: Keep error messages specific and helpful for debugging
5. **Commit messages**: Follow conventional commits format (feat:, fix:, test:, docs:)

---

**Estimated Total Time:** 2-3 hours
**Estimated Lines Changed:** ~150 lines across 4 files
**Risk Level:** Low (filtering logic, no schema changes)
