# JellyWatch Enhancement: Jellysink Port Implementation Plan

**Created:** 2026-01-01
**Author:** Sisyphus AI Agent
**Status:** Planning Complete - Ready for Implementation

---

## Executive Summary

This document outlines the plan to port battle-tested components from `jellysink` into `jellywatch`, adding:
1. **Structured debug logging** - Know exactly what errors occur and why
2. **Release group blacklist** - 10,992 known release groups from srrDB.com
3. **Advanced naming functions** - Hyphen-aware parsing, abbreviation preservation, garbage detection
4. **Validation utilities** - Timeout-aware filesystem operations

### Expected Outcome
- Better filename parsing for edge cases
- Full visibility into daemon errors via log files
- Fallback detection when simple parsing produces garbage

---

## Source Material

### From: `/home/nomadx/Documents/jellysink/internal/scanner/`

| File | Lines | Components to Port |
|------|-------|-------------------|
| `blacklist.go` | 1,563 | `KnownReleaseGroups` map (10,992 groups), `PreservedAcronyms`, `AllCapsLegitTitles`, `CodecMarkers`, helper functions |
| `fuzzy.go` | 969 | `StripReleaseGroup()`, `CleanMovieName()`, `titleCaseWithOrdinals()`, `IsGarbageTitle()`, `stripOrphanedReleaseGroups()`, pre-compiled regex patterns |
| `validation.go` | 396 | `statWithTimeout()`, `openDirWithTimeout()` |

### To: `/home/nomadx/Documents/jellywatch/internal/`

| Target | Purpose |
|--------|---------|
| `internal/logging/` | NEW - Structured logging with file output |
| `internal/naming/blacklist.go` | NEW - Release group database |
| `internal/naming/advanced.go` | NEW - Advanced parsing functions |
| `internal/naming/naming.go` | MODIFY - Add routing to advanced parser |
| `internal/transfer/health.go` | MODIFY - Add timeout utilities |

---

## Phase 1: Structured Logging System

### Objective
Replace scattered `log.Printf` calls with structured logging that writes to both stdout and a rotating log file.

### New Files

#### `internal/logging/logging.go`
```go
package logging

// Logger provides structured logging with file output
// - Writes to stdout for daemon visibility
// - Writes to ~/.config/jellywatch/logs/jellywatch.log
// - Includes timestamp, level, component, message
// - Supports log rotation (keep last 5 files, 10MB each)
```

### Functions to Implement

| Function | Purpose |
|----------|---------|
| `NewLogger(config LogConfig) *Logger` | Create logger with file + stdout output |
| `(l *Logger) Debug(component, msg string, fields ...Field)` | Debug level logging |
| `(l *Logger) Info(component, msg string, fields ...Field)` | Info level logging |
| `(l *Logger) Warn(component, msg string, fields ...Field)` | Warning level logging |
| `(l *Logger) Error(component, msg string, err error, fields ...Field)` | Error level with error capture |
| `(l *Logger) Close()` | Flush and close log file |

### Log Format
```
2026-01-01T22:48:27+11:00 [INFO] [handler] Processing file | path=/mnt/NVME3/.../movie.mkv
2026-01-01T22:48:28+11:00 [ERROR] [transfer] rsync failed | path=/mnt/.../movie.mkv | error=exit status 23 | attempt=2/3
2026-01-01T22:48:30+11:00 [WARN] [naming] Garbage title detected | input=Muzzle K9 ita eng | fallback=advanced
```

### Files to Modify

| File | Changes |
|------|---------|
| `cmd/jellywatchd/main.go` | Initialize logger, pass to components |
| `internal/daemon/handler.go` | Replace `log.Printf` with structured logger |
| `internal/daemon/server.go` | Add logger for health/metrics endpoints |
| `internal/transfer/rsync.go` | Log transfer attempts, failures, retries |
| `internal/organizer/organizer.go` | Log organization decisions |

### Config Addition
```toml
[logging]
level = "info"          # debug, info, warn, error
file = "~/.config/jellywatch/logs/jellywatch.log"
max_size_mb = 10
max_backups = 5
```

### Deliverables
- [ ] `internal/logging/logging.go` - Core logger implementation
- [ ] `internal/logging/rotate.go` - Log rotation logic
- [ ] `internal/config/config.go` - Add LogConfig struct
- [ ] All daemon components updated to use structured logger
- [ ] Log directory created on first run

### Audit Criteria (Phase 1)
- [ ] All `log.Printf` calls in daemon replaced with structured logger
- [ ] Log file created and written to on daemon start
- [ ] Error logs include full error context (path, attempt number, error message)
- [ ] Log rotation works (test with small max_size)
- [ ] No compilation errors, tests pass

---

## Phase 2: Blacklist and Data Structures

### Objective
Port the release group database and supporting data structures from jellysink.

### New Files

#### `internal/naming/blacklist.go`
```go
package naming

// KnownReleaseGroups contains 10,992 release group names from srrDB.com
// Used to detect and strip orphaned release group artifacts from titles
var KnownReleaseGroups = buildReleaseGroupMap()

// PreservedAcronyms are TV show acronyms that should NOT be stripped
// even if they match release group patterns (TNG, NCIS, CSI, etc.)
var PreservedAcronyms map[string]bool

// AllCapsLegitTitles are uppercase words that are legitimate titles
// Prevents false positives on "IT", "FBI", "LOST", etc.
var AllCapsLegitTitles map[string]bool

// CodecMarkers are technical indicators that are definitely not titles
var CodecMarkers map[string]bool
```

### Data to Port

| Variable | Source Location | Size |
|----------|-----------------|------|
| `KnownReleaseGroups` | `blacklist.go:27-1438` | 10,992 entries |
| `PreservedAcronyms` | `blacklist.go:1440-1461` | ~25 entries |
| `AllCapsLegitTitles` | `blacklist.go:1502-1537` | ~35 entries |
| `CodecMarkers` | `blacklist.go:1463-1500` | ~35 entries |

### Helper Functions to Port

| Function | Source | Purpose |
|----------|--------|---------|
| `IsKnownReleaseGroup(word string) bool` | `blacklist.go:1539-1547` | Check if word is release group |
| `IsPreservedAcronym(word string) bool` | `blacklist.go:1549-1552` | Check if acronym should be kept |
| `IsCodecMarker(word string) bool` | `blacklist.go:1554-1557` | Check if word is codec marker |
| `IsAllCapsLegitTitle(word string) bool` | `blacklist.go:1559-1562` | Check if uppercase word is valid title |

### Deliverables
- [ ] `internal/naming/blacklist.go` - Complete blacklist implementation
- [ ] All 10,992 release groups ported
- [ ] All helper maps and functions ported
- [ ] Unit tests for blacklist lookups

### Audit Criteria (Phase 2)
- [ ] `KnownReleaseGroups` map contains exactly 10,992 entries
- [ ] `IsKnownReleaseGroup("yts")` returns `true`
- [ ] `IsKnownReleaseGroup("rome")` returns `false` (legitimate title)
- [ ] `IsPreservedAcronym("tng")` returns `true`
- [ ] `IsCodecMarker("x264")` returns `true`
- [ ] No compilation errors, tests pass

---

## Phase 3: Advanced Naming Functions

### Objective
Port the sophisticated parsing functions that handle edge cases the simple parser misses.

### New Files

#### `internal/naming/advanced.go`
```go
package naming

import (
    "golang.org/x/text/cases"
    "golang.org/x/text/language"
)

// Pre-compiled regex patterns (ported from jellysink/fuzzy.go init())
var (
    releasePatterns     []*regexp.Regexp
    preHyphenRegexes    []*regexp.Regexp
    hyphenSuffixRegexes []*regexp.Regexp
    collapseSpacesRegex *regexp.Regexp
    abbrevRegex         *regexp.Regexp
    upperTokenRegex     *regexp.Regexp
)
```

### Functions to Port

| Function | Source Lines | Purpose |
|----------|--------------|---------|
| `StripReleaseGroup(name string) string` | `fuzzy.go:331-478` | Hyphen-aware stripping, preserves "Monte-Cristo" |
| `CleanMovieName(name string) string` | `fuzzy.go:776-854` | Full pipeline: strip → clean → title case |
| `titleCaseWithOrdinals(s string) string` | `fuzzy.go:860-936` | Smart casing: preserves 1st, 2nd, R.I.P.D. |
| `IsGarbageTitle(title string) bool` | `fuzzy.go:655-772` | Detects failed parsing (codec markers, leetspeak) |
| `stripOrphanedReleaseGroups(name string) string` | `fuzzy.go:550-653` | Removes trailing garbage tokens |
| `init()` patterns | `fuzzy.go:32-183` | Pre-compiled regex patterns |

### Regex Patterns to Port (from fuzzy.go init)

```go
// Resolution markers
`\b\d{3,4}[pi]\b`           // 1080p, 720p, 2160p
`\b(4K|UHD)\b`

// HDR formats
`\b(HDR10\+?|HDR10Plus|Dolby\s?Vision|DoVi|DV|HDR|HLG|PQ|SDR)\b`

// Audio formats
`\b(DTS-HD\s?MA|DTS-HD\s?HRA|DTS-HD|DTS-X|DTS-ES)\b`
`\b(DD\+?|DDP|E?AC3|AAC|AC3)\d\s\d\b`
`\b(TrueHD|Atmos|FLAC|PCM|Opus|MP3|DTS)\b`

// Video codecs
`\bH\s?26[456]\b`
`\b(x264|x265|x266|HEVC|AVC|AV1|H264|H265|H266)\b`

// Source types
`\b(BluRay|Blu-ray|BDRip|BRRip|REMUX|WEB-DL|WEBDL|WEBRip|WEB)\b`
`\b(HDTV|PDTV|SDTV|DVDRip|DVD|DVDSCR)\b`

// Streaming platforms
`\b(AMZN|NF|DSNP|HMAX|HULU|ATVP|PCOK|PMTP)\b`

// Language/subtitle markers
`\b(ITA|FRE|FRA|ENG|EN|ESP|ES|SPA|SUB|SUBS|SUBBED|DUB|DUBBED|DUAL|MULTI)\b`

// Release tags
`\b(PROPER|REPACK|iNTERNAL|INTERNAL|LiMiTED|LIMITED|UNRATED|EXTENDED)\b`

// Abbreviation detection
`\b(?:[A-Za-z]\.[A-Za-z]\.(?:[A-Za-z]\.)+|U\.S\.)`
```

### Dependency Addition
```bash
go get golang.org/x/text@latest
```

### Modify Existing Files

#### `internal/naming/naming.go` - Add Routing Logic
```go
// ParseMovieName now routes between simple and advanced parsers
func ParseMovieName(filename string) (*MovieInfo, error) {
    // Try simple parser first (fast path)
    info, err := parseMovieSimple(filename)
    if err != nil {
        return parseMovieAdvanced(filename)
    }
    
    // Check if result looks like garbage
    if IsGarbageTitle(info.Title) {
        log.Debug("naming", "Garbage detected, using advanced parser", 
            Field("input", filename), Field("garbage_title", info.Title))
        return parseMovieAdvanced(filename)
    }
    
    return info, nil
}
```

### Deliverables
- [ ] `internal/naming/advanced.go` - All advanced functions
- [ ] `go.mod` updated with `golang.org/x/text` dependency
- [ ] `internal/naming/naming.go` - Routing logic added
- [ ] Existing `ParseMovieName` renamed to `parseMovieSimple`
- [ ] Unit tests for advanced parser

### Audit Criteria (Phase 3)
- [ ] `CleanMovieName("Movie.Name.2024.1080p.BluRay.x264-GROUP")` returns `"Movie Name (2024)"`
- [ ] `CleanMovieName("The.Count.of.Monte-Cristo.2024.WEB-DL")` returns `"The Count of Monte-Cristo (2024)"` (hyphen preserved)
- [ ] `CleanMovieName("R.I.P.D.2.2022.BluRay")` returns `"R.I.P.D. 2 (2022)"` (abbreviation preserved)
- [ ] `IsGarbageTitle("Muzzle K9 ita eng Licdom")` returns `true`
- [ ] `IsGarbageTitle("The Matrix")` returns `false`
- [ ] `titleCaseWithOrdinals("the 1st avenger")` returns `"The 1st Avenger"`
- [ ] Fallback routing works: garbage input → advanced parser
- [ ] No compilation errors, all tests pass

---

## Phase 4: Integration and Testing

### Objective
Wire everything together, update the daemon, and validate with real-world test cases.

### Integration Tasks

#### 4.1 Update Daemon Main
```go
// cmd/jellywatchd/main.go
func main() {
    // Initialize logger first
    logger := logging.NewLogger(cfg.Logging)
    defer logger.Close()
    
    // Pass logger to all components
    handler, err := daemon.NewMediaHandler(daemon.MediaHandlerConfig{
        Logger: logger,
        // ... existing config
    })
    if err != nil {
        logger.Fatal("failed to create handler", logging.F("error", err))
    }
}
```

#### 4.2 Update Handler Logging
```go
// internal/daemon/handler.go
func (h *MediaHandler) processFile(path string) {
    h.logger.Info("handler", "Processing file", logging.Field("path", path))
    
    // ... processing logic ...
    
    if err != nil {
        h.logger.Error("handler", "Organization failed", err,
            logging.Field("path", path),
            logging.Field("media_type", mediaType))
        h.stats.RecordError()
        return
    }
}
```

#### 4.3 Update Transfer Logging
```go
// internal/transfer/rsync.go
func (r *RsyncBackend) Transfer(src, dst string, opts Options) error {
    for attempt := 1; attempt <= opts.MaxRetries; attempt++ {
        r.logger.Debug("transfer", "Starting rsync",
            logging.Field("src", src),
            logging.Field("dst", dst),
            logging.Field("attempt", attempt))
        
        err := r.doTransfer(src, dst, opts)
        if err != nil {
            r.logger.Warn("transfer", "Rsync attempt failed",
                logging.Field("attempt", attempt),
                logging.Field("max_retries", opts.MaxRetries),
                logging.Field("error", err.Error()))
            continue
        }
        return nil
    }
    return fmt.Errorf("transfer failed after %d attempts", opts.MaxRetries)
}
```

### Test Cases

#### Real-World Filenames to Test
```go
var testCases = []struct {
    input    string
    expected string
}{
    // Standard cases (simple parser should handle)
    {"Movie.Title.2024.1080p.BluRay.x264-RARBG.mkv", "Movie Title (2024)"},
    {"For.All.Mankind.S04E02.MULTI.1080p.WEB.H264-ANESSE.mkv", "For All Mankind S04E02"},
    
    // Edge cases (advanced parser needed)
    {"Muzzle-K9-Squadra.Antidroga.2023.UpScaled.2160p.H265.10.bit.DV.HDR10ita.eng.AC3.5.1.sub.ita.eng.Licdom.mkv", "Muzzle (2023)"},
    {"The.Count.of.Monte-Cristo.2024.1080p.WEB-DL.DDP5.1.H.264-FLUX.mkv", "The Count of Monte-Cristo (2024)"},
    {"R.I.P.D.2.Rise.of.the.Damned.2022.1080p.BluRay.x264-YTS.mkv", "R.I.P.D. 2 Rise of the Damned (2022)"},
    {"D.E.B.S.2004.1080p.BluRay.x264-GROUP.mkv", "D.E.B.S. (2004)"},
    {"U.S.Marshals.1998.1080p.BluRay.x264-GROUP.mkv", "U.S. Marshals (1998)"},
    
    // Multi-language releases
    {"Movie.2024.MULTI.1080p.AMZN.WEB.H264.iTA.ENG.SPA-GROUP.mkv", "Movie (2024)"},
    {"Film.2024.FRENCH.1080p.BluRay.x264-VENUE.mkv", "Film (2024)"},
    
    // Leetspeak release groups
    {"Some.Movie.2024.1080p.WEB-DL.H.264-D3FiL3R.mkv", "Some Movie (2024)"},
}
```

### Deliverables
- [ ] `cmd/jellywatchd/main.go` - Logger initialization
- [ ] All daemon components using structured logger
- [ ] `internal/naming/naming_test.go` - Comprehensive test suite
- [ ] `internal/naming/advanced_test.go` - Advanced parser tests
- [ ] `internal/naming/blacklist_test.go` - Blacklist tests
- [ ] Integration test with real daemon startup

### Audit Criteria (Phase 4)
- [ ] Daemon starts without errors
- [ ] Log file created at configured location
- [ ] Processing a file logs: start → decision → result
- [ ] Errors include full context (path, error message, retry count)
- [ ] All test cases pass
- [ ] Italian movie edge case parses correctly with advanced parser
- [ ] Hyphenated titles preserved (Monte-Cristo)
- [ ] Abbreviations preserved (R.I.P.D., D.E.B.S., U.S.)
- [ ] `go test ./...` passes
- [ ] `go build ./...` succeeds

---

## File Summary

### New Files (4)
| File | Lines (est.) | Purpose |
|------|--------------|---------|
| `internal/logging/logging.go` | ~150 | Structured logger core |
| `internal/logging/rotate.go` | ~80 | Log rotation |
| `internal/naming/blacklist.go` | ~1,600 | Release group database |
| `internal/naming/advanced.go` | ~500 | Advanced parsing functions |

### Modified Files (6)
| File | Changes |
|------|---------|
| `internal/config/config.go` | Add LogConfig struct |
| `internal/naming/naming.go` | Add routing, rename to parseMovieSimple |
| `internal/daemon/handler.go` | Use structured logger |
| `internal/daemon/server.go` | Use structured logger |
| `internal/transfer/rsync.go` | Use structured logger |
| `cmd/jellywatchd/main.go` | Initialize logger |

### New Tests (3)
| File | Coverage |
|------|----------|
| `internal/naming/naming_test.go` | Simple + routing tests |
| `internal/naming/advanced_test.go` | Advanced parser tests |
| `internal/naming/blacklist_test.go` | Blacklist lookup tests |

---

## Execution Timeline

| Phase | Duration | Deliverables |
|-------|----------|--------------|
| Phase 1: Logging | ~1 hour | Logger + file output + rotation |
| Phase 1 Audit | ~15 min | Code review, test verification |
| Phase 2: Blacklist | ~30 min | 10,992 release groups + helpers |
| Phase 2 Audit | ~15 min | Verify counts, test lookups |
| Phase 3: Advanced | ~1.5 hours | All parsing functions + regex |
| Phase 3 Audit | ~20 min | Test edge cases, verify output |
| Phase 4: Integration | ~1 hour | Wire together + tests |
| Phase 4 Audit | ~20 min | End-to-end verification |

**Total Estimated Time:** ~5 hours

---

## Rollback Plan

If issues occur after deployment:

```bash
# Stop daemon
tmux send-keys -t omo-jellywatchd C-c

# Checkout previous version
git checkout main

# Rebuild
go build -o jellywatchd ./cmd/jellywatchd

# Restart
tmux send-keys -t omo-jellywatchd './jellywatchd' Enter
```

---

## Success Metrics

After implementation:
1. **Error Visibility**: Any error produces a log entry with full context
2. **Edge Case Handling**: Italian/multi-language releases parse correctly
3. **No Regressions**: All previously working filenames still work
4. **Performance**: No noticeable slowdown in daemon operation
5. **Maintainability**: Clear separation between simple and advanced parsers

---

## Approval

- [ ] Plan reviewed by user
- [ ] Ready to begin Phase 1

---

*Document generated by Sisyphus AI Agent*
