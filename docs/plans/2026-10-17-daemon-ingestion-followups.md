# Daemon Ingestion Follow-ups Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Bring seeding-safe torrent intake onto current `main` and let the daemon import multi-episode ranges when it can safely replace every overlapping episode file.

**Architecture:** Keep the existing torrent source-policy design, including its config and setup surfaces, and integrate current `main`'s retry suppression with the feature branch's reloadable watch paths. Keep range conflict decisions in the organizer: discover all intersecting files, prove full episode coverage and better quality from import history, publish the range, then remove replaced files and refresh inventory. If evidence is missing, retain the source and library files and report a reviewable conflict.

**Tech Stack:** Go, existing SQLite database, `internal/naming`, `internal/quality`, `internal/transfer`, and the current daemon/config/setup packages.

---

### Task 1: Rebase the torrent branch onto current `main`

**Files:**
- Modify during conflict resolution: `internal/scanner/periodic.go`
- Review: `internal/scanner/periodic_test.go`, `internal/daemon/reload/scanner_reloadable.go`, `internal/daemon/reload/scanner_reloadable_test.go`

**Step 1: Confirm branch and worktree state**

Run from `/home/nomadx/Documents/plex2jellyfin/.worktrees/torrent-ingestion`:

```bash
git status --short --branch
git log --oneline --left-right main...HEAD
```

Expected: clean `feat/torrent-ingestion`; feature commits are replayed onto `main`.

**Step 2: Rebase onto current `main`**

```bash
git rebase main
```

Expected: rebase completes or pauses for conflict resolution. The read-only merge preview identified `internal/scanner/periodic.go` as the known conflict; resolve any additional replay conflicts without dropping behavior from either side.

**Step 3: Preserve both periodic scanner behaviors**

In `internal/scanner/periodic.go`, retain `retrySuppressed` from `main`, and retain the torrent branch's copied watch-path slice, `ReplaceWatchPaths`, `WatchPaths`, and locked reads in `Start` and `scanWatchDirectories`. Run `gofmt` on the resolved file.

**Step 4: Run focused scanner and reload checks**

```bash
go test ./internal/scanner ./internal/daemon/reload -count=1
```

Expected: both packages pass, covering retry suppression, watch-path replacement, and reload rollback.

**Step 5: Review the rebased feature diff**

```bash
git diff --check main...HEAD
git diff --stat main...HEAD
```

Expected: no whitespace errors; all config, daemon, organizer, transfer, setup, and frontend surfaces from the approved torrent design remain present.

---

### Task 2: Add overlap decision regression cases

**Files:**
- Modify: `internal/organizer/organizer_test.go`
- Use existing test helpers: `setupTestEnv`, `createTestFile`, `recordingTransferer`, and the temporary SQLite database helpers

**Step 1: Add a test for a safely replaceable single-episode overlap**

Set up an incoming `S01E07-E08` source and an existing lower-quality `S01E07` target with a successful import decision that records the target's original source filename. Assert that the incoming range is imported, the lower-quality single is removed only after transfer succeeds, and the canonical range target exists.

**Step 2: Add a test for an existing range that extends beyond the candidate**

Set up existing `S01E07-E09` and incoming `S01E07-E08`. Assert the organizer reports a reviewable overlap, preserves both files, and performs no transfer.

**Step 3: Add tests for equal, lower, and unavailable quality evidence**

For each case, assert the source and all existing files remain in place and the transferer is not called. The unavailable case has no successful decision for the existing target.

**Step 4: Add a transfer-failure preservation check**

Make the recording transferer fail for the incoming range. Assert all old files remain and the source is retained.

**Step 5: Run the focused organizer tests to confirm the new cases expose the bug**

```bash
go test ./internal/organizer -run 'TestOrganizeTV.*(Range|Overlap)' -count=1
```

Expected: the safe replacement case fails against current behavior; the preservation cases pass.

---

### Task 3: Locate every existing file covered by a range

**Files:**
- Modify: `internal/organizer/dedup.go`
- Modify: `internal/organizer/dedup_test.go`

**Step 1: Add coverage tests**

Cover a single episode, an existing range beginning before the requested range, an existing range beginning inside it, and an unrelated season. The helper must return each matching physical path once.

**Step 2: Run the focused dedup check**

```bash
go test ./internal/organizer -run 'TestFind.*Episode.*Range' -count=1
```

Expected: FAIL because current lookup returns only the first file matching one episode key.

**Step 3: Add the smallest range lookup helper**

Scan video files in the season directory once, parse their season/start/end, and return unique paths whose episode coverage intersects the incoming range. Do not add a general indexing abstraction.

**Step 4: Re-run the focused check**

```bash
go test ./internal/organizer -run 'TestFind.*Episode.*Range' -count=1
```

Expected: PASS for each coverage case.

---

### Task 4: Replace only fully covered, lower-quality files

**Files:**
- Modify: `internal/organizer/organizer.go`
- Modify: `internal/database/media_files.go` only if an existing path lookup helper is needed
- Modify: `internal/organizer/organizer_test.go`

**Step 1: Build the overlap decision from existing records**

For each intersecting target, parse its full episode coverage. Resolve its most recent successful `ParseDecision` with `GetDecisionByTargetPath` and parse the recorded `SourceFilename` for quality. Compare that with the incoming source's release quality. If any target has episodes outside the candidate range, lacks quality evidence, or is equal/better quality, return a skip reason that preserves the established `multi-episode range overlaps existing episode` marker used by the handler's stable-skip logic.

**Step 2: Re-run organizer cases before changing transfer behavior**

```bash
go test ./internal/organizer -run 'TestOrganizeTV.*(Range|Overlap)' -count=1
```

Expected: quality and coverage guards pass; the higher-quality single-file replacement case remains the only expected failure.

**Step 3: Publish before removing conflicts**

Transfer the incoming range to its canonical target first. Only after successful transfer, remove the fully covered lower-quality files. Never remove the target itself or the source. On a filesystem removal error, leave its database row intact and report an inventory/conflict error.

**Step 4: Refresh database inventory for every changed path**

Extend `refreshInventoryAfterTransfer` to accept the replaced paths as a slice. Index the new target before deleting stale source/replaced rows, and remove rows only for files that were actually removed. Preserve the existing source-retention handling for seeding roots.

**Step 5: Run organizer regression checks**

```bash
go test ./internal/organizer -run 'TestOrganizeTV.*(Range|Overlap)|TestOrganizerRetainedSource' -count=1
```

Expected: safe replacements import once, uncertain overlaps preserve all existing data, and retained sources remain available on repeat scans.

---

### Task 5: Run package and repository checks

**Files:**
- Review: `internal/daemon`, `internal/scanner`, `internal/daemon/reload`, `internal/config`, `internal/transfer`, `internal/setup`, `cmd/installer`, and `cmd/plex2jellyfin`

**Step 1: Run focused source-policy and transfer checks**

```bash
go test ./internal/daemon/... ./internal/transfer ./internal/config ./internal/setup ./cmd/installer ./cmd/plex2jellyfin -count=1
```

Expected: all listed packages pass with ordinary move/copy and seeding source-retention behavior intact.

**Step 2: Run the full Go suite**

```bash
go test ./... -count=1
```

Expected: all Go packages pass.

**Step 3: Check formatting and final diff**

```bash
gofmt -w internal/organizer/dedup.go internal/organizer/organizer.go internal/organizer/dedup_test.go internal/organizer/organizer_test.go
git diff --check
git status --short
```

Expected: formatting is clean and only intended implementation files are changed beyond the approved torrent branch.

**Step 4: Commit the overlap fix separately**

```bash
git add internal/organizer/dedup.go internal/organizer/organizer.go internal/organizer/dedup_test.go internal/organizer/organizer_test.go internal/database/media_files.go
git commit -m "fix(organizer): safely replace episode overlaps"
```

Skip `internal/database/media_files.go` if Task 4 does not modify it.
