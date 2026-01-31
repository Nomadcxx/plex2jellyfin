// cmd/installer/utils.go
package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"
)

// getConfigDir returns the config directory for the actual user (not root)
func getConfigDir() (string, error) {
	// Check SUDO_USER first (running with sudo)
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" && sudoUser != "root" {
		u, err := user.Lookup(sudoUser)
		if err == nil {
			return filepath.Join(u.HomeDir, ".config"), nil
		}
	}

	// Fallback to current user
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".config"), nil
}

// getActualUser returns the actual username (not root when using sudo)
func getActualUser() string {
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" && sudoUser != "root" {
		return sudoUser
	}
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return "unknown"
}

// detectExistingInstall checks for existing JellyWatch installation
func detectExistingInstall() (bool, string, time.Time) {
	configDir, err := getConfigDir()
	if err != nil {
		return false, "", time.Time{}
	}

	dbPath := filepath.Join(configDir, "jellywatch", "media.db")
	info, err := os.Stat(dbPath)
	if err != nil {
		return false, "", time.Time{}
	}

	return true, dbPath, info.ModTime()
}

// isOllamaInstalled checks if ollama binary exists
func isOllamaInstalled() bool {
	_, err := exec.LookPath("ollama")
	return err == nil
}

// isOllamaRunning checks if ollama API is responding
func isOllamaRunning(url string) bool {
	// Quick HTTP check
	cmd := exec.Command("curl", "-s", "-o", "/dev/null", "-w", "%{http_code}",
		fmt.Sprintf("%s/api/tags", url))
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "200"
}

// detectPackageManager returns the system's package manager
func detectPackageManager() string {
	managers := []struct {
		name string
		path string
	}{
		{"pacman", "/usr/bin/pacman"},
		{"apt", "/usr/bin/apt"},
		{"dnf", "/usr/bin/dnf"},
		{"yum", "/usr/bin/yum"},
		{"zypper", "/usr/bin/zypper"},
	}

	for _, pm := range managers {
		if _, err := os.Stat(pm.path); err == nil {
			return pm.name
		}
	}
	return ""
}

// runCommand executes a command and logs to file
func runCommand(name string, cmd *exec.Cmd, logFile *os.File) error {
	if logFile != nil {
		logFile.WriteString(fmt.Sprintf("[%s] Running: %s\n",
			time.Now().Format("15:04:05"), cmd.String()))
	}

	output, err := cmd.CombinedOutput()

	if logFile != nil {
		if len(output) > 0 {
			logFile.Write(output)
			logFile.WriteString("\n")
		}
		if err != nil {
			logFile.WriteString(fmt.Sprintf("[%s] Error: %v\n\n",
				time.Now().Format("15:04:05"), err))
		} else {
			logFile.WriteString(fmt.Sprintf("[%s] Success\n\n",
				time.Now().Format("15:04:05")))
		}
		logFile.Sync()
	}

	return err
}

// splitPaths splits comma-separated paths and trims whitespace
func splitPaths(paths string) []string {
	if strings.TrimSpace(paths) == "" {
		return nil
	}
	parts := strings.Split(paths, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// validatePath checks if a path exists and is a directory
func validatePath(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", path)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}
	return nil
}

type DetectedMediaServer struct {
	Name  string
	User  string
	Group string
}

func detectMediaServer() *DetectedMediaServer {
	candidates := []struct {
		name  string
		user  string
		group string
	}{
		{"jellyfin", "jellyfin", "jellyfin"},
		{"plex", "plex", "plex"},
		{"emby", "emby", "emby"},
	}

	for _, c := range candidates {
		if _, err := user.Lookup(c.user); err == nil {
			return &DetectedMediaServer{Name: c.name, User: c.user, Group: c.group}
		}
	}
	return nil
}

func detectMediaGroup() string {
	candidates := []string{"media", "video", "render"}
	for _, g := range candidates {
		if _, err := user.LookupGroup(g); err == nil {
			return g
		}
	}
	return ""
}

func isUserInGroup(username, groupname string) bool {
	u, err := user.Lookup(username)
	if err != nil {
		return false
	}
	gids, err := u.GroupIds()
	if err != nil {
		return false
	}
	g, err := user.LookupGroup(groupname)
	if err != nil {
		return false
	}
	for _, gid := range gids {
		if gid == g.Gid {
			return true
		}
	}
	return false
}
