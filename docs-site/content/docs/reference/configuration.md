---
title: Configuration
description: Complete config.toml reference for local and container deployments.
---

Config file: `~/.config/plex2jellyfin/config.toml` on bare metal, `/config/.config/plex2jellyfin/config.toml` in Docker (see [Docker](/docs/getting-started/docker#where-config-lives-in-the-container)).

A fully annotated template ships as [`config.toml.example`](https://github.com/Nomadcxx/plex2jellyfin/blob/main/config.toml.example) in the repo root and is installed to `/usr/share/doc/plex2jellyfin/config.toml.example` by the deb/rpm packages. Generate a starting point with:

```bash
plex2jellyfin config init
```

## `[watch]`

Directories to watch for new media files — typically where Sonarr/Radarr/download clients drop completed downloads.

```toml
[watch]
movies = ["/path/to/downloads/movies"]
tv = ["/path/to/downloads/tv"]
```

## `[libraries]`

Destination Jellyfin library directories. The organizer writes renamed, moved files here.

```toml
[libraries]
movies = ["/path/to/jellyfin/Movies"]
tv = ["/path/to/jellyfin/TV Shows"]
```

Both `[watch]` and `[libraries]` accept multiple paths per array for multi-drive setups — the daemon and CLI treat them as a pooled set of roots.

## `[daemon]`

```toml
[daemon]
enabled        = true
scan_frequency = "5m"
health_addr    = ":8686"
```

- `enabled` — whether the daemon's file-watcher and periodic scan run at all
- `scan_frequency` — how often the convergence loop re-scans the library for drift (Go duration string, e.g. `"5m"`, `"1h"`)
- `health_addr` — address the daemon's health-check HTTP endpoint binds to

## `[options]`

```toml
[options]
dry_run           = false
verify_checksums  = false
delete_source     = true
```

- `dry_run` — global override: preview every operation without writing/moving/deleting anything
- `verify_checksums` — verify file integrity (checksum) after a move before deleting the source
- `delete_source` — whether the source file is deleted after a successful move (set `false` to copy instead of move)

## `[ai]`

```toml
[ai]
enabled                = true
ollama_endpoint        = "http://localhost:11434"
model                  = "minimax-m2.5:cloud"
fallback_model         = "kimi-k2.6:cloud"
confidence_threshold   = 0.8
auto_trigger_threshold = 0.6
timeout_seconds        = 30
cache_enabled          = true
auto_resolve_risky     = false
max_retries            = 3
hourly_limit           = 10
daily_limit            = 50
```

- `enabled` — turn AI-assisted naming on/off globally
- `ollama_endpoint` — Ollama server URL (local install or [Ollama Cloud](https://ollama.com))
- `model` / `fallback_model` — primary and fallback model names; the fallback is used if the primary errors or trips the circuit breaker
- `confidence_threshold` — parse confidence below this triggers an AI lookup during `audit --generate` and daemon ingestion
- `auto_trigger_threshold` — separate, typically lower, threshold below which the daemon automatically queries the AI in real time (vs. leaving the file for a manual `audit` pass)
- `timeout_seconds` — per-request timeout
- `cache_enabled` — cache AI responses to avoid repeat calls for the same input
- `auto_resolve_risky` — whether to auto-apply AI suggestions the tool flags as risky, vs. always queuing them for manual review
- `max_retries` — retry attempts before falling back
- `hourly_limit` / `daily_limit` — rate limits on AI calls, to protect a local Ollama instance or a metered cloud model

## `[sonarr]` / `[radarr]`

```toml
[sonarr]
enabled          = false
url              = "http://localhost:8989"
api_key          = ""
notify_on_import = true

[radarr]
enabled          = false
url              = "http://localhost:7878"
api_key          = ""
notify_on_import = true
```

Get `api_key` from Sonarr/Radarr under **Settings &rarr; General &rarr; API Key**. `notify_on_import` tells Sonarr/Radarr about files the organizer moves so their own libraries stay in sync.

## `[permissions]`

Optional. Sets ownership and mode on files the daemon moves — useful when Jellyfin runs as a different user than the download client on bare metal.

```toml
[permissions]
user      = "jellyfin"      # Username or numeric UID
group     = "jellyfin"      # Group name or numeric GID
file_mode = "0644"          # File permissions (rw-r--r--)
dir_mode  = "0755"          # Directory permissions (rwxr-xr-x)
```

Leave `user`/`group` empty (or omit the section) to preserve source ownership.

> **Requires root on bare metal; unavailable in Docker**
>
> `plex2jellyfin-daemon` must run as root to chown files to a different user. The bundled systemd unit runs it as root with a minimal capability set (`CAP_CHOWN`, `CAP_FOWNER`, `CAP_DAC_OVERRIDE`) rather than full root privileges.
>
> **This feature has no effect inside the Docker image** — the container always runs the daemon as the `PUID`/`PGID` user with no elevated process left to chown to something else. Use `PUID`/`PGID` instead. Full explanation on the [Docker page](/docs/getting-started/docker#the-permissions-chown-feature-is-unavailable-in-container).

## `[jellyfin]`

```toml
[jellyfin]
enabled           = true
url               = "http://localhost:8096"
api_key           = "..."
webhook_secret    = "..."
plugin_daemon_url = "http://192.168.1.10:5522"
```

Connects to Jellyfin's API so the daemon can query and correlate library items after organizing files.

- `plugin_daemon_url` — base URL the companion plugin calls back to
  (the plugin appends `/api/v1/webhooks/jellyfin`). Set by the wizard;
  must be reachable *from Jellyfin's network*, so never `localhost`
  when either side is containerized.

### Jellyfin path mappings

When Jellyfin runs in a container with bind mounts, its view of a file's path (container-internal) differs from the daemon's view (host filesystem path, or this tool's own container mounts). Configure path mappings so the post-organize feedback loop can correlate Jellyfin items with daemon paths:

```toml
[[jellyfin.path_mappings]]
jellyfin = "/tv5"
daemon   = "/mnt/STORAGE5/TVSHOWS"

[[jellyfin.path_mappings]]
jellyfin = "/movies"
daemon   = "/mnt/STORAGE2/MOVIES"
```

Mappings apply longest-prefix first. **Without these, the sweeper labels parse-decision rows for organized files as FAIL** once it can no longer correlate a Jellyfin item with a daemon-known path — this is the single most common misconfiguration when both Jellyfin and Plex2Jellyfin run in containers with different mount layouts.

## `[setup]`

Written by the web setup wizard; you normally never edit it.

```toml
[setup]
version   = 1
completed = true
```

| Key | Meaning |
|---|---|
| `version = 1, completed = true` | Setup finished; the dashboard is available. |
| `version = 1, completed = false` | A wizard activation was interrupted or failed — the web UI returns to the wizard's Review step. |
| absent (`version 0`) | A config written before the wizard existed. Treated as configured when it contains at least one complete incoming/library pair, so upgrades never force existing installs through setup. |

Deleting the whole `[setup]` block (and keeping a valid config) is safe;
deleting your `[watch]`/`[libraries]` sections as well sends the next web
visit back through the wizard.

## Full example

```toml
[watch]
movies = ["/downloads/movies"]
tv     = ["/downloads/tv"]

[libraries]
movies = ["/media/Movies"]
tv     = ["/media/TV Shows"]

[daemon]
enabled        = true
scan_frequency = "5m"
health_addr    = ":8686"

[ai]
enabled                = true
ollama_endpoint        = "http://localhost:11434"
model                  = "minimax-m2.5:cloud"
fallback_model         = "kimi-k2.6:cloud"
confidence_threshold   = 0.8
auto_trigger_threshold = 0.6
timeout_seconds        = 30
cache_enabled          = true
auto_resolve_risky     = false
max_retries            = 3
hourly_limit           = 10
daily_limit            = 50

[options]
dry_run       = false
delete_source = true

[sonarr]
enabled          = true
url              = "http://localhost:8989"
api_key          = "..."
notify_on_import = true

[radarr]
enabled          = true
url              = "http://localhost:7878"
api_key          = "..."
notify_on_import = true

[jellyfin]
enabled           = true
url               = "http://localhost:8096"
api_key           = "..."
webhook_secret    = "..."
plugin_daemon_url = "http://192.168.1.10:5522"

[[jellyfin.path_mappings]]
jellyfin = "/tv"
daemon   = "/mnt/storage1/TVSHOWS"
```

Validate a config with:

```bash
plex2jellyfin config show
plex2jellyfin config test
```
