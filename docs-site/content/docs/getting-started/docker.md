---
title: Docker
description: Run the daemon, dashboard, and CLI in one container image.
---

One image, three binaries. `plex2jellyfin-daemon` and `plex2jellyfin-web` run together under the container's entrypoint; the `plex2jellyfin` CLI is available inside the same image for one-off commands.

```bash
docker run --rm ghcr.io/nomadcxx/plex2jellyfin:latest plex2jellyfin version
```

> **Permissions are the #1 source of Docker pain here**
>
> Read the [PUID/PGID](#puidpgid-and-file-ownership) and [`[permissions]` chown feature](#the-permissions-chown-feature-is-unavailable-in-container) sections below before you file a "files end up owned by root" issue — it's almost always a PUID/PGID mismatch, not a bug.

## Quick start

```bash
docker run -d \
  --name plex2jellyfin \
  -e PUID=1000 -e PGID=1000 \
  -v ./config:/config \
  -v /path/to/downloads:/watch \
  -v /path/to/media:/library \
  -p 5522:5522 \
  --restart unless-stopped \
  ghcr.io/nomadcxx/plex2jellyfin:latest
```

## Volumes

| Container path | Purpose |
|---|---|
| `/config` | Home directory for the in-container user. Holds `.config/plex2jellyfin/config.toml`, the SQLite `media.db`, lock files, and audit/postmortem reports. Must be writable. |
| `/watch` | Mount point(s) matched to your `[watch]` section — where Sonarr/Radarr/download clients drop new files. |
| `/library` | Mount point(s) matched to your `[libraries]` section — your Jellyfin `Movies` / `TV Shows` roots. |

The container declares all three as `VOLUME` in the image, so even without explicit `-v` flags Docker creates anonymous volumes for them — always bind-mount all three explicitly or you'll lose data between container recreations.

If your host layout uses more than one watch or library directory (multi-drive setups are common with this tool), bind-mount each one to its own path under `/watch` or `/library` and mirror that layout in `config.toml`'s `[watch]` and `[libraries]` arrays. Path mappings inside the container must match what you write into the config — see [Configuration](/docs/reference/configuration).

## PUID/PGID and file ownership

The image follows the [linuxserver.io](https://docs.linuxserver.io/general/understanding-puid-and-pgid/) convention:

```bash
-e PUID=1000 -e PGID=1000
```

On container start, the entrypoint creates a `p2j` user/group with the requested UID/GID, `chown -R`s `/config` to that UID/GID, then drops from root to `p2j` via `su-exec` before launching the daemon and web server. Everything the daemon writes — renamed media files, the database, logs — is written as that UID/GID.

**Set `PUID`/`PGID` to match the UID/GID that already owns your media on the host**, typically the UID/GID Jellyfin itself runs as. Find it with:

```bash
id jellyfin
# or, if Jellyfin also runs in a container:
docker exec jellyfin id
```

If PUID/PGID don't match your host's Jellyfin user, Jellyfin may lose read/write access to files the daemon moves into `/library`, even though the move itself succeeds inside the container.

Only `/config` gets the automatic `chown -R` on start — `/watch` and `/library` keep whatever ownership they already have on the host. If files land in `/library` owned by an unexpected user, check that PUID/PGID match the host UID/GID you expect, and that the host directories themselves are writable by that UID/GID.

## The `[permissions]` chown feature is unavailable in-container

Bare-metal installs support an optional `[permissions]` block in `config.toml` that chowns and chmods every file the daemon moves, so Jellyfin (running as a different system user) can read them:

```toml
[permissions]
user      = "jellyfin"
group     = "jellyfin"
file_mode = "0644"
dir_mode  = "0755"
```

On bare metal this works because the systemd unit runs `plex2jellyfin-daemon` as **root** with a minimal capability set (`CAP_CHOWN`, `CAP_FOWNER`, `CAP_DAC_OVERRIDE`), which lets it chown files to a different user after moving them.

**In the container, this doesn't apply.** The entrypoint always drops privileges to the `PUID`/`PGID` user before the daemon starts — there is no root process left to chown to some *other* user, and no spare capability to do it with. If you set `[permissions]` in a containerized deployment, the daemon has nothing to elevate to and the setting has no effect.

The container's equivalent of `[permissions]` is **`PUID`/`PGID` alone**: set them to the UID/GID that should own everything the daemon writes, and skip the `[permissions]` block entirely in your container's `config.toml`.

## SELinux hosts (Fedora, RHEL, CentOS)

If your host enforces SELinux, bind-mounted directories are denied to the container by default even though the `docker run`/`docker-compose` command looks correct. Add the `:z` (shared) or `:Z` (private) relabel suffix to each volume:

```bash
docker run -d \
  --name plex2jellyfin \
  -e PUID=1000 -e PGID=1000 \
  -v ./config:/config:Z \
  -v /path/to/downloads:/watch:z \
  -v /path/to/media:/library:z \
  -p 5522:5522 \
  --restart unless-stopped \
  ghcr.io/nomadcxx/plex2jellyfin:latest
```

Use `:Z` for `/config` (private to this container) and `:z` for `/watch`/`/library` if those paths are also shared with other containers (e.g. Sonarr, Radarr, Jellyfin itself all mounting the same media root) — `:z` relabels for shared access, `:Z` relabels exclusively to one container and will lock other containers out of that path.

## Rootless Podman

The image and entrypoint work under rootless Podman, with one extra consideration: rootless containers map container UIDs into a range of *subordinate* UIDs/GIDs owned by your host user (`/etc/subuid`, `/etc/subgid`), not directly onto host UIDs. A `PUID=1000` inside the container does **not** correspond to host UID 1000 unless you've configured the mapping to do so.

Two common approaches:

- Run with `--userns=keep-id` so the container's UID namespace matches your host user 1:1, then set `PUID`/`PGID` to your normal host UID/GID.
- Or leave the default subordinate-UID mapping and set `PUID`/`PGID` to the *in-container* UID, then adjust host-side directory ownership to match whatever host UID that maps to (check with `podman unshare cat /proc/self/uid_map` from inside the container's namespace, or `podman exec <container> id`).

Either way, verify the mapping with `podman exec plex2jellyfin id` and confirm the reported UID/GID's host-side ownership lines up with your `/library` and `/watch` mounts before trusting a migration run.

## Full compose walkthrough

[`docker-compose.example.yml`](https://github.com/Nomadcxx/plex2jellyfin/blob/main/docker-compose.example.yml):

```yaml
services:
  plex2jellyfin:
    image: ghcr.io/nomadcxx/plex2jellyfin:latest
    container_name: plex2jellyfin
    environment:
      - PUID=1000
      - PGID=1000
    volumes:
      - ./config:/config
      - /path/to/downloads:/watch
      - /path/to/media:/library
    ports:
      - "5522:5522"
    restart: unless-stopped
```

1. Copy the file and edit the volume paths for your host layout:

   ```bash
   curl -O https://raw.githubusercontent.com/Nomadcxx/plex2jellyfin/main/docker-compose.example.yml
   ```

2. Set `PUID`/`PGID` to match the UID/GID your Jellyfin instance runs as (see [above](#puidpgid-and-file-ownership)).
3. If your Jellyfin also runs in a container with bind mounts whose paths differ from this container's `/library`, add `[[jellyfin.path_mappings]]` entries to `config.toml` so the post-organize feedback loop can correlate Jellyfin items with daemon paths — see [Configuration](/docs/reference/configuration#jellyfin-path-mappings).
4. Bring it up:

   ```bash
   docker compose -f docker-compose.example.yml up -d
   ```

5. Open `http://<host>:5522/` and finish setup in the browser. On a fresh
   `/config` volume the web UI walks you through everything — no exec, no
   hand-written TOML:

   1. create the admin password (first visit forces this);
   2. the **setup wizard** at `/setup` collects watch and library paths
      (use the *container* paths: `/watch/...`, `/library/...`), optional
      Sonarr/Radarr/Jellyfin connections, optional Ollama, and runtime
      behavior — every path is validated before you can continue;
   3. **Review & activate** writes the config atomically and starts the
      daemon; you land on the dashboard and can optionally kick off the
      initial scan.

   The wizard only appears while the install is unconfigured. Existing
   configs (including ones written by hand on older releases) skip it.

   > On releases older than the setup wizard, configure manually instead:
   > `docker exec -it plex2jellyfin plex2jellyfin config init`, edit
   > `/config/.config/plex2jellyfin/config.toml`, then restart.

6. Check logs:

   ```bash
   docker logs -f plex2jellyfin
   ```

## Where config lives in the container

Config lives at `/config/.config/plex2jellyfin/config.toml` — the same relative layout as a bare-metal install rooted at `$HOME`, because `HOME=/config` is set in the image and the daemon resolves its config path the same way on both bare metal and in-container.

## Running CLI commands against a container

The CLI binary is bundled in the same image. Use `docker exec` against a running container so it shares the mounted config/database:

```bash
docker exec -it plex2jellyfin plex2jellyfin status
docker exec -it plex2jellyfin plex2jellyfin scan
docker exec -it plex2jellyfin plex2jellyfin duplicates generate
```

Or run a throwaway container for one-off, no-daemon use, reusing the same volumes:

```bash
docker run --rm \
  -e PUID=1000 -e PGID=1000 \
  -v ./config:/config \
  -v /path/to/downloads:/watch \
  -v /path/to/media:/library \
  ghcr.io/nomadcxx/plex2jellyfin:latest \
  plex2jellyfin status
```

Passing any extra arguments to `docker run <image> <command> ...` runs that one-off command as the `PUID`/`PGID` user instead of starting the daemon and web server.
