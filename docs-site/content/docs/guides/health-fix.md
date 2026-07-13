---
title: What health --fix Changes
description: The exact Sonarr/Radarr settings plex2jellyfin health --fix can modify, and why.
---

`plex2jellyfin health` checks that your Sonarr/Radarr configuration is
compatible with the daemon owning file imports. `plex2jellyfin health --fix`
is a **dry-run by default**: it prints what would change and does not modify
*arr settings until you pass `--dry-run=false`.

```bash
plex2jellyfin health --fix                  # preview only (default)
plex2jellyfin health --fix --dry-run=false  # apply the changes below
```

## Completed Download Handling → disabled (critical)

- Sonarr/Radarr: `enableCompletedDownloadHandling` → `false`

The daemon owns the move from the download directory into the library. If
the *arr also imports completed downloads, the two race: whoever gets there
first moves the file and the loser logs failures — or worse, you get a copy
in both places. With CDH off, Sonarr/Radarr still search, grab, and monitor
quality; plex2jellyfin does the import and then notifies them
(`notify_on_import`).

## Rename support → enabled (warning)

- Sonarr: `renameEpisodes` → `true`
- Radarr: `renameMovies` → `true`

The daemon can ask the *arrs to rename files they already manage in the
library (repair flows use `RenameSeries`/`RenameFiles`). Those API commands
are no-ops unless rename support is on, so `--fix` enables it. It does not
change your naming *format* — only whether renames are permitted.

## Nothing else

`health --fix` touches no other settings. Root folders, quality profiles,
indexers, and download clients are only ever *read* for validation.
