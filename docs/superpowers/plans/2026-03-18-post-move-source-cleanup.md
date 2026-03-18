# Post-Move Source Directory Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** After jellywatchd successfully moves a media file to the library, automatically clean up the now-empty source download directory (and walk up parent dirs to the watch root), removing SABnzbd junk files along the way.

**Architecture:** Two new private methods on `MediaHandler` in `handler.go`: `containsVideoFilesRecursive()` reuses the existing `IsMediaFile()` extension set and short-circuits with `fs.SkipAll` on first hit; `cleanupSourceDir()` gates on source-file absence, walks bottom-up removing video-free directories, stops at watch roots. Called from both `processFile()` and `applyAIResult()` after confirmed success.

**Tech Stack:** Go 1.24.0, stdlib only (`os`, `path/filepath`, `io/fs`), `testing` + `testify` packages.

---

### Task 1: Add `containsVideoFilesRecursive` and `cleanupSourceDir` with tests

**Files:**
- Modify: `internal/daemon/handler.go` (add two methods after `IsMediaFile` at line 639)
- Modify: `internal/daemon/handler_test.go` (add tests)

- [ ] **Step 1: Write the failing tests**

Add to `internal/daemon/handler_test.go`. Add these imports at the top of the file if not already present:

```go
import (
    // existing imports...
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)
```

Add the `makeHandler` helper and test functions:

```go
func makeHandler(t *testing.T, watchRoots []string) *MediaHandler {
    t.Helper()
    return &MediaHandler{
        tvWatchPaths:    watchRoots,
        movieWatchPaths: watchRoots,
        logger:          logging.Nop(),
    }
}

func TestCleanupSourceDir_SimpleMovie(t *testing.T) {
    root := t.TempDir() // simulates watch root
    dlDir := filepath.Join(root, "Movie.Name.2025.1080p-GROUP")
    require.NoError(t, os.MkdirAll(dlDir, 0755))

    // junk file left by SABnzbd — still present after move
    junk := filepath.Join(dlDir, "abc123.txt")
    require.NoError(t, os.WriteFile(junk, []byte("nzb"), 0644))

    // simulate: video was already moved (not present on disk)
    movedFile := filepath.Join(dlDir, "movie.mkv")

    h := makeHandler(t, []string{root})
    h.cleanupSourceDir(movedFile)

    // download dir should be gone
    _, err := os.Stat(dlDir)
    assert.True(t, os.IsNotExist(err), "download dir should be removed")

    // watch root must still exist
    _, err = os.Stat(root)
    assert.NoError(t, err, "watch root must not be removed")
}

func TestCleanupSourceDir_SourceStillExists_NoCleanup(t *testing.T) {
    root := t.TempDir()
    dlDir := filepath.Join(root, "Movie.Name.2025.1080p-GROUP")
    require.NoError(t, os.MkdirAll(dlDir, 0755))

    // video file still present (keepSource / dry-run scenario)
    videoFile := filepath.Join(dlDir, "movie.mkv")
    require.NoError(t, os.WriteFile(videoFile, []byte("data"), 0644))

    h := makeHandler(t, []string{root})
    h.cleanupSourceDir(videoFile)

    // dir must remain untouched
    _, err := os.Stat(dlDir)
    assert.NoError(t, err, "download dir must remain when source still exists")
}

func TestCleanupSourceDir_FileDirectlyInWatchRoot_NoCleanup(t *testing.T) {
    root := t.TempDir()
    // Parent of movedFile IS the watch root — loop must exit immediately
    movedFile := filepath.Join(root, "movie.mkv")

    h := makeHandler(t, []string{root})
    h.cleanupSourceDir(movedFile)

    // watch root must still exist
    _, err := os.Stat(root)
    assert.NoError(t, err, "watch root must not be removed")
}

func TestCleanupSourceDir_SeasonPackRemainingEpisodes(t *testing.T) {
    root := t.TempDir()
    packDir := filepath.Join(root, "Show.S01-S04.1080p")
    s1 := filepath.Join(packDir, "Season 1")
    s2 := filepath.Join(packDir, "Season 2")
    require.NoError(t, os.MkdirAll(s1, 0755))
    require.NoError(t, os.MkdirAll(s2, 0755))

    // Season 2 still has a video file pending
    pending := filepath.Join(s2, "Show.S02E01.mkv")
    require.NoError(t, os.WriteFile(pending, []byte("data"), 0644))

    // Season 1 is now empty — last episode just moved (not on disk)
    movedFile := filepath.Join(s1, "Show.S01E02.mkv")

    h := makeHandler(t, []string{root})
    h.cleanupSourceDir(movedFile)

    // Season 1 dir should be gone (empty, no videos)
    _, err := os.Stat(s1)
    assert.True(t, os.IsNotExist(err), "empty Season 1 should be removed")

    // packDir must remain (Season 2 still has a pending video)
    _, err = os.Stat(packDir)
    assert.NoError(t, err, "pack dir must remain while Season 2 has pending video")
}

func TestCleanupSourceDir_LastEpisodeSeasonPack_FullCleanup(t *testing.T) {
    root := t.TempDir()
    packDir := filepath.Join(root, "Show.S01.1080p")
    s1 := filepath.Join(packDir, "Season 1")
    require.NoError(t, os.MkdirAll(s1, 0755))

    // Only junk remains in packDir
    junk := filepath.Join(packDir, "nzb_marker.txt")
    require.NoError(t, os.WriteFile(junk, []byte("x"), 0644))

    // Last video just moved (not on disk)
    movedFile := filepath.Join(s1, "Show.S01E01.mkv")

    h := makeHandler(t, []string{root})
    h.cleanupSourceDir(movedFile)

    // Both Season 1 and packDir should be gone
    _, err := os.Stat(s1)
    assert.True(t, os.IsNotExist(err), "Season 1 should be removed")
    _, err = os.Stat(packDir)
    assert.True(t, os.IsNotExist(err), "pack dir should be removed (only junk remained)")

    // Watch root must still exist
    _, err = os.Stat(root)
    assert.NoError(t, err)
}

func TestContainsVideoFilesRecursive(t *testing.T) {
    root := t.TempDir()
    subDir := filepath.Join(root, "sub")
    require.NoError(t, os.MkdirAll(subDir, 0755))

    h := makeHandler(t, []string{})

    // Empty dir: no videos
    assert.False(t, h.containsVideoFilesRecursive(root))

    // Junk file only: no videos
    require.NoError(t, os.WriteFile(filepath.Join(root, "marker.txt"), []byte("x"), 0644))
    assert.False(t, h.containsVideoFilesRecursive(root))

    // Add a video in subdirectory
    require.NoError(t, os.WriteFile(filepath.Join(subDir, "ep.mkv"), []byte("x"), 0644))
    assert.True(t, h.containsVideoFilesRecursive(root))
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/nomadx/Documents/jellywatch && go test ./internal/daemon/... -run "TestCleanup|TestContainsVideo" -v
```

Expected: compile error — `h.cleanupSourceDir` and `h.containsVideoFilesRecursive` undefined.

- [ ] **Step 3: Add `containsVideoFilesRecursive` and `cleanupSourceDir` to handler.go**

Add `"io/fs"` to the import block in `handler.go` if not already present.

Insert the two new methods after the closing `}` of `IsMediaFile()` at line 639:

```go
// containsVideoFilesRecursive reports whether dir or any of its descendants
// contains at least one file recognised by IsMediaFile.
// Short-circuits on the first match via fs.SkipAll.
func (h *MediaHandler) containsVideoFilesRecursive(dir string) bool {
    found := false
    _ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
        if err != nil {
            return nil
        }
        if !d.IsDir() && h.IsMediaFile(path) {
            found = true
            return fs.SkipAll
        }
        return nil
    })
    return found
}

// cleanupSourceDir removes the source download directory after a successful move,
// walking up parent directories until it reaches a watch root.
//
// Gate: if sourcePath still exists the source was preserved (keepSource, dry-run),
// so cleanup is skipped entirely.
//
// At each level it checks whether any video files remain; if none do, the whole
// directory tree at that level is removed via os.RemoveAll (clearing junk files,
// SABnzbd markers, empty subdirs, etc.). Concurrent RemoveAll on the same path is
// safe — os.RemoveAll returns nil for non-existent paths.
func (h *MediaHandler) cleanupSourceDir(sourcePath string) {
    // Gate: source still present means keepSource or dry-run — do nothing.
    if _, err := os.Stat(sourcePath); err == nil {
        return
    }

    // Build a set of watch roots for O(1) boundary checks.
    // filepath.Clean strips trailing slashes, matching normalizeEventPath behaviour.
    rootSet := make(map[string]struct{})
    for _, p := range h.tvWatchPaths {
        rootSet[filepath.Clean(p)] = struct{}{}
    }
    for _, p := range h.movieWatchPaths {
        rootSet[filepath.Clean(p)] = struct{}{}
    }

    dir := filepath.Dir(sourcePath)
    for {
        clean := filepath.Clean(dir)
        if _, isRoot := rootSet[clean]; isRoot {
            return // never remove a watch root
        }

        if h.containsVideoFilesRecursive(dir) {
            return // more episodes pending — leave directory alone
        }

        if err := os.RemoveAll(dir); err != nil {
            h.logger.Warn("handler", "Failed to remove source directory",
                logging.F("dir", dir), logging.F("error", err.Error()))
            // Continue walking up — a permission error on one level
            // should not prevent cleanup of parent directories.
        } else {
            h.logger.Info("handler", "Cleaned up source directory", logging.F("dir", dir))
        }

        parent := filepath.Dir(dir)
        if parent == dir {
            return // filesystem root reached — stop
        }
        dir = parent
    }
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /home/nomadx/Documents/jellywatch && go test ./internal/daemon/... -run "TestCleanup|TestContainsVideo" -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/nomadx/Documents/jellywatch
git add internal/daemon/handler.go internal/daemon/handler_test.go
git commit -m "feat: add cleanupSourceDir and containsVideoFilesRecursive to MediaHandler"
```

---

### Task 2: Wire cleanup into `processFile()`

**Files:**
- Modify: `internal/daemon/handler.go` — add one call after `sendNotificationsWithTracking` inside the `result.Success` block

The injection point is inside the `if result.Success {` block, after `sendNotificationsWithTracking` (currently line 530) and before the closing `}` of that block.

- [ ] **Step 1: Add the cleanup call in `processFile()`**

Find the `result.Success` block in `processFile()`. It currently ends:

```go
		// Send notifications and track results
		sonarrNotified, radarrNotified = h.sendNotificationsWithTracking(result, mediaType)
	} else if result.Skipped {
```

Change to:

```go
		// Send notifications and track results
		sonarrNotified, radarrNotified = h.sendNotificationsWithTracking(result, mediaType)

		// Clean up the now-empty source download directory.
		h.cleanupSourceDir(path)
	} else if result.Skipped {
```

- [ ] **Step 2: Run the full daemon test suite**

```bash
cd /home/nomadx/Documents/jellywatch && go test ./internal/daemon/... -v 2>&1 | grep -E "^(--- PASS|--- FAIL|FAIL|ok)"
```

Expected: all PASS.

- [ ] **Step 3: Commit**

```bash
cd /home/nomadx/Documents/jellywatch
git add internal/daemon/handler.go
git commit -m "feat: wire cleanupSourceDir into processFile after successful organize"
```

---

### Task 3: Wire cleanup into `applyAIResult()`

**Files:**
- Modify: `internal/daemon/handler.go` — add one call inside the `result.Success` block of `applyAIResult`

The injection point is after `RecordTV` (currently line 1012) and before the closing `}` of the `if result != nil && result.Success` block.

- [ ] **Step 1: Add the cleanup call in `applyAIResult()`**

Find the success block in `applyAIResult()`. It currently reads:

```go
	if result != nil && result.Success {
		h.logger.Info("handler", "AI-enhanced organization successful",
			logging.F("filename", item.Filename),
			logging.F("target", result.TargetPath))
		if item.MediaType == "movie" {
			h.stats.RecordMovie(result.BytesCopied)
		} else {
			h.stats.RecordTV(result.BytesCopied)
		}
	}
```

Change to:

```go
	if result != nil && result.Success {
		h.logger.Info("handler", "AI-enhanced organization successful",
			logging.F("filename", item.Filename),
			logging.F("target", result.TargetPath))
		if item.MediaType == "movie" {
			h.stats.RecordMovie(result.BytesCopied)
		} else {
			h.stats.RecordTV(result.BytesCopied)
		}

		// Clean up the now-empty source download directory.
		h.cleanupSourceDir(item.Path)
	}
```

- [ ] **Step 2: Run the full daemon test suite**

```bash
cd /home/nomadx/Documents/jellywatch && go test ./internal/daemon/... -v 2>&1 | grep -E "^(--- PASS|--- FAIL|FAIL|ok)"
```

Expected: all PASS.

- [ ] **Step 3: Commit**

```bash
cd /home/nomadx/Documents/jellywatch
git add internal/daemon/handler.go
git commit -m "feat: wire cleanupSourceDir into applyAIResult for AI-enhanced path"
```

---

### Task 4: Build, install, and verify

- [ ] **Step 1: Run the full build**

```bash
cd /home/nomadx/Documents/jellywatch && go build ./...
```

Expected: clean build, no errors.

- [ ] **Step 2: Run all tests**

```bash
cd /home/nomadx/Documents/jellywatch && go test ./internal/daemon/... ./internal/organizer/... ./internal/sync/... -count=1
```

Expected: all packages PASS.

- [ ] **Step 3: Build binaries**

```bash
cd /home/nomadx/Documents/jellywatch
go build -o /tmp/jellywatch ./cmd/jellywatch && go build -o /tmp/jellywatchd ./cmd/jellywatchd && echo "Build OK"
```

- [ ] **Step 4: Install and restart service**

```bash
echo "dnommm78" | sudo -S systemctl stop jellywatchd.service
echo "dnommm78" | sudo -S cp /tmp/jellywatch /usr/local/bin/jellywatch
echo "dnommm78" | sudo -S cp /tmp/jellywatchd /usr/local/bin/jellywatchd
echo "dnommm78" | sudo -S systemctl start jellywatchd.service
sleep 2
systemctl status jellywatchd.service --no-pager | head -20
```

Expected: `Active: active (running)`.

- [ ] **Step 5: One-time cleanup of existing empty watch dirs**

Now that the daemon handles future cleanup automatically, sweep existing empty download folders once:

```bash
# Dry-run first — review what would be removed
for dir in /mnt/NVME3/Sabnzbd/complete/movies/*/; do
  if ! find "$dir" \( -name "*.mkv" -o -name "*.mp4" -o -name "*.avi" \
    -o -name "*.mov" -o -name "*.wmv" -o -name "*.flv" -o -name "*.webm" \
    -o -name "*.m4v" -o -name "*.mpg" -o -name "*.mpeg" -o -name "*.m2ts" \
    -o -name "*.ts" \) 2>/dev/null | grep -q .; then
    echo "Would remove: $dir"
  fi
done

# Then remove after reviewing the above output
for dir in /mnt/NVME3/Sabnzbd/complete/movies/*/; do
  if ! find "$dir" \( -name "*.mkv" -o -name "*.mp4" -o -name "*.avi" \
    -o -name "*.mov" -o -name "*.wmv" -o -name "*.flv" -o -name "*.webm" \
    -o -name "*.m4v" -o -name "*.mpg" -o -name "*.mpeg" -o -name "*.m2ts" \
    -o -name "*.ts" \) 2>/dev/null | grep -q .; then
    rm -rf "$dir" && echo "Removed: $dir"
  fi
done

# Repeat for TV
for dir in /mnt/NVME3/Sabnzbd/complete/tv/*/; do
  if ! find "$dir" \( -name "*.mkv" -o -name "*.mp4" -o -name "*.avi" \
    -o -name "*.mov" -o -name "*.wmv" -o -name "*.flv" -o -name "*.webm" \
    -o -name "*.m4v" -o -name "*.mpg" -o -name "*.mpeg" -o -name "*.m2ts" \
    -o -name "*.ts" \) 2>/dev/null | grep -q .; then
    rm -rf "$dir" && echo "Removed: $dir"
  fi
done
```
