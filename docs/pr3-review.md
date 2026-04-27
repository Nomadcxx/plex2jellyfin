# PR #3 Code Review — Jellyfin Webhook Integration

**Date:** 2026-02-22
**PR:** feature/jellyfin-phase2-webhooks → main
**Scope:** 25 files, +2596 / -86 lines
**Tests:** All 4 packages PASS (jellyfin, database, api, notify)

---

## Merge Recommendation: ⛔ REQUEST CHANGES

One blocking issue must be resolved before merge.

---

## Blocking

### Daemon drops non-playback webhook events

| File | Severity |
|------|----------|
| `internal/daemon/handler.go` | **BLOCKING** |
| `internal/daemon/server.go` | **BLOCKING** |
| `cmd/jellywatchd/main.go` | **BLOCKING** |

The daemon webhook handler (`internal/daemon/handler.go`) only processes `PlaybackStart` and `PlaybackStop` events. It silently drops `ItemAdded`, `TaskCompleted`, and `LibraryChanged`.

The API handler (`internal/api/webhooks.go`) implements all 5 event types correctly — but the daemon is the primary runtime path. This means:

- `jellyfin_items` table is **never populated** in production (only via API server)
- `runJellyfinVerificationPass` is **never triggered** in production
- The entire ItemAdded → DB persistence → verification pipeline is dead code in daemon mode

**Fix:** Unify event handling so the daemon processes all 5 event types, or extract a shared handler that both daemon and API server call.

---

## Critical

### Large untested API client surface

| File | Severity |
|------|----------|
| `internal/jellyfin/client.go` | Critical |
| `internal/jellyfin/items.go` | Critical |
| `internal/jellyfin/library.go` | Critical |
| `internal/jellyfin/plugin.go` | Critical |
| `internal/jellyfin/sessions.go` | Critical |
| `internal/jellyfin/types.go` | Critical |
| `internal/jellyfin/verify.go` | Critical |

New Jellyfin API client with substantial surface area (HTTP calls, JSON parsing, error handling) has **zero tests**. Only `playback_lock_test.go` and `deferred_queue_test.go` exist in the jellyfin package.

At minimum, add:
- Unit tests for client methods with httptest mock server
- Test for verify.go logic (verification pass)
- Test for error paths (API unreachable, malformed responses)

---

## Important

### Missing tests for new API endpoints

| File | Severity |
|------|----------|
| `internal/api/media_managers.go` | Important |
| `internal/api/dashboard.go` | Important |

New API endpoints with no test coverage.

### Schema v12 migration has no regression test

| File | Severity |
|------|----------|
| `internal/database/schema.go` | Important |

Migration itself is safe (additive, new table only). But there's no test verifying the migration runs cleanly or that the `jellyfin_items` table has the expected schema post-migration.

---

## Minor

### No request body size limit on webhook handler

| File | Severity |
|------|----------|
| `internal/api/webhooks.go` | Minor |

`json.NewDecoder(r.Body)` with no `http.MaxBytesReader`. An attacker could send an arbitrarily large body to exhaust memory.

**Fix:** Wrap body with `http.MaxBytesReader(w, r.Body, maxBytes)` (e.g., 1MB limit).

### Double library refresh on notification fallback

| File | Severity |
|------|----------|
| `internal/notify/jellyfin.go` | Minor |

When the primary notification method fails and falls back, the library refresh may be triggered twice. Not harmful but wasteful.

### Fail-open API fallback should be documented

| File | Severity |
|------|----------|
| `internal/organizer/organizer.go` | Minor |

When the Jellyfin API is unreachable, the organizer proceeds with file operations (fail-open). This is a reasonable default but should be documented — users may expect fail-closed behavior.

### HasSuffix auth bypass is fragile

| File | Severity |
|------|----------|
| `internal/api/server.go` | Minor |

Auth middleware uses `strings.HasSuffix(r.URL.Path, "/webhook/jellyfin")` to skip auth. This could be bypassed with path manipulation (e.g., `/evil/../webhook/jellyfin`). Consider using an explicit route-level bypass instead.

---

## No Issues Found

These files were reviewed and are clean:

| File | Notes |
|------|-------|
| `internal/jellyfin/playback_lock.go` | Correct RWMutex usage, defensive copies, no races |
| `internal/jellyfin/deferred_queue.go` | Thread-safe, proper locking, defensive copies |
| `internal/database/jellyfin_items.go` | Parameterized SQL, correct locking, proper error handling |
| `internal/jellyfin/playback_lock_test.go` | Good coverage of lock lifecycle |
| `internal/jellyfin/deferred_queue_test.go` | Good coverage of queue operations |
| `internal/api/webhooks_test.go` | Tests webhook secret validation and event dispatch |
| `internal/notify/notify_test.go` | Tests notification dispatch |
| `internal/jellyfin/webhook_types.go` | Clean type definitions |
| `embedded/templates/*.html` | UI templates, no logic issues |
| `configs/openapi.yaml` | API spec matches implementation |

---

## Concurrency Assessment

**PlaybackLockManager:** Correct. RWMutex with proper read/write lock usage. Defensive copies prevent data races. No deadlock paths.

**DeferredQueue:** Correct. Same RWMutex pattern, atomic get+delete on RemoveForPath.

**Organizer integration:** Checks both source and target paths for locks before operating. Defers to queue when locked. Safety is opt-in via functional options.

**No concurrency bugs found.**

---

## Summary

The core concurrency design (playback locks + deferred queue) is solid. The webhook secret validation uses constant-time comparison. SQL is parameterized throughout.

The blocking issue is the daemon/API handler divergence — the daemon silently drops 3 of 5 webhook event types, making the ItemAdded pipeline dead code in production. This must be fixed before merge.

Secondary concern is the large untested Jellyfin client surface. This should have at least basic smoke tests before merge.

### Required Before Merge

1. **Unify webhook event handling** — daemon must process all 5 events
2. **Add Jellyfin client tests** — at minimum, mock HTTP tests for client methods
3. **Add daemon webhook integration test** — verify ItemAdded persists to DB via daemon path
