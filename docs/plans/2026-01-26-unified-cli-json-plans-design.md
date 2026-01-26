# Unified CLI Commands with JSON Plans

**Date:** 2026-01-26
**Status:** Approved

## Overview

Both `duplicates` and `consolidate` commands use identical workflow with JSON file storage for plans. This keeps the database clean (source of truth for library state only) while providing a consistent user experience.

## Workflow

Both commands follow the same pattern:

```
jellywatch <cmd>              # Show analysis (no files created)
jellywatch <cmd> --generate   # Create plans JSON file
jellywatch <cmd> --dry-run    # Preview plans from JSON
jellywatch <cmd> --execute    # Execute plans, delete JSON on success
```

## File Storage

Plans are stored as JSON files:

```
~/.config/jellywatch/plans/
  duplicates.json
  consolidate.json
```

**Lifecycle:**
- `--generate` always overwrites the file (no buildup)
- `--execute` deletes the file after successful execution
- `--dry-run` reads but doesn't modify
- If file is missing/corrupt, commands say "No plans found. Run --generate first."

## JSON Structure

### duplicates.json

```json
{
  "created_at": "2026-01-26T19:00:00Z",
  "command": "duplicates",
  "summary": {
    "total_groups": 12,
    "files_to_delete": 15,
    "space_reclaimable": 45000000000
  },
  "plans": [
    {
      "group_id": "abc123",
      "title": "Robots",
      "year": 2005,
      "media_type": "movie",
      "season": null,
      "episode": null,
      "keep": {
        "id": 1,
        "path": "/mnt/STORAGE5/MOVIES/Robots (2005)/Robots.mkv",
        "size": 4300000000,
        "quality_score": 284,
        "resolution": "720p",
        "source_type": "BluRay"
      },
      "delete": {
        "id": 2,
        "path": "/mnt/STORAGE1/MOVIES/Robots (2005)/Robots.mkv",
        "size": 4400000000,
        "quality_score": 84,
        "resolution": "unknown",
        "source_type": "unknown"
      }
    }
  ]
}
```

### consolidate.json

```json
{
  "created_at": "2026-01-26T19:00:00Z",
  "command": "consolidate",
  "summary": {
    "total_conflicts": 36,
    "total_moves": 150,
    "total_bytes": 265000000000
  },
  "plans": [
    {
      "conflict_id": 1,
      "title": "American Dad",
      "year": 2005,
      "media_type": "series",
      "target_location": "/mnt/STORAGE7/TVSHOWS/American Dad! (2005)",
      "operations": [
        {
          "action": "move",
          "source_path": "/mnt/STORAGE8/TVSHOWS/American Dad (2005)/S01E01.mkv",
          "target_path": "/mnt/STORAGE7/TVSHOWS/American Dad! (2005)/S01E01.mkv",
          "size": 500000000
        }
      ]
    }
  ]
}
```

## Implementation

### New Package: internal/plans/

```go
package plans

type DuplicatePlan struct {
    CreatedAt time.Time
    Command   string
    Summary   DuplicateSummary
    Plans     []DuplicateGroup
}

type ConsolidatePlan struct {
    CreatedAt time.Time
    Command   string
    Summary   ConsolidateSummary
    Plans     []ConsolidateGroup
}

func LoadDuplicatePlans() (*DuplicatePlan, error)
func SaveDuplicatePlans(plan *DuplicatePlan) error
func DeleteDuplicatePlans() error

func LoadConsolidatePlans() (*ConsolidatePlan, error)
func SaveConsolidatePlans(plan *ConsolidatePlan) error
func DeleteConsolidatePlans() error

func GetPlansDir() string  // ~/.config/jellywatch/plans/
```

### Command Changes

**duplicates_cmd.go:**
- Add `--generate` flag
- `--generate`: Analyze duplicates, save to JSON
- `--dry-run`: Load JSON, display plans
- `--execute`: Load JSON, execute deletions, delete JSON on success
- No flags: Show analysis only (current behavior)

**consolidate_cmd.go:**
- Keep `--generate` flag
- Remove database table usage
- Use JSON file instead
- Same pattern as duplicates

### Database Changes

- Remove usage of `consolidation_plans` table in consolidate command
- Table can remain for backwards compatibility (or remove in future cleanup)
- No new migrations needed

### Wizard Integration

The fix wizard will:
1. Call the same analysis functions
2. For "apply all", save to JSON then execute
3. For interactive mode, execute one at a time without JSON

## Migration Path

1. Implement `internal/plans/` package
2. Update `duplicates` command
3. Update `consolidate` command (switch from DB to JSON)
4. Update wizard to use JSON when appropriate
5. (Optional) Remove `consolidation_plans` table usage entirely
