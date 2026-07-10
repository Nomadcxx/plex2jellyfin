---
title: SABnzbd Setup
description: Category setup and the bundled deobfuscation post-processing script.
---

Plex2Jellyfin watches the directories SABnzbd finishes downloads into, so
the SABnzbd side is mostly about putting completed jobs in the right place
with a usable filename.

## Categories and folders

Give TV and movies their own categories with separate completed folders,
and point `[watch]` at those folders:

```toml
[watch]
tv     = ["/downloads/complete/tv"]
movies = ["/downloads/complete/movies"]
```

Leave SABnzbd's own sorting/renaming off — the daemon does the renaming,
and double-renaming is how files get lost.

## Obfuscated releases: the bundled deobfuscation script

Some releases arrive with intentionally garbage filenames
(`a8f3c09b1d…mkv`). SABnzbd's built-in deobfuscation catches most of it;
the bundled post-processing script covers a common leftover: jobs whose
*name* carries a clean `SxxEyy` marker while the video file inside is
still hex noise. It only performs low-risk renames — PAR2 hash-confirmed
names, or a single video file when the job name has an explicit episode
marker — so it will never guess.

Install ([`scripts/sabnzbd/plex2jellyfin_deobfuscate.py`](https://github.com/Nomadcxx/plex2jellyfin/blob/main/scripts/sabnzbd/plex2jellyfin_deobfuscate.py)):

```bash
cp scripts/sabnzbd/plex2jellyfin_deobfuscate.py /path/to/sabnzbd/scripts/
chmod +x /path/to/sabnzbd/scripts/plex2jellyfin_deobfuscate.py
```

Then in SABnzbd: **Config → Categories** → set **Script** to
`plex2jellyfin_deobfuscate.py` for your TV categories (it exits cleanly on
non-TV categories: `tv`, `series`, `shows`, `sonarr` are recognized).

The script runs after SABnzbd's own post-processing and before the daemon
picks the file up, so by the time plex2jellyfin parses it there's a real
episode marker to work with.
