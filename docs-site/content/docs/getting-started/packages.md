---
title: Debian / Fedora Packages
description: Install the deb/rpm packages and configure them so the daemon actually runs.
---

Download the `.deb` or `.rpm` from [GitHub Releases](https://github.com/Nomadcxx/plex2jellyfin/releases/latest):

```bash
sudo apt install ./plex2jellyfin_*_amd64.deb      # Debian/Ubuntu
sudo dnf install ./plex2jellyfin-*.x86_64.rpm     # Fedora
```

The package installs the three binaries, systemd units, and an example
config — **no configuration**. The daemon exits (and systemd restart-loops
it) until a config with watch directories exists, so configure before
enabling anything.

## 1. Create and edit the config

As your normal user (not root):

```bash
plex2jellyfin config init
$EDITOR ~/.config/plex2jellyfin/config.toml
```

Set at minimum `[watch]` (where downloads arrive), `[libraries]` (where
organized media should go), and any *arr / Jellyfin API keys. The
[configuration reference](/docs/reference/configuration) covers every key.

## 2. Verify

```bash
plex2jellyfin config test
```

This checks paths exist and connections authenticate.

## 3. Point the services at your user's config

The services run as root (the `[permissions]` chown feature needs
`CAP_CHOWN`) and locate your config through the `SUDO_USER` environment
variable. Package installs must set it once per service:

```bash
sudo systemctl edit plex2jellyfin-daemon
sudo systemctl edit plex2jellyfin-web
```

Add to each drop-in:

```ini
[Service]
Environment=SUDO_USER=<your username>
```

The Quick Install script generates units with this baked in; packages
cannot, because they don't know your username at build time.

## 4. Enable

```bash
sudo systemctl enable --now plex2jellyfin-daemon plex2jellyfin-web
journalctl -u plex2jellyfin-daemon -f     # watch it come up
```

The web UI on `:5522` will ask you to create an admin password on first
visit.
