# Post-Move Source Directory Cleanup

## Problem

After plex2jellyfin-daemon successfully moves a media file from a watch directory to a library, the source directory is left behind. SABnzbd download folders accumulate as empty shells containing only junk files (`.txt` markers, `.nzb` files, etc.). This requires manual cleanup.

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

### Injection Points

Two call sites in `internal/daemon/handler.go`:

1. **`processFile()`** — after a successful organize call returns (the main path)
2. **`applyAIResult()`** — after a successful organize call returns for AI-enhanced files (these bypass `processFile` and would otherwise never get cleaned up)

Both call the same `cleanupSourceDir()` method.

The handler is the right place (not the organizer) because the handler owns the watch roots — the safety boundary that prevents cleanup from escaping the download area.

### Gate: Source File Existence Check

Cleanup runs only when the source file **no longer exists** at its original path. This is the single canonical gate and naturally handles all preservation scenarios:

- **Normal move:** rsync deletes source → file gone → cleanup runs
- **`keepSource` mode:** organizer copies instead of moving → file still exists → cleanup skipped
- **Dry run:** organizer returns success but doesn't move → file still exists → cleanup skipped

### Algorithm

```
cleanupSourceDir(sourcePath, watchRoots):
  if fileExists(sourcePath):
    return                        // source preserved (keepSource, dryRun, etc.)
  dir = parent(sourcePath)
  for dir is not a watchRoot:
    if containsVideoFilesRecursive(dir):
      return                      // more episodes pending, leave it
    os.RemoveAll(dir)             // only junk remains, nuke it
    log INFO "cleaned up source directory" dir
    dir = parent(dir)             // walk up
```

**Bottom-up, patient approach:**
1. Verify source file was actually removed (gate check)
2. Check the immediate parent directory for remaining video files
3. If no video files remain anywhere under that directory → `os.RemoveAll` (removes junk, subdirs, everything)
4. Walk up to the next parent, repeat the check
5. Stop when hitting a watch root (never remove watch roots) or when videos are still present

### Safety Rails

- **Watch root boundary:** Hard check via `filepath.Clean` comparison against all configured watch roots. Never removes a watch root. Note: `filepath.Clean` strips trailing slashes, matching the normalization already used in `normalizeEventPath()`.
- **Source existence gate:** Only runs when source file no longer exists at original path (see Gate section).
- **Video check reuses `IsMediaFile`:** `containsVideoFilesRecursive(dir)` walks the subtree using `filepath.WalkDir` and calls the existing `IsMediaFile()` helper (handler.go line 631) rather than duplicating the extension set.
- **Logged:** Each directory removal is logged at INFO level. Failures from `os.RemoveAll` are logged at WARN level and do not halt the walk — cleanup continues up the tree.
- **Concurrent safety:** If two goroutines process the last two episodes simultaneously and both determine the parent is empty, both may call `os.RemoveAll` on the same path. This is safe — `os.RemoveAll` on a non-existent path returns nil (no-op).

### Files Modified

| File | Change |
|------|--------|
| `internal/daemon/handler.go` | Add `cleanupSourceDir()` and `containsVideoFilesRecursive()` methods; call cleanup after successful organize in both `processFile()` and `applyAIResult()` |

### No New Dependencies

Uses only `os.RemoveAll`, `os.Stat`, `filepath.WalkDir`, `filepath.Clean`, `filepath.Dir` — all stdlib.

### Edge Cases

| Scenario | Behavior |
|----------|----------|
| Single file directly in watch root (no subfolder) | `parent(file) == watchRoot` → loop exits immediately, no cleanup |
| Season pack with episodes still pending | `containsVideoFilesRecursive` finds remaining `.mkv` files → stops, leaves directory |
| Last episode of a multi-season pack finishes | Walk removes leaf season dir, then parent pack dir (now empty/junk-only), stops at watch root |
| Nested SABnzbd structure (`_UNPACK_` leftover) | `_UNPACK_` dirs contain no video files → swept up by `RemoveAll` on parent |
| Download folder has subtitle files (`.srt`) but no video | Treated as junk — removed. Subtitles should already be copied by `OrganizeFolder` if applicable |
| `keepSource` / dry run | Source file still exists at original path → gate check skips cleanup entirely |
| Concurrent processing of last two episodes in same folder | Both may call `RemoveAll` on same path — safe, `RemoveAll` is no-op on missing paths |
| `os.RemoveAll` fails (permissions, etc.) | Logged at WARN, cleanup continues walking up rather than aborting |

### Test Plan

| Test | Assertion |
|------|-----------|
| Single movie download: source dir cleaned after move | Parent dir and junk files removed |
| Season pack with remaining episodes: cleanup stops | Parent dir still exists with remaining video files |
| Last episode of season pack: full tree cleaned | Leaf dir, parent dir removed; stops at watch root |
| File directly in watch root (no subfolder) | No cleanup attempted, watch root untouched |
| Source file still exists (keepSource/dryRun gate) | No cleanup runs |
| `os.RemoveAll` permission error | Logged at WARN, walk continues upward |
| Concurrent cleanup of same directory | No errors, no panics |
| Watch root with trailing slash in config | `filepath.Clean` normalizes, cleanup still respects boundary |
