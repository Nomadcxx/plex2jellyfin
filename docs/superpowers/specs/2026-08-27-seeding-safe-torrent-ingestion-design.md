# Seeding-Safe Torrent Ingestion Design

**Date:** 2026-08-27

**Status:** Approved

## Problem

Plex2Jellyfin is the import authority and its database is the library source of truth. Sonarr and Radarr may schedule downloads and receive post-import rescans, but they must not rename or import completed media.

The current qBittorrent categories write directly into `/mnt/STORAGE5/TVSHOWS` and `/mnt/STORAGE5/MOVIES`. Those paths bypass Plex2Jellyfin's watch roots, parsing, canonical naming, placement, audit log, and database updates. That is why Stargirl seasons 1 and 3 appeared as separate Jellyfin series while the SABnzbd-imported season 2 was canonical. Other torrent clients can cause the same failure when their final download directory is a media-library root.

Pointing a torrent client at an existing watch root is also unsafe. The daemon organizer always moves successful imports, removing the source file the client needs for seeding. Although `[options].delete_source = false` is documented and exposed by setup surfaces, the daemon does not currently wire that option into its organizers.

## Goals

- Import completed torrents from any filesystem-based client through the same parsing, library selection, naming, audit, and database pipeline as SABnzbd downloads.
- Retain torrent source files until the client's ratio/time policy removes them.
- Use hardlinks when source and target are on the same filesystem, with verified copying across filesystems.
- Keep existing SABnzbd move behavior and backward-compatible configuration.
- Support manually added torrents when the user assigns or selects a TV or Movies completion directory.
- Expose the feature consistently in CLI setup, the TUI installer, web setup, and web path settings.
- Reload watch roots and their transfer policies atomically.

## Non-goals

- Plex2Jellyfin will not manage torrent ratios, stop seeding, or remove torrents from a client.
- Plex2Jellyfin will not infer an import type for uncategorised torrents.
- Sonarr/Radarr completed-download handling will remain disabled.
- The change will not trigger library-wide Jellyfin refreshes.
- The first release will not add downloader API integration. Filesystem completion paths are sufficient.

## Configuration

Extend the existing `[watch]` table with two optional arrays:

```toml
[watch]
tv = ["/mnt/NVME3/Sabnzbd/complete/tv"]
movies = ["/mnt/NVME3/Sabnzbd/complete/movies"]
seeding_tv = ["/mnt/STORAGE5/.downloads/torrents/complete/tv"]
seeding_movies = ["/mnt/STORAGE5/.downloads/torrents/complete/movies"]

[options]
delete_source = true
```

The policies are fixed and intentionally small:

| Source root | Policy |
|---|---|
| `watch.tv`, `watch.movies` with `delete_source = true` | Move after successful import |
| `watch.tv`, `watch.movies` with `delete_source = false` | Copy and retain source |
| `watch.seeding_tv`, `watch.seeding_movies` | Hardlink when possible, otherwise copy; always retain source |

Existing configurations load unchanged because the new arrays default to empty. Generated TOML always emits the new keys so saved configurations do not silently lose them.

## Torrent-Client Contract

Plex2Jellyfin integrates with a completed-download filesystem contract rather than a client API:

- incomplete data lives outside watched seeding roots;
- a completed torrent lands in either a TV or Movies seeding root;
- the client continues seeding from that completed path;
- Plex2Jellyfin never changes or removes the source contents;
- the client owns ratio limits, seed duration, and eventual source removal;
- uncategorised or unclassified downloads remain outside automatic intake.

Clients that preallocate a final video filename directly inside a watched root are unsafe because a debounce interval cannot prove the writer has finished. They must use a separate incomplete directory or a temporary extension that is atomically renamed only after completion.

## Client Layouts

### qBittorrent

qBittorrent receives only a staging mount, not a media-library mount:

```text
host:      /mnt/STORAGE5/.downloads/torrents
container: /downloads
```

Configure qBittorrent as follows:

```text
Keep incomplete torrents in: /downloads/incomplete
TVSHOWS category save path:   /downloads/complete/tv
MOVIES category save path:    /downloads/complete/movies
Default/unclassified path:    /downloads/complete/unclassified
```

Plex2Jellyfin watches only the category completion directories. Partial files and uncategorised torrents remain outside its intake boundary. A manually added torrent is imported after it completes and has either the `TVSHOWS` or `MOVIES` category.

### Transmission

Transmission uses a separate incomplete directory and the TV or Movies completion root as the torrent's final download directory. Transmission has no native category equivalent, so the user or automation must select the correct final directory when adding each torrent. A single unclassified default directory remains unwatched.

### Deluge

Deluge uses a separate incomplete directory and its Label plugin's per-label `move_completed` paths. TV and Movies labels map to the corresponding Plex2Jellyfin seeding roots. Downloads without one of those labels remain in an unwatched default directory.

Other clients, including rTorrent/ruTorrent, are supported when their directory or completion rules satisfy the same contract. No client-specific code runs inside Plex2Jellyfin.

## Runtime Architecture

### Source policy

Introduce a small, concurrency-safe source-policy resolver owned by the daemon. It classifies a source path as `move`, `copy`, or `seed` using filesystem-boundary-aware containment. Seeding roots take their fixed `seed` policy; ordinary roots use the current `delete_source` value.

The organizer resolves the policy per source operation. This avoids mutable organizer-wide `keepSource` state and preserves correct behavior when TV episodes, movies, season packs, deferred operations, and AI retries run concurrently.

Repeated transfer branches in the organizer are consolidated behind one helper. That helper chooses:

- the existing transferer's `Move` for `move`;
- the existing transferer's `Copy` for `copy`;
- a hardlink-first transferer's `Copy` for `seed`.

The same resolved policy controls post-import database cleanup. Only the canonical library target remains indexed as library media; retaining a download source does not make it a second canonical library file.

### Hardlink-first transfer

A transfer wrapper in `internal/transfer` attempts an atomic hardlink publication in the destination directory. It delegates to the existing copy backend only when the link fails because source and destination are on different devices or the filesystem does not support hardlinks.

Unexpected failures such as permissions or a missing source are returned instead of being hidden behind a copy. The source is never removed. A successful hardlink reports zero physical bytes copied while retaining the source size as the logical total.

Hardlinks share inode ownership, mode, and modification time. Applying configured library permissions or import time therefore also changes those attributes at the client's source path, but not file contents. Torrent clients hash contents, so seeding remains valid. Deployments must keep the client and Plex2Jellyfin in the same media group.

### Watch and policy reload

The daemon's reload transaction must update these together:

- filesystem watcher roots;
- periodic scanner roots;
- handler TV/movie source hints;
- transfer-policy roots;
- `delete_source` for ordinary roots.

Prepare validates the candidate configuration before any commit. Commit swaps immutable root slices/policy under a lock. Rollback restores both watcher and handler state if a later reload step fails. This closes the existing split-brain risk where the watcher sees new roots but the handler classifies with old roots.

## Path Validation

Containment checks use `filepath.Rel` semantics, not string prefixes. For example, `/downloads/tv2` is not inside `/downloads/tv`.

Configuration is rejected when:

- an ordinary watch root overlaps a seeding watch root;
- a watch root overlaps a library root;
- the same canonical path appears with conflicting media types or policies.

CLI, TUI, and web setup report these errors before saving. Manual TOML receives the same validation during daemon startup and reload. Symlink resolution is not required for this release; paths are cleaned absolute paths, matching existing configuration behavior.

## Import Data Flow

1. The torrent client writes partial data outside Plex2Jellyfin's watch roots.
2. On completion, the client publishes the release into a media-typed seeding root.
3. Plex2Jellyfin receives or periodically discovers the completed media file.
4. The handler identifies TV or movie type from the configured category root, parses the release, and chooses the canonical library target.
5. The organizer resolves the source policy and hardlinks or copies while retaining the torrent source.
6. Plex2Jellyfin indexes the canonical target, records the parse/import decision, and sends scoped Sonarr/Radarr/Jellyfin notifications as currently implemented.
7. The torrent client continues seeding from the release-named source path.
8. When its seed policy is satisfied, the client may remove the torrent and source. The canonical library target remains.

## Failure Handling

- A failed hardlink fallback copy leaves the source intact and does not record a successful import.
- Cross-volume copy uses the existing disk-health, timeout, retry, and partial-file behavior.
- If the torrent client removes the source during import, the operation fails or becomes `VANISHED`; it never deletes a valid target as compensation.
- Repeated completion events use existing destination-quality and already-organized checks.
- Source-directory cleanup observes that retained source files still exist and skips them.
- Housekeeping may remove an empty abandoned release directory after the client removes its contents, but it must not treat live seed data as disposable.
- A reload validation or commit failure restores the previous configuration and runtime policy.

## Setup and Management Surfaces

All configuration writers and editors must preserve the new arrays:

- `config init` and `config.toml.example`;
- shared Go setup draft and validation;
- CLI setup prompts and review summary;
- TUI installer path inputs, confirmation, and hand-written TOML;
- OpenAPI schema and generated TypeScript API types;
- web setup hydration, validation, path editors, and review summary;
- web Watch Paths settings API and page;
- daemon startup, initial scan, periodic scan, watcher, reload, and housekeeping inputs;
- README, configuration reference, Docker examples, a client-neutral torrent workflow, and recipes for qBittorrent, Transmission, and Deluge;
- embedded frontend assets rebuilt from the updated web source.

The TUI exposes two optional fixed fields, `TV seeding completions` and `Movie seeding completions`, in addition to its existing ordinary watch fields. Its dynamic custom folders remain ordinary watch roots.

## Regression Test Matrix

| Area | Required proof |
|---|---|
| Config | Old TOML loads with empty seeding arrays; new arrays survive load/save and section updates |
| Validation | Boundary-safe containment; ordinary/seeding and watch/library overlaps fail; adjacent prefixes pass |
| Transfer | Same-device import creates equal inodes and retains source; forced cross-device path uses copy; unexpected link failure is returned |
| Organizer | Move removes ordinary source; copy retains ordinary source; seed retains source and selects hardlink-first transfer for movie, episode, and season-pack paths |
| Database | Canonical destination is indexed once; retained staging path is not treated as a library duplicate |
| Daemon | All four root lists are watched/scanned; source hints classify torrent paths; `delete_source = false` works |
| Reload | Watcher and handler policy change together; rollback restores both |
| Cleanup | Live seed release directories are untouched; normal successful move cleanup still works |
| Setup | Shared draft, CLI, API, web, and TUI round-trip seeding roots without erasing existing settings |
| Frontend | Hydration defaults missing seeding arrays; validation accepts seeding-only intake paired with a library |
| Ingestion forms | Completed directory moves, file renames, temporary extensions, multi-file torrents, and season packs converge safely |
| Permissions | Client-specific umasks work with the configured shared media group |
| Source lifecycle | Canonical hardlinks survive client source rename/removal; source removal during copy cannot produce a false success |
| Documentation | The generic contract and qBittorrent, Transmission, and Deluge recipes are explicit; direct-library paths are rejected |

Run focused package tests after each task, then run the complete Go suite, web tests/build, OpenAPI consistency check, and repository lint/build gates before deployment.

## Deployment and Stargirl Repair

Deployment is staged to avoid losing active seed state:

1. Create the host incomplete, TV completion, movie completion, and unclassified directories with the media group's ownership.
2. Deploy the new Plex2Jellyfin build and configure seeding watch roots.
3. Change the active torrent client's incomplete and media-typed completion paths; remove its media-library mount if container layout permits.
4. Add a small completed test torrent or test fixture and verify source retention, canonical import, database decision, and scoped Jellyfin visibility.
5. Use qBittorrent's `Set location` operation to relocate active Stargirl season 1 and 3 data into the TV completion staging area without breaking torrent state.
6. Let Plex2Jellyfin import both seasons into the canonical Stargirl series, then verify inode/copy state, episode counts, provider identity, and Jellyfin paths.
7. Remove the obsolete direct-library Stargirl directories only after those checks pass. Do not run a library-wide Jellyfin refresh.

Rollback restores the previous Plex2Jellyfin package and configuration, then points qBittorrent categories at staging paths that Plex2Jellyfin does not watch. Existing canonical imports remain valid; retained torrent sources allow re-import if necessary.
