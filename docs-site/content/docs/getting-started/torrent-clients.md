---
title: Torrent Clients
description: Seed completed torrents while Plex2Jellyfin imports them.
---

A torrent stays useful after it finishes. Point your client at a media library root to "import" it, though, and you lose everything Plex2Jellyfin does: filename parsing, canonical naming, placement across drives, the audit log, and the media database. Two clients writing the same library also split one show into several Jellyfin series.

Seeding roots solve this. You give Plex2Jellyfin a directory of completed downloads, it imports from there, and your client keeps seeding the original files.

## The contract

Plex2Jellyfin talks to no download client. It watches directories, so any client works if its layout satisfies six rules:

- Incomplete data lives outside every watched root.
- A finished torrent lands in a TV or Movies seeding root.
- The client keeps seeding from that path.
- Plex2Jellyfin never modifies or deletes what it finds there.
- Your client owns ratio limits, seed time, and eventual removal.
- Unclassified downloads land somewhere Plex2Jellyfin does not watch.

One client behaviour breaks this: preallocating the final video filename inside a watched root before the data arrives. Plex2Jellyfin cannot tell a preallocated file from a finished one, so it may import a partial download. Use a separate incomplete directory, or a temporary extension your client renames atomically on completion.

## Configuration

```toml
[watch]
# Ordinary intake: Plex2Jellyfin moves or copies these, per delete_source.
movies = ["/downloads/complete/movies"]
tv     = ["/downloads/complete/tv"]

# Seeding intake: sources always stay in place.
seeding_movies = ["/torrents/complete/movies"]
seeding_tv     = ["/torrents/complete/tv"]

[libraries]
movies = ["/media/Movies"]
tv     = ["/media/TV Shows"]
```

Ordinary and seeding roots are both intake, so either one pairs with a library. Configure seeding roots alone if every download arrives by torrent.

Roots may not overlap each other or a library path. The daemon rejects such a layout at startup, and all three setup surfaces block it before you save.

## Move, copy, hardlink

`delete_source` in `[options]` governs ordinary watch roots only:

| Root | `delete_source = true` | `delete_source = false` |
|---|---|---|
| Ordinary | Move | Copy |
| Seeding | Hardlink, or copy across filesystems | Hardlink, or copy across filesystems |

A hardlink gives the library a second name for the same data. The file appears in Jellyfin, your client keeps seeding, and the disk holds one copy. When the seeding root and the library sit on different filesystems, Plex2Jellyfin copies instead, so plan for double the space on those titles.

Keep both on one filesystem to get hardlinks. A bind mount does not change this: `/torrents` and `/media` hardlink only if they resolve to the same host filesystem.

## Permissions

Your torrent client writes the source; Plex2Jellyfin reads it and writes the library. Give both a shared media group, and set the client's umask so group members can read what it creates (`002` for group-writable files, `022` when group-read is enough).

Seeding roots need read access, not write. Plex2Jellyfin never touches the source contents.

## qBittorrent

Mount staging only. Do not give qBittorrent your media library.

```text
host:      /mnt/STORAGE5/.downloads/torrents
container: /downloads
```

Under Options → Downloads:

```text
Keep incomplete torrents in:  /downloads/incomplete
Default Save Path:            /downloads/complete/unclassified
```

Then per category (right-click a category → Edit category):

```text
TVSHOWS save path:            /downloads/complete/tv
MOVIES save path:             /downloads/complete/movies
```

Configure `seeding_tv` and `seeding_movies` to the category paths. Uncategorised torrents stay in the default path, which nothing watches, so a mislabelled torrent waits for you instead of landing in your library under a guessed name. A manually added torrent imports once it completes and carries one of the two categories.

## Transmission

Transmission has no categories. Set an incomplete directory under Preferences → Downloading, then choose the TV or Movies completion root as each torrent's download directory when you add it:

```text
Incomplete directory:  /downloads/incomplete
Default directory:     /downloads/complete/unclassified
```

Leave the default pointing somewhere unwatched. Anything you forget to route stays there.

## Deluge

Deluge pairs an incomplete directory with the Label plugin's per-label move-on-complete paths.

Under Preferences → Downloads:

```text
Download to:       /downloads/incomplete
Move completed to: /downloads/complete/unclassified
```

Then enable the Label plugin and, for each label, set `Move completed to`:

```text
tv label:      /downloads/complete/tv
movies label:  /downloads/complete/movies
```

Unlabelled downloads land in the unwatched default.

## Other clients

rTorrent, ruTorrent, and anything else works the same way. Route completed downloads into a media-typed directory, keep partial data elsewhere, and never point the client at a library root. Plex2Jellyfin runs no client-specific code.

## Verifying

After the first torrent completes:

```bash
plex2jellyfin status
```

Check that the file exists in both places and shares an inode with the source:

```bash
stat -c '%i %n' /torrents/complete/tv/Show.S01E01.mkv
find /media/'TV Shows' -samefile /torrents/complete/tv/Show.S01E01.mkv
```

Matching inode numbers mean the import hardlinked. Different numbers mean it copied, which is expected across filesystems.
