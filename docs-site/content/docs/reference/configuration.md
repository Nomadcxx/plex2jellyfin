---
title: Configuration
description: Complete config.toml reference for local and container deployments.
---

Config file: `~/.config/plex2jellyfin/config.toml` on bare metal, `/config/.config/plex2jellyfin/config.toml` in Docker (see [Docker](/docs/getting-started/docker#where-config-lives-in-the-container)).

A starter template ships as [`config.toml.example`](https://github.com/Nomadcxx/plex2jellyfin/blob/main/config.toml.example). For a file that matches the current schema (including defaults), generate:

```bash
plex2jellyfin config init
```

This page documents every key `config init` / `Config.ToTOML()` emits, plus a few loadable extras noted where they matter. Defaults below are from `DefaultConfig` unless stated otherwise.

## `[setup]`

Wizards write this block; you normally do not edit it.

| Key | Type | Default | Description |
|---|---|---|---|
| `version` | int | `0` until a wizard runs | Setup schema version. |
| `completed` | bool | `false` | `true` after CLI/web/TUI setup finishes. Web then skips guided setup (password creation only). |

Completing setup on any surface completes it for all. The web server adopts older configured installs that predate this block. To re-run web guided setup, set `completed = false` and restart `plex2jellyfin-web`.

## `[watch]`

| Key | Type | Description |
|---|---|---|
| `movies` | string[] | Download drop dirs for movies. |
| `tv` | string[] | Download drop dirs for TV. |

```toml
[watch]
movies = ["/path/to/downloads/movies"]
tv = ["/path/to/downloads/tv"]
```

## `[libraries]`

| Key | Type | Description |
|---|---|---|
| `movies` | string[] | Destination movie library roots. |
| `tv` | string[] | Destination TV library roots. |

```toml
[libraries]
movies = ["/path/to/jellyfin/Movies"]
tv = ["/path/to/jellyfin/TV Shows"]
```

Both `[watch]` and `[libraries]` accept multiple paths for multi-drive setups.

## `[sonarr]` / `[radarr]`

| Key | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Enable *arr API notifications. |
| `url` | string | `""` | Base URL (e.g. `http://localhost:8989`). |
| `api_key` | string | `""` | From Settings → General → API Key. |
| `notify_on_import` | bool | `true` | Tell *arr about files the organizer moved. |

```toml
[sonarr]
enabled = false
url = "http://localhost:8989"
api_key = ""
notify_on_import = true

[radarr]
enabled = false
url = "http://localhost:7878"
api_key = ""
notify_on_import = true
```

## `[jellyfin]`

| Key | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Enable Jellyfin API / feedback loop. |
| `url` | string | `""` | Jellyfin base URL. |
| `api_key` | string | `""` | From Dashboard → API Keys. |
| `notify_on_import` | bool | `true` | Trigger library refresh after organizes. |
| `playback_safety` | bool | `true` | Block moves while media is actively streamed. |
| `verify_after_refresh` | bool | `false` | Query Jellyfin after refresh to verify identification. |
| `webhook_secret` | string | `""` | Shared secret; webhook requests must send `X-Plex2Jellyfin-Webhook-Secret`. |
| `plugin_enabled` | bool | `false` | Companion plugin expected / configured. |
| `plugin_shared_secret` | string | `""` | Usually the same value as `webhook_secret`. |
| `plugin_auto_scan` | bool | `true` | Ask Jellyfin to scan after organizing. |
| `plugin_verify_on_startup` | bool | `false` | Run plugin verification when the daemon starts. |
| `plugin_verify_interval` | int | `0` | Hours between automatic verifications (`0` = disabled). |
| `plugin_daemon_url` | string | `""` | Base URL the plugin calls back to (appends `/api/v1/webhooks/jellyfin`). Must be reachable **from Jellyfin**; never `localhost` when either side is containerized. |

```toml
[jellyfin]
enabled = true
url = "http://localhost:8096"
api_key = "..."
notify_on_import = true
playback_safety = true
verify_after_refresh = false
webhook_secret = "..."
plugin_enabled = true
plugin_shared_secret = "..."
plugin_auto_scan = true
plugin_verify_on_startup = false
plugin_verify_interval = 0
plugin_daemon_url = "http://192.168.1.10:5522"
```

### `[[jellyfin.path_mappings]]`

| Key | Type | Description |
|---|---|---|
| `jellyfin` | string | Path prefix as Jellyfin reports it (often container-internal). |
| `daemon` | string | Same root as Plex2Jellyfin sees it (host / `[libraries]` path). |

```toml
[[jellyfin.path_mappings]]
jellyfin = "/tv5"
daemon   = "/mnt/STORAGE5/TVSHOWS"

[[jellyfin.path_mappings]]
jellyfin = "/movies"
daemon   = "/mnt/STORAGE2/MOVIES"
```

Longest-prefix match. Without mappings when paths diverge: organizes still succeed, but webhooks never match `parse_decisions`, `jellyfin_item_id` stays empty, PASS/DRIFT/FAIL never runs, and the sweeper eventually marks uncorrelated rows FAIL. Full guide: [Path mappings](/docs/getting-started/path-mappings).

## `[metadata_recovery]`

Passive recovery checks Jellyfin for metadata that arrives after import. Active repair asks Jellyfin to refresh items (off by default).

| Key | Type | Default | Description |
|---|---|---|---|
| `passive_enabled` | bool | `true` | Periodically reconcile missing metadata from Jellyfin. |
| `repair_enabled` | bool | `false` | Actively request Jellyfin refreshes. |
| `passive_interval_minutes` | int | `60` | Minutes between passive passes. |
| `passive_batch_size` | int | `25` | Max items per passive pass. |
| `repair_batch_size` | int | `5` | Max items per repair pass. |
| `repair_cooldown_hours` | int | `6` | Hours before re-repairing the same item. |
| `needs_review_after` | int | `4` | Failures before an item is marked needs-review. |

## `[daemon]`

| Key | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Whether the daemon’s watcher / periodic scan are considered enabled in config (wizards set `true` on activate). |
| `scan_frequency` | string | `"5m"` | Convergence re-scan interval (Go duration, e.g. `"5m"`, `"1h"`). |
| `health_addr` | string | `":8686"` | Daemon health HTTP bind address. |

## `[options]`

| Key | Type | Default | Description |
|---|---|---|---|
| `dry_run` | bool | `false` | Preview flag for some CLI flows. **The daemon always organizes for real** and forces this off at startup. |
| `verify_checksums` | bool | `false` | Verify integrity after transfer before deleting the source. |
| `delete_source` | bool | `true` | Delete source after a successful move (`false` = copy). |

Loadable but not always emitted by `config init`: `transfer_concurrency_per_volume` (int; caps parallel transfers to the same destination mount; `<=0` disables; unset uses the daemon default of `2`).

## `[ai]`

| Key | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Global AI naming assist. |
| `ollama_endpoint` | string | `"http://localhost:11434"` | Ollama URL (local or [cloud](https://ollama.com)). |
| `model` | string | `"qwen2.5vl:7b"` | Primary model. |
| `fallback_model` | string | `""` | Used if primary errors / circuit-breaks. |
| `confidence_threshold` | float | `0.8` | Cutoff for `audit --generate` candidates. |
| `auto_trigger_threshold` | float | `0.6` | Daemon queues AI only when confidence is **below** this. Deterministic movie/TV identity bypasses AI even when low. |
| `timeout_seconds` | int | `30` | Per-request timeout. |
| `cache_enabled` | bool | `true` | Cache AI responses. |
| `cloud_model` | string | `"nemotron-3-nano:30b-cloud"` | Optional cloud model name. |
| `auto_resolve_risky` | bool | `false` | Auto-apply suggestions flagged risky. |
| `max_retries` | int | `3` | Retries before fallback. |
| `hourly_limit` | int | `10` | Max AI calls per hour. |
| `daily_limit` | int | `50` | Max AI calls per day. |
| `enhancement_interval_seconds` | int | `30` | Spacing between daemon AI enhancement workers. |

Loadable nested tables (not emitted by `ToTOML` today; defaults apply if omitted):

```toml
[ai.circuit_breaker]
failure_threshold = 5
failure_window_seconds = 120
cooldown_seconds = 30

[ai.keepalive]
enabled = true
interval_seconds = 300
idle_timeout_seconds = 1800
```

`retry_delay` (duration, default `500ms`) is also loadable.

## `[logging]`

| Key | Type | Default | Description |
|---|---|---|---|
| `level` | string | `"info"` | Log level. |
| `file` | string | `""` | Log file path (`""` = default under the config dir). |
| `max_size_mb` | int | `10` | Rotate after this many MB. |
| `max_backups` | int | `5` | Rotated files to keep. |
| `max_age_days` | int | `0` | Delete rotated files older than this (`0` = age pruning off). |
| `compress` | bool | `false` | Gzip rotated backups. |

## `[api]`

| Key | Type | Default | Description |
|---|---|---|---|
| `allowed_origins` | string[] | `["http://localhost:3000"]` | CORS origins for the web UI. Same-origin production usually needs no change; reverse proxies / external dev servers do. |

## `[permissions]`

Optional. Emitted by `ToTOML` only when ownership/mode fields are set. Sets ownership and mode on files the daemon moves (bare metal).

| Key | Type | Description |
|---|---|---|
| `user` | string | Username or numeric UID (empty = leave owner). |
| `group` | string | Group name or GID. |
| `file_mode` | string | Octal file mode (e.g. `"0664"`). |
| `dir_mode` | string | Octal directory mode (e.g. `"0775"`). |

> **Requires root on bare metal; unavailable in Docker.** The systemd unit runs the daemon as root with `CAP_CHOWN` / `CAP_FOWNER` / `CAP_DAC_OVERRIDE`. Inside Docker, use `PUID`/`PGID` instead — see [Docker](/docs/getting-started/docker#the-permissions-chown-feature-is-unavailable-in-container).

## `[jellystat]`

Optional. Emitted only when enabled or a URL is set.

| Key | Type | Description |
|---|---|---|
| `enabled` | bool | Enable Jellystat integration. |
| `url` | string | Jellystat base URL. |
| `api_key` | string | Jellystat API key. |

## Auth (top-level)

Managed by the web UI. Emitted near the top of the file when set:

| Key | Type | Description |
|---|---|---|
| `password_hash` | string | bcrypt hash of the admin password. Delete to reset. |
| `secure_cookies` | bool | Prefer secure cookie flags (HTTPS deployments). |

Legacy plaintext `password` is still read and migrated to `password_hash` on load; new writes should not persist plaintext.

## Loadable extras (not in `ToTOML`)

| Section / key | Notes |
|---|---|
| `[tmdb]` `enabled`, `api_key` | Optional TMDB fallback for housekeeping verification. |
| `[options].transfer_concurrency_per_volume` | See `[options]` above. |
| `[ai].circuit_breaker` / `keepalive` / `retry_delay` | See `[ai]` above. |

## Full example

```toml
[setup]
version = 1
completed = true

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

[options]
dry_run          = false
verify_checksums = false
delete_source    = true

[ai]
enabled                      = true
ollama_endpoint              = "http://localhost:11434"
model                        = "qwen2.5vl:7b"
fallback_model               = ""
cloud_model                  = "nemotron-3-nano:30b-cloud"
confidence_threshold         = 0.8
auto_trigger_threshold       = 0.6
timeout_seconds              = 30
cache_enabled                = true
auto_resolve_risky           = false
max_retries                  = 3
hourly_limit                 = 10
daily_limit                  = 50
enhancement_interval_seconds = 30

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
enabled                   = true
url                       = "http://localhost:8096"
api_key                   = "..."
notify_on_import          = true
playback_safety           = true
verify_after_refresh      = false
webhook_secret            = "..."
plugin_enabled            = true
plugin_shared_secret      = "..."
plugin_auto_scan          = true
plugin_verify_on_startup  = false
plugin_verify_interval    = 0
plugin_daemon_url         = "http://192.168.1.10:5522"

[[jellyfin.path_mappings]]
jellyfin = "/tv"
daemon   = "/mnt/storage1/TVSHOWS"

[metadata_recovery]
passive_enabled          = true
repair_enabled           = false
passive_interval_minutes = 60
passive_batch_size       = 25
repair_batch_size        = 5
repair_cooldown_hours    = 6
needs_review_after       = 4

[logging]
level        = "info"
file         = ""
max_size_mb  = 10
max_backups  = 5
max_age_days = 0
compress     = false

[api]
allowed_origins = ["http://localhost:3000"]

[permissions]
user      = "jellyfin"
group     = "media"
file_mode = "0664"
dir_mode  = "0775"
```

Validate with:

```bash
plex2jellyfin config show
plex2jellyfin config test
```
