# Automatic Jellyfin plugin installation from the setup wizard

Date: 2026-07-11
Status: approved (design), not yet implemented

## Problem

The companion Jellyfin plugin is required for the feedback loop (item
confirmation, orphan detection, playback events), but installing it is
entirely manual today: clone the plugin repo, build with the .NET 9 SDK,
copy into Jellyfin's plugins directory, restart, then hand-configure the
shared secret and webhook URL in the dashboard. Basic users — the setup
wizard's audience — will not do this, and file-copy automation is a dead
end because Jellyfin frequently runs in a container whose filesystem the
installer cannot reach.

## Mechanism: Jellyfin's own package API

Everything runs over the Jellyfin HTTP API using the admin API key the
wizard already collects. Jellyfin downloads and installs the plugin
itself, so container topology is irrelevant, and the plugin becomes
visible to Jellyfin's normal update system.

Verified API contract (Jellyfin 10.11, all behind admin elevation):

| Step | Endpoint |
|------|----------|
| ABI/version gate | `GET /System/Info` |
| Register plugin repo | `GET /Repositories` then `POST /Repositories` (POST **replaces the whole list** — must read-modify-write, never clobber existing repos) |
| Install | `POST /Packages/Installed/Plex2Jellyfin?assemblyGuid=<guid>&version=<v>` |
| Restart to load | `POST /System/Restart` — in-process since Jellyfin 10.9 (no process exit, works in Docker without a restart policy; our floor is 10.11) |
| Detect loaded plugin | `GET /plex2jellyfin/health` (anonymous endpoint the plugin already exposes) |
| Push configuration | `POST /Plugins/{guid}/Configuration` with `SharedSecret`, `Plex2JellyfinUrl`, forwarding flags |
| Verify reverse path | `POST /plex2jellyfin/test-webhook` (new plugin endpoint, see prerequisites) fires a signed synthetic event at the daemon; CLI confirms the daemon recorded it |

Rejected alternatives: direct file injection (container filesystems
unreachable, ownership guessing, invisible to Jellyfin updates) and an
API-first/file-fallback hybrid (doubles the failure modes for a corner
case the manual docs already cover).

## Prerequisites (plex2jellyfin-plugin repo, ships first)

1. **Real GUID.** The manifest GUID is a placeholder
   (`a1b2c3d4-e5f6-7890-abcd-ef1234567890`). Mint a real one now —
   changing it later breaks upgrade continuity for every install. The
   only existing install is the JellyWatch-era plugin, which the rebrand
   already orphaned, so the break is free today and never again.
2. **Plugin 1.2.0**: `POST /plex2jellyfin/test-webhook` endpoint
   (admin-authorized) that sends one signed synthetic event to the
   configured daemon URL; and quiet-when-unconfigured forwarding — if no
   `SharedSecret` is set, the forwarder sends nothing (today it would
   retry-spam the default `localhost:3000`). This covers the
   installed-mid-wizard-then-aborted case.
3. **Release CI**: tag push → dotnet build → zip → GitHub Release →
   regenerate repo-level `manifest.json` (array-of-packages format,
   md5 checksum, `targetAbi`, release-asset `sourceUrl`) committed to
   `main`. Stable manifest URL:
   `https://raw.githubusercontent.com/Nomadcxx/plex2jellyfin-plugin/main/manifest.json`.

## Engine (`internal/jellyfin/plugininstall`, main repo)

One Go package exposing the flow as discrete, individually-reportable
stages so every surface can render per-stage progress and per-stage
failure:

1. `Inspect` — server version + ABI gate, current plugin presence and
   version (`GET /Plugins` + `GET /plex2jellyfin/health`).
2. `RegisterRepo` — read-modify-write of the repository list; skip if
   already present.
3. `Install` — install or upgrade; skip if current.
4. `Restart` — only ever runs with explicit consent passed in by the
   caller; afterwards polls health until ready or deadline (60s / 2s).
5. `Configure` — push `SharedSecret`, `Plex2JellyfinUrl`, forwarding
   flags.
6. `Verify` — trigger `test-webhook`, confirm the daemon recorded the
   event.

Every stage is idempotent; re-running the whole flow on a healthy
install is a fast no-op. All side effects (HTTP client, prompts, clock)
are injected, following the `setupDeps` pattern in
`cmd/plex2jellyfin/setup_cmd.go`.

## Surfaces

### This slice: CLI

- **Standalone command** `plex2jellyfin plugin` with subcommands
  `install`, `verify`, `status` — visible in root help. `install` runs
  stages 1–6; `verify` runs 1, 5(read-only check), 6; `status` runs 1
  and prints what it found. All read Jellyfin connection + secret from
  the saved config.
- **TUI wizard, Jellyfin step** (after URL/API key validate): detect
  plugin → offer install (default Yes) → install → offer restart
  (**default No** — it is a live media server, possibly mid-playback)
  → wait for health → report. Runs stages 1–4.
- **TUI wizard, "Feedback loop" step** (after daemon activation, when
  the webhook secret exists and the daemon can receive events): prompt
  for the webhook URL with a detected default — primary LAN IP of the
  host + web listen port; when `DetectRuntime` reports a container,
  warn that the Docker gateway IP or a compose service name is likely
  required — then run stages 5–6.

Split rationale: install/restart need only the Jellyfin connection and
are cheapest to do while the user is already "in" the Jellyfin step;
configure/verify structurally cannot run earlier because the secret is
generated at apply time and verification needs a running daemon.

### Slice 2 (follow-up, not in this spec's plan)

Web wizard step + settings card reusing the same engine behind new API
endpoints, following the manual-mount convention.

### Degradation paths (wizard never blocks on the plugin)

| Situation | Behavior |
|-----------|----------|
| Plugin current | one-line skip |
| Plugin stale | offer upgrade (same install call) |
| Server ABI mismatch (not 10.11.x) | explain, skip, link manual guide |
| Install declined or failed | continue; closing summary prints `plex2jellyfin plugin install` |
| Restart declined | continue; closing summary prints "restart Jellyfin, then: plex2jellyfin plugin verify" |
| Restart timeout | same recovery line; setup completion is unaffected |
| Configure/verify failed | summary line + `plex2jellyfin plugin verify` pointer |

Setup completion (`setup.completed = true`) never depends on any plugin
stage.

### Out of scope

`cmd/installer` is unchanged — it already hands off to the wizards,
which is where Jellyfin credentials live.

## Testing

- Engine unit tests against a fake Jellyfin (`httptest.Server`): fresh
  install, already-installed no-op, upgrade, ABI mismatch, repo-list
  preservation (regression: existing third-party repos must survive the
  read-modify-write), declined restart, restart timeout, configure push
  body, verify round-trip.
- Wizard tests extend the existing scripted `setup_cmd_test.go` deps
  with plugin stages: happy path, decline-install, decline-restart
  (asserts recovery line in output), plugin failure does not fail setup.
- One end-to-end run against a real Jellyfin 10.11 container (install →
  restart → configure → verify) before shipping, same discipline as the
  Docker first-start test.
