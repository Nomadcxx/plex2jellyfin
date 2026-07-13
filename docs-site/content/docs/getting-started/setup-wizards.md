---
title: Setup wizards
description: First-run configuration via web UI, CLI, or TUI — same outcomes, different surfaces.
---

All three wizards write the same `config.toml`, activate `plex2jellyfin-daemon`, optionally install the Jellyfin companion plugin, run an initial library index into `media.db`, and chown the config directory so your interactive user can read the DB afterward.

| Surface | How you start | Best when |
|---|---|---|
| **TUI** | `install.sh` / `plex2jellyfin-installer` | Fresh machine, want guided terminal UI |
| **CLI** | `plex2jellyfin setup` | Headless / SSH, prefer prompts over browser |
| **Web** | Open `:5522` after web install | Prefer a browser; packages / web fresh-build |

## Shared outcomes

Every surface covers the same concerns (order differs — see per-surface sections below):

1. **Escalate** — CLI/TUI re-exec with sudo when needed (chown, systemd). Web already runs as a privileged service.
2. **Auth** — Web requires creating an admin password before the wizard. CLI/TUI do not use the web session.
3. **Paths** — Watch dirs (download drops) and library roots (Jellyfin Movies / TV). Multi-path paste is supported in the web UI (comma or newline separated).
4. **Services / *arr / Jellyfin / AI** — Optional Sonarr, Radarr, Jellyfin URL + API key, Ollama. When Jellyfin is enabled, wizards detect unmapped library roots and require [path mappings](/docs/getting-started/path-mappings) if Jellyfin’s paths differ from `[libraries]`.
5. **Permissions** — Target user/group for moved files (`[permissions]`).
6. **Config write + daemon + initial scan + plugin** — atomic config; enable/start daemon when units exist; index libraries (soft-stall ~12 minutes per hung mount); companion plugin install/verify when Jellyfin is enabled (needed together with path mappings for the feedback loop).
7. **Complete** — `[setup] completed = true` in config.

Initial scan soft-stall: a library with no walk progress for ~12 minutes is skipped so one hung mount cannot block the rest. Time-based heartbeats keep UIs alive on slow NAS shares.

## Web wizard

1. Install via [Option C](/docs/getting-started/installation#option-c--build--web-setup) or packages, then open `http://<host>:5522/`.
2. Create the admin password.
3. Walk Media → Services → AI → Runtime → Review. On Services, Test Jellyfin and add [path mappings](/docs/getting-started/path-mappings) until unmapped roots are empty (Continue is blocked otherwise).
4. **Apply** saves config, can install/configure the companion plugin, and starts/enables the daemon.
5. **Initial library scan** streams progress over SSE (`GET /api/v1/setup/index/stream`) when libraries are configured. Closing the tab leaves setup incomplete so you can retry.
6. Results screen shows indexed counts; open the dashboard when done.

If Apply already saved config but indexing was interrupted, reopen setup and retry the index step (or re-Apply when the UI offers it).

## CLI wizard

```bash
sudo plex2jellyfin setup
```

Prompt order (simplified):

1. Paths → Sonarr/Radarr → Jellyfin (connect + **path mappings** if roots diverge) → companion plugin install/restart prompts → AI → runtime/permissions.
2. Confirm → write config → enable/start daemon (and optionally web).
3. Initial library scan with per-library progress bars.
4. Plugin verify / callback URL when applicable; stamp `[setup] completed`.

If Jellyfin’s VirtualFolders list fails after a successful connect, the CLI warns and can skip mapping prompts — add `[[jellyfin.path_mappings]]` later and re-test. Same config schema as the web draft.

## TUI installer

Built from `./cmd/installer` (also via `install.sh`). Collects the same path/integration fields, including path mappings after a successful Jellyfin Test (Continue blocked while unmapped roots remain).

Task order differs from CLI/web:

1. Write `config.toml` (including live `[[jellyfin.path_mappings]]` when collected).
2. Optionally install the companion plugin.
3. **Initial library scan**, then start systemd units.
4. Post-scan plugin configure/verify when enabled.

Prefer this when you want install + configure in one interactive session.

## After setup

```bash
plex2jellyfin status
plex2jellyfin plugin verify    # if Jellyfin was configured
systemctl status plex2jellyfin-daemon plex2jellyfin-web
```

If Jellyfin runs with bind mounts that differ from `[libraries]`, confirm [path mappings](/docs/getting-started/path-mappings). Continue with the [migration guide](/docs/getting-started/migration-guide) for duplicates / consolidate / audit.
