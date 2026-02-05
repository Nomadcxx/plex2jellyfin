// Package paths provides sudo-aware path resolution for JellyWatch.
//
// When running with sudo, these functions correctly resolve paths to the
// original user's directories (via SUDO_USER) instead of root's directories.
package paths

import (
	"os"
	"os/user"
	"path/filepath"
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
// If running with sudo, returns the SUDO_USER's config directory, not root's.
// On Linux this is typically ~/.config
func UserConfigDir() (string, error) {
	homeDir, err := UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".config"), nil
}

// JellyWatchDir returns the JellyWatch config directory.
// This is ~/.config/jellywatch for the actual user.
func JellyWatchDir() (string, error) {
	configDir, err := UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "jellywatch"), nil
}

// DatabasePath returns the path to the JellyWatch database.
// This is ~/.config/jellywatch/media.db for the actual user.
func DatabasePath() (string, error) {
	dir, err := JellyWatchDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "media.db"), nil
}

// ConfigPath returns the path to the JellyWatch config file.
// This is ~/.config/jellywatch/config.toml for the actual user.
func ConfigPath() (string, error) {
	dir, err := JellyWatchDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// PlansDir returns the directory for plan files.
// This is ~/.config/jellywatch/plans for the actual user.
func PlansDir() (string, error) {
	dir, err := JellyWatchDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "plans"), nil
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
