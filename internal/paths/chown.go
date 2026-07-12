package paths

import (
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
)

// ChownConfigDir recursively chowns ~/.config/plex2jellyfin to ownerUser:ownerGroup.
// Empty ownerUser falls back to ActualUser(); empty group falls back to the user.
// No-op when not root (caller must already be escalated, like the TUI/CLI setup).
func ChownConfigDir(ownerUser, ownerGroup string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	dir, err := Plex2JellyfinDir()
	if err != nil {
		return err
	}
	who := ownerUser
	if who == "" {
		who = ActualUser()
	}
	if who == "" || who == "root" || who == "unknown" {
		return nil
	}
	group := ownerGroup
	if group == "" {
		group = who
	}
	u, err := user.Lookup(who)
	if err != nil {
		return err
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return err
	}
	gid := uid
	if g, err := user.LookupGroup(group); err == nil {
		if n, err := strconv.Atoi(g.Gid); err == nil {
			gid = n
		}
	} else if who == group {
		if n, err := strconv.Atoi(u.Gid); err == nil {
			gid = n
		}
	} else {
		return err
	}
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		_ = os.Chown(path, uid, gid)
		return nil
	})
}
