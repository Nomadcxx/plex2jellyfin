# JellyWatch as Source of Truth - Design Document

**Date:** 2026-01-30  
**Status:** Approved  
**Problem:** TV shows misclassified into Movie libraries due to combined library selection and year-ignoring fuzzy match

## Executive Summary

JellyWatch will become the **sole decision-maker** for all file organization. Sonarr/Radarr will transition to passive metadata providers that sync their paths FROM JellyWatch's database.

### Root Causes Identified

1. **Combined Library List** - `handler.go:139` merges TV + Movie libraries, allowing TV to match Movie folders
2. **Year-Ignoring Match** - `scanner.go:62` strips years before comparison, causing "Dracula (2020)" to match "Dracula (2025)"
3. **Competing Systems** - Sonarr creates folders after JellyWatch organizes, causing duplicates like "Dracula (2020) (2020)"

---

## Section 1: Architecture Overview

**Current State:**
```
Sonarr/Radarr → Downloads → JellyWatch → Library
                              ↓
                    Sonarr/Radarr also imports (CONFLICT)
```

**Target State:**
```
Sonarr/Radarr → Downloads → JellyWatch → Library → Jellyfin
                    ↓              ↓
               media.db ←──── Sync Service
                    ↓
              Sonarr/Radarr (path updates only)
```

**Key Changes:**
- JellyWatch = sole decision-maker for all file locations
- Sonarr/Radarr = passive metadata providers and download managers
- Sonarr/Radarr auto-import disabled; they only update paths from JellyWatch

---

## Section 2: Database as Source of Truth

JellyWatch's `media.db` becomes the canonical source. Sonarr/Radarr sync their `series.Path` and `movie.Path` fields from our database.

### Existing Tables

| Table | Key Fields | Source of Truth For |
|-------|------------|---------------------|
| `series` | `canonical_path`, `library_root`, `tvdb_id`, `sonarr_id` | TV show folder locations |
| `movies` | `canonical_path`, `library_root`, `tmdb_id`, `radarr_id` | Movie folder locations |
| `media_files` | `path`, `library_root`, `media_type` | Individual file locations |

### New Fields Required

```sql
-- Track sync state with external systems
ALTER TABLE series ADD COLUMN sonarr_synced_at DATETIME;
ALTER TABLE series ADD COLUMN sonarr_path_dirty BOOLEAN DEFAULT 0;

ALTER TABLE movies ADD COLUMN radarr_synced_at DATETIME;
ALTER TABLE movies ADD COLUMN radarr_path_dirty BOOLEAN DEFAULT 0;
```

### Sync Flow

1. JellyWatch organizes file → updates `canonical_path` in DB → sets `*_path_dirty = 1`
2. Sync service runs (immediate + periodic)
3. For each dirty record: call Sonarr/Radarr API to update their path
4. On success: set `*_path_dirty = 0`, update `*_synced_at`

---

## Section 3: Sync Service Design

**Approach:** Hybrid (immediate attempt + periodic retry)

### Architecture

```go
type SyncService struct {
    db       *database.MediaDB
    sonarr   *sonarr.Client
    radarr   *radarr.Client
    
    // Immediate sync channel
    syncChan chan SyncRequest
    
    // Periodic retry interval
    retryInterval time.Duration  // e.g., 5 minutes
}

type SyncRequest struct {
    MediaType string  // "series" or "movie"
    ID        int64   // Database ID
}
```

### Flow

```
File Organized
      ↓
 Set dirty=1 in DB
      ↓
 Queue immediate sync ──→ syncChan
      ↓
 Sync worker attempts API call
      ↓
 Success? ──→ dirty=0, synced_at=now
      ↓
 Failure? ──→ stays dirty=1, retry on next periodic sweep
```

### Periodic Sweep

```go
func (s *SyncService) runRetryLoop(ctx context.Context) {
    ticker := time.NewTicker(s.retryInterval)
    for {
        select {
        case <-ticker.C:
            s.syncDirtyRecords()
        case <-ctx.Done():
            return
        }
    }
}

func (s *SyncService) syncDirtyRecords() {
    // Sync dirty series to Sonarr
    dirtySeries, _ := s.db.GetDirtySeries()
    for _, series := range dirtySeries {
        if series.SonarrID > 0 {
            err := s.sonarr.UpdateSeriesPath(series.SonarrID, series.CanonicalPath)
            if err == nil {
                s.db.MarkSeriesSynced(series.ID)
            }
        }
    }
    
    // Sync dirty movies to Radarr
    dirtyMovies, _ := s.db.GetDirtyMovies()
    for _, movie := range dirtyMovies {
        if movie.RadarrID > 0 {
            err := s.radarr.UpdateMoviePath(movie.RadarrID, movie.CanonicalPath)
            if err == nil {
                s.db.MarkMovieSynced(movie.ID)
            }
        }
    }
}
```

---

## Section 4: Sonarr/Radarr Configuration

**Approach:** Auto-configure with user confirmation (hybrid)

### Installer Flow

When Sonarr/Radarr integration is enabled, the installer will:

1. Connect to Sonarr/Radarr API
2. Read current settings
3. Display proposed changes
4. Apply changes only with user confirmation

### Settings to Modify

**Sonarr:**
| Setting | API Field | Target Value |
|---------|-----------|--------------|
| Rename Episodes | `renameEpisodes` | `false` |
| Create Empty Series Folders | `createEmptySeriesFolders` | `false` |
| Root Folders | `/api/v3/rootfolder` | Remove all (keep series data) |

**Radarr:**
| Setting | API Field | Target Value |
|---------|-----------|--------------|
| Rename Movies | `renameMovies` | `false` |
| Create Empty Movie Folders | `createEmptyMovieFolders` | `false` |
| Root Folders | `/api/v3/rootfolder` | Remove all (keep movie data) |

### New API Methods Required

```go
// internal/sonarr/config.go
type MediaManagementConfig struct {
    ID                         int  `json:"id"`
    RenameEpisodes             bool `json:"renameEpisodes"`
    CreateEmptySeriesFolders   bool `json:"createEmptySeriesFolders"`
    // ... other fields preserved
}

func (c *Client) GetMediaManagementConfig() (*MediaManagementConfig, error)
func (c *Client) UpdateMediaManagementConfig(cfg *MediaManagementConfig) error

type RootFolder struct {
    ID   int    `json:"id"`
    Path string `json:"path"`
}

func (c *Client) GetRootFolders() ([]RootFolder, error)
func (c *Client) DeleteRootFolder(id int) error
```

### Installer UI

```
┌────────────────────────────────────────────────────────────┐
│  Sonarr Integration                                        │
│                                                            │
│  ✓ Connected - Sonarr v4.0.2                              │
│                                                            │
│  ⚠ JellyWatch needs to disable Sonarr's auto-import       │
│    to prevent conflicts.                                   │
│                                                            │
│  Changes to apply:                                         │
│    • Disable "Rename Episodes"                             │
│    • Disable "Create Empty Series Folders"                 │
│    • Remove root folders (keeps series data)               │
│                                                            │
│  Current root folders:                                     │
│    /mnt/STORAGE1/TVSHOWS                                   │
│    /mnt/STORAGE5/TVSHOWS                                   │
│                                                            │
│  ▸ Apply changes (Recommended)                             │
│    Skip - configure manually                               │
│    Cancel Sonarr integration                               │
└────────────────────────────────────────────────────────────┘
```

---

## Section 5: Library Selection Fix

### Issue 1: Combined Library List

**Location:** `internal/daemon/handler.go:139`

**Current:**
```go
allLibs := append(cfg.TVLibraries, cfg.MovieLibs...)
org, err := organizer.NewOrganizer(allLibs, orgOpts...)
```

**Fix:** Create separate organizers or pass correct libraries per operation:

```go
// Option A: Separate organizers
tvOrg, err := organizer.NewOrganizer(cfg.TVLibraries, orgOpts...)
movieOrg, err := organizer.NewOrganizer(cfg.MovieLibs, orgOpts...)

// Option B: Pass libraries per call (requires API change)
result, err = h.organizer.OrganizeTVEpisodeAuto(path, h.tvLibraries, ...)
```

### Issue 2: Year-Ignoring Match

**Location:** `internal/library/scanner.go:62`

**Current:**
```go
if normalizedDir == normalizedTitle ||
   normalizedDir == normalizedTitle+year ||
   strings.HasPrefix(normalizedDir, normalizedTitle+"(") {
    return filepath.Join(library, dirName)
}
```

**Fix:** Extract and compare years when both are present:

```go
func (s *Selector) findShowDirInLibrary(library, normalizedTitle, year string) string {
    entries, err := os.ReadDir(library)
    if err != nil {
        return ""
    }

    for _, entry := range entries {
        if !entry.IsDir() {
            continue
        }

        dirName := entry.Name()
        normalizedDir := normalizeTitle(dirName)
        
        // Extract year from directory if present
        dirYear := extractYearFromDir(dirName)
        
        // If both have years, they MUST match
        if year != "" && dirYear != "" && year != dirYear {
            continue  // Different years - not a match
        }
        
        // Match patterns (now year-safe):
        if normalizedDir == normalizedTitle ||
            normalizedDir == normalizedTitle+year ||
            strings.HasPrefix(normalizedDir, normalizedTitle+"(") {
            return filepath.Join(library, dirName)
        }
    }

    return ""
}

// Helper to extract year from folder name
func extractYearFromDir(dirName string) string {
    yearPattern := regexp.MustCompile(`\((\d{4})\)`)
    matches := yearPattern.FindStringSubmatch(dirName)
    if len(matches) >= 2 {
        return matches[1]
    }
    return ""
}
```

### Implementation Priority

| Fix | Priority | Reason |
|-----|----------|--------|
| Separate library lists | **HIGH** | Prevents TV/Movie cross-contamination entirely |
| Year-aware matching | **MEDIUM** | Additional safety for same-library conflicts |

---

## Section 6: Migration Plan

### Detection Logic

```go
// internal/migrate/detector.go
type MisplacementType int

const (
    TVInMovies MisplacementType = iota
    MovieInTV
    DuplicateFolder
    EmptyFolder
)

type Misplacement struct {
    Type        MisplacementType
    CurrentPath string
    CorrectPath string
    MediaType   string  // "tv" or "movie"
    Title       string
    Year        string
    FileCount   int
    TotalSize   int64
}

func DetectMisplacements(cfg Config, db *database.MediaDB) ([]Misplacement, error) {
    var issues []Misplacement
    
    // 1. Find TV episodes in Movie libraries (look for SxxExx patterns)
    for _, movieLib := range cfg.MovieLibraries {
        issues = append(issues, findTVInMovieLibrary(movieLib, cfg.TVLibraries)...)
    }
    
    // 2. Find Movies in TV libraries (no SxxExx, has year)
    for _, tvLib := range cfg.TVLibraries {
        issues = append(issues, findMoviesInTVLibrary(tvLib, cfg.MovieLibraries)...)
    }
    
    // 3. Find duplicate folders (same title in multiple libraries)
    issues = append(issues, findDuplicateFolders(cfg)...)
    
    // 4. Find empty folders (0 media files)
    issues = append(issues, findEmptyFolders(cfg)...)
    
    return issues, nil
}
```

### CLI Interface

```
$ jellywatch migrate

Scanning for misplaced media...

Found 3 potential issues:

1. TV Show in Movies Library
   Dracula (2020) S01E01-E03
   Current:   /mnt/STORAGE5/MOVIES/Dracula (2025)/Season 01/
   Should be: /mnt/STORAGE1/TVSHOWS/Dracula (2020)/Season 01/
   [M]ove  [S]kip  [A]ll  [Q]uit

2. Duplicate Show Folders
   Dracula (2020) exists in 2 locations:
   - /mnt/STORAGE5/MOVIES/Dracula (2020) (2020)/ (0 episodes)
   - /mnt/STORAGE1/TVSHOWS/Dracula (2020)/ (3 episodes)
   [D]elete empty  [S]kip

3. Empty Folder
   /mnt/STORAGE5/MOVIES/Dracula (2025)/ (0 files after move)
   [D]elete  [S]kip

Migration complete:
  - 3 files moved
  - 2 empty folders deleted
  - Sonarr paths updated
  - Radarr paths updated
```

### Post-Migration Actions

1. Update JellyWatch database with new canonical paths
2. Call Sonarr/Radarr APIs to update their paths (via sync service)
3. Trigger Jellyfin library scan (optional, via API if configured)

---

## Implementation Order

| Phase | Component | Priority | Effort |
|-------|-----------|----------|--------|
| 1 | Library selection fix (separate TV/Movie) | Critical | Low |
| 2 | Year-aware matching | High | Low |
| 3 | Database schema (dirty flags) | High | Low |
| 4 | Sync service (hybrid) | High | Medium |
| 5 | Sonarr/Radarr config API methods | Medium | Low |
| 6 | Installer config flow | Medium | Medium |
| 7 | Migration CLI | Medium | Medium |

### Phase 1-2: Immediate Fixes (prevent new issues)
- Fix library selection to prevent TV/Movie cross-contamination
- Add year-aware matching for additional safety

### Phase 3-4: Sync Infrastructure (enable path syncing)
- Add dirty flag columns to database
- Implement hybrid sync service

### Phase 5-6: Configuration Automation (new installs)
- Add Sonarr/Radarr config API methods
- Update installer to configure Sonarr/Radarr automatically

### Phase 7: Migration (fix existing issues)
- Implement migration detector and CLI
- Allow users to fix pre-existing misplacements

---

## Files to Modify

| File | Changes |
|------|---------|
| `internal/daemon/handler.go` | Separate TV/Movie library handling |
| `internal/library/scanner.go` | Year-aware matching |
| `internal/library/selector.go` | (May need updates for separate selectors) |
| `internal/organizer/organizer.go` | (May need API changes) |
| `internal/database/schema.go` | Add dirty flag columns |
| `internal/database/series.go` | Add dirty flag methods |
| `internal/database/movies.go` | Add dirty flag methods |
| `internal/sync/sync.go` | Enhance with dirty flag sync |
| `internal/sonarr/config.go` | New file - config API methods |
| `internal/radarr/config.go` | New file - config API methods |
| `cmd/installer/screens.go` | Add Sonarr/Radarr config screen |
| `cmd/installer/tasks.go` | Add Sonarr/Radarr config task |
| `internal/migrate/` | New package - migration logic |
| `cmd/jellywatch/migrate_cmd.go` | New file - migrate CLI |

---

## Success Criteria

1. **No new misplacements:** TV episodes never end up in Movie libraries (and vice versa)
2. **Year conflicts resolved:** "Dracula (2020)" never matches "Dracula (2025)"
3. **Paths stay synced:** Sonarr/Radarr paths match JellyWatch's canonical paths within 5 minutes
4. **No duplicate folders:** Sonarr/Radarr cannot create folders that conflict with JellyWatch
5. **Migration works:** Existing misplacements can be detected and fixed interactively
