# Movie-name self-correction — design

Date: 2026-07-22
Status: approved (design), pending implementation plan

## Problem

Release `scary.movie.extended.cut.2026.extended.1080p.10bit.webrip.6ch.x265.hevc-psa`
was parsed to title "scary movie cut" and organized into
`/mnt/STORAGE1/MOVIES/Scary Movie Cut (2026)/`. The correct title is
"Scary Movie" (2026); the parser kept the word "cut" from "extended cut".
Jellyfin cannot match "Scary Movie Cut" against TMDB, so the item has empty
`ProviderIds` and shows unidentified.

### Verified guardrail trace

Every guardrail was checked against code, the live `media.db`, and the Jellyfin
API. The item's `parse_decisions` row (id 232) shows `jellyfin_identified=0`,
`metadata_state=missing_provider_ids`, `metadata_check_count=3`,
`metadata_repair_count=0`. Jellyfin's own view of the item shows `ProviderIds: {}`.

1. Primary parser (`internal/naming/naming.go`) strips `EXTENDED` but has no
   edition-cut vocabulary — the bad title is born here.
2. Advanced parser (`internal/naming/advanced.go`) knows "cut" is an edition
   marker but is unreachable: it only runs as a fallback when the primary parse
   fails or yields a garbage-looking title, and "scary movie cut" looks
   plausible; and even its "cut" handling only fires for hyphen-glued tokens.
3. AI enhancement only triggers at parse time on low-confidence/garbage parses,
   not on a confident-but-wrong regex parse.
4. `looksPolluted` (housekeeping) only flags folders still containing raw release
   tags (1080P, WEB-DL, x264); "Scary Movie Cut" is clean of those.
5. Parser-drift repair (`internal/housekeeping/engine.go`, `parserDriftMovieRename`)
   re-runs the same deterministic parser on the same source filename, reproduces
   "scary movie cut", so `srcPath == dstPath` and no drift is detected. It heals
   imports only after the parser *code* improves; it is structurally blind to a
   stable mis-parse.
6. The Jellyfin/TMDB reconciler (`internal/jellyfin/metadata_recovery.go`,
   `sweep.go`) DID detect it — it set `jellyfin_identified=0` and
   `metadata_state=missing_provider_ids`.

### Root cause

Detection works; remediation is the wrong tool. The only remediation for
`missing_provider_ids` is `RefreshItemFullMetadata` — asks Jellyfin to re-run its
TMDB search against the *current, wrong* name, which fails identically forever;
after N attempts the row parks in `needs_review`. There is no feedback edge from
"Jellyfin/TMDB cannot identify this" back to "the title is probably wrong —
re-derive it and rename."

Supporting facts (all code-verified):
- `RunPassive` (the hourly reconciler loop, `cmd/plex2jellyfin-daemon/main.go:868`)
  only classifies. The reconciler's `RunRepair` has no automatic loop; it is
  manual WebUI/CLI only.
- The `metadata_recovery.repair_enabled` flag does **not** gate the reconciler;
  it gates a separate TV-only `RepairUnknownSeasons` routine
  (`main.go:636`). There is currently no automatic movie remediation at all.

## Approach (phased)

Two phases. Phase 1 fixes the motivating bug and its whole class using existing,
tested machinery. Phase 2 adds a TMDB-feedback edge for the residual long tail.

### Design decisions (owner-approved)

- Safety posture: auto-apply only on an exact-title **and** exact-year match to a
  *different* title; everything less certain is flagged for human review.
- Grace period: an item must be `missing_provider_ids` for >= 48h before
  verifier-driven correction is attempted (gives genuinely-new releases time to
  land in TMDB; guards a correctly-named new release Jellyfin has not matched
  yet). The positive-match requirement is the strong guard; 48h is the weak guard
  whose main job is to avoid churning API calls on new releases.
- Candidate generation: vocab-strip known edition words first, then progressive
  trailing-token trim as fallback.

---

## Phase 1 — parser vocabulary + drift executor hardening

Fixes the motivating bug and the entire edition-word-leak class with existing
machinery, and eliminates the Phase-2 ping-pong risk at its source.

### 1.1 Edition-cut vocabulary in the primary parser

Add edition-cut markers to the primary release-strip patterns at
`internal/naming/naming.go:69` (the list currently containing
`PROPER|REPACK|iNTERNAL|LIMITED|EXTENDED|REMASTERED|…`): `CUT`,
`DIRECTOR'?S CUT`, `FINAL CUT`, `THEATRICAL`, `UNCUT`, `UNRATED`, and any others
already known to `advanced.go`'s `editionMarkers`. Ensure dot/space-separated
tokens are handled, not only hyphen-glued forms.

Guard against false stripping of real titles that legitimately contain these
words (e.g. "Final Cut (2022)", "Time Cut (2024)", "Urban Legends: Final Cut
(2000)"). The strip must be conservative — only strip an edition token when it is
release metadata (typically trailing, adjacent to other release tokens), never
when it is an interior title word. Cover these known-good titles as regression
fixtures so they are not damaged.

Deliverable: corpus test asserting `scary.movie.extended.cut.2026...` →
"Scary Movie" (2026), plus the three known-good titles above remain unchanged.

### 1.2 Drift executor hardening

Once 1.1 ships, the existing `detectParserDriftRepairs` / `parserDriftMovieRename`
will re-derive `Scary Movie (2026)` from the source filename, see it differs from
the current `Scary Movie Cut (2026)` folder, and enqueue a rename that the
existing `execParserDriftRename` executes. Two fixes to that executor
(`internal/housekeeping/engine.go`):

1. **Stop resetting `TargetAt` to now.** The current `UpdateOrganize` call sets
   `TargetAt: &now`, which resets the 48h grace clock and re-triggers the
   15-minute `recentlyImported` wait. A rename is not a fresh import — preserve
   the original `TargetAt`.
2. **Add a targeted Jellyfin rescan after the move.** No targeted-rescan
   primitive exists today: `RefreshLibrary()` scans all libraries (violates the
   hard no-library-wide-refresh constraint — it would restamp DateCreated across
   the library and flood Recently Added for all users), and per-item
   `RefreshItem*` operate on a now-stale item ID. Add one client method wrapping
   Jellyfin's `POST /Library/Media/Updated` scoped to the new path, and call it
   after a successful move. This scans only that path with no DateCreated
   restamp.

### Hard constraints (Phase 1)

- Never trigger a library-wide Jellyfin metadata refresh.
- Do not damage titles that legitimately contain edition words.

---

## Phase 2 — verifier-driven corrector (long tail)

For stable mis-parses that vocabulary cannot fix: unknown/novel edition words,
and garbage-but-plausible titles. Adds the genuine TMDB-feedback edge.

### 2.1 Unit: `MovieNameCorrector`

A self-contained decision unit. Input: a persistently-unidentified movie
`parse_decision`. Output: one of {auto-correct with a new title/year/tmdb_id,
flag-for-review, leave-alone}. Depends only on `tmdb.Verifier` and the row.
Unit-testable with a fake verifier. It performs **no** filesystem mutation and
**no** enqueue — it only decides.

Procedure:
1. Confirm the current name is genuinely unmatched: `verifier.lookup(currentTitle,
   year)` with the strict-year semantics below. If it matches → leave alone (the
   name is fine, Jellyfin is behind). This guards correctly-named releases.
2. Generate candidates: (a) strip known edition words; (b) if none match,
   progressive trailing-token trim ("Scary Movie Cut" → "Scary Movie" → "Scary").
   Cap auto-apply candidates at >= 2 tokens.
3. Verify each candidate at the parsed year with strict-year semantics. Accept
   the first candidate that returns an exact-title **and** exact-year match to a
   different title.
4. Outcome:
   - Confident exact-title+exact-year match to a different title, candidate
     >= 2 tokens → auto-correct.
   - Candidates tried, only a loose/near/single-token match → flag-for-review.
   - Nothing matched either way → leave-alone (too new / obscure); no rename.

### 2.2 Strict-year lookup (mandatory)

`tmdb.Verifier.lookup` today does **not** enforce exact year: `pickByYear`
(`internal/tmdb/verifier.go:357`) allows a ±1-year tolerance, and `lookup` has a
single-candidate fallback that ignores year entirely. Left as-is, this makes
`Avatar Fire and Ash (2025)` → trim "Avatar" → single exact-title candidate →
**Avatar (2009)** reachable as an auto-rename.

For auto-apply the corrector must require integer-equal year against
`match.Year`, after lookup returns. The ±1 tolerance and the single-candidate
fallback are permitted only to produce **review-only** suggestions, never
auto-apply. Implement either as a `lookupStrict` variant or as a post-lookup
re-check in the corrector; do not weaken the existing `lookup` used by
housekeeping dedup.

### 2.3 Trigger seam: enqueue, do not mutate from `RunPassive`

When the reconciler classifies a **movie** row as `missing_provider_ids` with
`target_at` older than 48h and `correction_enabled` is set, it runs the corrector
(pure verifier calls — safe inline) and **enqueues** the outcome as a
housekeeping task. It never renames from within `RunPassive` (a read-only
classifier whose contract is "read Jellyfin, update DB"). Enqueuing through the
housekeeping queue reuses the bounded-concurrency drainer, dedup, the
repair-event audit trail, and the existing human-approval WebUI path.

- Auto-correct outcome → a new auto-apply task kind (2.4).
- Flag-for-review outcome → a flag-kind task mirroring `TaskKindYearMismatch`,
  which executes only on human approval via the existing WebUI flow. No new
  review-storage is built; the suggested title rides in the task payload.

### 2.4 Execution: new task kind

Do not reuse `TaskKindParserDriftRename`: its executor re-runs
`ParseMovieNameVerbose` and overwrites `parsed_title`, which would clobber the
verifier's corrected title back to the parser's wrong output, and it triggers no
rescan. Add a new task kind (e.g. `TaskKindVerifierRename`).

- Extract the shared move mechanics from `execParserDriftRename` into a helper
  (folder rename src→dst, empty-dst-dir handling, `renameWithFallback`,
  media_files DB update, stat-both-sides for the manual-move race:
  "src missing, dst exists ⇒ DB-update only").
- The new executor writes the **verifier-supplied** title/year/tmdb_id into the
  parse_decision (never re-parses), preserves `TargetAt`, and calls the targeted
  rescan from 1.2 after the move.
- **Multi-file folders:** update *all* parse_decision and media_files rows whose
  path is under the renamed `srcDir`, not just the triggering row — otherwise the
  siblings (multi-version releases, extras) become orphans the reconciler then
  flags as `target_file_missing` / `path_mismatch`.
- **Stamp corrected rows to exclude them from drift detection.** For Phase-2
  cases the parser still cannot derive the right title (that is why the corrector
  exists), so `detectParserDriftRepairs` would re-derive the wrong parser title,
  see it differs from the corrected folder, and enqueue a rename *back* — the
  ping-pong Phase 1 avoids only where vocabulary can fix the parse. A verifier
  correction must mark the row (e.g. a `corrected_by_verifier` flag /
  `existing_match_method = "verifier"`) and `parserDriftMovieRename` must skip any
  row so marked. This exclusion is required for Phase 2, not optional.
- Record a repair event storing the verifier `Match` (tmdb_id, title, year) as
  evidence, so a bad auto-rename is one-click revertible and the review card can
  show why.

### 2.5 Loop guards and idempotency

- **Attempt cap:** reuse the `MetadataRepairCount` / `NeedsReviewAfter` pattern;
  after the cap the row goes to `needs_review` and correction stops. Record the
  last-tried candidate so an identical candidate is never retried, and so
  progressive-trim does not walk "Scary Movie" → "Scary" on successive cycles.
- **Idempotency / dedup:** a `correction_pending` state (or a unique
  (kind, parse_decision_id, pending) constraint on the task table) so the next
  passive cycle does not enqueue the same correction again before the first
  drains.
- **Post-rescan check:** if the targeted rescan still yields no ProviderIds, the
  row returns to `missing_provider_ids`; the attempt cap and last-candidate
  record prevent an infinite loop.

### 2.6 Config

Add `metadata_recovery.correction_enabled` (distinct from the misnamed
`repair_enabled`). Owner intent: enabled. This is the real "active
reconciliation" behavior.

### Hard constraints (Phase 2)

- Auto-apply requires exact-title + integer-equal year + candidate >= 2 tokens.
- Never mutate the filesystem from `RunPassive`; always enqueue.
- Never trigger a library-wide Jellyfin metadata refresh.
- Do not weaken the shared `tmdb.Verifier.lookup` used by housekeeping dedup.

## Testing

Phase 1:
- Parser corpus test: `scary.movie.extended.cut.2026...` → "Scary Movie" (2026);
  "Final Cut (2022)", "Time Cut (2024)", "Urban Legends: Final Cut (2000)"
  unchanged.
- Drift executor: `TargetAt` preserved across a drift rename; targeted rescan
  invoked with the new path; no library-wide refresh call.

Phase 2:
- Corrector decision table with a fake verifier: exact match different title →
  auto; ±1-year / single-candidate / single-token → review; current-name match →
  leave-alone; nothing → leave-alone.
- Strict-year: `Avatar Fire and Ash (2025)` never auto-applies to Avatar (2009).
- Enqueue seam: `RunPassive` enqueues, never renames; flag outcome maps to the
  review task kind.
- New executor: writes verifier title (no re-parse), preserves `TargetAt`,
  updates all rows under the folder, records the Match in the repair event.
- Loop guards: attempt cap honored; identical candidate not retried;
  correction_pending prevents duplicate enqueue.
- Drift exclusion: a verifier-corrected row (whose source filename still
  parses to the wrong title) is skipped by `parserDriftMovieRename` — no
  rename-back is enqueued.

## Open items deferred

- Leading-junk titles (only trailing-trim is in scope; observed cases are
  trailing).
- TV self-correction (this design is movie-only).

## Provenance

Design independently audited by Kimi-3.0 (`opencode-go/kimi-k3`) via opencode;
its audit produced the phased approach, the strict-year requirement, the
enqueue-don't-mutate seam, and the ping-pong resolution. Verdict: proceed with
changes (incorporated above).
