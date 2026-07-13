---
title: Path mappings
description: Translate Jellyfin container paths to Plex2Jellyfin host paths so the feedback loop can confirm organizes.
---

Path mappings (`[[jellyfin.path_mappings]]`) tell Plex2Jellyfin how to translate paths Jellyfin reports into the paths the daemon uses on disk.

## Why they matter

Organizes write files under your `[libraries]` roots (host view). The companion plugin posts webhooks with Jellyfin’s view of the same files (often container-internal roots like `/movies1` or `/tv1`).

The daemon’s path translator (`JellyfinToDaemon`, longest-prefix match) rewrites those webhook paths before correlating them with `parse_decisions`. Without a matching mapping:

- Organizes still succeed.
- `jellyfin_item_id` on decisions stays empty.
- Labeler PASS / DRIFT / FAIL never attaches (no provider IDs from confirmation).

You need **both** the companion plugin **and** correct path mappings for the feedback loop.

## When you need them

Add mappings when Jellyfin Locations differ from `[libraries]` — the usual case when Jellyfin runs in Docker with bind mounts.

Skip only when Jellyfin and Plex2Jellyfin already share the same path layout (same strings on both sides).

## How to configure

One row per library root:

```toml
[[jellyfin.path_mappings]]
jellyfin = "/movies1"
daemon = "/mnt/STORAGE1/MOVIES"

[[jellyfin.path_mappings]]
jellyfin = "/tv1"
daemon = "/mnt/STORAGE1/TVSHOWS"
```

- `jellyfin` — prefix as Jellyfin reports it (container path).
- `daemon` — same root as Plex2Jellyfin sees it (host / `[libraries]` path).
- Nested mounts: longest matching prefix wins.

Wizards (web, CLI `plex2jellyfin setup`, TUI installer) detect unmapped Jellyfin virtual-folder roots after a successful Jellyfin Test and prompt you to add rows until the list is empty.

**CLI exception:** if listing VirtualFolders fails after connect, the wizard warns and may skip the mapping loop — add mappings manually and re-test. Web and TUI still block Continue while the last successful Test reported unmapped roots.

## How to verify

1. **Setup Test Jellyfin** — unmapped list should be empty; web Continue is blocked while any remain.
2. **Daemon log** — when at least one mapping is configured, look for `"Jellyfin path mappings configured"`. Uncovered roots still produce a startup warn via the same detection used in wizards.
3. **Activity / decisions** — after an organize + Jellyfin ingest, webhook paths should resolve under `[libraries]` and decisions should gain `jellyfin_item_id` / labels instead of staying unmatched on `/moviesN`.

## Related

- [Setup wizards](/docs/getting-started/setup-wizards)
- [Jellyfin companion plugin](/docs/getting-started/jellyfin-plugin)
- [Docker](/docs/getting-started/docker)
- [Configuration → Jellyfin path mappings](/docs/reference/configuration#jellyfin-path-mappings)
- [Architecture → first organize confirmation](/docs/reference/architecture#first-organize--confirmation--labels)
- [Troubleshooting](/docs/troubleshooting#jellyfin-path-mappings-and-parse-decisions-marked-fail)
