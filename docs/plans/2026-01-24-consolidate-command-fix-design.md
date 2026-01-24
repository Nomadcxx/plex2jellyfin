# Consolidate Command Fix Design

## Problem Statement

The current `jellywatch consolidate --generate` command only creates DELETE plans for duplicate files, but during scanning, 46 cross-drive conflicts were identified that require MOVE plans to consolidate split libraries across different drives.

This limitation prevents effective library consolidation when media files exist in multiple locations across different drives or storage devices.

## Current Behavior

The `runGeneratePlans()` function in `cmd/jellywatch/consolidate_cmd.go` currently:

1. Creates a Planner instance
2. Calls `planner.GeneratePlans(ctx)` which only generates DELETE plans for duplicates
3. Reports statistics showing only delete plans

As a result, the cross-drive conflicts that require MOVE operations are not addressed.

## Proposed Solution

Modify `runGeneratePlans()` to utilize the existing Consolidator functionality which can generate MOVE plans for cross-drive consolidation:

1. Replace the call to `planner.GeneratePlans(ctx)` with calls to `consolidator.GenerateAllPlans()`
2. Store these plans in the database using the existing Consolidator.StorePlan() method
3. Maintain backward compatibility with existing reporting functions

## Detailed Changes

### 1. Core Modification: runGeneratePlans() function

Replace the existing implementation with one that:
- Creates a Consolidator instance instead of just a Planner
- Calls `consolidator.GenerateAllPlans()` to get all plans including MOVE operations
- Stores each generated plan in the database for execution tracking
- Updates the reporting logic to properly count MOVE plans alongside DELETE plans

### 2. Command Help Text Update

Update the command help text to accurately reflect that the command:
- Generates both DELETE and MOVE plans for complete consolidation
- Handles cross-drive conflicts in addition to simple duplicates

### 3. Statistics Reporting Enhancement

Enhance the console output to correctly categorize and report:
- Delete plans (inferior duplicates)
- Move plans (cross-drive conflicts)
- Rename plans (Jellyfin compliance)

## Implementation Approach

### Minimal Changes Strategy

Since the Consolidator class already has the functionality we need:

1. Import the config package in `consolidate_cmd.go` (already has database)
2. Instantiate Consolidator alongside Planner in `runGeneratePlans()`
3. Call `consolidator.GenerateAllPlans()` instead of `planner.GeneratePlans()`
4. Iterate through generated plans and store them using `consolidator.StorePlan()`
5. Update statistics collection to include MOVE plans from the stored database records
6. Update help text in the command definition

### Why This Approach Works

- No changes needed to existing Planner, Consolidator, or Executor classes
- Leverages existing database schema for consolidation_plans table which already supports "move" actions
- Uses existing Consolidator methods that are already tested and functional
- Maintains all existing functionality while extending to cross-drive scenarios

## Technical Details

### File Affected
- `cmd/jellywatch/consolidate_cmd.go`: Primary file requiring modification

### Key Functions Modified
- `runGeneratePlans()`: Main implementation change
- Command help text in `newConsolidateCmd()`: Documentation update

### No Changes Required To
- `internal/consolidate/planner.go`: Planner class unchanged
- `internal/consolidate/consolidate.go`: Consolidator class unchanged
- `internal/consolidate/executor.go`: Executor already handles "move" actions
- Database schema: Already supports "move" action type in consolidation_plans table

## Impact Assessment

### Positive Impacts
- Enables consolidation of split libraries across multiple drives
- Completes the CONDOR system's intended functionality for cross-drive conflict resolution
- Provides comprehensive library optimization including space consolidation

### Risk Mitigation
- Low risk as we're using existing, tested code paths
- Dry-run capability allows preview before actual execution
- Existing DELETE plan functionality remains intact
- All changes are confined to a single function with clear boundaries

## Expected Outcome

After implementing this change, the `jellywatch consolidate --generate` command will:
- Detect and process 46 cross-drive conflicts identified in scans
- Generate appropriate MOVE plans for consolidating split libraries
- Report accurate statistics for all plan types (DELETE, MOVE, RENAME)
- Enable effective execution of cross-drive consolidation when `--execute` is used

This completes the full CONDOR consolidation workflow as originally designed, supporting both intra-drive duplicate elimination and inter-drive library consolidation.