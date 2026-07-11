<div align="center">
  <img src="assets/plex2jellyfin-header.png" alt="plex2jellyfin" width="640" />
</div>

Your Plex library, renamed the way Jellyfin wants it. Migrate the whole thing once, then a daemon keeps every new download clean.

> **Beta.** Destructive operations run as generate → dry-run → execute, so you preview every change before it touches a file. Back up anything you can't re-download.

## Quick Links

- [Full Documentation](https://nomadcxx.github.io/plex2jellyfin/docs/) - Install guides, Docker walkthrough, migration guide, CLI and config reference

## Installation

### Quick Install

```bash
curl -sSL https://raw.githubusercontent.com/Nomadcxx/plex2jellyfin/main/install.sh | sudo bash
```

The installer walks you through watch paths, library paths, *arr keys, AI, permissions, and systemd units. Re-run it to update; it preserves your existing `config.toml`.

### Docker

One image, three binaries: the daemon and web UI run together under the entrypoint, and the CLI is available for one-off commands (`docker run --rm <image> plex2jellyfin version`).

```yaml
services:
  plex2jellyfin:
    image: ghcr.io/nomadcxx/plex2jellyfin:latest
    container_name: plex2jellyfin
    environment:
      - PUID=1000
      - PGID=1000
    volumes:
      - ./config:/config
      - /path/to/downloads:/watch
      - /path/to/media:/library
    ports:
      - "5522:5522"
    restart: unless-stopped
```

```bash
docker compose -f docker-compose.example.yml up -d
```

`PUID`/`PGID` (linuxserver.io-style, default `1000:1000`) set the user everything runs as inside the container; `/config` is chowned to match on start. Set them to the UID/GID that should own files under `/library`.

> The container runs as that non-root user, so the `[permissions]` chown feature has nothing to elevate to and is unavailable in-container. `PUID`/`PGID` replace it. The [Docker guide](https://nomadcxx.github.io/plex2jellyfin/docs/getting-started/docker/) covers SELinux and rootless setups.

### Pre-built Packages (Debian/Ubuntu/Fedora)

Download the `.deb` or `.rpm` from [GitHub Releases](https://github.com/Nomadcxx/plex2jellyfin/releases/latest):

```bash
sudo apt install ./plex2jellyfin_*_amd64.deb      # Debian/Ubuntu
sudo dnf install ./plex2jellyfin-*.x86_64.rpm     # Fedora
```

The package installs binaries and systemd units but no configuration — the web UI's **setup wizard** handles that. Two steps on the host, the rest in the browser:

```bash
# 1. Tell the services which user's config to read
sudo systemctl edit plex2jellyfin-daemon          # opens a drop-in; add the two lines below
sudo systemctl edit plex2jellyfin-web
```

```ini
[Service]
Environment=SUDO_USER=<your username>
```

```bash
# 2. Enable (the daemon restart-loops until the wizard writes a config — expected)
sudo systemctl enable --now plex2jellyfin-daemon plex2jellyfin-web
```

Then open `http://<host>:5522/`: create the admin password, and the setup wizard collects watch/library paths, optional *arr / Jellyfin / Ollama connections, validates everything, writes the config atomically, and activates the daemon. Headless setups can still configure manually with `plex2jellyfin config init` + `config test` — see the [package walkthrough](https://nomadcxx.github.io/plex2jellyfin/docs/getting-started/packages/).

The services run as root (the `[permissions]` chown feature needs `CAP_CHOWN`) and locate your config through `SUDO_USER`. The Quick Install script generates units with this set automatically; package installs set it once via the drop-in above.

### Build from Source

Requires Go 1.24+:

```bash
git clone https://github.com/Nomadcxx/plex2jellyfin.git
cd plex2jellyfin
go build -o installer ./cmd/installer
sudo ./installer
```

### Jellyfin Plugin — install this too

The companion plugin ([Nomadcxx/plex2jellyfin-plugin](https://github.com/Nomadcxx/plex2jellyfin-plugin))
is **not optional** if you want the feedback loop: it forwards
item-added/updated/removed and playback events from Jellyfin back to
plex2jellyfin, which is how organized files get confirmed against real
Jellyfin items (and how orphan detection works). Without it,
plex2jellyfin can move files but never sees whether Jellyfin actually
recognized them.

The setup wizard installs and configures it automatically when you
connect Jellyfin (or run `plex2jellyfin plugin install` on an existing
setup). Manual build instructions live in the
[plugin repository](https://github.com/Nomadcxx/plex2jellyfin-plugin).

## What It Does

Plex papers over messy release names with fuzzy matching; Jellyfin takes your folders at face value. Point it at a Plex-era library and shows split into duplicate entries, seasons land under "Season Unknown", and movies show up titled `1080p.BluRay.x265`.

**Migrate.** Point the CLI at the library Plex left behind:

```bash
plex2jellyfin scan                       # index everything into a local SQLite db
plex2jellyfin status                     # see what you have and what's broken
plex2jellyfin duplicates generate        # find the same content stored twice
plex2jellyfin duplicates dry-run         # preview which copies would go
plex2jellyfin duplicates execute         # keep the best copy, delete the rest
plex2jellyfin consolidate generate       # find series scattered across drives
plex2jellyfin consolidate execute        # merge each series onto one drive
plex2jellyfin audit --generate           # AI rename proposals for the stragglers
plex2jellyfin audit --execute            # apply approved fixes
```

**Guard.** After migration, `plex2jellyfin-daemon` watches your download directories: it parses `Show.Name.S01E01.1080p.WEB-DL.x264-RARBG.mkv`, renames it to `TV Shows/Show Name (2019)/Season 01/Show Name (2019) S01E01.mkv`, moves it to the right drive, and notifies Jellyfin. A convergence loop re-checks the library on a schedule. Ambiguous filenames can go to an optional local LLM (Ollama) behind a confidence threshold; the regex parser handles the bulk without it.

**Out of scope:** Plex server metadata. User accounts, watch states, ratings, and playlists stay behind. This tool migrates the files.

## CLI

`plex2jellyfin --help` shows `scan`, `status`, `duplicates`, `consolidate`, `config`, and `version`. The hidden `audit` command sends library kind (Movies vs TV), folder path, and current parse to your configured LLM; see [`docs/ai-context.md`](docs/ai-context.md).

<details>
<summary><b>Advanced commands</b> (hidden from root help)</summary>

```bash
plex2jellyfin organize /downloads/file.mkv   # Organize a single file
plex2jellyfin organize-folder /downloads/X   # Organize a directory tree
plex2jellyfin watch /downloads               # Foreground watcher
plex2jellyfin validate <path>                # Check library against Jellyfin naming rules
plex2jellyfin cleanup {cruft|empty}          # Remove cruft files or empty dirs
plex2jellyfin monitor                        # Tail daemon activity log
plex2jellyfin daemon {status|reload|stop}    # Control the running daemon
plex2jellyfin repair series-dedupe           # Repair duplicate series rows
plex2jellyfin postmortem collect --since 96h # Generate evidence bundle for review
plex2jellyfin sonarr ...                     # Sonarr integration commands
plex2jellyfin radarr ...                     # Radarr integration commands
plex2jellyfin health                         # Verify *arr setup is compatible
plex2jellyfin orphans                        # Detect / remediate orphaned Jellyfin episodes
```
</details>

## Architecture

| Binary | Role |
|---|---|
| `plex2jellyfin` | CLI for migration: scan, audit, duplicates, consolidation. The primary interface. |
| `plex2jellyfin-daemon` | Watches download dirs, runs the periodic library scan, executes the housekeeping queue, exposes a Unix-domain control socket. |
| `plex2jellyfin-web` | HTTP server on `:5522` hosting the embedded dashboard. **Work in progress**; the CLI and daemon carry the core workflow. |

```mermaid
flowchart TB
    A[Sonarr / Radarr] -->|downloads| B["Download Client
    (SABnzbd, NZBGet, qBittorrent, ...)"]
    B -->|drops file| C[Watch Directory]
    C -->|inotify| D[plex2jellyfin-daemon]
    D -->|rename + move| E[Jellyfin Library]
    D -->|state + audit| DB[(SQLite media.db)]
    D -.->|low confidence| O[Ollama]
    O -.->|suggested rename| D
    D <-->|control.sock| W[plex2jellyfin-web :5522]
    W <-->|browser| U[You]
    D -->|periodic| H[Housekeeping queue]
    H -->|consolidate / dedupe / fix-naming| E
    J[Plex2Jellyfin plugin in Jellyfin] -->|item + playback webhooks| W
    E -.->|scanned by| J
```

More detail in [`docs/architecture.md`](docs/architecture.md).

## Naming Rules

**Movies:** `Movies/Movie Name (YYYY)/Movie Name (YYYY).ext`

**TV Shows:** `TV Shows/Show Name (Year)/Season 01/Show Name (Year) S01E01.ext`

The parser strips release-group noise (`1080p`, `x264`, `WEB-DL`, `RARBG`, `-YTS`, etc.) and pulls resolution, source, and HDR from the parent directory when the filename lacks them, so quality grouping works on legacy libraries.

## Configuration

Config lives at `~/.config/plex2jellyfin/config.toml`. Annotated template: [`config.toml.example`](config.toml.example). See the [configuration reference](https://nomadcxx.github.io/plex2jellyfin/docs/reference/configuration/).

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

[ai]
enabled              = true
ollama_endpoint      = "http://localhost:11434"
model                = "minimax-m2.5:cloud"
confidence_threshold = 0.8
```

<details>
<summary><b>Sonarr / Radarr</b></summary>

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
</details>

<details>
<summary><b>Jellyfin path mappings</b> (container/host mount differences)</summary>

When Jellyfin runs in a container with bind mounts, configure path mappings so the post-organize feedback loop can correlate Jellyfin items with daemon paths:

```toml
[jellyfin]
enabled        = true
url            = "http://localhost:8096"
api_key        = "..."
webhook_secret = "..."

[[jellyfin.path_mappings]]
jellyfin = "/tv"
daemon   = "/mnt/storage1/TVSHOWS"
```

Without these, the sweeper marks parse-decision rows for organized files as FAIL.
</details>

<details>
<summary><b>File permissions</b> (bare-metal installs)</summary>

If Jellyfin runs as a different user, set ownership on moved files:

```toml
[permissions]
user      = "jellyfin"
group     = "jellyfin"
file_mode = "0644"
dir_mode  = "0755"
```

`plex2jellyfin-daemon` needs root to chown; the bundled systemd unit drops to `CAP_CHOWN`, `CAP_FOWNER`, `CAP_DAC_OVERRIDE`. In Docker, use `PUID`/`PGID` instead.
</details>

## Services

```bash
systemctl status plex2jellyfin-daemon                    # daemon
systemctl status plex2jellyfin-web                       # web UI on :5522
systemctl --user status plex2jellyfin-postmortem.timer  # scheduled evidence collection
journalctl -u plex2jellyfin-daemon -f
```

The web UI talks to the daemon over a Unix-domain socket; no TCP between them. The postmortem timer collects parse decisions, repair events, and suspicious items into an evidence bundle at `~/.config/plex2jellyfin/reports/latest/` every 4 days. Review it yourself or hand `agent-prompt.md` to an LLM.

## Building

```bash
make                                     # frontend + all binaries into bin/
go build -o bin/plex2jellyfin ./cmd/plex2jellyfin
cd web && npm run build                  # rebuild dashboard (embedded into plex2jellyfin-web)
```

## License

GPL-3.0-or-later
