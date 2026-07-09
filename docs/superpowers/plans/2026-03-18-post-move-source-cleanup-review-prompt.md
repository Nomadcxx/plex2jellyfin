# Review Request: Post-Move Source Directory Cleanup

## What You Are Reviewing

An implementation plan for a new feature in the **plex2jellyfin-daemon** Go daemon. The plan is at:

`docs/superpowers/plans/2026-03-18-post-move-source-cleanup.md`

The spec it is derived from is at:

`docs/superpowers/specs/2026-03-18-post-move-source-cleanup-design.md`

---

## Feature Summary

After plex2jellyfin-daemon successfully moves a media file from a SABnzbd download directory to a Jellyfin media library, the source download folder is left behind as an empty shell containing only junk files (SABnzbd `.txt` markers, etc.). The feature adds automatic cleanup: after each successful move the daemon removes the source directory, walks up to clean empty parent directories, and stops at the configured watch root boundary.

---

## How the Daemon Works (Context for the Reviewer)

- `internal/daemon/handler.go` — the central event handler. Two code paths lead to a successful organize:
  1. `processFile()` — the main path for files with regex-parseable names
  2. `applyAIResult()` — the AI-enhanced path for low-confidence filenames, which bypasses `processFile` entirely
- Files arrive from SABnzbd in a watch directory (e.g. `/mnt/NVME3/Sabnzbd/complete/movies/`). Each download is in its own subfolder. Season packs can have nested subfolders (`Show.S01-S04/Season 1/`, `Show.S01-S04/Season 2/`, etc.).
- After a successful move, rsync deletes the source **file**. The source **directory** and any junk files remain.
- The `MediaHandler` struct holds `tvWatchPaths` and `movieWatchPaths` — these are the watch root boundaries that cleanup must never cross.
- `IsMediaFile()` at line 631 of `handler.go` is the existing extension check (12 video extensions). The new `containsVideoFilesRecursive()` must reuse this rather than duplicating the list.

---

## Design Decisions and Reasoning

**Why the handler, not the organizer:**
The organizer (`internal/organizer/organizer.go`) is a library-focused component — it knows about target libraries but not about watch roots. Cleanup needs the watch root boundary. Putting it in the handler keeps the organizer's concerns clean.

**Gate: source file existence check:**
Cleanup is only triggered when `os.Stat(sourcePath)` returns an error (file not found). This single check covers: normal moves (file gone), `keepSource` mode (file still there, skip cleanup), and dry-run (file still there, skip cleanup). No need to thread a flag through.

**Bottom-up walk:**
Season packs drain one episode at a time. Each episode's cleanup walks up from its parent. The intermediate directories are only removed once they contain no video files anywhere in their subtree. This means a `Show.S01-S04/` folder will naturally survive until the last episode of the last season is processed.

**`os.RemoveAll` for the whole directory:**
Once we determine no video files remain, the entire directory (including junk `.txt`, `.nzb`, etc.) is removed in one call. No need to selectively delete individual files.

**`fs.SkipAll` in the video walk:**
`containsVideoFilesRecursive` short-circuits on the first video found using `fs.SkipAll` (Go 1.20+, this project uses 1.24). This prevents scanning an entire large season pack when we only need to know if _any_ video is present.

**Concurrent safety:**
The daemon processes multiple files concurrently (one goroutine per debounced file event). Two goroutines could both determine a directory is empty and both call `os.RemoveAll` on the same path. This is safe — `os.RemoveAll` returns nil on a non-existent path.

---

## What to Review

Please verify:

1. **Correctness of the two new methods** (`containsVideoFilesRecursive`, `cleanupSourceDir`) — logic, edge cases, safety.
2. **Test quality** — do the tests in Task 1 actually verify the intended behavior? Are edge cases covered? Is the TDD order correct (tests before implementation)?
3. **Injection points** — Task 2 (`processFile`) and Task 3 (`applyAIResult`) — are the right lines being modified? Would cleanup fire at the wrong time (e.g. on a skip result)?
4. **One-time cleanup script** in Task 4 Step 5 — is the `find` command correct, safe for the described directory structure?
5. **Anything missing** from the spec that isn't in the plan.

---

## Key Files to Read

- `docs/superpowers/plans/2026-03-18-post-move-source-cleanup.md` — the plan (primary review target)
- `docs/superpowers/specs/2026-03-18-post-move-source-cleanup-design.md` — the spec
- `internal/daemon/handler.go` — the file being modified; pay attention to:
  - `processFile()` — injection point for Task 2
  - `applyAIResult()` — injection point for Task 3
  - `IsMediaFile()` at line 631 — the extension check the new code reuses
  - `MediaHandlerConfig` and `MediaHandler` struct — `tvWatchPaths`, `movieWatchPaths` fields
- `internal/daemon/handler_test.go` — existing test patterns (uses `logging.Nop()`, not `logging.NewNoop()`)
