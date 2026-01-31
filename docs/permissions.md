# JellyWatch Permissions Guide

Understanding file permissions is critical for a smooth media server experience. This guide explains how to configure JellyWatch so that all your applications (Jellyfin, Sonarr, Radarr, and the jellywatch CLI) can access your media files.

## The Problem

Your media stack typically runs as different users:

| Application | Typical User | What It Does |
|-------------|--------------|--------------|
| SABnzbd/qBittorrent | `sabnzbd`/`qbittorrent` | Downloads files |
| Sonarr | `sonarr` | Manages TV shows |
| Radarr | `radarr` | Manages movies |
| JellyWatch daemon | `root` | Organizes files to library |
| Jellyfin/Plex | `jellyfin`/`plex` | Serves media |
| jellywatch CLI | Your user | Manages duplicates, consolidation |

If files are owned by the wrong user or have restrictive permissions, some applications won't be able to read, write, or delete them.

## The Solution: Shared Group

The standard approach is to use a **shared group** that all media applications belong to.

### Step 1: Create a Media Group

```bash
sudo groupadd media
```

### Step 2: Add All Users to the Group

```bash
sudo usermod -aG media sabnzbd
sudo usermod -aG media sonarr
sudo usermod -aG media radarr
sudo usermod -aG media jellyfin
sudo usermod -aG media $USER  # Your user for CLI access
```

### Step 3: Configure JellyWatch Permissions

In your `~/.config/jellywatch/config.toml`:

```toml
[permissions]
user = ""           # Empty = files owned by root (daemon runs as root)
group = "media"     # All apps share this group
file_mode = "0664"  # rw-rw-r-- (owner and group can read/write)
dir_mode = "0775"   # rwxrwxr-x (owner and group can read/write/traverse)
```

## Permission Mode Explained

### File Modes

| Mode | Meaning | When to Use |
|------|---------|-------------|
| `0644` | Owner: rw, Group: r, Other: r | Read-only access for group (Jellyfin only reads) |
| `0664` | Owner: rw, Group: rw, Other: r | **Recommended** - Group can modify/delete |
| `0666` | Everyone: rw | Not recommended (security risk) |

### Directory Modes

| Mode | Meaning | When to Use |
|------|---------|-------------|
| `0755` | Owner: rwx, Group: rx, Other: rx | Read-only access for group |
| `0775` | Owner: rwx, Group: rwx, Other: rx | **Recommended** - Group can create/delete files |
| `0777` | Everyone: rwx | Not recommended (security risk) |

**Important**: To delete a file, you need **write permission on the directory**, not just the file!

## Common Configurations

### Configuration A: Jellyfin-Owned Files (Simplest)

Best if Jellyfin is your only media consumer and you don't need CLI management.

```toml
[permissions]
user = "jellyfin"
group = "jellyfin"
file_mode = "0644"
dir_mode = "0755"
```

### Configuration B: Shared Group (Recommended)

Best for most setups with multiple applications.

```toml
[permissions]
user = ""           # Owned by root
group = "media"     # Shared group
file_mode = "0664"  # Group can write
dir_mode = "0775"   # Group can write (needed for delete!)
```

### Configuration C: Your User Owns Everything

Best if you're the only user and run everything yourself.

```toml
[permissions]
user = "yourusername"
group = "media"
file_mode = "0664"
dir_mode = "0775"
```

## Troubleshooting

### "Permission denied" when running jellywatch CLI

**Symptom**: `jellywatch duplicates execute` fails with permission errors.

**Cause**: Files or directories don't allow your user to write/delete.

**Fix**: 
1. Check your group membership: `groups $USER`
2. Ensure you're in the `media` group (or whatever group you configured)
3. Ensure `dir_mode = "0775"` (not `0755`) so group can delete

### Files created with wrong ownership

**Symptom**: New files are owned by wrong user.

**Cause**: Daemon not running as root, or permission config incorrect.

**Fix**:
1. Verify daemon runs as root: `ps aux | grep jellywatchd`
2. Check systemd service: `systemctl cat jellywatchd | grep User`
3. If not root, update service file to `User=root`

### Jellyfin can't see new files

**Symptom**: Files appear in library folder but Jellyfin doesn't show them.

**Cause**: Jellyfin user can't read the files.

**Fix**:
1. Check file permissions: `ls -la /path/to/library/`
2. Ensure jellyfin user is in the configured group
3. Rescan library in Jellyfin

## Fixing Existing Files

If you have existing files with wrong permissions:

```bash
# Change ownership recursively
sudo chown -R root:media /path/to/library/

# Change file permissions
sudo find /path/to/library -type f -exec chmod 664 {} \;

# Change directory permissions
sudo find /path/to/library -type d -exec chmod 775 {} \;
```

## Security Notes

1. **Daemon runs as root**: This is necessary for `chown()` to work. The systemd service uses Linux capabilities to restrict what root can do.

2. **Avoid 0777/0666**: World-writable files are a security risk. Use group permissions instead.

3. **Minimal group membership**: Only add users that genuinely need access to the media group.

## Quick Reference

| Goal | user | group | file_mode | dir_mode |
|------|------|-------|-----------|----------|
| Jellyfin-only, read-only | `jellyfin` | `jellyfin` | `0644` | `0755` |
| Shared access, no CLI delete | `""` | `media` | `0644` | `0755` |
| **Shared access, CLI can delete** | `""` | `media` | `0664` | `0775` |
| Your user owns all | `yourusername` | `media` | `0664` | `0775` |

## More Information

- [Jellyfin Permissions Docs](https://jellyfin.org/docs/general/administration/hardware-acceleration/#linux-setups)
- [TRaSH Guides - Permissions](https://trash-guides.info/Hardlinks/How-to-setup-for/Docker/)
- [Servarr Wiki - Permissions](https://wiki.servarr.com/docker-guide#consistent-and-well-planned-paths)
