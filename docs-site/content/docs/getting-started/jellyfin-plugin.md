---
title: Jellyfin Plugin
description: The companion plugin is required for the feedback loop - install it into Jellyfin.
---

Plex2Jellyfin has a companion Jellyfin plugin, developed in its own
repository at [Nomadcxx/plex2jellyfin-plugin](https://github.com/Nomadcxx/plex2jellyfin-plugin),
and it is **not optional**
if you want the full pipeline: the plugin forwards item-added / updated /
removed and playback events from Jellyfin back to plex2jellyfin. That's how
an organized file gets *confirmed* against a real Jellyfin item (with its
IMDB/TMDB/TVDB IDs), how the Trace view shows "matched item …", and how
orphan detection works. Without it, plex2jellyfin can move files but never
learns whether Jellyfin recognized them.

If Jellyfin’s library paths differ from the daemon’s `[libraries]` roots
(common with Docker bind mounts), you also need
[path mappings](/docs/getting-started/path-mappings) — the plugin alone is
not enough when `/movies1` never matches `/mnt/.../MOVIES`.

## Compatibility

- Jellyfin **10.11.x** (`targetAbi 10.11.0.0`)
- Jellyfin 10.11 moved to .NET 9, so the plugin targets `net9.0` and will
  not load on 10.10 or older. Building it needs the .NET 9 SDK (or newer).

## Automatic install (recommended)

The setup wizard installs the plugin for you: when you connect Jellyfin
during `plex2jellyfin setup`, the wizard registers the plugin
repository, has Jellyfin download the plugin, asks before restarting
Jellyfin, then pushes the webhook secret and URL into the plugin and
verifies the loop with a signed test event.

On an existing install:

```bash
plex2jellyfin plugin install   # install + configure + verify
plex2jellyfin plugin verify    # re-check the feedback loop any time
plex2jellyfin plugin status    # what Jellyfin reports about the plugin
```

This works for Jellyfin in Docker too — Jellyfin downloads the plugin
itself, so no filesystem access is needed. The one URL you may need to
adjust is the callback URL: it is *Jellyfin's* view of plex2jellyfin,
so never `localhost` when either side runs in a container.

## Manual build and install (fallback)

```bash
git clone https://github.com/Nomadcxx/plex2jellyfin-plugin.git
cd plex2jellyfin-plugin
./build.sh
./install.sh /var/lib/jellyfin/plugins/Plex2Jellyfin
sudo systemctl restart jellyfin
```

The plugin directory varies by install method — `~/.local/share/jellyfin/plugins/`
for user installs, `/var/lib/jellyfin/plugins/` for distro packages,
`/config/plugins/` inside the official Docker image.

## Configure the webhook secret

Only needed for a manual install — the wizard and `plugin install` push
the secret and URL into the plugin automatically. The plugin
authenticates its events with a shared secret. Set the same value on
both sides:

1. In `~/.config/plex2jellyfin/config.toml`:

   ```toml
   [jellyfin]
   enabled        = true
   url            = "http://localhost:8096"
   api_key        = "..."
   webhook_secret = "pick-something-long-and-random"
   ```

2. In Jellyfin → Dashboard → Plugins → Plex2Jellyfin: set the same secret
   and the plex2jellyfin webhook URL (`http://<host>:5522/api/v1/webhooks/jellyfin`).

## Verify it works

Organize any file (or wait for the daemon), then:

```bash
plex2jellyfin trace
```

A working plugin shows `jellyfin   matched item <id> (...)` on recently
organized files. `not confirmed yet` on everything means events aren't
arriving — check the secret and the webhook URL.
