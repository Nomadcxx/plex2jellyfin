# Parse-Decisions Pipeline — Audit Findings & Fixes

This document captures the audit conducted after the 6-chunk parse-decisions
pipeline implementation. Findings are tagged HIGH / MED / LOW. Most were
resolved in the same audit pass; the path-translation gap (P-T below) was
discovered during operator review and fixed subsequently.

## HIGH-risk

| ID | Area | Finding | Fix |
|----|------|---------|-----|
| H1 | `internal/daemon/handler.go` | Early-return paths in the organize flow left orphaned decision rows (no outcome ever written). | Mark decision with OUTCOME=`failed` + reason at every early return. |
| H2 | — | (False positive: queueForAI path was correctly carrying `parse_decision_id` already.) | No code change. |
| H3 | `cleanupSourceDir` | Source dir was purged before any SUCCESS row existed, so a later transient failure could leave the move half-applied with no breadcrumb. | Gate `PurgeNonAllowed` on `HasRecentSuccessForSource` (recent SUCCESS row for the source). |
| H4 | `cmd/jellywatchd/main.go` | Labeler and sweeper goroutines could panic and tear down the daemon without a stack trace. | `defer recover()` at the top of each goroutine; log and continue. |
| H5 | `internal/jellyfin/sweep.go` | `UpdateOutcome` errors were swallowed. | Propagate error from `sweepByPath`. |
| H6 | `internal/database/parse_decisions.go` | `UpdateOutcome` was non-idempotent — a retry could overwrite a real resolve. | Added `WHERE jellyfin_resolved_at IS NULL` first-write-wins guard. |
| H7 | `internal/labeling/runner.go` | TOCTOU window: row read in `RunOnce`, written in `labelOne`, could be modified between the two. | Re-fetch via `GetDecision` inside `labelOne` before write. |
| H8 | `internal/labeling/runner.go` + migration 15 | Stale labels never re-derived if upstream parser changed. | New `auto_label_at` column + `QueryStaleLabeledDecisions` + idempotent `UpdateAutoLabelAt` + 14-day re-derive pass. |
| H9 | `cmd/jellywatch/parses_cmd.go` | `--failures` and `--drift` were both accepted simultaneously, producing nonsense output. | `MarkFlagsMutuallyExclusive("failures","drift")`. |
| **P-T** | `internal/jellyfin/sweep.go`, `internal/api/webhooks.go`, `internal/daemon/handler.go` | **Path translation gap: 0/16 production rows received provider IDs because Jellyfin's container-internal paths (e.g. `/tv5/...`) never matched daemon-side `target_path` rows (`/mnt/STORAGE5/TVSHOWS/...`).** | Added `JellyfinConfig.PathMappings` (TOML array of `{jellyfin, daemon}` pairs) + `jellyfin.PathTranslator` (longest-prefix-first). Wired into both webhook handlers (translate inbound `ItemPath` → daemon view) and the sweeper (translate `item.Path` before lookup). |

## MED-risk

| ID | Area | Finding | Fix |
|----|------|---------|-----|
| M1 | `internal/daemon/handler.go` | `existing_match_method` column was never populated. | Set `ExistingMatchMethod` in decision payload at every match site. |
| M2 | `internal/daemon/handler.go` | AI-queued decisions were not flagged "queued" so a daemon restart would re-process them. | New `MarkDecisionQueued` column + write before `queueForAI` returns. |
| M3 | `internal/jellyfin/sweep.go` | No rate limit between paginated Jellyfin requests. | Per-page sleep, default 50 ms, configurable via `SetPageDelay`, ctx-cancellable. |
| M4 | `cmd/jellywatchd/main.go` | Startup delays were not cancellable. | Verified existing `select { case <-time.After(): ; case <-ctx.Done(): }` pattern in both labeler and sweeper. |
| M5 | `internal/jellyfin/client.go` | HTTP requests had no per-request timeout and no ctx propagation. | New `requestCtx` / `getCtx` helpers using `http.NewRequestWithContext`; 30 s per-request timeout. |
| M6 | `internal/database/parse_decisions.go` | `UpdateAutoLabel` was non-idempotent. | New `UpdateAutoLabelAt` with `WHERE auto_label_at IS NULL` first-write-wins guard (same pattern as H6). |
| M7 | `internal/labeling/runner.go` | TTL boundary was inclusive on one side, exclusive on the other. | Tightened comparison + added boundary tests. |

## LOW-risk

| ID | Area | Finding | Fix |
|----|------|---------|-----|
| L1 | `internal/daemon/handler.go` | `target_at` was set on rows skipped before the move started. | Leave `TargetAt` nil for skipped paths. |
| L2 | `internal/organizer/organizer_test.go` | Test relied on incidental ordering. | Explicit assertion that incoming files remain after no-op. |
| L5 | `internal/labeling/fuzzy.go` | "Hunting Party" did not match "Hunting With Dogs" because the bridge-token "with" was lowercased. | Case-sensitive bridge-token check using `originalTokens`. |
| L6 | `internal/naming/corpus_test.go` | Year assertion implicitly accepted empty string. | Explicit `info.Year == ""` assertion. |
| L7 | `internal/database/schema.go` | Migration 14 had no header comment. | Added comment block. |

## Webui findings (post-deploy audit)

| ID | Area | Finding | Fix |
|----|------|---------|-----|
| W1 | `internal/api/handlers.go` `/activity/stream` | Returned a stub event. | Subscribe to `activity.Logger`; emit real events as `event: activity` SSE frames. |
| W2 | `internal/api/handlers.go` `/scan/stream` | Returned a stub. | Snapshot-based feed: scan_started / scan_completed transitions + 2 s status snapshots + 15 s heartbeat. |
| W3 | `internal/api/handlers.go` `/scattered/{id}/consolidate` | `dryRun` was accepted but ignored. | Short-circuit on `dryRun=true`; return `{success, dryRun, filesMoved, bytesMoved, targetPath, moves[{from,to,bytes}]}` for the Preview dialog. |

## Configuration: path mappings

When Jellyfin runs in a container with bind mounts whose roots differ from
the daemon's view of the filesystem, the post-organize feedback loop
(sweeper + webhook) cannot correlate Jellyfin items to `parse_decisions`
rows without explicit translation. Configure mappings in `config.toml`:

```toml
[[jellyfin.path_mappings]]
jellyfin = "/tv5"
daemon   = "/mnt/STORAGE5/TVSHOWS"

[[jellyfin.path_mappings]]
jellyfin = "/movies"
daemon   = "/mnt/STORAGE2/MOVIES"
```

Mappings are matched longest-prefix first; boundary-safe (`/mnt/STORAGE5`
will not match `/mnt/STORAGE50/...`). Without mappings, the translator
becomes a no-op and paths must match exactly between the two views.

## Verification

- `go test ./internal/jellyfin ./internal/labeling ./internal/database ./internal/daemon ./internal/api ./internal/config` → all pass.
- `TestSweep_PathTranslationResolvesContainerPaths` and
  `TestSweep_NoTranslatorMissesContainerPath` lock in the regression.
- Live verification after deploy: `parse_decisions` rows that previously
  remained unresolved now receive provider IDs from the sweeper once a
  mapping is configured.
