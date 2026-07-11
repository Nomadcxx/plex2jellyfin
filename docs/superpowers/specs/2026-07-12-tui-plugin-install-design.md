# TUI installer: companion plugin auto-install

**Date:** 2026-07-12
**Status:** Approved
**Scope:** Fresh-install wizard path of `cmd/installer` only. Update mode and
uninstall mode are untouched; users who update can run
`plex2jellyfin plugin install` afterwards if needed.

## Goal

Bring the TUI installer to parity with the CLI wizard's plugin flow
(spec `2026-07-11-plugin-auto-install-design.md`): consent-gated install of
the companion plugin through Jellyfin's package API, consent-gated restart,
and a verified feedback loop — while fitting the installer's collect-then-execute
architecture (input screens gather state; the task pipeline acts on it).

A second, related goal: a config written by the TUI installer must be
recognized as completed setup by the other surfaces. Today the web UI skips
its wizard only via the `HasMediaPair` heuristic fallback in
`setup.NeedsSetup()` because the TUI writes no `[setup]` section. That is
fragile: when `setup.CurrentVersion` bumps, TUI-installed users would be
re-wizarded. This slice closes the gap deterministically.

## Non-goals

- No interactive install on the Jellyfin screen (no long blocking waits on
  input screens). Chosen and approved: toggles collect consent, the pipeline acts.
- No plugin handling in update mode.
- No changes to the CLI wizard, web wizard, engine, or plugin repo — the
  `internal/jellyfin/plugininstall` engine is consumed as-is.

## 1. Jellyfin screen: consent toggles + guidance

`renderJellyfin` (screens.go) grows two toggle rows plus a guidance footer,
rendered only when `jellyfinEnabled && jellyfinTested` (consent to install
against a server we could not reach is meaningless):

```
  Enable: Yes
  URL:     http://localhost:8096
  API Key: ••••••••
  Webhook Secret: ••••••••
  ✓ Connected - 10.11.11

▸ Install companion plugin: Yes
  Restart Jellyfin after install: Yes   (recommended)

  The companion plugin closes the feedback loop: it confirms organized
  files against real Jellyfin items and powers orphan detection. It only
  loads after Jellyfin restarts — without a restart the daemon runs
  degraded until you restart Jellyfin yourself.
```

- The guidance footer uses the muted style (`FgMuted`) and mirrors the CLI
  wizard's framing ("required for the feedback loop … confirms organized
  files against real Jellyfin items and powers orphan detection").
- Both toggles default **Yes**, so a user who rushes through the wizard gets
  the correct full process (install → restart → verified feedback loop)
  rather than a silently degraded one. This deliberately diverges from the
  CLI wizard's restart default of No: the TUI fresh-install context is an
  interactive guided session where a restart is expected, and the toggle +
  guidance footer keep the choice visible for anyone who needs to defer it.
- Model fields: `pluginInstall bool` (default `true`),
  `pluginRestart bool` (default `true`).
- Focus: `nextJellyfinInput`/`prevJellyfinInput` extend their existing
  "Enable row + inputs" total by two rows when the toggles are visible;
  up/down toggles the focused row's boolean, matching the Enable row idiom.
- If the user never runs `[T]` the toggles do not appear and the defaults
  stand; the pipeline still attempts install when Jellyfin is enabled
  (the engine's Inspect reports unreachable/ABI failure as a degraded skip,
  never an abort — same contract as the CLI).

## 2. Web step: callback URL field

`initWebInputs` gains a second input, **Plugin callback URL**, shown only
when `jellyfinEnabled && pluginInstall`:

- Pre-filled `http://<setup.DetectAdvertiseIP()>:<webPort>`.
- Editing the port field re-derives the URL default unless the user has
  manually edited the URL field (`callbackURLEdited bool` guards this).
- When the web UI is disabled (`webEnabled == false`), the default uses
  port `8686` instead — the daemon's health server mounts the same
  `/api/v1/webhooks/jellyfin` route (verified live in the 2026-07-12 e2e).
- Saved to `m.pluginDaemonURL` by `saveWebInputs`.

## 3. Task pipeline: install early, verify late

Fresh-install main task list, inserted after "Write config":

| Task | optional | Behavior |
|------|----------|----------|
| Install Jellyfin plugin | yes | `Engine.Inspect` (ABI gate 10.11.x) → `RegisterRepo` (read-modify-write; existing repos preserved) → `Install`. Marked skipped when Jellyfin disabled or `pluginInstall` off. |
| Restart Jellyfin | yes | Only when `pluginRestart`: `Restart` → `WaitReady` (60s poll shown as spinner subtask). Declined restart → statusSkipped with note, not an error. |

`postScanTasks`, appended after `startService` / `startWebService`:

| Task | optional | Behavior |
|------|----------|----------|
| Configure plugin feedback loop | yes | `Configure(pluginDaemonURL, webhookSecret)` → `Verify` (signed test event round-trip). Runs only when the plugin loaded (restart consented + WaitReady succeeded) AND a callback listener started (`serviceStartNow` or `webStartNow`); otherwise skipped. Outcome stored on the model. |

- The engine is constructed the same way the CLI does:
  `plugininstall.New(m.jellyfinURL, m.jellyfinAPIKey, &http.Client{Timeout: 15s})`.
- `optional: true` reuses the existing degrade path in `handleTaskComplete`:
  a failed plugin task becomes statusSkipped + an entry in `m.errors`; the
  install itself never aborts. This is the same failure contract as the CLI
  wizard and matches how `startService` already behaves.
- Model outcome fields: `pluginVerified bool`, `pluginOutcome string`
  (one of `verified`, `needs-restart`, `failed`, `skipped`).

## 4. Config emission

`generateConfigString` (tasks.go) changes — this is hand-written TOML, so
every field must be explicitly emitted (the silent-field-loss trap):

- `[jellyfin]` block gains:
  - `plugin_enabled = <pluginInstall>` (key verified: config.go:268)
  - `plugin_daemon_url = "<pluginDaemonURL>"` (key verified: config.go:277)
- New block, written unconditionally on fresh install:

```toml
[setup]
version = <setupdomain.CurrentVersion>
completed = true
```

With `[setup]` present, `setup.NeedsSetup()` takes the explicit path: the
web UI skips its wizard and only asks the user to create a password (auth
setup is a separate gate and is unchanged). CLI and web wizards already
write this marker; the TUI was the only surface that did not.

## 5. Complete screen

One plugin status line in the summary, by `pluginOutcome`:

- `verified` → `✓ Companion plugin verified — feedback loop active`
- `needs-restart` → `○ Plugin downloaded; restart Jellyfin to load it, then run: plex2jellyfin plugin verify`
- `failed` → `✗ Plugin step failed — run: plex2jellyfin plugin install`
  (detail already in the errors list rendered above)
- `skipped` (install toggle off or Jellyfin disabled) → no line.

## 6. Error handling summary

- No plugin failure aborts installation; every plugin task is `optional`.
- Declined restart is a first-class outcome (`needs-restart`), not an error.
- Verify-impossible situations (no listener started) skip quietly with the
  recovery pointer on the Complete screen.
- ABI-unsupported or unreachable Jellyfin at pipeline time: install task
  degrades to skipped with the engine's error message in `m.errors`.

## 7. Testing

Extends existing installer test patterns (`tasks_test.go`, table-driven):

1. `generateConfigString`: asserts `[setup]` block, `plugin_enabled`,
   `plugin_daemon_url` presence and values; plus a round-trip test that the
   emitted string parses via `config.Load`-equivalent with fields intact.
2. Task-gating table tests: Jellyfin off / `pluginInstall` off /
   `pluginRestart` declined / no listener started → correct skip decisions
   for each of the three tasks.
3. Callback URL derivation: web enabled port 5522, custom port, web
   disabled → 8686 fallback, manual edit wins over re-derivation.
4. Engine HTTP behavior is NOT re-tested here — the 9 httptest fakes from
   the CLI slice (Task 7) own that. TUI tests cover wiring and consent
   gating only.

## Decisions log

- Fresh install only (user, 2026-07-12).
- Toggles on Jellyfin screen + pipeline execution, not interactive install
  (user, 2026-07-12).
- Callback URL editable on the Web step, live-derived from the port field
  (user, 2026-07-12).
- Approach 1 (install in main pipeline, verify in postScanTasks) approved;
  user noted it matches how start-service/start-web are already structured.
- Restart recommendation surfaced inline + guidance footer (user asked for
  the recommendation, wording delegated).
- Both toggles default Yes so rushing through the wizard yields the full
  correct process instead of a degraded skip (user, 2026-07-12).
