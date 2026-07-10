---
title: Jellyfin Plugin
description: The companion plugin is required for the feedback loop - install it into Jellyfin.
---

Plex2Jellyfin ships a companion Jellyfin plugin, and it is **not optional**
if you want the full pipeline: the plugin forwards item-added / updated /
removed and playback events from Jellyfin back to plex2jellyfin. That's how
an organized file gets *confirmed* against a real Jellyfin item (with its
IMDB/TMDB/TVDB IDs), how the Trace view shows "matched item …", and how
orphan detection works. Without it, plex2jellyfin can move files but never
learns whether Jellyfin recognized them.

## Compatibility

- Jellyfin **10.11.x** (`targetAbi 10.11.0.0`)
- Jellyfin 10.11 moved to .NET 9, so the plugin targets `net9.0` and will
  not load on 10.10 or older. Building it needs the .NET 9 SDK (or newer).

## Build and install

From the plex2jellyfin repo root:

```bash
./plugin/build.sh
./plugin/install.sh /var/lib/jellyfin/plugins/Plex2Jellyfin
sudo systemctl restart jellyfin
```

The plugin directory varies by install method — `~/.local/share/jellyfin/plugins/`
for user installs, `/var/lib/jellyfin/plugins/` for distro packages,
`/config/plugins/` inside the official Docker image.

## Configure the webhook secret

The plugin authenticates its events with a shared secret. Set the same
value on both sides:

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
