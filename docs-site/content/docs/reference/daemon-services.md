---
title: Daemon & Services
description: Configure the daemon, web service, and postmortem timer.
---

The installer and deb/rpm packages register the daemon and web systemd units. The repository also includes a user-level postmortem timer for the author's deployment; packages do not install it. On Docker, the entrypoint runs the daemon and web server as background processes instead; see [Docker](/docs/getting-started/docker).

Setup wizards **enable the daemon for boot** (`systemctl enable --now`) when the unit exists. Fresh-build **web** install enables only `plex2jellyfin-web` until the wizard runs; fresh-build **CLI** install leaves units installed but stopped until `plex2jellyfin setup`.

```bash
systemctl status plex2jellyfin-daemon                   # daemon
systemctl status plex2jellyfin-web                      # web UI on :5522
systemctl --user status plex2jellyfin-postmortem.timer  # scheduled evidence collection
journalctl -u plex2jellyfin-daemon -f
```

## plex2jellyfin-daemon.service

```ini
[Unit]
Description=Plex2Jellyfin Media File Watcher
After=network.target

[Service]
Type=simple
User=root
Group=root
ExecStart=/usr/bin/plex2jellyfin-daemon
Restart=on-failure
RestartSec=10
PrivateTmp=true

CapabilityBoundingSet=CAP_CHOWN CAP_FOWNER CAP_DAC_OVERRIDE
AmbientCapabilities=CAP_CHOWN CAP_FOWNER CAP_DAC_OVERRIDE

[Install]
WantedBy=multi-user.target
```

Runs as **root**, but with `CapabilityBoundingSet`/`AmbientCapabilities` restricted to only `CAP_CHOWN`, `CAP_FOWNER`, and `CAP_DAC_OVERRIDE` — the minimum needed to chown moved files to a different user when `[permissions]` is configured (see [Configuration](/docs/reference/configuration#permissions)). `NoNewPrivileges` is deliberately left disabled because the daemon needs to retain the ability to change file ownership after moving a file.

Watches configured `[watch]` directories, parses and renames incoming files, runs the periodic convergence scan (`scan_frequency` in `[daemon]`), and executes the housekeeping queue. Exposes a Unix-domain control socket used by `plex2jellyfin daemon {status|reload|stop}` and by `plex2jellyfin-web`.

## plex2jellyfin-web.service

```ini
[Unit]
Description=Plex2Jellyfin Web UI Server
Documentation=https://github.com/Nomadcxx/plex2jellyfin
After=network.target plex2jellyfin-daemon.service
Wants=plex2jellyfin-daemon.service

[Service]
Type=simple
User=root
Group=root
ExecStart=/usr/bin/plex2jellyfin-web --host 0.0.0.0 --port 5522
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=plex2jellyfin-web

NoNewPrivileges=true
ProtectSystem=full
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

`plex2jellyfin-web` depends on `plex2jellyfin-daemon` and talks to it exclusively over the Unix-domain control socket — no TCP between them. It hosts the embedded dashboard and proxies API calls to the daemon.

`ProtectHome` is deliberately left unset: the web server needs read/write access to the user's config directory (`config.toml`, lock files, `media.db` live under `~/.config/plex2jellyfin`), and web-triggered library maintenance actions (e.g. consolidation) need write access to non-system media mounts under normal filesystem permissions rather than a hardcoded allow-list.

> **Work in progress**
>
> The dashboard trails the CLI and daemon in maturity. Prefer the CLI for anything destructive; treat the web UI as a status/review surface first.

## Postmortem timer

```bash
systemctl --user status plex2jellyfin-postmortem.timer
```

```ini
# plex2jellyfin-postmortem.timer
[Timer]
OnBootSec=20m
OnUnitActiveSec=96h
Persistent=true
Unit=plex2jellyfin-postmortem.service
```

```ini
# plex2jellyfin-postmortem.service
[Service]
Type=oneshot
ExecStart=/usr/local/bin/plex2jellyfin postmortem collect --since 96h
```

A **user** unit (not system-wide), installed under `systemd/user/`. Runs every 4 days (`OnUnitActiveSec=96h`), starting 20 minutes after boot if the previous run was missed (`Persistent=true`). Each run collects parse decisions, repair events, housekeeping state, and suspicious items into an evidence bundle at `~/.config/plex2jellyfin/reports/latest/`.

Review the bundle yourself, or hand `agent-prompt.md` from the bundle to an LLM for a summarized health check. See `plex2jellyfin postmortem collect --help` and the [CLI Reference](/docs/reference/cli#postmortem) for manual invocation.

## Managing services

```bash
# start/stop/restart the underlying systemd units
sudo systemctl enable --now plex2jellyfin-daemon plex2jellyfin-web
sudo systemctl restart plex2jellyfin-daemon
sudo systemctl stop plex2jellyfin-web plex2jellyfin-daemon

# talk to a running daemon over its control socket
plex2jellyfin daemon status
plex2jellyfin daemon reload   # reload config without restarting the process
plex2jellyfin daemon stop     # stop via the control socket instead of systemctl

# tail daemon activity without journalctl
plex2jellyfin monitor
```

`daemon reload` is the preferred way to pick up a `config.toml` change without dropping in-flight file watches — a full `systemctl restart` also works but re-establishes the watcher and control socket from scratch.

## Docker equivalent

There's no systemd inside the container. The image's `entrypoint.sh` starts `plex2jellyfin-daemon` and `plex2jellyfin-web` as background processes under `tini`, forwards `TERM`/`INT` to both on `docker stop`, and waits on the web process to determine the container's exit status. See the [Docker page](/docs/getting-started/docker) for volumes, PUID/PGID, and permission handling in that environment — the postmortem timer isn't run automatically in the container; invoke `plex2jellyfin postmortem collect` manually or via `docker exec` from a host-side cron job if you want it there.
