# WebUI Control Plane — Design 1 (Settings + Daemon/DB Lifecycle)

**Date:** 2026-04-27
**Status:** Draft — pending user review
**Scope:** Chunks 1 & 2 of the three-chunk control-plane initiative.
**Out of scope:** Chunk 3 (logs viewer, AI usage tracking, credential vault) — covered by a sibling design doc.
**Implementation plans (3, follow-on):**
1. Settings CRUD (chunk 1)
2. Daemon & Database lifecycle (chunk 2)
3. Observability (chunk 3, separate design)

---

## 1. Problem

The webui today is built for analysis (duplicates, consolidation, parse decisions). Configuration and operational control are absent: the only mutable settings field is the AI section; there is no UI for daemon start/stop/restart, no way to trigger DB cleanup or rebuild from the UI, and the watch folders / library locations / Sonarr / Radarr / Jellyfin sections are read-only. Users must edit `~/.config/jellywatch/config.toml` by hand and run `systemctl` to apply changes.

This design closes those gaps for a **local-only deployment** (no auth gating; users are explicitly told not to expose `jellyweb` to the network).

## 2. Goals & Non-Goals

### Goals
- Webui can edit every config section the daemon understands.
- Webui can start, stop, restart, and reload the daemon.
- Webui can re-scan media and reset the database with TUI-installer-grade progress UX.
- All destructive operations have appropriate safety gates and crash-safe semantics.
- The CLI grows a thin `jellywatch daemon …` command set so scripts and post-install hooks have parity with the UI.

### Non-Goals
- Authentication / authorization (deferred — local-only).
- Network-transparent IPC (deferred — local-only).
- Cross-version compatibility during in-place upgrades (deferred — installer restarts both processes together).
- Logs viewer, metrics, AI cost tracking (chunk 3).
- Credential vault / secret rotation (chunk 3).

## 3. Architecture

### 3.1 Process layout

```
┌─────────────────────┐       Unix socket        ┌─────────────────────┐
│      jellyweb       │  ~/.config/jellywatch/   │     jellywatchd     │
│                     │      control.sock        │                     │
│  • Next.js UI       │ ◄──────────────────────► │  • Scanner          │
│  • REST API (5522)  │   {STATUS, RELOAD,       │  • AI matcher       │
│  • SSE relay        │    STOP, RESCAN,         │  • IPC server       │
│  • Settings writes  │    RESET_DB, ATTACH,     │  • Reload supervisor│
│  • config.toml RW   │    CANCEL}               │  • Op registry      │
└─────────────────────┘                          └─────────────────────┘
       │                                                    │
       └─────────► ~/.config/jellywatch/config.toml ◄───────┘
                       (jellyweb writes,
                        jellywatchd reads on RELOAD)
```

### 3.2 Boundaries
- **`jellyweb` is the only runtime writer of `config.toml`.** Installer and `jellywatch config init` are install-time writers; their writes coincide with daemon (re)starts so reload semantics don't apply to them.
- All writers (`jellyweb`, installer, `jellywatch config init`) use **atomic write** (`tmp` → `fsync` → `rename`) plus an **`flock(2)` advisory lock** on `config.toml` for the write window.
- **`jellywatchd` exposes no HTTP control surface.** All mutation/lifecycle commands travel over the IPC socket. Daemon `health_addr` remains for chunk-3 observability.
- **`jellyweb` owns "start"** of the daemon: it spawns the daemon process if not running, or delegates to systemd if a unit is detected. Start logic is daemon-launch-strategy aware: prefer systemd → fallback to direct exec.
- **Long-running ops execute inside the daemon.** Their progress streams travel IPC → `jellyweb` → SSE → browser.

### 3.3 Bootstrap order on a fresh install
1. `jellyweb` starts first (always available, used for setup wizard).
2. User configures paths via webui.
3. `jellyweb` writes config and asks daemon to start.
4. Start strategy resolution (in order):
   - If a systemd user unit exists → `systemctl --user start jellywatchd`.
   - Else if a system unit exists and `jellyweb` runs with `polkit`/sudo authorization → `systemctl start jellywatchd`.
   - Else → `jellyweb` exec-spawns the daemon **detached** (`setsid`, stdout/stderr redirected to a daemon log file) so `jellyweb`'s lifecycle does not bind the daemon's. This path is intended for development and non-systemd hosts.
5. UI surfaces "daemon installed?" so unintegrated installs can be diagnosed and offers a "Install systemd unit" button when missing.

## 4. IPC Protocol

### 4.1 Transport
- Unix domain socket at `~/.config/jellywatch/control.sock`.
- File permissions `0600`, owned by daemon UID.
- **`SO_PEERCRED`** check on each connection: daemon refuses peers whose UID differs from its own (defense-in-depth alongside socket perms).
- Stale-socket detection on daemon startup (try connect → if dead, unlink and recreate).
- Daemon removes the socket on graceful shutdown.

### 4.2 Wire format
- **Newline-delimited JSON.** One message per line.
- **Max single frame: 64 KB.** Larger frames close the connection.
- **Max in-flight requests per connection: 8.**
- **Idle connection timeout: 5 minutes** with no commands and no heartbeats.

### 4.3 Frame schema

```json
// Request
{"v": 1, "id": "uuid-v7", "cmd": "STATUS", "args": {...}}

// Single-result response
{"id": "uuid-v7", "type": "result", "data": {...}}

// Streaming response
{"id": "uuid-v7", "type": "progress", "phase": "...", "msg": "...", "current": 0, "total": 0}
{"id": "uuid-v7", "type": "heartbeat", "ts": 1735300000}
{"id": "uuid-v7", "type": "done", "data": {...}}
{"id": "uuid-v7", "type": "error", "code": "...", "msg": "..."}
```

### 4.4 Commands (v1)

| Command   | Args                                           | Response               | Notes                                            |
|-----------|------------------------------------------------|------------------------|--------------------------------------------------|
| `STATUS`  | —                                              | single result          | Cheap; webui polls every 5s                      |
| `RELOAD`  | —                                              | single result          | Two-phase prepare/commit (see §4.6)              |
| `STOP`    | `{graceful, timeout_s}`                        | single result + close  | Daemon exits after responding                    |
| `RESCAN`  | `{paths?: [...], dry_run}`                     | streamed               | Walks watch folders, re-indexes                  |
| `RESET_DB`| `{confirm: "media.db", preserve: ["audit_log"]}` | streamed             | `confirm` must equal literal DB filename         |
| `ATTACH`  | `{op_id}`                                      | streamed               | Re-subscribe to in-flight op                     |
| `CANCEL`  | `{op_id}`                                      | single result          | Propagates ctx.Cancel; op responds with `CANCELLED` |

### 4.5 Liveness
- Heartbeat frames every 5s on streaming responses (daemon → client) and 10s on the request side (client → daemon).
- Three consecutive missed heartbeats → connection is treated as dead.
- **Client disconnect ≠ op abort.** A long op continues in the daemon; the op_id stays addressable via `STATUS` and `ATTACH`.

### 4.6 Two-phase RELOAD with rollback

```go
type Reloadable interface {
    Name() string
    Prepare(ctx context.Context, oldCfg, newCfg *config.Config) (Commit, Rollback, error)
}
type Commit   func() error  // must be infallible — only swaps validated state
type Rollback func()        // releases pre-staged resources
```

Phase 1 — `Prepare` runs on every registered subsystem; failures abort.
Phase 2 — `Commit` runs on every subsystem only if all prepares succeeded; otherwise `Rollback` runs on the successful prepares and the daemon stays in its previous consistent state.

Failed reloads return:
```json
{"type": "result", "data": {"reloaded": ["scanner", "logging"], "failed": [{"name": "ai", "error": "..."}]}}
```

### 4.7 Concurrency
- `STATUS` / `ATTACH` / `CANCEL` are always concurrent.
- `RESCAN` and `RESET_DB` mutually exclude each other and themselves.
- `RELOAD` may preempt — it waits briefly for any in-flight op's safe checkpoint, then runs. (`RESCAN` checkpoints between files; `RESET_DB` checkpoints between table phases.)

### 4.8 Idempotency
- Destructive commands carry a client-supplied `op_id` (UUIDv7).
- 10-minute dedup window: re-issued `op_id` returns the original op's stream.

### 4.9 Crash recovery for destructive ops
- Before mutation, the daemon writes `{op_id, cmd, args, started_at, state: "in_progress"}` to `~/.config/jellywatch/op_log.jsonl`.
- On op completion, the line is rewritten with `done`, `failed`, or `cancelled`.
- On daemon **startup**, it scans `op_log.jsonl` for `in_progress` records.
- If found, the daemon refuses normal startup; `STATUS` returns `{state: "interrupted", interrupted_op: {...}}`. The webui shows a recovery screen requiring an explicit user decision (discard or resume) before any other operation is allowed.

### 4.10 Error taxonomy
Defined enum, shared between client and server in `internal/daemon/ipc/errors.go`:
```
BUSY, BAD_REQUEST, VERSION_MISMATCH, UNAUTHORIZED, NOT_FOUND,
CONFLICT, INTERRUPTED, CANCELLED, TIMEOUT, INTERNAL, NOT_IMPLEMENTED
```

### 4.11 Versioning
- Every request carries `v: 1`.
- Daemon rejects unknown versions with `VERSION_MISMATCH`.
- New commands added in v1 are opt-in (a daemon that doesn't know `FOO` returns `NOT_IMPLEMENTED`).

## 5. REST API (`jellyweb` ↔ browser)

### 5.1 Settings — read

```
GET /settings                    → list of section names + last_modified + config_hash
GET /settings/{section}          → JSON of TOML subtree
                                   (secret fields masked unless ?reveal=1 with X-Reveal-Token)
```

Sections: `paths`, `libraries`, `sonarr`, `radarr`, `jellyfin`, `ai`, `daemon`, `logging`, `permissions`, `options`.

**Masking rule:** fields tagged `secret:"true"` in the Go struct return as `"****" + last4` by default. Reveal requires explicit user click → `X-Reveal-Token` (a short-lived nonce issued by a `POST /settings/reveal-token` endpoint), and the reveal is recorded in the audit log.

### 5.2 Settings — write

Whole-section replace:
```
PUT /settings/sonarr     PUT /settings/radarr     PUT /settings/jellyfin
PUT /settings/ai         PUT /settings/daemon     PUT /settings/logging
PUT /settings/options
```

Array CRUD (paths, libraries):
```
GET    /settings/paths/{kind}              kind ∈ {movies, tv}
POST   /settings/paths/{kind}              body {path}            → 201 + full array
DELETE /settings/paths/{kind}/{index}                              → 204
PUT    /settings/paths/{kind}              body {paths: [...]}    → bulk reorder/replace

GET    /settings/libraries/{kind}
POST   /settings/libraries/{kind}
DELETE /settings/libraries/{kind}/{index}
PUT    /settings/libraries/{kind}
```

### 5.3 Validation & connection-test

```
POST /settings/{section}/validate    body: candidate JSON
                                     → {ok, schema_errors, warnings, connection_tests}
                                     Pure: never writes.

POST /settings/sonarr/test
POST /settings/radarr/test
POST /settings/jellyfin/test
POST /settings/ai/test               (wraps existing /ai/test-connection)

POST /paths/preflight                body: {path, kind: "watch"|"library"}
                                     → {readable, writable, owner_uid,
                                        daemon_uid_can_access, free_space_bytes, warnings}
```

### 5.4 Save pipeline (single canonical path)

```
1. Schema validate body                         → 4xx on failure
2. Acquire flock on config.toml                  → 503 if held >2s
3. Re-read config from disk (defensive)
4. Apply section change to in-memory struct
5. Run validation pipeline (non-blocking warnings)
6. Atomic write: tmp → fsync → rename
7. Release flock
8. Send RELOAD via IPC; wait up to 3s
9. Respond 200:
   {
     saved: true,
     validation: { warnings: [...] },
     reload: { ok, reloaded: [...], failed: [...] }
   }
```

Three end-states the UI distinguishes:
- **Green:** `validation.warnings == [] && reload.ok`.
- **Yellow:** validation has warnings or some non-critical signal; reload OK.
- **Red:** saved but reload reports failures — UI shows which subsystems and offers retry / revert.

### 5.5 Daemon lifecycle

```
GET  /daemon/status        → {state: "running"|"stopped"|"interrupted",
                              uptime_s, version, current_op?,
                              config_hash, interrupted_op?}
POST /daemon/start         → 202 + op_id
POST /daemon/stop          → 202 + op_id
POST /daemon/restart       → 202 + op_id
POST /daemon/reload        → 202 + op_id      (manual config edit case)
POST /daemon/recover       body: {action: "discard"|"resume"}    interrupted state only
```

### 5.6 Database lifecycle

```
POST /database/rescan      body: {paths?, dry_run}                → 202 + op_id
POST /database/reset       body: {confirm: "media.db",
                                  preserve: ["audit_log"]}        → 202 + op_id
```

### 5.7 SSE progress relay

```
GET /events/op/{op_id}                         streams progress frames
GET /events/op/{op_id}/replay?since=N          reattach with backfill
```

The handler `ATTACH`es to the IPC op and relays frames as SSE events. Closes when daemon emits `done` / `error` / `cancelled`.

### 5.8 OpenAPI hygiene
Every new endpoint added to `api/openapi.yaml`. Spec drift is a CI failure (existing AI mutation routes that drifted from the spec are corrected as part of this work).

## 6. Frontend

### 6.1 Routes

```
/settings                  overview dashboard
/settings/paths            watch folder editor
/settings/libraries        library location editor
/settings/sonarr           sonarr config
/settings/radarr           radarr config
/settings/jellyfin         jellyfin config
/settings/ai               AI section (extends existing)
/settings/daemon           daemon settings + lifecycle controls
/settings/database         DB rescan / reset
/settings/options          general toggles
/settings/logging          log level + rotation
```

Layout: persistent left rail with section navigation + status pills (green/yellow/red dot per section, fed by a single `/settings` overview poll every 30s). Right pane is the section's form.

### 6.2 Components

```
web/src/
  app/settings/
    layout.tsx              left-rail nav + overview poller
    page.tsx                overview cards
    [section]/page.tsx      one file per section route

  components/settings/
    SettingsForm.tsx        generic wrapper: dirty-state, save pipeline,
                            reload-result handling, three-state toast
    PathListEditor.tsx      shared by /paths and /libraries
    SecretField.tsx         masked-by-default with reveal flow
    TestConnectionButton.tsx
    ConfirmDestructive.tsx  typed-confirm modal
    ConfirmReversible.tsx   simple modal for restart/stop
    ProgressCard.tsx        SSE-driven progress (TUI-installer style)
    SubsystemReloadStatus.tsx green/red per-subsystem after reload

  hooks/
    useSettings.ts          extended with per-section CRUD
    useDaemon.ts            status polling + lifecycle actions
    useOpStream.ts          SSE subscription with auto-reattach
    usePathPreflight.ts     debounced /paths/preflight
```

Each section page is small (<200 LOC) — schema, render, save handler. Heavy lifting is in `SettingsForm.tsx`.

### 6.3 Path editor flow

Add path:
1. Modal with input + filesystem autocomplete.
2. Debounced `POST /paths/preflight` while typing → live status (readable / writable / daemon-uid access / free space).
3. Add disabled if preflight is red.
4. On submit → `POST /settings/paths/{kind}` → list refreshes.

Remove path: typed-confirm (paste the path) — daemon will stop watching it.
Drag-reorder: bulk PUT.

### 6.4 Daemon lifecycle UI
Status header polls `/daemon/status` every 5s. Buttons: Reload from disk, Restart, Stop. Restart/Stop use simple modal; Reload is one-click. If `state == "interrupted"`, the page is replaced by a recovery banner that requires discard/resume before any other action.

### 6.5 Database lifecycle UI
Two cards:
- **Re-scan media** (preserves history) — `Start re-scan` + dry-run checkbox.
- **Reset database** (destructive) — typed-confirm modal asking the user to type `media.db`. On confirm → 202 + op_id → page transitions to a full-screen `ProgressCard` with installer-style phase feed.

### 6.6 SSE reattach
`useOpStream` keeps the active `op_id` in session storage. On page reload or navigation, it re-attaches via `?since=N`. Lost progress lines stream from the daemon's op buffer.

## 7. Validation & Safety Pipeline (consolidated)

| Layer            | Where                  | Blocking?       | Purpose                            |
|------------------|------------------------|------------------|------------------------------------|
| Schema           | `jellyweb` per request  | Yes (4xx)        | Reject malformed bodies            |
| Filesystem       | `/paths/preflight`      | No (warning)     | Detect inaccessible paths early    |
| Connection       | `*/test`                | No (warning)     | Surface bad credentials early      |
| Section validate | `/settings/{s}/validate`| No (warning)     | Bundles all of the above pre-save  |
| Atomic write     | `cfg.Save()`            | Yes (5xx if fail)| Crash-safe persistence             |
| `flock(2)`       | `cfg.Save()`            | Yes (503 if held)| Mutual exclusion across writers    |
| IPC RELOAD       | daemon                  | Two-phase        | Prepare → commit, with rollback    |
| Op log           | daemon                  | Crash recovery   | Detect interrupted destructive ops |
| `SO_PEERCRED`    | daemon socket accept    | Yes              | UID-bound IPC                      |

## 8. CLI parity additions

```
jellywatch daemon status        → STATUS via IPC
jellywatch daemon reload        → RELOAD via IPC
jellywatch daemon stop          → STOP via IPC
```

(`start` stays as `systemctl start jellywatchd` or direct invocation; no daemon to talk to.) New file `cmd/jellywatch/daemon_cmd.go`; shared IPC client in `internal/daemon/ipc/client.go`.

## 9. File-level deltas

### Backend
```
internal/daemon/ipc/protocol.go          NEW   message types, command names, error codes
internal/daemon/ipc/server.go            NEW   listener, accept loop, dispatch, op registry
internal/daemon/ipc/client.go            NEW   shared by jellyweb + CLI
internal/daemon/ipc/op_log.go            NEW   crash-safe op log + recovery
internal/daemon/ipc/oplog_test.go        NEW
internal/daemon/reload/supervisor.go     NEW   two-phase Reloadable runner
internal/daemon/reload/registry.go       NEW   register subsystems' Prepare funcs
internal/daemon/reload/*_reloadable.go   NEW   one per subsystem (scanner, ai, ratelimit, log, ...)
cmd/jellywatchd/main.go                  EDIT  start IPC server; call ipc.RecoverInterruptedOps
internal/scanner/scanner.go              EDIT  add Prepare(); honor ctx.Cancel between files
internal/database/maintenance.go         NEW   ResetDatabase, Rescan with progress chan
internal/api/settings_handlers.go        NEW   per-section read/write/validate/test handlers
internal/api/paths_handlers.go           NEW   array CRUD for paths, libraries
internal/api/daemon_handlers.go          NEW   /daemon/* lifecycle handlers
internal/api/database_handlers.go        NEW   /database/* lifecycle handlers
internal/api/sse_relay.go                NEW   IPC ATTACH → SSE relay
internal/api/server.go                   EDIT  mount new routes
internal/config/config.go                EDIT  atomic write + flock; struct tags for secrets
api/openapi.yaml                         EDIT  add all new routes
cmd/jellywatch/daemon_cmd.go             NEW   CLI subcommands
```

### Frontend
```
web/src/app/settings/layout.tsx                          NEW
web/src/app/settings/page.tsx                            EDIT  → overview cards
web/src/app/settings/paths/page.tsx                      NEW
web/src/app/settings/libraries/page.tsx                  NEW
web/src/app/settings/sonarr/page.tsx                     NEW
web/src/app/settings/radarr/page.tsx                     NEW
web/src/app/settings/jellyfin/page.tsx                   NEW
web/src/app/settings/ai/page.tsx                         EDIT  → adopt SettingsForm
web/src/app/settings/daemon/page.tsx                     NEW
web/src/app/settings/database/page.tsx                   NEW
web/src/app/settings/options/page.tsx                    NEW
web/src/app/settings/logging/page.tsx                    NEW
web/src/components/settings/SettingsForm.tsx             NEW
web/src/components/settings/PathListEditor.tsx           NEW
web/src/components/settings/SecretField.tsx              NEW
web/src/components/settings/TestConnectionButton.tsx     NEW
web/src/components/settings/ConfirmDestructive.tsx       NEW
web/src/components/settings/ConfirmReversible.tsx        NEW
web/src/components/settings/ProgressCard.tsx             NEW
web/src/components/settings/SubsystemReloadStatus.tsx    NEW
web/src/hooks/useSettings.ts                             EDIT
web/src/hooks/useDaemon.ts                               NEW
web/src/hooks/useOpStream.ts                             NEW
web/src/hooks/usePathPreflight.ts                        NEW
web/src/lib/api/client.ts                                EDIT  → typed clients for new routes
```

## 10. Error Handling

### 10.1 jellyweb → browser
- 4xx: schema / validation failure, returns `{error: {code, message, details}}`.
- 503: `flock` couldn't be acquired in 2s.
- 502/504: IPC to daemon failed or timed out — UI shows "Daemon unreachable" and offers `/daemon/start`.
- 200 with `reload.failed != []`: write succeeded but daemon couldn't apply — UI shows red banner + retry/revert.

### 10.2 daemon ↔ IPC
- Errors carry a code from the §4.10 enum.
- Long-running ops emit a single `error` frame and close.
- Cancelled ops emit `{code: "CANCELLED"}`, distinct from genuine errors.

### 10.3 Subsystem reload failure
- Two-phase prepare/commit means failure leaves previous state intact (§4.6).
- UI shows which subsystems failed and their reasons; `[Retry reload]` and `[Revert config]` are offered.

### 10.4 Crashed daemon
- `op_log.jsonl` records in-progress destructive ops.
- On startup, daemon refuses normal operation if any are mid-flight; webui shows recovery screen requiring discard/resume.

## 11. Testing

### 11.1 Backend
- **Unit:** every IPC command handler, the two-phase reload supervisor (mock subsystems with controllable Prepare/Commit/Rollback failure), config save (flock contention, partial-write injection), op log replay.
- **Integration:** start `jellywatchd` + `jellyweb` in test harness, drive via IPC client + REST. Cover happy path, reload failure, mid-rescan cancel, mid-reset crash + recovery (kill daemon process, restart, verify recovery flow).
- **Property:** atomic write + flock contention with N parallel writers (no partial files, no lost updates).

### 11.2 Frontend
- **Unit:** `SettingsForm`, `PathListEditor`, `useOpStream` (replay/reattach), `usePathPreflight` (debounce).
- **Integration (Vitest + msw):** save pipeline three-state branching, SSE reattach after simulated tab close.
- **E2E (Playwright, optional):** add path → see preflight → save → see daemon reload pill turn green; reset DB → typed confirm → progress card → completion.

### 11.3 Coverage targets
- New backend packages: 80% line coverage minimum.
- IPC server: 100% command coverage (every command exercised in tests).
- Reload supervisor: every prepare/commit/rollback combination tested.

## 12. Rollout

- Single PR per implementation plan (3 PRs total over chunks 1, 2, 3).
- No feature flag — local-only deployment, every install gets the new UI.
- Migration: none; existing config.toml files are upward-compatible (we only add new optional fields).
- Documentation: `docs/webui-control-plane.md` user guide added in chunk 1's PR.

## 13. Open questions deferred to chunk 3
- Where do operational logs surface in the UI?
- AI usage / cost tracking shape?
- Credential masking → credential vault migration path?
- Metrics / health dashboard layout?

---

*This is design 1 of 2. Design 2 covers chunk 3 (observability) and will be brainstormed separately after chunks 1+2 ship.*
