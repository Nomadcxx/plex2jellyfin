// Package paths provides sudo-aware path resolution for Plex2Jellyfin.
//
// When running with sudo, these functions correctly resolve paths to the
// original user's directories (via SUDO_USER) instead of root's directories.
package paths

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
)

// UserHomeDir returns the home directory of the actual user.
// If running with sudo, returns the SUDO_USER's home directory, not root's.
func UserHomeDir() (string, error) {
	// Check SUDO_USER first (running with sudo)
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" && sudoUser != "root" {
		u, err := user.Lookup(sudoUser)
		if err == nil {
			return u.HomeDir, nil
		}
		// Fall through if lookup fails
	}

	// Fallback to current user
	return os.UserHomeDir()
}

// UserConfigDir returns the config directory of the actual user.
// If running with sudo, returns the SUDO_USER's config directory, not root's
// (built from their home directory, since we cannot know their XDG_CONFIG_HOME).
// Otherwise it defers to os.UserConfigDir(), which honors $XDG_CONFIG_HOME when
// set and falls back to ~/.config on Linux (and containers set $HOME directly).
func UserConfigDir() (string, error) {
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" && sudoUser != "root" {
		homeDir, err := UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(homeDir, ".config"), nil
	}
	return os.UserConfigDir()
}

// Plex2JellyfinDir returns the Plex2Jellyfin config directory.
// This is ~/.config/plex2jellyfin for the actual user.
func Plex2JellyfinDir() (string, error) {
	configDir, err := UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "plex2jellyfin"), nil
}

// DatabasePath returns the path to the Plex2Jellyfin database.
// This is ~/.config/plex2jellyfin/media.db for the actual user.
func DatabasePath() (string, error) {
	dir, err := Plex2JellyfinDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "media.db"), nil
}

// ConfigPath returns the path to the Plex2Jellyfin config file.
// This is ~/.config/plex2jellyfin/config.toml for the actual user.
func ConfigPath() (string, error) {
	dir, err := Plex2JellyfinDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// OpLogPath returns the path to the Plex2Jellyfin op log.
// This is ~/.config/plex2jellyfin/op_log.jsonl for the actual user.
func OpLogPath() (string, error) {
	dir, err := Plex2JellyfinDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "op_log.jsonl"), nil
}

// PlansDir returns the directory for plan files.
// This is ~/.config/plex2jellyfin/plans for the actual user.
func PlansDir() (string, error) {
	dir, err := Plex2JellyfinDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "plans"), nil
}

// ReportsDir returns the directory for postmortem report bundles.
// This is ~/.config/plex2jellyfin/reports for the actual user.
func ReportsDir() (string, error) {
	dir, err := Plex2JellyfinDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "reports"), nil
}

// ActualUser returns the actual username (not root when using sudo).
func ActualUser() string {
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" && sudoUser != "root" {
		return sudoUser
	}
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return "unknown"
}

// ChownToActualUser chowns paths to SUDO_USER (or no-ops when not root / no
// target user). Used so a root daemon writing under the user's config dir
// leaves files the interactive user can also write (CLI scan, etc.).
func ChownToActualUser(paths ...string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	name := ActualUser()
	if name == "" || name == "root" || name == "unknown" {
		return nil
	}
	u, err := user.Lookup(name)
	if err != nil {
		return err
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return err
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return err
	}
	var first error
	for _, p := range paths {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			if first == nil {
				first = err
			}
			continue
		}
		if err := os.Chown(p, uid, gid); err != nil && first == nil {
			first = err
		}
	}
	return first
}
