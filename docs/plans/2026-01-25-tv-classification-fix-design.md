# TV Show Classification Fix Design

**Date:** 2026-01-25
**Status:** Approved

## Problem Statement

TV shows with date-based naming (The Daily Show, Colbert, SNL, etc.) get misclassified as movies because the classification logic only recognizes `SxxExx` and `NxN` patterns.

Additionally, the daemon ignores which watch folder a file came from - files in the TV watch folder should default to TV classification.

## Root Cause

1. **No date-based episode detection** - Only `SxxExx` and `NxN` patterns trigger TV classification
2. **Watch folder source ignored** - Files from `/complete/tv/` get no classification hint

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Date formats | `YYYY.MM.DD`, `YYYY-MM-DD`, `YYYY_MM_DD` | Covers release group variations |
| Watch folder behavior | Force TV classification | Trust user's folder config |
| Hint mechanism | Path check in handler, pass hint to naming | Clean separation, no config in naming package |

## Architecture

### New Types

**File:** `internal/naming/types.go`

```go
package naming

type SourceHint int

const (
    SourceUnknown SourceHint = iota
    SourceTV
    SourceMovie
)
```

### Date Pattern Detection

**File:** `internal/naming/naming.go`

```go
var episodeDateRegex = regexp.MustCompile(`(19|20)\d{2}[.\-_](0[1-9]|1[0-2])[.\-_](0[1-9]|[12]\d|3[01])`)

func IsTVEpisodeFilename(filename string) bool {
    baseName := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))

    if episodeSERegex.MatchString(baseName) {
        return true
    }
    if episodeXRegex.MatchString(baseName) {
        return true
    }
    if episodeDateRegex.MatchString(baseName) {
        return true
    }

    return false
}
```

### Updated Classification with Hint

**File:** `internal/naming/deobfuscate.go`

```go
func IsTVEpisodeFromPath(path string, hint SourceHint) bool {
    if hint == SourceTV {
        return true
    }
    if hint == SourceMovie {
        return false
    }

    // SourceUnknown: use filename-based detection
    filename := filepath.Base(path)
    if IsTVEpisodeFilename(filename) {
        return true
    }

    if IsObfuscatedFilename(filename) {
        dir := filepath.Dir(path)
        for i := 0; i < 3; i++ {
            if dir == "/" || dir == "." || dir == "" {
                break
            }
            folderName := filepath.Base(dir)
            if IsTVEpisodeFilename(folderName) {
                return true
            }
            dir = filepath.Dir(dir)
        }
    }

    return false
}
```

### Handler Integration

**File:** `internal/daemon/handler.go`

```go
type MediaHandler struct {
    // ... existing fields
    tvWatchPaths    []string
    movieWatchPaths []string
}

type MediaHandlerConfig struct {
    // ... existing fields
    TVWatchPaths    []string
    MovieWatchPaths []string
}

func (h *MediaHandler) getSourceHint(path string) naming.SourceHint {
    for _, tvPath := range h.tvWatchPaths {
        if strings.HasPrefix(path, tvPath) {
            return naming.SourceTV
        }
    }
    for _, moviePath := range h.movieWatchPaths {
        if strings.HasPrefix(path, moviePath) {
            return naming.SourceMovie
        }
    }
    return naming.SourceUnknown
}

func (h *MediaHandler) processFile(path string) {
    // ... existing setup
    sourceHint := h.getSourceHint(path)
    isTVEpisode := naming.IsTVEpisodeFromPath(path, sourceHint)
    // ... rest unchanged
}
```

### Daemon Integration

**File:** `cmd/jellywatchd/main.go`

```go
handler := daemon.NewMediaHandler(daemon.MediaHandlerConfig{
    // ... existing fields
    TVWatchPaths:    cfg.Watch.TV,
    MovieWatchPaths: cfg.Watch.Movies,
})
```

## Files to Modify

| File | Action |
|------|--------|
| `internal/naming/types.go` | Create - SourceHint type |
| `internal/naming/naming.go` | Modify - add date regex, update IsTVEpisodeFilename |
| `internal/naming/deobfuscate.go` | Modify - update IsTVEpisodeFromPath signature |
| `internal/daemon/handler.go` | Modify - add watch paths, getSourceHint, update processFile |
| `cmd/jellywatchd/main.go` | Modify - pass watch paths to handler |
| `internal/naming/naming_test.go` | Modify - add date pattern tests |

## Testing

### Unit Tests

- Date pattern detection: `The.Daily.Show.2026.01.22` → TV
- Date with different separators: `Show.2026-01-22`, `Show.2026_01_22` → TV
- Source hint overrides: SourceTV + no pattern → TV
- Source hint overrides: SourceMovie + date pattern → Movie
- Existing SxxExx patterns still work

### Manual Testing

- [ ] Daily Show episode from TV folder → classified as TV
- [ ] Movie from Movie folder → classified as Movie
- [ ] File with SxxExx in TV folder → classified as TV
- [ ] Date-pattern file with SourceUnknown → classified as TV
