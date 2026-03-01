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

	"github.com/Nomadcxx/jellywatch/internal/paths"
)

// getConfigDir returns the config directory for the actual user (not root)
func getConfigDir() (string, error) {
	return paths.UserConfigDir()
}

// getActualUser returns the actual username (not root when using sudo)
func getActualUser() string {
	return paths.ActualUser()
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

// CommandError captures command execution details for error reporting
type CommandError struct {
	Name    string // e.g., "jellywatch"
	Command string // full command string
	Output  string // combined stdout/stderr
	Err     error  // underlying error
}

func (e *CommandError) Error() string {
	if e.Output != "" {
		return fmt.Sprintf("%s: %v\n%s", e.Name, e.Err, e.Output)
	}
	return fmt.Sprintf("%s: %v", e.Name, e.Err)
}

// runCommand executes a command and logs to file.
// Returns *CommandError on failure with full output context.
func runCommand(name string, cmd *exec.Cmd, logFile *os.File) error {
	cmdStr := cmd.String()
	if logFile != nil {
		logFile.WriteString(fmt.Sprintf("[%s] Running: %s\n",
			time.Now().Format("15:04:05"), cmdStr))
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

	if err != nil {
		return &CommandError{
			Name:    name,
			Command: cmdStr,
			Output:  string(output),
			Err:     err,
		}
	}
	return nil
}

// resolveInstallerProjectRoot finds the jellywatch source root for build/install tasks.
func resolveInstallerProjectRoot() (string, error) {
	cwd, _ := os.Getwd()
	exe, _ := os.Executable()
	return resolveProjectRoot(cwd, exe)
}

// resolveProjectRoot resolves project root from cwd and executable location.
func resolveProjectRoot(cwd, execPath string) (string, error) {
	candidates := []string{}
	if strings.TrimSpace(cwd) != "" {
		candidates = append(candidates, cwd)
	}
	if strings.TrimSpace(execPath) != "" {
		exeDir := filepath.Dir(execPath)
		candidates = append(candidates, exeDir, filepath.Dir(exeDir))
	}

	seen := map[string]bool{}
	for _, c := range candidates {
		clean := filepath.Clean(c)
		if seen[clean] {
			continue
		}
		seen[clean] = true
		if root, ok := findProjectRoot(clean); ok {
			return root, nil
		}
	}

	return "", fmt.Errorf("could not locate jellywatch source root (run installer from repository root)")
}

func findProjectRoot(start string) (string, bool) {
	if strings.TrimSpace(start) == "" {
		return "", false
	}
	dir := filepath.Clean(start)
	for {
		if fileExists(filepath.Join(dir, "go.mod")) && fileExists(filepath.Join(dir, "cmd", "jellywatch", "main.go")) {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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
