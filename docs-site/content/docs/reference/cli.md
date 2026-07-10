---
title: CLI Reference
description: Commands for migration, repair, audit, and daemon control.
---

`plex2jellyfin --help` shows the primary workflows. Several maintenance and integration commands exist but stay hidden from the root `--help` output to keep it short — they're documented here. Every command also accepts `--help` for its own flag list.

## Primary

The core one-shot migration workflow. See the [Migration Guide](/docs/getting-started/migration-guide) for the recommended run order.

| Command | Description |
|---|---|
| `plex2jellyfin scan` | Index libraries into `media.db`. |
| `plex2jellyfin status` | Show database statistics and deployment health. |
| `plex2jellyfin duplicates generate` | Find duplicate media. |
| `plex2jellyfin duplicates dry-run` | Preview the deletion plan. |
| `plex2jellyfin duplicates execute` | Keep the best copy, remove the rest. |
| `plex2jellyfin consolidate generate` | Find TV series scattered across drives. |
| `plex2jellyfin consolidate dry-run` | Preview consolidation moves. |
| `plex2jellyfin consolidate execute` | Merge scattered series into a single library path. |
| `plex2jellyfin config` | Manage configuration (see [below](#config)). |

### scan flags

| Flag | Default | Description |
|---|---|---|
| `--path <dir>` | — | Scan a specific file or directory under a configured library root instead of everything |
| `--sonarr` | `false` | Also sync from Sonarr |
| `--radarr` | `false` | Also sync from Radarr |
| `--filesystem` | `true` | Scan the filesystem |
| `--stats` | `true` | Show database stats after scan |
| `--rebuild-movies-from-files` | `false` | Rebuild movie rows from indexed movie media files |
| `--analyze` | `false` | After scanning, analyze duplicates and scattered media |
| `--json` | `false` | Emit a machine-readable JSON summary |

## AI audit

Reviews files with low parse confidence and proposes renames via the configured LLM (see [`[ai]`](/docs/reference/configuration#ai)):

```bash
plex2jellyfin audit --generate            # identify low-confidence files
plex2jellyfin audit --generate --dry-run  # preview AI rename suggestions
plex2jellyfin audit --execute             # apply approved fixes
```

| Flag | Default | Description |
|---|---|---|
| `--generate` | `false` | Generate AI suggestions for low-confidence files |
| `--dry-run` | `false` | Preview changes without executing |
| `--execute` | `false` | Execute the generated audit plan |
| `--threshold <float>` | `0.8` | Confidence threshold below which a file is audited |
| `--limit <n>` | `0` | Maximum number of files to audit (`0` = uncapped) |

The audit sends library kind (Movies vs TV), folder path, and current parse as context — never raw file contents.

## Advanced (hidden from root `--help`)

```bash
plex2jellyfin organize /downloads/file.mkv   # Organize a single file
plex2jellyfin organize-folder /downloads/X   # Organize a directory tree
plex2jellyfin watch /downloads               # Foreground watcher
plex2jellyfin validate <path>                # Check library against Jellyfin naming rules
plex2jellyfin cleanup {cruft|empty}          # Remove cruft files or empty dirs
plex2jellyfin monitor                        # Tail daemon activity log
plex2jellyfin daemon {status|reload|stop}    # Control the systemd-managed daemon
plex2jellyfin repair series-dedupe           # Repair duplicate series rows
plex2jellyfin repair unknown-seasons         # Repair "Season Unknown" placements
plex2jellyfin postmortem collect --since 96h # Generate an evidence bundle for review
plex2jellyfin sonarr ...                     # Sonarr integration commands
plex2jellyfin radarr ...                     # Radarr integration commands
plex2jellyfin health                         # Verify *arr setup is compatible
plex2jellyfin orphans                        # Detect / remediate orphaned Jellyfin episodes
plex2jellyfin migrate                        # Migrate path mismatches between database and Sonarr/Radarr
plex2jellyfin parses                         # Query and manage parse decisions
plex2jellyfin fix                            # Interactive wizard to clean up your media library
plex2jellyfin database ...                   # Database management commands
plex2jellyfin serve                          # Start the API server (used internally by plex2jellyfin-web)
```

### cleanup

| Subcommand | Description |
|---|---|
| `cleanup cruft` | Delete sample files, `.nfo`, `.txt`, and other non-media cruft |
| `cleanup empty` | Delete empty directories |

### daemon

Controls the running daemon over its control socket rather than starting/stopping it directly — use `systemctl` for that (see [Daemon & Services](/docs/reference/daemon-services)).

| Subcommand | Description |
|---|---|
| `daemon status` | Show daemon status |
| `daemon reload` | Reload daemon config without restarting the process |
| `daemon stop` | Stop the daemon |

### database

| Subcommand | Description |
|---|---|
| `database init` | Initialize a fresh database |
| `database reset` | Delete and reinitialize the database |
| `database path` | Show the database file path |
| `database cleanup-housekeeping` | Collapse duplicate housekeeping manual-review failures |

### repair

| Subcommand | Description |
|---|---|
| `repair series-dedupe` | Merge duplicate series rows that share the same canonical path |
| `repair unknown-seasons` | Repair files/rows stuck under "Season Unknown" |

### postmortem

```bash
plex2jellyfin postmortem collect --since 96h
```

Generates an evidence bundle (parse decisions, repair events, housekeeping state, suspicious items) for manual review or handing to an LLM. Runs automatically every 4 days via the `plex2jellyfin-postmortem.timer` user unit — see [Daemon & Services](/docs/reference/daemon-services#postmortem-timer).

## Sonarr / Radarr integration

```bash
plex2jellyfin sonarr status                  # Check Sonarr connection status
plex2jellyfin sonarr queue                   # List Sonarr download queue
plex2jellyfin sonarr clear-stuck             # Clear stuck items from Sonarr queue
plex2jellyfin sonarr import <path>           # Trigger Sonarr import scan for a path

plex2jellyfin radarr status                  # Check Radarr connection status
plex2jellyfin radarr queue                   # List Radarr download queue
plex2jellyfin radarr clear-stuck             # Clear stuck items from Radarr queue
plex2jellyfin radarr import <path>           # Trigger Radarr import scan for a path
plex2jellyfin radarr movies                  # List movies in Radarr library
```

Requires `[sonarr]`/`[radarr]` configured with a valid `url` and `api_key` — see [Configuration](/docs/reference/configuration#sonarr-radarr).

## config

```bash
plex2jellyfin config init          # Create default configuration file
plex2jellyfin config show          # Display current configuration
plex2jellyfin config test          # Test configuration and connections
plex2jellyfin config path          # Show config file path
```

`config init --force` overwrites an existing config file; without `--force` it refuses to clobber one.

## health

```bash
plex2jellyfin health
```

Verifies your Sonarr/Radarr setup is compatible with Plex2Jellyfin's expectations (root folder layout, naming settings, etc.) before you rely on the integration.

## orphans

```bash
plex2jellyfin orphans
```

Detects Jellyfin library items that no longer correspond to a file the daemon knows about (deleted, moved outside the tool, etc.) and offers remediation.

## version

```bash
plex2jellyfin version
```

Prints version information.
