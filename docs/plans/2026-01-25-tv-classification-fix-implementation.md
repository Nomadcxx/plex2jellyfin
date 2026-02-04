# TV Show Classification Fix Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix TV show misclassification by adding date-based episode detection and watch folder source hints.

**Architecture:** Add `SourceHint` type to naming package, update classification functions to accept hints, pass hints from handler based on configured watch paths.

**Tech Stack:** Go stdlib (regexp, strings, path/filepath)

---

## Task 1: Create SourceHint Type

**Files:**
- Create: `internal/naming/types.go`

**Step 1: Create the types file**

```go
// internal/naming/types.go
package naming

// SourceHint indicates where a file originated from
type SourceHint int

const (
	SourceUnknown SourceHint = iota // Unknown origin, use filename-only detection
	SourceTV                        // File came from TV watch folder
	SourceMovie                     // File came from Movie watch folder
)

// String returns a human-readable representation of the hint
func (h SourceHint) String() string {
	switch h {
	case SourceTV:
		return "tv"
	case SourceMovie:
		return "movie"
	default:
		return "unknown"
	}
}
```

**Step 2: Verify compilation**

Run: `go build ./internal/naming/...`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add internal/naming/types.go
git commit -m "feat(naming): add SourceHint type for watch folder classification"
```

---

## Task 2: Add Date-Based Episode Detection

**Files:**
- Modify: `internal/naming/naming.go`

**Step 1: Add the date regex after existing patterns (around line 27)**

```go
var (
	yearRegex       = regexp.MustCompile(`\b(19|20)\d{2}\b`)
	yearParenRegex  = regexp.MustCompile(`\((\d{4})\)`)
	episodeSERegex  = regexp.MustCompile(`[Ss](\d{1,2})[Ee](\d{1,2})`)
	episodeXRegex   = regexp.MustCompile(`(\d{1,2})x(\d{1,2})`)
	// Date-based episode pattern: YYYY.MM.DD, YYYY-MM-DD, YYYY_MM_DD
	episodeDateRegex = regexp.MustCompile(`(19|20)\d{2}[.\-_](0[1-9]|1[0-2])[.\-_](0[1-9]|[12]\d|3[01])`)
	releasePatterns []*regexp.Regexp
)
```

**Step 2: Update IsTVEpisodeFilename function**

Replace the existing function:

```go
func IsTVEpisodeFilename(filename string) bool {
	baseName := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))

	// Standard SxxExx pattern
	if episodeSERegex.MatchString(baseName) {
		return true
	}

	// NxN pattern
	if episodeXRegex.MatchString(baseName) {
		return true
	}

	// Date-based pattern (daily shows like The Daily Show, Colbert, SNL)
	if episodeDateRegex.MatchString(baseName) {
		return true
	}

	return false
}
```

**Step 3: Update IsMovieFilename for clarity**

```go
func IsMovieFilename(filename string) bool {
	return !IsTVEpisodeFilename(filename)
}
```

**Step 4: Verify compilation**

Run: `go build ./internal/naming/...`
Expected: Build succeeds

**Step 5: Commit**

```bash
git add internal/naming/naming.go
git commit -m "feat(naming): add date-based episode detection for daily shows"
```

---

## Task 3: Update IsTVEpisodeFromPath to Accept Hint

**Files:**
- Modify: `internal/naming/deobfuscate.go`

**Step 1: Update the function signature and add hint logic**

Replace the existing `IsTVEpisodeFromPath` function:

```go
// IsTVEpisodeFromPath returns true if path represents a TV episode.
// The hint parameter indicates the source watch folder (if known).
func IsTVEpisodeFromPath(path string, hint SourceHint) bool {
	// If hint says TV, trust it (user configured this folder for TV)
	if hint == SourceTV {
		return true
	}

	// If hint says Movie, trust it
	if hint == SourceMovie {
		return false
	}

	// SourceUnknown: use filename-based detection
	filename := filepath.Base(path)
	if IsTVEpisodeFilename(filename) {
		return true
	}

	// Check parent folders for obfuscated files
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

// IsMovieFromPath returns true if path represents a movie (not a TV episode).
func IsMovieFromPath(path string, hint SourceHint) bool {
	return !IsTVEpisodeFromPath(path, hint)
}
```

**Step 2: Verify compilation fails (callers need updating)**

Run: `go build ./...`
Expected: Build fails with "not enough arguments" errors - this is expected

**Step 3: Commit partial progress**

```bash
git add internal/naming/deobfuscate.go
git commit -m "feat(naming): update IsTVEpisodeFromPath to accept source hint"
```

---

## Task 4: Update Handler with Watch Path Tracking

**Files:**
- Modify: `internal/daemon/handler.go`

**Step 1: Add watch path fields to MediaHandler struct**

Add after the existing fields (around line 32):

```go
type MediaHandler struct {
	organizer       *organizer.Organizer
	notifyManager   *notify.Manager
	tvLibraries     []string
	movieLibs       []string
	tvWatchPaths    []string // TV watch folders for source hint
	movieWatchPaths []string // Movie watch folders for source hint
	debounceTime    time.Duration
	pending         map[string]*time.Timer
	mu              sync.Mutex
	dryRun          bool
	stats           *Stats
	logger          *logging.Logger
	sonarrClient    *sonarr.Client
	activityLogger  *activity.Logger
}
```

**Step 2: Add fields to MediaHandlerConfig**

```go
type MediaHandlerConfig struct {
	TVLibraries     []string
	MovieLibs       []string
	TVWatchPaths    []string // New
	MovieWatchPaths []string // New
	DebounceTime    time.Duration
	DryRun          bool
	Timeout         time.Duration
	Backend         transfer.Backend
	NotifyManager   *notify.Manager
	Logger          *logging.Logger
	TargetUID       int
	TargetGID       int
	FileMode        os.FileMode
	DirMode         os.FileMode
	SonarrClient    *sonarr.Client
	ConfigDir       string
}
```

**Step 3: Update NewMediaHandler to store watch paths**

In the return statement, add:

```go
return &MediaHandler{
	organizer:       org,
	notifyManager:   cfg.NotifyManager,
	tvLibraries:     cfg.TVLibraries,
	movieLibs:       cfg.MovieLibs,
	tvWatchPaths:    cfg.TVWatchPaths,    // Add this
	movieWatchPaths: cfg.MovieWatchPaths, // Add this
	debounceTime:    cfg.DebounceTime,
	pending:         make(map[string]*time.Timer),
	dryRun:          cfg.DryRun,
	stats:           NewStats(),
	logger:          cfg.Logger,
	sonarrClient:    cfg.SonarrClient,
	activityLogger:  activityLogger,
}
```

**Step 4: Add getSourceHint method**

Add before processFile:

```go
// getSourceHint determines if a path is under a configured TV or Movie watch folder
func (h *MediaHandler) getSourceHint(path string) naming.SourceHint {
	// Check TV watch folders
	for _, tvPath := range h.tvWatchPaths {
		if strings.HasPrefix(path, tvPath) {
			return naming.SourceTV
		}
	}

	// Check Movie watch folders
	for _, moviePath := range h.movieWatchPaths {
		if strings.HasPrefix(path, moviePath) {
			return naming.SourceMovie
		}
	}

	return naming.SourceUnknown
}
```

**Step 5: Update processFile to use hint**

Change line 268 from:

```go
isTVEpisode := naming.IsTVEpisodeFromPath(path)
```

To:

```go
sourceHint := h.getSourceHint(path)
isTVEpisode := naming.IsTVEpisodeFromPath(path, sourceHint)
```

**Step 6: Verify compilation**

Run: `go build ./internal/daemon/...`
Expected: Build succeeds

**Step 7: Commit**

```bash
git add internal/daemon/handler.go
git commit -m "feat(daemon): add watch folder source hints to handler"
```

---

## Task 5: Update Daemon Main to Pass Watch Paths

**Files:**
- Modify: `cmd/jellywatchd/main.go`

**Step 1: Update MediaHandlerConfig creation**

Find the handler creation (around line 147) and add the new fields:

```go
handler := daemon.NewMediaHandler(daemon.MediaHandlerConfig{
	TVLibraries:     cfg.Libraries.TV,
	MovieLibs:       cfg.Libraries.Movies,
	TVWatchPaths:    cfg.Watch.TV,      // Add this
	MovieWatchPaths: cfg.Watch.Movies,  // Add this
	DebounceTime:    10 * time.Second,
	Timeout:         5 * time.Minute,
	Backend:         transfer.ParseBackend(backendName),
	NotifyManager:   notifyMgr,
	Logger:          logger,
	TargetUID:       targetUID,
	TargetGID:       targetGID,
	FileMode:        fileMode,
	DirMode:         dirMode,
	SonarrClient:    sonarrClient,
	ConfigDir:       configDir,
})
```

**Step 2: Verify full compilation**

Run: `go build ./...`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add cmd/jellywatchd/main.go
git commit -m "feat(daemon): pass watch paths to handler for classification hints"
```

---

## Task 6: Fix Other Callers of IsTVEpisodeFromPath

**Files:**
- Search for and update all callers

**Step 1: Find all callers**

Run: `grep -r "IsTVEpisodeFromPath\|IsMovieFromPath" --include="*.go" .`

Update each caller to pass `naming.SourceUnknown` if no hint is available (for CLI commands, etc.)

**Step 2: Update each caller**

For each file that calls these functions outside the daemon (likely CLI tools), add `naming.SourceUnknown` as the second argument:

```go
// Before
isTVEpisode := naming.IsTVEpisodeFromPath(path)

// After
isTVEpisode := naming.IsTVEpisodeFromPath(path, naming.SourceUnknown)
```

**Step 3: Verify full compilation**

Run: `go build ./...`
Expected: Build succeeds with no errors

**Step 4: Commit**

```bash
git add -A
git commit -m "fix: update all IsTVEpisodeFromPath callers with hint parameter"
```

---

## Task 7: Add Unit Tests

**Files:**
- Modify: `internal/naming/naming_test.go`

**Step 1: Add date pattern tests**

```go
func TestIsTVEpisodeFilename_DatePatterns(t *testing.T) {
	tests := []struct {
		filename string
		want     bool
	}{
		// Date patterns - should be TV
		{"The.Daily.Show.2026.01.22.Guest.Name.1080p.WEB.mkv", true},
		{"Colbert.2026-01-22.1080p.WEB.mkv", true},
		{"SNL.2026_01_22.Host.1080p.mkv", true},
		{"Last.Week.Tonight.2026.01.19.1080p.mkv", true},

		// Standard patterns still work
		{"Show.S01E05.mkv", true},
		{"Show.1x05.mkv", true},

		// Movies - should not be TV
		{"Movie.Name.2026.1080p.mkv", false},
		{"Another.Movie.2024.BluRay.mkv", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := IsTVEpisodeFilename(tt.filename)
			if got != tt.want {
				t.Errorf("IsTVEpisodeFilename(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestIsTVEpisodeFromPath_SourceHint(t *testing.T) {
	tests := []struct {
		path string
		hint SourceHint
		want bool
	}{
		// SourceTV forces TV classification
		{"/downloads/movie.mkv", SourceTV, true},
		{"/downloads/no.pattern.mkv", SourceTV, true},

		// SourceMovie forces Movie classification
		{"/downloads/Show.S01E05.mkv", SourceMovie, false},
		{"/downloads/Daily.Show.2026.01.22.mkv", SourceMovie, false},

		// SourceUnknown uses filename detection
		{"/downloads/Show.S01E05.mkv", SourceUnknown, true},
		{"/downloads/Daily.Show.2026.01.22.mkv", SourceUnknown, true},
		{"/downloads/Movie.2024.mkv", SourceUnknown, false},
	}

	for _, tt := range tests {
		name := fmt.Sprintf("%s_hint_%v", filepath.Base(tt.path), tt.hint)
		t.Run(name, func(t *testing.T) {
			got := IsTVEpisodeFromPath(tt.path, tt.hint)
			if got != tt.want {
				t.Errorf("IsTVEpisodeFromPath(%q, %v) = %v, want %v", tt.path, tt.hint, got, tt.want)
			}
		})
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/naming/... -v`
Expected: All tests pass

**Step 3: Commit**

```bash
git add internal/naming/naming_test.go
git commit -m "test(naming): add tests for date patterns and source hints"
```

---

## Task 8: Build and Manual Test

**Step 1: Build all binaries**

Run: `go build ./...`
Expected: All packages build successfully

**Step 2: Run all tests**

Run: `go test ./... -v`
Expected: All tests pass

**Step 3: Manual test with Daily Show filename**

```bash
# Test the naming detection directly
go run ./cmd/jellywatch organize "/tmp/The.Daily.Show.2026.01.22.Guest.1080p.WEB.mkv" --dry-run
```

Expected: Should recognize as TV episode

**Step 4: Final commit**

```bash
git add -A
git commit -m "feat: complete TV classification fix with date detection and source hints"
```

---

## Summary

| Task | Description | Files |
|------|-------------|-------|
| 1 | Create SourceHint type | `internal/naming/types.go` |
| 2 | Add date-based detection | `internal/naming/naming.go` |
| 3 | Update IsTVEpisodeFromPath signature | `internal/naming/deobfuscate.go` |
| 4 | Add watch path tracking to handler | `internal/daemon/handler.go` |
| 5 | Pass watch paths from daemon | `cmd/jellywatchd/main.go` |
| 6 | Fix other callers | Various |
| 7 | Add unit tests | `internal/naming/naming_test.go` |
| 8 | Build and manual test | N/A |
