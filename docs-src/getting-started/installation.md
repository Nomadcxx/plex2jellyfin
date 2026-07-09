# Installation

Plex2Jellyfin ships as an interactive installer, a Go build from source, or distro packages (deb/rpm). Docker is covered separately on the [Docker page](docker.md).

Every path installs the same three binaries: `plex2jellyfin` (CLI), `plex2jellyfin-daemon`, and `plex2jellyfin-web`.

## One-liner (recommended)

```bash
curl -sSL https://raw.githubusercontent.com/Nomadcxx/plex2jellyfin/main/install.sh | sudo bash
```

This clones the repo into a temporary directory, builds the interactive installer TUI, and runs it. Requires `go` (1.21+; the project itself targets **Go 1.24+**) and `git` on `PATH`. The installer walks you through:

- Watch paths (where Sonarr/Radarr/download clients drop new files)
- Library paths (your Jellyfin `Movies` / `TV Shows` roots)
- Sonarr / Radarr connection details
- AI (Ollama) configuration
- File permissions (chown behavior — see [Configuration](../reference/configuration.md#permissions))
- systemd service registration

Re-run the installer any time to change settings; it preserves your existing `config.toml`.

## Manual build

```bash
git clone https://github.com/Nomadcxx/plex2jellyfin.git
cd plex2jellyfin
go build -o installer ./cmd/installer
sudo ./installer
```

Or build the binaries directly without the TUI:

```bash
make                                                    # build all binaries into bin/
go build -o bin/plex2jellyfin         ./cmd/plex2jellyfin
go build -o bin/plex2jellyfin-daemon  ./cmd/plex2jellyfin-daemon
go build -o bin/plex2jellyfin-web     ./cmd/plex2jellyfin-web
cd web && npm run build                                 # rebuild dashboard (embedded into plex2jellyfin-web)
```

`plex2jellyfin-web` embeds the compiled Next.js dashboard, so `web/out` must exist before `go build` picks it up (the Makefile handles this ordering).

Run the full test sweep with `./test-all.sh`.

## Distro packages (deb/rpm)

Releases are built with [GoReleaser](https://goreleaser.com) and published to the project's GitHub Releases page as `.deb` and `.rpm` packages (via [nfpm](https://nfpm.goreleaser.com)), alongside plain tarballs for `cli`, `daemon`, `web`, and `installer`.

```bash
# Debian / Ubuntu
sudo dpkg -i plex2jellyfin_<version>_linux_amd64.deb

# Fedora / RHEL
sudo rpm -i plex2jellyfin-<version>.linux.amd64.rpm
```

The package installs:

- `/usr/bin/plex2jellyfin`, `/usr/bin/plex2jellyfin-daemon`, `/usr/bin/plex2jellyfin-web`
- `/usr/lib/systemd/system/plex2jellyfin-daemon.service`
- `/usr/lib/systemd/system/plex2jellyfin-web.service`
- `/usr/share/doc/plex2jellyfin/config.toml.example`

Postinstall prints the next steps and reloads systemd; preremove stops and disables both services. After installing:

```bash
cp /usr/share/doc/plex2jellyfin/config.toml.example ~/.config/plex2jellyfin/config.toml
# edit config.toml for your paths
sudo systemctl enable --now plex2jellyfin-daemon plex2jellyfin-web
```

Packages are `amd64`/`arm64`, Linux only.

## Requirements

- **Go 1.24+** — only needed if building from source; packages and the installer script handle this for you (the installer script itself still needs a system Go to build the TUI).
- **Node.js** (for `npm run build`) — only needed if you're rebuilding the web dashboard from source.
- **systemd** — the installer and packages register systemd units; running without systemd means starting the daemon and web server manually.
- **Ollama** (optional) — only required if you enable `[ai]` in `config.toml` for AI-assisted rename suggestions.

## Next steps

- [Migration Guide](migration-guide.md) — run the one-shot migration workflow against your existing library
- [Configuration](../reference/configuration.md) — full `config.toml` reference
- [Daemon & Services](../reference/daemon-services.md) — systemd units in detail
