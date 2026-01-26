# Fix Wizard and API Design

**Date:** 2026-01-26
**Status:** Approved

## Overview

This design addresses multiple issues with the current duplicate/consolidate workflow and introduces a unified interactive wizard with an OpenAPI-based API for future web UI integration.

### Problems Addressed

1. **SQL NULL error** - `consolidate --execute` crashes with "converting NULL to int64 is unsupported"
2. **Confusing scan output** - Everything shows as "Conflicts" without distinguishing duplicates from scattered media
3. **Inconsistent command interfaces** - `duplicates` is one-step, `consolidate` requires `--generate` first
4. **Missing guidance** - Scan doesn't tell users what to do next
5. **Broken dry-run** - Crashes due to SQL NULL bug before showing anything

## Design

### Command Structure

```
jellywatch scan          # Scan libraries, show summary with clear categories
jellywatch fix           # NEW: Interactive wizard to handle all issues
jellywatch duplicates    # Granular: handle duplicates only
jellywatch consolidate   # Granular: handle conflicts only (simplified)
```

### Scan Output

After scan completes, show categorized results:

```
=== Scan Complete ===
TV Series: 539 | Movies: 2880

=== Issues Found ===

📁 DUPLICATES (same content, different quality): 12 groups
   → These have inferior copies that can be DELETED to save 45 GB

🔀 SCATTERED MEDIA (same title in multiple locations): 36 items
   → These need files MOVED to consolidate into one folder

=== What's Next ===

1. Handle duplicates first (recommended):
   jellywatch fix              # Interactive wizard (handles all)
   jellywatch duplicates       # Review duplicates only

2. Then consolidate scattered media:
   jellywatch consolidate      # Review and organize
```

### The `fix` Command (Interactive Wizard)

**Flags:**
```
jellywatch fix [flags]
  --duplicates-only    Only handle duplicates
  --consolidate-only   Only handle consolidation
  --yes, -y            Auto-accept all suggestions (non-interactive)
  --dry-run            Show what would happen without changes
```

**Wizard Flow:**

```
🧹 Jellywatch Fix Wizard

Found 12 duplicate groups (45 GB reclaimable)
Found 36 scattered media items (need consolidation)

=== Step 1: Duplicates ===
Handle duplicates now? [Y/n]: y

[1/12] Robots (2005) - 2 copies found
  KEEP:   720p BluRay (4.3 GB) - /mnt/STORAGE5/MOVIES/Robots (2005)/...
  DELETE: unknown (4.4 GB)     - /mnt/STORAGE1/MOVIES/Robots (2005)/...

  Space saved: 4.4 GB

  [K]eep suggestion / [S]wap / [Skip] / [A]ll remaining: k

✅ Deleted 1 file, reclaimed 4.4 GB

=== Step 2: Consolidation ===
Handle scattered media now? [Y/n]: y

[1/36] American Dad (2005) - scattered across 2 locations
  Target: /mnt/STORAGE7/TVSHOWS/American Dad! (2005) (has most episodes)
  Moving: 12 files from /mnt/STORAGE8/TVSHOWS/American Dad (2005)

  [M]ove / [S]kip / [A]ll remaining: m

✅ Consolidated 36 items
```

**Interactive Options:**
- `[K]eep` - Accept the suggested action
- `[S]wap` - For duplicates: keep the other file instead
- `[Skip]` - Skip this item, move to next
- `[A]ll` - Accept suggestion for all remaining items
- `[Q]uit` - Exit wizard, keep remaining items unchanged

**Error Handling:**
- If a file operation fails: `[R]etry / [S]kip / [Q]uit`
- Summary at end shows successes and failures

### Bug Fixes

**1. SQL NULL Error**

Change `SourceFileID` from `int64` to `sql.NullInt64` in `internal/consolidate/planner.go`:

```go
type ConsolidationPlan struct {
    SourceFileID sql.NullInt64  // Changed from int64
    // ...
}
```

Update all `Scan()` calls and check `.Valid` before accessing value.

**2. Dry-Run**

- Consolidate: Already works once NULL bug is fixed
- Duplicates: Add `--dry-run` flag that shows what would be deleted without confirmation/action

**3. Unified Interfaces**

- Remove `--generate` from consolidate - generate plans on-the-fly like duplicates
- Both commands: show results, `--execute` to act, `--dry-run` to preview

## Architecture

### Service Layer

```
┌─────────────┐     ┌─────────────────┐     ┌──────────────┐
│   CLI       │────▶│  HTTP Server    │◀────│   Web UI     │
│  (fix cmd)  │     │  (OpenAPI)      │     │  (future)    │
└─────────────┘     └─────────────────┘     └──────────────┘
                            │
                    ┌───────┴───────┐
                    │   Service     │
                    │   Layer       │
                    └───────┬───────┘
                            │
                    ┌───────┴───────┐
                    │   Database    │
                    └───────────────┘
```

### OpenAPI Specification

```yaml
openapi: 3.1.0
info:
  title: Jellywatch API
  version: 1.0.0

paths:
  /api/v1/duplicates:
    get:
      summary: Get duplicate analysis
      responses:
        200:
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/DuplicateAnalysis'

  /api/v1/duplicates/{groupId}/delete:
    post:
      summary: Delete inferior file from duplicate group
      parameters:
        - name: groupId
          in: path
          required: true
          schema:
            type: string
        - name: fileId
          in: query
          description: Specific file to delete (default: auto-select inferior)
          schema:
            type: integer

  /api/v1/scattered:
    get:
      summary: Get scattered media analysis

  /api/v1/scattered/{itemId}/consolidate:
    post:
      summary: Consolidate scattered media item

  /api/v1/scan:
    post:
      summary: Trigger library scan

  /api/v1/scan/status:
    get:
      summary: Get scan status (SSE for progress)
      responses:
        200:
          content:
            text/event-stream: {}

components:
  schemas:
    DuplicateAnalysis:
      type: object
      properties:
        groups:
          type: array
          items:
            $ref: '#/components/schemas/DuplicateGroup'
        totalFiles:
          type: integer
        reclaimableBytes:
          type: integer

    DuplicateGroup:
      type: object
      properties:
        id:
          type: string
        title:
          type: string
        year:
          type: integer
        mediaType:
          type: string
          enum: [movie, series]
        files:
          type: array
          items:
            $ref: '#/components/schemas/MediaFile'
        bestFileId:
          type: integer
        reclaimableBytes:
          type: integer

    ScatteredAnalysis:
      type: object
      properties:
        items:
          type: array
          items:
            $ref: '#/components/schemas/ScatteredItem'
        totalMoves:
          type: integer
        totalBytes:
          type: integer

    ScatteredItem:
      type: object
      properties:
        id:
          type: integer
        title:
          type: string
        year:
          type: integer
        mediaType:
          type: string
        locations:
          type: array
          items:
            type: string
        targetLocation:
          type: string
        filesToMove:
          type: integer

    MediaFile:
      type: object
      properties:
        id:
          type: integer
        path:
          type: string
        size:
          type: integer
        resolution:
          type: string
        sourceType:
          type: string
        qualityScore:
          type: integer
```

**Code Generation:**
```bash
# Using oapi-codegen
oapi-codegen -package api -generate types,server api/openapi.yaml > api/api.gen.go
```

**SSE for Progress:**
```
GET /api/v1/scan/status
Accept: text/event-stream

event: progress
data: {"type":"scanning","current":150,"total":500,"message":"Scanning STORAGE3..."}

event: complete
data: {"type":"complete","duplicates":12,"scattered":36}
```

## File Structure

### New Files

```
api/
  openapi.yaml          # OpenAPI 3.1 spec

internal/api/
  server.go             # HTTP server implementation
  handlers.go           # Request handlers
  sse.go                # Server-sent events for progress

internal/service/
  cleanup.go            # Core cleanup operations
  analysis.go           # Duplicate/scatter analysis

internal/wizard/
  wizard.go             # Core wizard logic
  duplicates.go         # Duplicate handling step
  consolidate.go        # Consolidation step

cmd/jellywatch/
  fix_cmd.go            # Interactive wizard command
```

### Modified Files

```
cmd/jellywatch/
  scan_cmd.go           # Improved categorized output
  duplicates_cmd.go     # Add --dry-run
  consolidate_cmd.go    # Remove --generate, simplify

internal/consolidate/
  planner.go            # Fix sql.NullInt64 for SourceFileID
```

## Implementation Priority

1. **Fix SQL NULL bug** - Unblocks everything else
2. **Improve scan output** - Clear categorization and guidance
3. **Service layer** - Core operations decoupled from CLI
4. **Fix command** - Interactive wizard
5. **OpenAPI spec & server** - API for future web UI
6. **Simplify consolidate** - Remove --generate step
