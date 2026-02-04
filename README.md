<div align="center">
  <img src="assets/jellywatch-header.png" alt="JellyWatch" />
  <p><em>Because Sonarr and Radarr can't be trusted with naming conventions</em></p>
</div>
THIS IS WIP
---

## What It Does

Watches your download directories. Renames files to Jellyfin's standards. Moves them to the right place. Optionally asks a local AI when it's not sure what something is.

```bash
curl -sSL https://raw.githubusercontent.com/Nomadcxx/jellywatch/main/install.sh | sudo bash
```

## The Problem

Your *arr stack downloads `Movie.Name.2024.1080p.WEB-DL.x264-RARBG.mkv`. Jellyfin wants `Movie Name (2024)/Movie Name (2024).mkv`. This fixes that.

## How It Works

```mermaid
flowchart TB
    subgraph Input[" "]
        A[Sonarr/Radarr] -->|sends| B[Download]
    end
    
    B -->|drops| C[Watch Dir]
    C -->|detects| D[JellyWatch]
    
    D -->|indexes| DB[(DB)]
    DB -->|tells| D
    
    D -->|rename & move| E[Library]
    E -->|serves| F[Jellyfin]
    
    D -.->|queries| A
    
    G[scan] -->|updates| DB
    H[audit] -->|fixes| DB
    I[duplicates] -->|finds in| DB
    J[consolidate] -->|finds in| DB
    DB -.->|operates on| E
```

See [docs/architecture.md](docs/architecture.md) for the detailed flow.

## Quick Commands

```bash
jellywatch scan                    # Index your library
jellywatch watch /downloads        # Watch for new files
jellywatch organize /library       # Fix existing files

jellywatch migrate                 # Sync Sonarr/Radarr paths with database
jellywatch migrate --dry-run       # Preview sync changes

jellywatch audit generate          # Find low-confidence parses
jellywatch audit dry-run           # Preview AI suggestions
jellywatch audit execute           # Apply fixes
```

### AI Context

The audit command uses local AI (Ollama) to analyze low-confidence files.
AI receives contextual information including:

- Library type (Movies vs TV Shows) - prevents misclassifying shows as movies
- Folder path - helps identify correct series from directory structure
- Existing metadata - current parse and confidence score as hints

See [docs/ai-context.md](docs/ai-context.md) for details.

```bash
jellywatch duplicates generate     # Find duplicates
jellywatch duplicates execute      # Keep the best, delete the rest

jellywatch consolidate generate    # Find scattered series
jellywatch consolidate execute     # Bring them home
```

## Naming Rules

**Movies:** `Movies/Movie Name (YYYY)/Movie Name (YYYY).ext`

**TV Shows:** `TV Shows/Show Name (Year)/Season 01/Show Name (Year) S01E01.ext`

All the release group junk gets stripped: `1080p`, `x264`, `WEB-DL`, `RARBG`, `-YTS`, the works.

## Configuration

Lives at `~/.config/jellywatch/config.toml`

```toml
[watch]
movies = ["/downloads/movies"]
tv = ["/downloads/tv"]

[libraries]
movies = ["/media/Movies"]
tv = ["/media/TV Shows"]

[ai]
enabled = true
ollama_endpoint = "http://localhost:11434"
model = "llama3.1"
confidence_threshold = 0.8

[options]
delete_source = true    # Remove originals after moving
```

### Integrations

Works with Sonarr and Radarr for metadata. Point to their APIs:

```toml
[sonarr]
enabled = true
url = "http://localhost:8989"
api_key = "your-key"

[radarr]
enabled = true
url = "http://localhost:7878"
api_key = "your-key"
```

### Permissions

If Jellyfin runs as a different user:

```toml
[permissions]
user = "jellyfin"
group = "jellyfin"
file_mode = "0644"
dir_mode = "0755"
```

**IMPORTANT**: The jellywatchd daemon must run as root to set file ownership. The systemd service is configured to run as root with minimal capabilities (CAP_CHOWN, CAP_FOWNER, CAP_DAC_OVERRIDE) for security.

If you see permission errors in the logs, verify the daemon is running as root:
```bash
ps aux | grep jellywatchd
# Should show "root" as the user
```

If running as a non-root user, file ownership will fail and you'll see errors like:
```
chown failed (daemon not running as root): target uid=XXX gid=XXX
```

To fix: Update the systemd service to run as root:
```bash
sudo systemctl stop jellywatchd
sudo nano /etc/systemd/system/jellywatchd.service
# Change User=<username> to User=root
sudo systemctl daemon-reload
sudo systemctl start jellywatchd
```

## Daemon

Runs as a systemd service. The installer sets this up.

```bash
systemctl status jellywatchd
journalctl -u jellywatchd -f
```

## Manual Install

```bash
git clone https://github.com/Nomadcxx/jellywatch.git
cd jellywatch
go build -o installer ./cmd/installer
sudo ./installer
```

Requires Go 1.21+.

## License

GPL-3.0 or later
