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
config — **no configuration yet**. Prefer the [web setup wizard](/docs/getting-started/setup-wizards#web-wizard):
it collects paths and connections, validates them, writes the config,
enables the daemon for boot, and runs the initial library index. If
Jellyfin uses Docker bind mounts that differ from your library roots,
configure [path mappings](/docs/getting-started/path-mappings) on the
Services step (or in `config.toml`) so the feedback loop can confirm organizes.

## 1. Point the services at your user's config

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

## 2. Enable

```bash
sudo systemctl enable --now plex2jellyfin-daemon plex2jellyfin-web
```

The daemon restart-loops until a config exists — that's expected; it
settles as soon as the wizard writes one.

## 3. Finish setup in the browser

Open `http://<host>:5522/`:

1. create the admin password (first visit forces this);
2. the **setup wizard** collects watch and library paths, optional
   Sonarr/Radarr/Jellyfin connections, optional Ollama, and runtime
   behavior — every path is validated before you can continue;
3. **Review & activate** writes `~/.config/plex2jellyfin/config.toml`
   atomically and starts the daemon.

```bash
journalctl -u plex2jellyfin-daemon -f     # watch it come up
```

## Manual alternative (headless / older releases)

The wizard is optional. The CLI path still works and is the only path on
releases older than the wizard:

```bash
plex2jellyfin config init
$EDITOR ~/.config/plex2jellyfin/config.toml   # [watch], [libraries], API keys
plex2jellyfin config test
sudo systemctl restart plex2jellyfin-daemon plex2jellyfin-web
```

The [configuration reference](/docs/reference/configuration) covers every
key.
