---
title: Troubleshooting
description: Diagnose permissions, path mappings, parsing, and service failures.
---

## Permissions

The most common category of issue. Your media stack typically runs several applications as different users:

| Application | Typical user | What it does |
|---|---|---|
| SABnzbd/qBittorrent | `sabnzbd`/`qbittorrent` | Downloads files |
| Sonarr | `sonarr` | Manages TV shows |
| Radarr | `radarr` | Manages movies |
| Plex2Jellyfin daemon | `root` (bare metal) / `PUID:PGID` (Docker) | Organizes files into the library |
| Jellyfin/Plex | `jellyfin`/`plex` | Serves media |
| `plex2jellyfin` CLI | your user | Manages duplicates, consolidation |

If files are owned by the wrong user or have restrictive permissions, some of these can't read, write, or delete them.

Running in Docker? See [Docker &rarr; PUID/PGID and file ownership](/docs/getting-started/docker#puidpgid-and-file-ownership) first — most container permission issues are a PUID/PGID mismatch, and the `[permissions]` config below has no effect in-container.

### "Permission denied" running the CLI

**Symptom:** `plex2jellyfin duplicates execute` (or `consolidate execute`) fails with permission errors.

**Cause:** files or directories don't allow your user to write/delete.

**Fix:**

1. Check your group membership: `groups $USER`
2. Make sure your user is in whatever group owns the library files (a shared `media` group is a common pattern)
3. Directory mode matters as much as file mode — deleting a file requires **write permission on its parent directory**, not just the file itself. A `dir_mode` of `0755` blocks group deletes; `0775` allows them.

### Files created with the wrong ownership (bare metal)

**Symptom:** new files land owned by the wrong user.

**Cause:** the daemon isn't running as root, or `[permissions]` is misconfigured.

**Fix:**

1. Verify the daemon runs as root: `ps aux | grep plex2jellyfin-daemon`
2. Check the systemd unit: `systemctl cat plex2jellyfin-daemon | grep User`
3. Confirm `[permissions]` in `config.toml` — see [Configuration &rarr; `[permissions]`](/docs/reference/configuration#permissions)

### Files created with the wrong ownership (Docker)

`[permissions]` has no effect in Docker — see [why](/docs/getting-started/docker#the-permissions-chown-feature-is-unavailable-in-container). Set `PUID`/`PGID` to match the UID/GID that should own the files instead, and confirm the mapping with `docker exec plex2jellyfin id`.

### Jellyfin can't see new files

**Symptom:** files appear in the library folder on disk but Jellyfin doesn't show them.

**Cause:** the Jellyfin process's user can't read the files, or Jellyfin hasn't rescanned.

**Fix:**

1. Check permissions: `ls -la /path/to/library/`
2. Confirm the Jellyfin user/PUID can read those files (same group, or matching PUID/PGID in Docker)
3. Trigger a library rescan in Jellyfin

### Fixing existing files (bare metal)

```bash
sudo chown -R root:media /path/to/library/
sudo find /path/to/library -type f -exec chmod 664 {} \;
sudo find /path/to/library -type d -exec chmod 775 {} \;
```

Adjust the owner/group and modes to whatever `[permissions]` you've configured.

## Jellyfin path mappings and parse decisions marked FAIL

**Symptom:** organized files exist and look correct, but `plex2jellyfin status` or the parse-decisions data shows them as FAIL.

**Cause:** the post-organize feedback loop (sweeper + Jellyfin webhook) can't correlate a Jellyfin library item's path with the daemon's own path for that file. This happens whenever Jellyfin runs in a container with bind mounts whose paths differ from what the daemon sees — for example Jellyfin sees `/tv/Show/...` but the daemon organized the file to `/mnt/storage1/TVSHOWS/Show/...`.

**Fix:** add `[[jellyfin.path_mappings]]` entries to `config.toml` covering every root where the two views diverge. See [Configuration &rarr; Jellyfin path mappings](/docs/reference/configuration#jellyfin-path-mappings). Without them, every row eventually gets auto-labeled FAIL as the sweeper runs.

## AI audit issues

### AI proposes the wrong show or movie

**Symptom:** `audit --generate` suggests a title that has nothing to do with the actual file (e.g. suggests "History's Greatest Mysteries" for a `Prison Break` episode).

**Cause:** insufficient context reached the model, or folder naming is too obfuscated for even the folder-path hint to help.

**Fix:**

1. Set `DEBUG_AI=1` in the daemon/CLI environment and re-run to see exactly what context (library type, folder path, current parse) was sent in the prompt.
2. Verify the file sits under a sensibly named folder — the AI uses the parent directory name as a hint when the filename itself is ambiguous.
3. Raise `confidence_threshold` in `[ai]` (see [Configuration](/docs/reference/configuration#ai)) so borderline suggestions get rejected instead of applied.

### AI suggests the wrong media type (movie vs. TV)

**Fix:**

1. Confirm the file sits under the correct library root (`[watch]`/`[libraries]` in `config.toml`) — the AI is told which library type it's working in and trusts that.
2. Re-run `plex2jellyfin scan` after fixing the config so the database reflects the corrected library assignment.

### AI calls failing or timing out

**Symptom:** `audit --generate` errors out or silently skips files.

**Fix:**

1. Confirm Ollama is reachable at `ollama_endpoint` (`curl http://localhost:11434/api/tags` or your cloud endpoint).
2. Check `timeout_seconds` isn't too low for your model/hardware.
3. Check `hourly_limit`/`daily_limit` haven't been hit — the tool self-throttles to protect the endpoint.
4. If the primary `model` is failing repeatedly, the circuit breaker should fall through to `fallback_model` automatically; confirm both are valid, pulled model names.

## Daemon and services

### Daemon won't start

```bash
systemctl status plex2jellyfin-daemon
journalctl -u plex2jellyfin-daemon -n 100
```

Common causes: `config.toml` missing or invalid (`plex2jellyfin config test`), a watch/library path in the config that doesn't exist or isn't readable by root, or the control socket path already in use by a stale process.

### Web UI can't reach the daemon

`plex2jellyfin-web` requires `plex2jellyfin-daemon` to already be running — the systemd unit declares `Wants=`/`After=` on the daemon, but if the daemon crashed after startup, restart both:

```bash
sudo systemctl restart plex2jellyfin-daemon plex2jellyfin-web
```

They communicate only over a Unix-domain control socket, so a firewall or network issue can't be the cause — check that the socket file exists and is owned/readable by both processes' users.

### Config changes not taking effect

`config.toml` isn't hot-reloaded by default. After editing it:

```bash
plex2jellyfin daemon reload   # picks up config without dropping in-flight watches
# or
sudo systemctl restart plex2jellyfin-daemon
```

## Database

### Corrupted or inconsistent database

```bash
plex2jellyfin database path              # confirm which file you're looking at
plex2jellyfin database cleanup-housekeeping  # collapse duplicate housekeeping failures
plex2jellyfin database reset             # nuclear option: delete and reinitialize
```

`database reset` deletes all indexed state (not your media files) — you'll need to `plex2jellyfin scan` again afterward.

### Duplicate series rows / files landing under "Season Unknown"

```bash
plex2jellyfin repair series-dedupe
plex2jellyfin repair unknown-seasons
```

Targeted repair commands for these specific, known failure modes — see [CLI Reference &rarr; repair](/docs/reference/cli#repair).

## Still stuck?

Run a postmortem bundle and review the evidence yourself, or hand it to an LLM:

```bash
plex2jellyfin postmortem collect --since 96h
```

See [Daemon & Services &rarr; Postmortem timer](/docs/reference/daemon-services#postmortem-timer) and the [CLI Reference](/docs/reference/cli#postmortem).
