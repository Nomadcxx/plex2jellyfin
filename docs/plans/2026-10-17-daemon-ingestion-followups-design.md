# Daemon Ingestion Follow-ups Design

**Date:** 2026-10-17
**Status:** Approved

## Scope

This follow-up carries the approved seeding-safe torrent intake work forward from `feat/torrent-ingestion` onto current `main` and fixes the multi-episode overlap case found during the live daemon review.

The torrent work remains a filesystem intake contract. Ordinary watch roots keep their configured move/copy behavior. Seeding roots always retain the client's source, using a hardlink when possible and a copy across filesystems. The configured TV and Movies completion paths remain explicit so the daemon can classify media without client APIs.

## Current Evidence

- The torrent branch is clean and contains the previously approved source-retention design and implementation.
- It is based on the 0.1.11 release while `main` is at 0.1.12. A read-only merge preview found one content conflict in `internal/scanner/periodic.go`: current `main` adds retry suppression while the torrent branch adds reloadable watch paths. The merged file must preserve both behaviors.
- `OrganizeTVWithParsed` currently skips a multi-episode file when any member episode already exists. The handler then treats that skip as stable while the source and conflict target remain unchanged, so the missing episode is not imported automatically.
- A multi-episode media file is indivisible. Replacing an existing episode file can lose quality or unique episode coverage, and the canonical target filename does not retain the source release's quality tags.

## Approved Overlap Policy

The organizer may replace overlapping files only when both checks pass:

1. Every existing file that overlaps the incoming range is wholly covered by that incoming range. An existing range that also contains an episode outside the incoming range is preserved.
2. The incoming source is demonstrably higher quality than every file that would be replaced. Prefer the original source filename recorded for the successful import. If that evidence is missing or cannot be compared, preserve the existing file and report a reviewable conflict.

When both checks pass, publish the incoming range first. Remove the fully covered lower-quality files only after the transfer succeeds, then refresh the media inventory for the new target and removed paths. If publication fails, leave all existing files untouched. The incoming source remains intact under seeding policy.

When either check fails, leave the incoming source and all library files intact and return a specific overlap reason for the daemon activity log. The daemon must not call an ambiguous conflict an import success.

## Branch and Integration

Rebase the existing torrent worktree onto current `main`. Resolve `internal/scanner/periodic.go` by preserving both retry suppression and thread-safe watch-path replacement. Keep the torrent intake configuration, daemon policy, reload behavior, transfer support, and setup surfaces together because they form one user-configurable feature.

Keep the episode-range organizer change independently reviewable within the branch. Do not run a database repair or alter the live daemon as part of this code change. The existing approved torrent design remains the reference for setup, transfer, reload, validation, and deployment details.

## Verification Plan

- Check range coverage for a single-episode overlap, a fully covered existing range, and an existing range with an episode outside the new range.
- Check quality outcomes for higher, equal/lower, and unavailable source evidence. Existing files must remain untouched on every non-replacement path and on transfer failure.
- Check a retained seeding source through first import and repeat scan so the new overlap path does not consume or re-import client data.
- Recheck the merged periodic scanner behavior for retry suppression and watch-path replacement, and preserve the existing torrent configuration and setup round trips.
