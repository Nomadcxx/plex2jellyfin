# Post-Move Source Directory Cleanup

## Problem

After jellywatchd successfully moves a media file from a watch directory to a library, the source directory is left behind. SABnzbd download folders accumulate as empty shells containing only junk files (`.txt` markers, `.nzb` files, etc.). This requires manual cleanup.

## Examples

**Simple download:**
```
complete/movies/Movie.Name.2025.1080p-GROUP/
├── movie.mkv          ← moved to library
└── abc123.txt         ← SABnzbd marker, left behind
```

**Season pack:**
```
complete/tv/Show.S01-S04.1080p/
├── Season 1/
│   ├── S01E01.mkv     ← moved (first)
│   └── S01E02.mkv     ← moved (second, triggers Season 1/ cleanup)
├── Season 2/
│   └── S02E01.mkv     ← still pending, blocks parent cleanup
└── nzb_marker.txt
```

## Design

### Injection Point

`internal/daemon/handler.go` in `processFile()`, after a successful organize call returns. The handler owns the watch roots (safety boundary), so cleanup belongs here rather than in the organizer.

### Algorithm

```
cleanupSourceDir(sourcePath, watchRoots):
  dir = parent(sourcePath)
  for dir is not a watchRoot:
    if containsVideoFilesRecursive(dir):
      return                    // more episodes pending, leave it
    os.RemoveAll(dir)           // only junk remains, nuke it
    log INFO "cleaned up source directory" dir
    dir = parent(dir)           // walk up
```

**Bottom-up, patient approach:**
1. After a file moves, check its immediate parent directory
2. If no video files remain anywhere under that directory → `os.RemoveAll` (removes junk, subdirs, everything)
3. Walk up to the next parent, repeat the check
4. Stop when hitting a watch root (never remove watch roots) or when videos are still present

### Safety Rails

- **Watch root boundary:** Hard check via `filepath.Clean` comparison against all configured watch roots. Never removes a watch root.
- **Only on success:** Runs only after a successful organize result (not on skips, errors, or dry runs).
- **Video check is recursive:** `containsVideoFilesRecursive(dir)` walks the entire subtree using `filepath.WalkDir`, checking the same video extension set the watcher uses (`.mkv`, `.mp4`, `.avi`, `.mov`, `.wmv`, `.flv`, `.webm`, `.m4v`, `.mpg`, `.mpeg`, `.m2ts`, `.ts`).
- **Logged:** Each directory removal is logged at INFO level for auditability.

### Files Modified

| File | Change |
|------|--------|
| `internal/daemon/handler.go` | Add `cleanupSourceDir()` method; call it after successful organize in `processFile()` |

### No New Dependencies

Uses only `os.RemoveAll`, `os.ReadDir`, `filepath.WalkDir`, `filepath.Clean`, `filepath.Dir` — all stdlib.

### Edge Cases

| Scenario | Behavior |
|----------|----------|
| Single file directly in watch root (no subfolder) | `parent(file) == watchRoot` → loop exits immediately, no cleanup |
| Season pack with episodes still pending | `containsVideoFilesRecursive` finds remaining `.mkv` files → stops, leaves directory |
| Last episode of a multi-season pack finishes | Walk removes leaf season dir, then parent pack dir (now empty/junk-only), stops at watch root |
| Nested SABnzbd structure (`_UNPACK_` leftover) | `_UNPACK_` dirs contain no video files → removed as junk |
| Download folder has subtitle files (`.srt`) but no video | Treated as junk — removed. Subtitles should already be copied by `OrganizeFolder` if applicable |
| Dry run mode | Organizer returns success but doesn't move → cleanup should NOT run. Gated on `!dryRun` or checking that the source file no longer exists |
| Concurrent processing of episodes from same folder | Safe: `containsVideoFilesRecursive` is a read-only check. If another episode is still present, cleanup stops. Race window is narrow (debounce timer separates processing by 10s) |
