<div align="center">
  <img src="assets/jellywatch_brand.png" alt="JellyWatch" width="480" />
</div>

---

> ⚠️ **WORK IN PROGRESS — NOT STABLE**
>
> This project is under active development. Features may change, break, or disappear without notice. Not recommended for production use. Use at your own risk.

---

Because Sonarr and Radarr can't be trusted with naming conventions.

## What It Does

JellyWatch is a Linux background service that watches download directories, parses media filenames, and renames/moves files into a Jellyfin-compatible layout. A second service (`jellyweb`) ships a web dashboard at port `5522` for monitoring, queue management, duplicate review, and configuration. Optional Ollama integration provides AI-assisted parsing for ambiguous filenames.

```bash
curl -sSL https://raw.githubusercontent.com/Nomadcxx/jellywatch/main/install.sh | sudo bash
```

## The Problem

Your *arr stack downloads `Show.Name.S01E01.1080p.WEB-DL.x264-RARBG.mkv`. Jellyfin wants `TV Shows/Show Name (2019)/Season 01/Show Name (2019) S01E01.mkv`. JellyWatch fixes that automatically, with AI fallback when filenames are ambiguous, and a self-healing convergence loop for files that slip through.

## Architecture

JellyWatch ships **three** binaries:

| Binary | Role |
|---|---|
| `jellywatchd` | Background daemon. Watches download dirs, runs the periodic library scan, executes the housekeeping queue, and exposes a Unix-domain control socket at `~/.config/jellywatch/control.sock`. |
| `jellyweb` | HTTP server (default `:5522`). Hosts the embedded Next.js dashboard and proxies API calls to `jellywatchd` over the control socket. |
| `jellywatch` | CLI for one-shot scans, audits, organize/move operations, duplicate cleanup, and consolidation. |

```mermaid
flowchart TB
    A[Sonarr / Radarr] -->|downloads| B[Download Client]
    B -->|drops file| C[Watch Directory]
    C -->|inotify| D[jellywatchd]
    D -->|rename + move| E[Jellyfin Library]
    D -->|state + audit| DB[(SQLite media.db)]
    D -.->|low confidence| O[Ollama]
    O -.->|suggested rename| D
    D <-->|control.sock| W[jellyweb :5522]
    W <-->|browser| U[You]
    D -->|periodic| H[Housekeeping queue]
    H -->|consolidate / dedupe / fix-naming| E
```

See [`docs/architecture.md`](docs/architecture.md) for details.

## CLI Commands

`jellywatch --help` shows the primary workflows. Advanced and maintenance commands are available but hidden from the root help to keep it focused.

### Primary Commands

```bash
jellywatch scan                          # Index libraries into media.db
jellywatch status                        # DB statistics and deployment health
jellywatch duplicates generate           # Find duplicate media
jellywatch duplicates dry-run            # Preview deletion plan
jellywatch duplicates execute            # Keep the best copy, remove the rest
jellywatch consolidate generate          # Find TV series scattered across drives
jellywatch consolidate dry-run           # Preview consolidation moves
jellywatch consolidate execute           # Merge into a single library path
jellywatch config                        # Manage configuration
jellywatch version                       # Print version information
```

### AI Audit

Reviews files with low parse confidence and proposes renames via the configured LLM:

```bash
jellywatch audit --generate             # Identify low-confidence files
jellywatch audit --generate --dry-run   # Preview AI rename suggestions
jellywatch audit --execute              # Apply approved fixes
```

The model is given the library kind (Movies vs TV), folder path, and current parse as context. See [`docs/ai-context.md`](docs/ai-context.md).

### Duplicates & Consolidation

The daemon runs an automated convergence loop that pushes duplicate detection and consolidation jobs into the housekeeping queue continuously — visible in the web UI at `/scheduler`. The CLI commands above are for one-off manual maintenance.

### Advanced Commands (hidden from root help)

```bash
jellywatch organize /downloads/file.mkv  # Organize a single file
jellywatch organize-folder /downloads/X  # Organize a directory tree
jellywatch watch /downloads              # Foreground watcher
jellywatch validate <path>              # Check library against Jellyfin naming rules
jellywatch cleanup                      # Remove cruft files / empty dirs
jellywatch monitor                      # Tail jellywatchd activity log
jellywatch daemon {start|stop|restart}  # Control the systemd service
jellywatch serve                        # Run the API server in foreground
jellywatch repair series-dedupe         # Repair duplicate series rows
jellywatch database cleanup-housekeeping # Collapse duplicate housekeeping rows
jellywatch postmortem collect --since 96h # Generate evidence bundle for review
jellywatch sonarr ...                   # Sonarr integration commands
jellywatch radarr ...                   # Radarr integration commands
jellywatch health                       # Verify *arr setup is compatible
jellywatch migrate                      # Reconcile DB paths against *arr current state
jellywatch orphans                      # Detect / remediate orphaned Jellyfin episodes
jellywatch parses                       # Query parse_decisions table
```

## Web Dashboard

`jellyweb` serves the dashboard at `http://<host>:5522/`. Routes:

- `/` — overview (media counts, duplicate groups, recent activity)
- `/queue` — current move queue
- `/scheduler` — periodic jobs + housekeeping task list (pending / running / flagged / failed / done)
- `/duplicates` — duplicate groups awaiting review
- `/consolidation` — TV consolidation plans
- `/activity` — daemon activity stream
- `/jellyfin` — Jellyfin connection + path-mapping status
- `/onboarding`, `/login`, `/settings/*` — first-run and configuration

Every settings page maps to a section of `~/.config/jellywatch/config.toml`.

## Naming Rules

**Movies:** `Movies/Movie Name (YYYY)/Movie Name (YYYY).ext`

**TV Shows:** `TV Shows/Show Name (Year)/Season 01/Show Name (Year) S01E01.ext`

Release-group noise (`1080p`, `x264`, `WEB-DL`, `RARBG`, `-YTS`, etc.) is stripped during parse. Resolution / source / HDR are also extracted from the parent directory when missing from the filename, so quality grouping works on legacy libraries.

## Configuration

Lives at `~/.config/jellywatch/config.toml`. A full annotated template is in [`config.toml.example`](config.toml.example).

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
enabled              = true
ollama_endpoint      = "http://localhost:11434"
model                = "minimax-m2.5:cloud"
fallback_model       = "kimi-k2.6:cloud"
confidence_threshold = 0.8
auto_trigger_threshold = 0.6
timeout_seconds      = 30
cache_enabled        = true
auto_resolve_risky   = false
max_retries          = 3
hourly_limit         = 10
daily_limit          = 50

[options]
dry_run          = false
delete_source    = true
```

### Sonarr / Radarr

```toml
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
```

### Jellyfin path mappings

When Jellyfin runs in a container with bind mounts, configure path mappings so the post-organize feedback loop can correlate Jellyfin items with daemon paths:

```toml
[jellyfin]
enabled        = true
url            = "http://localhost:8096"
api_key        = "..."
webhook_secret = "..."

[[jellyfin.path_mappings]]
jellyfin = "/tv5"
daemon   = "/mnt/STORAGE5/TVSHOWS"
```

Without these, parse-decision rows for organized files are eventually labeled FAIL by the sweeper.

### File Permissions

If Jellyfin runs as a different user, set ownership on moved files:

```toml
[permissions]
user      = "jellyfin"
group     = "jellyfin"
file_mode = "0644"
dir_mode  = "0755"
```

> **Note:** `jellywatchd` must run as root to chown files. The bundled systemd unit drops to a minimal capability set: `CAP_CHOWN`, `CAP_FOWNER`, `CAP_DAC_OVERRIDE`.

## Services

The installer registers three systemd units:

```bash
systemctl status jellywatchd              # daemon
systemctl status jellyweb                # web UI on :5522
systemctl --user status jellywatch-postmortem.timer  # scheduled evidence collection
journalctl -u jellywatchd -f
```

`jellyweb` depends on `jellywatchd` and reaches it via the Unix-domain control socket — no TCP between them.

The postmortem timer runs every 4 days, collecting parse decisions, repair events, housekeeping state, and suspicious items into an evidence bundle at `~/.config/jellywatch/reports/latest/`. It opens a terminal with an `agent-prompt.md` for periodic human or LLM review.

## Install

**One-liner:**

```bash
curl -sSL https://raw.githubusercontent.com/Nomadcxx/jellywatch/main/install.sh | sudo bash
```

**Manual:**

```bash
git clone https://github.com/Nomadcxx/jellywatch.git
cd jellywatch
go build -o installer ./cmd/installer
sudo ./installer
```

The installer is interactive and configures watch paths, library paths, *arr keys, AI, permissions, and systemd units. A non-destructive update mode preserves an existing `config.toml`.

Requires **Go 1.24+** (see `go.mod`).

## Building from source

```bash
make                                       # build all binaries into bin/
go build -o bin/jellywatchd ./cmd/jellywatchd
go build -o bin/jellyweb    ./cmd/jellyweb
go build -o bin/jellywatch  ./cmd/jellywatch
cd web && npm run build                    # rebuild dashboard (embedded into jellyweb)
./test-all.sh                              # full test sweep
```

## License

GPL-3.0 or later
