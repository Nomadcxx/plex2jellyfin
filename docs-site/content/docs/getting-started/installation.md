---
title: Installation
description: Install plex2jellyfin via TUI, fresh-build scripts, Docker, AUR, or source.
---

Every install path ships the same binaries:

| Binary | Role |
|---|---|
| `plex2jellyfin` | CLI — setup, scan, duplicates, consolidate, plugin, status |
| `plex2jellyfin-daemon` | Watches download dirs, organizes arrivals, periodic library scan |
| `plex2jellyfin-web` | Dashboard + setup wizard on `:5522` |
| `plex2jellyfin-installer` | Interactive TUI (also built as `installer` from `./cmd/installer`) |

Config lives at `~/.config/plex2jellyfin/config.toml`. Systemd units typically run as root and resolve that path via `SUDO_USER` (fresh-build scripts inject it automatically).

After install, finish with a [setup wizard](/docs/getting-started/setup-wizards) (TUI, CLI, or web). Install the [Jellyfin companion plugin](/docs/getting-started/jellyfin-plugin) for the feedback loop.

## Option A — TUI installer

```bash
curl -sSL https://raw.githubusercontent.com/Nomadcxx/plex2jellyfin/main/install.sh | sudo bash
```

Requires Go 1.24+ and `git`. Clones into a temp dir, builds `./cmd/installer`, and runs the interactive wizard (paths, *arr, AI, permissions, systemd, initial scan). Re-run to update binaries; existing `config.toml` is preserved.

## Option B — Build + CLI setup

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Nomadcxx/plex2jellyfin/main/scripts/fresh-build-install.sh)
plex2jellyfin setup
```

Run as your normal user (not root). Needs Go, git, npm, and sudo. Installs binaries to `/usr/bin`, writes systemd units with `SUDO_USER`, then leaves services stopped until `plex2jellyfin setup` enables them.

## Option C — Build + web setup

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Nomadcxx/plex2jellyfin/main/scripts/fresh-build-install-web.sh)
```

Same build as Option B, then `systemctl enable --now plex2jellyfin-web` and prints the wizard URL (usually `http://127.0.0.1:5522/`). The browser wizard writes config, enables the daemon, and runs the initial library index.

## Option D — Docker

See the [Docker guide](/docs/getting-started/docker).

## Option E — AUR (Arch Linux)

```bash
yay -S plex2jellyfin
# or: paru -S plex2jellyfin
```

Installs binaries and systemd units. Finish with the web or CLI setup wizard afterward.

## Option F — Development (local tree)

```bash
git clone https://github.com/Nomadcxx/plex2jellyfin.git
cd plex2jellyfin
go build -o installer ./cmd/installer
sudo ./installer
```

Or, from an existing checkout:

```bash
PLEX2JELLYFIN_SETUP_MODE=cli bash scripts/fresh-build-from-tree.sh   # then plex2jellyfin setup
PLEX2JELLYFIN_SETUP_MODE=web bash scripts/fresh-build-from-tree.sh   # enable web + print URL
make                                                                   # frontend + all binaries into bin/
```

`plex2jellyfin-web` embeds `embedded/frontend` (built from `web/`). The Makefile and fresh-build scripts order that correctly.

## Requirements

- Linux (amd64 or arm64)
- Go 1.24+ for source / script / TUI builds
- npm for building the embedded web UI (script and `make` paths)
- Root or sudo for systemd unit install and `CAP_CHOWN` on bare metal
- Jellyfin 10.11.x if you use the companion plugin

## Verify

```bash
plex2jellyfin version
systemctl status plex2jellyfin-daemon plex2jellyfin-web
curl -s http://127.0.0.1:5522/api/v1/auth/status
```
