---
title: Setup wizards
description: First-run configuration via web UI, CLI, or TUI — same outcomes, different surfaces.
---

All three wizards write the same `config.toml`, activate `plex2jellyfin-daemon`, optionally install the Jellyfin plugin, run an initial library index into `media.db`, and chown the config directory so your interactive user can read the DB afterward.

| Surface | How you start | Best when |
|---|---|---|
| **TUI** | `install.sh` / `plex2jellyfin-installer` | Fresh machine, want guided terminal UI |
| **CLI** | `plex2jellyfin setup` | Headless / SSH, prefer prompts over browser |
| **Web** | Open `:5522` after web install | Prefer a browser; packages / web fresh-build |

## Shared steps

1. **Escalate** — CLI/TUI re-exec with sudo when needed (chown, systemd). Web already runs as a privileged service.
2. **Auth** — Web requires creating an admin password before the wizard. CLI/TUI do not use the web session.
3. **Paths** — Watch dirs (download drops) and library roots (Jellyfin Movies / TV). Multi-path paste is supported in the web UI (comma or newline separated).
4. **Services / *arr / Jellyfin / AI** — Optional Sonarr, Radarr, Jellyfin URL + API key, Ollama.
5. **Permissions** — Target user/group for moved files (`[permissions]`).
6. **Apply** — Atomic config write; enable daemon for boot (`systemctl enable --now` when units exist).
7. **Initial scan** — Per-library progress. Soft-stall: a library with no walk progress for ~12 minutes is skipped so one hung mount cannot block the rest. Time-based heartbeats keep UIs alive on slow NAS shares.
8. **Plugin** — When Jellyfin is enabled, wizards can install/verify the companion plugin.
9. **Complete** — `[setup] completed = true` in config.

## Web wizard

1. Install via [Option C](/docs/getting-started/installation#option-c--build--web-setup) or packages, then open `http://<host>:5522/`.
2. Create the admin password.
3. Walk Media → Services → AI → Runtime → Review.
4. **Apply** saves config and starts/enables the daemon (fast).
5. **Initial library scan** streams progress over SSE (`GET /api/v1/setup/index/stream`). Closing the tab leaves setup incomplete so you can retry.
6. Results screen shows indexed counts; open the dashboard when done.

If Apply already saved config but indexing was interrupted, reopen setup and retry the index step (or re-Apply when the UI offers it).

## CLI wizard

```bash
sudo plex2jellyfin setup
```

Prompt-driven. Escalates at start, always runs the initial scan with per-library progress bars, enables daemon (and can enable web). Same config schema as the web draft.

## TUI installer

Built from `./cmd/installer`. Collects the same path/integration fields, writes units, runs the initial scan, and prints next steps. Prefer this when you want install + configure in one interactive session.

## After setup

```bash
plex2jellyfin status
plex2jellyfin plugin verify    # if Jellyfin was configured
systemctl status plex2jellyfin-daemon plex2jellyfin-web
```

Continue with the [migration guide](/docs/getting-started/migration-guide) for duplicates / consolidate / audit.
