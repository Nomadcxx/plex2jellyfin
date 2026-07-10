# Plex2Jellyfin Jellyfin Plugin

Companion Jellyfin plugin for Plex2Jellyfin.

It provides:
- Custom `/plex2jellyfin/*` endpoints for Plex2Jellyfin integrations
- Event forwarding from Jellyfin to Plex2Jellyfin webhook endpoint

## Compatibility

- Jellyfin: `10.11.x` (targetAbi 10.11.0.0; 10.11 moved to .NET 9, so the
  plugin targets `net9.0` — it will not load on 10.10 or older)
- .NET SDK: `9.0+`

## Build

From repo root:

```bash
./plugin/build.sh
```

Or from `plugin/`:

```bash
./build.sh
```

Build output:
- `plugin/artifacts/plex2jellyfin_1.0.0.zip`
- `plugin/artifacts/package/manifest.json` with computed `checksum`

## Install (Local Testing)

1. Build artifact:

```bash
./plugin/build.sh
```

2. Install plugin files into Jellyfin plugin directory:

```bash
./plugin/install.sh ~/.local/share/jellyfin/plugins/Plex2Jellyfin
```

3. Restart Jellyfin:

```bash
sudo systemctl restart jellyfin
```

## Configure In Jellyfin

Open:
- `Dashboard -> Plugins -> Plex2Jellyfin`

Set:
- `Plex2Jellyfin URL` (the Plex2Jellyfin API/webhook base you are running)
- `Shared Secret` (must match Plex2Jellyfin webhook secret)
- Enable event forwarding options you want

## Webhook Contract

Event forwarder target:
- `POST /api/v1/webhooks/jellyfin`

Required auth header:
- `X-Plex2Jellyfin-Webhook-Secret: <secret>`

## Quick Test Checklist

1. Plugin appears in Jellyfin Dashboard.
2. `/plex2jellyfin/health` returns 200 in Jellyfin API context.
3. Trigger a playback start/stop and confirm Plex2Jellyfin receives webhook events.
4. Validate Plex2Jellyfin logs do not show webhook auth failures.

## Troubleshooting

- If build fails, run:
  - `dotnet --version`
  - `dotnet build -c Release plugin/Plex2Jellyfin.Plugin.csproj`
- If plugin does not load, verify:
  - `manifest.json` exists beside `Plex2Jellyfin.Plugin.dll`
  - `targetAbi` in manifest matches Jellyfin server ABI (`10.10.0.0`)
- If webhooks fail with 401, confirm shared secret matches exactly.
