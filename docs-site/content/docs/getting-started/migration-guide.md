---
title: Migration Guide
description: Run the one-shot workflow for an existing Plex-organized library.
---

This is the one-shot workflow for turning an existing Plex-organized library into a Jellyfin-compliant one. Run it once after installing, then hand the library to the [daemon](/docs/reference/daemon-services) to keep it clean as new downloads arrive.

Every destructive stage in this workflow follows the same **generate &rarr; dry-run &rarr; execute** pattern: `generate` computes a plan and stores it, `dry-run` prints what the plan would do without touching anything, `execute` applies it. Always read the dry-run output before executing.

## 1. Scan

Index every file across your configured library and watch paths into the local SQLite database (`~/.config/plex2jellyfin/media.db` on bare metal, `/config/.config/plex2jellyfin/media.db` in Docker):

```bash
plex2jellyfin scan
```

Useful flags:

- `--path <dir>` — scan a specific file or directory under a configured library root instead of everything
- `--sonarr` / `--radarr` — also sync titles/metadata from Sonarr/Radarr during the scan
- `--analyze` — after scanning, immediately analyze for duplicates and scattered series
- `--json` — emit a machine-readable summary instead of the human-readable one

This step must run first — every later command reads from the database it populates.

## 2. Status

See what you have and what's broken before changing anything:

```bash
plex2jellyfin status
```

Reports database statistics and deployment health: file counts, parse-confidence breakdown, duplicate groups pending review, stuck housekeeping items, and related issues. Unmapped Jellyfin library roots are detected at Jellyfin Test / daemon startup and in the [path mappings](/docs/getting-started/path-mappings) flow — not by `status`.

## 3. Duplicates

Find the same content stored more than once — common after years of re-downloading at different qualities or from different indexers:

```bash
plex2jellyfin duplicates generate    # find candidate duplicate groups
plex2jellyfin duplicates dry-run     # preview which copies would be removed
plex2jellyfin duplicates execute     # keep the best copy, delete the rest
```

`generate` scores each duplicate group and picks a "best copy" (resolution, source, size); `dry-run` shows exactly what `execute` would delete so you can catch a wrong pick before it's gone. If a plan looks wrong, don't run `execute` — edit the generated plan or exclude the group and re-generate.

## 4. Consolidate

Merge a TV series scattered across multiple drives — season 1 on `disk1`, season 3 on `disk2` — onto a single drive so Jellyfin sees one coherent show folder:

```bash
plex2jellyfin consolidate generate   # find series scattered across drives
plex2jellyfin consolidate dry-run    # preview consolidation moves
plex2jellyfin consolidate execute    # merge each series onto one drive
```

Movies aren't affected by consolidation (they're already single-file/single-folder); this stage only matters for multi-drive TV libraries.

## 5. Audit (AI-assisted)

Everything the regex parser couldn't confidently name — low parse-confidence files, ambiguous release names, weird folder structures — goes through this stage. Files that already match Jellyfin's `SxxExx` naming convention are unconditionally skipped; only stragglers reach here.

```bash
plex2jellyfin audit --generate              # identify low-confidence files, ask the AI for suggestions
plex2jellyfin audit --generate --dry-run    # preview AI rename suggestions without saving a plan
plex2jellyfin audit --execute               # apply approved fixes
```

Useful flags:

- `--threshold <float>` — confidence cutoff below which a file is considered "low confidence" and sent to the AI (default `0.8`)
- `--limit <n>` — cap how many files are audited in one run (`0` = uncapped)

The audit sends the library kind (Movies vs TV), folder path, and current parse to the configured LLM (Ollama, local or cloud — see [`[ai]`](/docs/reference/configuration#ai) in the configuration reference) as context, never raw file contents. Review the generated suggestions before executing; AI proposals can be wrong, especially for shows with unusual titling.

## After migration: hand off to the daemon

Once the library is clean, start `plex2jellyfin-daemon` (see [Daemon & Services](/docs/reference/daemon-services)) so it watches your download directories going forward. New files land, get parsed and renamed automatically, and a periodic convergence scan catches anything that drifts — no need to re-run the full migration workflow manually.

If you skip AI review during migration and files still land under "Season Unknown" or with release-tag titles, `plex2jellyfin repair series-dedupe` and `plex2jellyfin orphans` are targeted follow-up commands — see the [CLI Reference](/docs/reference/cli).

## Recap

```bash
plex2jellyfin scan
plex2jellyfin status
plex2jellyfin duplicates generate
plex2jellyfin duplicates dry-run
plex2jellyfin duplicates execute
plex2jellyfin consolidate generate
plex2jellyfin consolidate dry-run
plex2jellyfin consolidate execute
plex2jellyfin audit --generate
plex2jellyfin audit --generate --dry-run
plex2jellyfin audit --execute
```
