<div align="center">
  <img src="assets/jellywatch-header.png" alt="JellyWatch" />
  <p><em>Media file watcher and organizer for Jellyfin libraries</em></p>
  <p>Automatically monitors download directories and organizes files according to Jellyfin naming standards</p>
</div>

---

## Quick Install

```bash
curl -sSL https://raw.githubusercontent.com/Nomadcxx/jellywatch/main/install.sh | sudo bash
```

## Features

- **File System Monitoring**: Watches download directories for new media files
- **Automatic Organization**: Renames and organizes files according to Jellyfin standards
- **AI-Powered Parsing**: Uses local AI (Ollama) to parse ambiguous filenames when confidence is low
- **Confidence Scoring**: Tracks parse confidence for each file and flags uncertain parses for review
- **Audit Command**: Review and fix low-confidence parses with AI suggestions
- **Duplicate Detection**: Identifies duplicate files before moving
- **Consolidation**: Consolidates scattered media files into single locations
- **Compliance Validation**: Ensures all files follow Jellyfin naming conventions
- **Daemon Support**: Runs as a background service with systemd integration
- **Dry Run Mode**: Preview changes before applying them
- **File Permissions**: Automatically set ownership and permissions for Jellyfin access

## Naming Conventions

### Movies
```
Movies/Movie Name (YYYY)/Movie Name (YYYY).ext
```

### TV Shows
```
TV Shows/Show Name (Year)/Season 01/Show Name (Year) S01E01.ext
```

### Rules
- Year must be in parentheses: `(YYYY)`
- Season folders must be padded: `Season 01`, `Season 02`
- No special characters: `< > : " / \ | ? *`
- Release group markers removed: `1080p`, `x264`, `WEB-DL`, etc.
- Episode format: `SXXEYY` with padded numbers

## Installation

### Quick Install (Recommended)

```bash
curl -sSL https://raw.githubusercontent.com/Nomadcxx/jellywatch/main/install.sh | sudo bash
```

Or download and run manually:

```bash
wget https://raw.githubusercontent.com/Nomadcxx/jellywatch/main/install.sh
chmod +x install.sh
sudo ./install.sh
```

The installer will:
- Check for Go installation (1.21+ required)
- Clone the repository
- Build the interactive installer
- Guide you through configuration
- Install binaries to `/usr/local/bin`
- Set up systemd service
- Create config at `~/.config/jellywatch/config.toml`

### Manual Installation

Requirements:
- Go 1.21 or later
- Root privileges (for systemd and `/usr/local/bin`)

```bash
git clone https://github.com/Nomadcxx/jellywatch.git
cd jellywatch
go build -o installer ./cmd/installer
sudo ./installer
```

### Uninstall

```bash
sudo jellywatch-installer
# Select "Uninstall" option
```

Or run the installer again and choose uninstall mode.

## Usage

### Interactive Mode
```bash
jellywatch
```

### Watch Directory
```bash
jellywatch watch /path/to/downloads
```

### Organize Existing Files
```bash
jellywatch organize /path/to/library
```

### Scan Library with AI
```bash
jellywatch scan
```

### Audit Low-Confidence Parses
```bash
jellywatch audit --generate    # Find low-confidence files
jellywatch audit --dry-run     # Preview AI suggestions
jellywatch audit --execute     # Apply AI suggestions
```

### Find Duplicates
```bash
jellywatch duplicates --generate
jellywatch duplicates --dry-run
jellywatch duplicates --execute
```

### Consolidate Scattered Files
```bash
jellywatch consolidate --generate
jellywatch consolidate --dry-run
jellywatch consolidate --execute
```

### Validate Compliance
```bash
jellywatch validate /path/to/library
```

### Dry Run (Preview Changes)
```bash
jellywatch organize --dry-run /path/to/library
```

## Configuration

Configuration file location: `~/.config/jellywatch/config.toml`

```toml
[watch]
movies = ["/path/to/downloads/movies"]
tv = ["/path/to/downloads/tv"]

[libraries]
movies = ["/path/to/jellyfin/Movies"]
tv = ["/path/to/jellyfin/TV Shows"]

[daemon]
enabled = true
scan_frequency = "5m"

[options]
dry_run = false
verify_checksums = false
delete_source = true

[ai]
enabled = true
ollama_endpoint = "http://localhost:11434"
model = "llama3.1"
confidence_threshold = 0.8
auto_trigger_threshold = 0.6
cache_enabled = true
```

### File Permissions

If Jellyfin runs as a different user than your download client, configure permissions to ensure Jellyfin can access transferred files:

```toml
[permissions]
user = "jellyfin"      # Username or numeric UID
group = "jellyfin"     # Group name or numeric GID
file_mode = "0644"     # File permissions (rw-r--r--)
dir_mode = "0755"      # Directory permissions (rwxr-xr-x)
```

**Notes:**
- Leave `user`/`group` empty to preserve source file ownership
- The daemon must run as root (or have `CAP_CHOWN`) to change ownership
- Non-root processes will silently skip ownership changes but still apply mode changes
- Uses rsync's `--chown` and `--chmod` flags for efficient permission handling

## Daemon Service

The installer automatically sets up the systemd service. To manage it:

Check status:

```bash
systemctl status jellywatchd
journalctl -u jellywatchd -f
```

Start/stop/restart:

```bash
sudo systemctl start jellywatchd
sudo systemctl stop jellywatchd
sudo systemctl restart jellywatchd
```

Enable/disable auto-start:

```bash
sudo systemctl enable jellywatchd
sudo systemctl disable jellywatchd
```

## Development

```bash
# Run tests
go test ./...

# Run with race detector
go run -race ./cmd/jellywatch

# Build binaries
go build ./cmd/jellywatch
go build ./cmd/jellywatchd
```

## License

MIT
