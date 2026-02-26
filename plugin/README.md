# JellyWatch Jellyfin Plugin

Companion Jellyfin plugin for JellyWatch.

It provides:
- Custom `/jellywatch/*` endpoints for JellyWatch integrations
- Event forwarding from Jellyfin to JellyWatch webhook endpoint

## Compatibility

- Jellyfin: `10.10.x`
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
- `plugin/artifacts/jellywatch_1.0.0.zip`
- `plugin/artifacts/package/manifest.json` with computed `checksum`

## Install (Local Testing)

1. Build artifact:

```bash
./plugin/build.sh
```

2. Install plugin files into Jellyfin plugin directory:

```bash
./plugin/install.sh ~/.local/share/jellyfin/plugins/JellyWatch
```

3. Restart Jellyfin:

```bash
sudo systemctl restart jellyfin
```

## Configure In Jellyfin

Open:
- `Dashboard -> Plugins -> JellyWatch`

Set:
- `JellyWatch URL` (the JellyWatch API/webhook base you are running)
- `Shared Secret` (must match JellyWatch webhook secret)
- Enable event forwarding options you want

## Webhook Contract

Event forwarder target:
- `POST /api/v1/webhooks/jellyfin`

Required auth header:
- `X-Jellywatch-Webhook-Secret: <secret>`

## Quick Test Checklist

1. Plugin appears in Jellyfin Dashboard.
2. `/jellywatch/health` returns 200 in Jellyfin API context.
3. Trigger a playback start/stop and confirm JellyWatch receives webhook events.
4. Validate JellyWatch logs do not show webhook auth failures.

## Troubleshooting

- If build fails, run:
  - `dotnet --version`
  - `dotnet build -c Release plugin/JellyWatch.Plugin.csproj`
- If plugin does not load, verify:
  - `manifest.json` exists beside `JellyWatch.Plugin.dll`
  - `targetAbi` in manifest matches Jellyfin server ABI (`10.10.0.0`)
- If webhooks fail with 401, confirm shared secret matches exactly.
