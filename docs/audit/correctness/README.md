# Data Integrity & Error Handling Audit

Generated 2026-06-25. 6 domains, focus on correctness bugs, error handling gaps, concurrency, and security.

## Summary

| Domain | CRITICAL | HIGH | MEDIUM | LOW |
|--------|----------|------|--------|-----|
| A1: Organizer + Transfer | 3 | 3 | 6 | 5 |
| A2: Scanner → DB | 2 | 2 | 2 | 2 |
| A3: Daemon Event Loop | 4 | 5 | 4 | 3 |
| A4: AI Integration | 2 | 3 | 2 | 2 |
| A5: Jellyfin/*arr Sync | 4 | 7 | 5 | 2 |
| A6: Config/CLI/Security | 1 | 5 | 6 | 4 |
| **TOTAL** | **16** | **25** | **25** | **18** |

## CRITICAL Findings (16)

### A1: Organizer + Transfer
1. **Silent DB upsert after file move** — `_, _ = o.db.UpsertMovie(movieRecord)` ignores DB error after file already moved (`organizer.go:482,683`)
2. **Existing file removed before transfer** — os.Remove before transfer means failure leaves NO file at all (`organizer.go:450-454,648-652`)
3. **PV transferer truncates destination in-place** — `O_TRUNC` destroys existing file before transfer confirms success (`transfer/pv.go:129`)

### A2: Scanner → DB
4. **Dangling best_file_id after prune** — `PruneMissingMediaFiles` deletes `media_files` rows but leaves dangling `best_file_id` references in `movies`/`episodes` (no FK constraint) (`service/analysis.go:202-223`, schema migration 3)
5. **Organizer moves files without updating media_files.path** — stale rows pruned → dangling references (architectural gap between organizer and scanner)

### A3: Daemon Event Loop
6. **Unbounded IPC request goroutines** — no per-request timeout or rate limit (`ipc/server.go:227`)
7. **heartbeatLoop goroutine leak on streaming handler panic** (`ipc/server.go:93`)
8. **timer.Stop() race causes use-after-close on shutdown** — `timer.Stop()` return value ignored, stray timer fires on closed resources (`daemon/handler.go:439-459`)
9. **processFile goroutine from deferred queue unsynchronized** — no WaitGroup or shutdown gate (`daemon/handler.go:~1676`)

### A4: AI Integration
10. **Infinite re-queue loop for low-confidence AI results** — deleted from pendingAI without fallback or cache, periodic scanner re-queues forever (`daemon/handler.go:1966-1989`)
11. **Unbounded goroutine spawn on cache hits** — `go c.updateUsage(id)` on every cache hit (`ai/cache.go:74`)

### A5: Jellyfin/*arr Sync
12. **No retry logic in Jellyfin/Sonarr/Radarr clients** — transient 500/502/503 treated as terminal errors
13. **Webhook races with filesystem events** — `HandleJellyfinWebhookEvent` and `processFile` concurrently update `parse_decisions` with no mutex/transaction
14. **DeferredQueue is purely in-memory** — all deferred operations lost on daemon restart
15. **No rollback when Arr update succeeds but DB mark fails** — Sonarr/Radarr now point to path that may not exist on disk

### A6: Config/CLI/Security
16. **Privilege escalation via os.Args[0] / PATH hijack** — `Escalate()` uses `os.Args[0]` not `os.Executable()` → attacker can substitute binary before sudo (`privilege/escalate.go:27-32`)

## Top HIGH Findings (25)

Key ones across domains:
- `Move()` returns `nil` error when source removal fails (A1)
- Season pack partial failure leaves inconsistent state (A1)
- `processFile` non-transactional — failure after parent upserts creates orphaned rows (A2)
- `loggedErrors`/`transientWarned` cap resets entire map → log bursts (A3)
- Unrecovered panic in scheduler `fire()` crashes daemon (A3)
- Circuit breaker bypassed — daemon calls `Matcher` directly (A4)
- Webhook handler returns 200 before DB writes complete (A5)
- Replay deferred operations spawns unbounded goroutines (A5)
- Path traversal via config/plan files in CLI commands (A6)
- API keys in plaintext config.toml (A6)
- No centralized config validation (A6)
- `daemon.enabled` and `daemon.health_addr` config fields ignored (A6)

## Comparison with Ponytail Audit

| Aspect | Ponytail Audit (Jun 24) | Correctness Audit (Jun 25) |
|--------|------------------------|---------------------------|
| Focus | Dead code, duplication, over-engineering | Bugs, error handling, concurrency, security |
| Domains | 9 (parser-brain, pipeline, daemon, loops, DB, arr, CLI, web, AI) | 6 (organizer, scanner, daemon, AI, sync, config) |
| CRITICALs | 0 (by design — ponytail finds waste, not bugs) | 16 |
| HIGHs | ~180 (mostly structural duplication) | 25 |
| Methodology | Count lines, find copies, check callers | Trace error propagation, check rollback, goroutine lifecycle |

## Next Steps

1. Fix the 16 CRITICALs first — these can cause data loss or daemon crashes
2. Address A6 security items (privilege escalation, path traversal) immediately
3. Address A3 concurrency bugs (timer race, goroutine leaks)
4. Then move to HIGH items
