package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) startInstallation() (tea.Model, tea.Cmd) {
	m.step = stepInstalling

	if m.uninstallMode {
		m.tasks = []installTask{
			{name: "Check privileges", description: "Checking root access", execute: checkPrivileges, status: statusPending},
			{name: "Stop daemon", description: "Stopping jellywatchd service", execute: stopDaemon, status: statusPending},
			{name: "Disable service", description: "Disabling systemd service", execute: disableService, status: statusPending},
			{name: "Remove binaries", description: "Removing binaries", execute: removeBinaries, optional: true, status: statusPending},
		}
	} else {
		m.tasks = []installTask{
			{name: "Check privileges", description: "Checking root access", execute: checkPrivileges, status: statusPending},
			{name: "Check dependencies", description: "Verifying Go installation", execute: checkDependencies, status: statusPending},
			{name: "Build binaries", description: "Building jellywatch and jellywatchd", execute: buildBinaries, status: statusPending},
			{name: "Install binaries", description: "Installing to /usr/local/bin", execute: installBinaries, status: statusPending},
			{name: "Write config", description: "Writing configuration file", execute: writeConfig, status: statusPending},
			{name: "Setup systemd", description: "Installing systemd service", execute: setupSystemd, status: statusPending},
			{name: "Start service", description: "Starting jellywatchd", execute: startService, optional: true, status: statusPending},
		}
	}

	m.currentTaskIndex = 0
	m.tasks[0].status = statusRunning
	return m, tea.Batch(m.spinner.Tick, executeTaskCmd(0, &m))
}

func checkPrivileges(m *model) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("root privileges required - run with sudo")
	}
	return nil
}

func checkDependencies(m *model) error {
	if _, err := exec.LookPath("go"); err != nil {
		return fmt.Errorf("Go not found - install Go 1.21+")
	}
	return nil
}

func buildBinaries(m *model) error {
	cmds := []struct {
		args []string
		name string
	}{
		{[]string{"go", "build", "-o", "jellywatch", "./cmd/jellywatch"}, "jellywatch"},
		{[]string{"go", "build", "-o", "jellywatchd", "./cmd/jellywatchd"}, "jellywatchd"},
		{[]string{"go", "build", "-o", "jellywatch-installer", "./cmd/installer"}, "installer"},
	}

	for _, c := range cmds {
		cmd := exec.Command(c.args[0], c.args[1:]...)
		if err := runCommand(c.name, cmd, m.logFile); err != nil {
			return fmt.Errorf("failed to build %s", c.name)
		}
	}
	return nil
}

func installBinaries(m *model) error {
	binaries := []string{"jellywatch", "jellywatchd", "jellywatch-installer"}
	for _, bin := range binaries {
		if _, err := os.Stat(bin); os.IsNotExist(err) {
			continue
		}
		cmd := exec.Command("install", "-Dm755", bin, filepath.Join("/usr/local/bin", bin))
		if err := runCommand("install "+bin, cmd, m.logFile); err != nil {
			return fmt.Errorf("failed to install %s", bin)
		}
		os.Remove(bin)
	}
	return nil
}

func writeConfig(m *model) error {
	configDir, err := getConfigDir()
	if err != nil {
		return err
	}

	jellywatchDir := filepath.Join(configDir, "jellywatch")
	if err := os.MkdirAll(jellywatchDir, 0755); err != nil {
		return err
	}

	var watchTV, watchMovies []string
	for _, wf := range m.watchFolders {
		paths := splitPaths(wf.Paths)
		if wf.Type == "tv" {
			watchTV = append(watchTV, paths...)
		} else {
			watchMovies = append(watchMovies, paths...)
		}
	}

	config := fmt.Sprintf(`[watch]
movies = [%s]
tv = [%s]

[libraries]
movies = [%s]
tv = [%s]

[daemon]
enabled = %t
scan_frequency = "%s"

[options]
dry_run = false
verify_checksums = false
delete_source = true

[permissions]
user = "%s"
group = "%s"
file_mode = "%s"
dir_mode = "%s"
`,
		formatPathList(watchMovies),
		formatPathList(watchTV),
		formatPathList(splitPaths(m.movieLibraryPaths)),
		formatPathList(splitPaths(m.tvLibraryPaths)),
		m.serviceEnabled,
		scanFrequencyToString(m.scanFrequency),
		m.permUser,
		m.permGroup,
		m.permFileMode,
		m.permDirMode,
	)

	if m.sonarrEnabled {
		config += fmt.Sprintf(`
[sonarr]
enabled = true
url = "%s"
api_key = "%s"
`, m.sonarrURL, m.sonarrAPIKey)
	}

	if m.radarrEnabled {
		config += fmt.Sprintf(`
[radarr]
enabled = true
url = "%s"
api_key = "%s"
`, m.radarrURL, m.radarrAPIKey)
	}

	if m.aiEnabled && m.aiModel != "" {
		config += fmt.Sprintf(`
[ai]
enabled = true
ollama_url = "%s"
model = "%s"
`, m.aiOllamaURL, m.aiModel)
	}

	configPath := filepath.Join(jellywatchDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		return err
	}

	actualUser := getActualUser()
	if actualUser != "root" && actualUser != "" {
		exec.Command("chown", "-R", actualUser+":"+actualUser, jellywatchDir).Run()
	}

	return nil
}

func setupSystemd(m *model) error {
	if !m.serviceEnabled {
		return nil
	}

	actualUser := getActualUser()
	configDir, err := getConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	serviceContent := fmt.Sprintf(`[Unit]
Description=JellyWatch Media Organizer Daemon
After=network.target

[Service]
Type=simple
User=%s
ExecStart=/usr/local/bin/jellywatchd
Restart=on-failure
RestartSec=5
Environment=HOME=%s
WorkingDirectory=%s

[Install]
WantedBy=multi-user.target
`, actualUser, filepath.Dir(configDir), filepath.Dir(configDir))

	servicePath := "/etc/systemd/system/jellywatchd.service"
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return err
	}

	exec.Command("systemctl", "daemon-reload").Run()

	if err := exec.Command("systemctl", "enable", "jellywatchd.service").Run(); err != nil {
		return fmt.Errorf("failed to enable service")
	}

	return nil
}

func startService(m *model) error {
	if !m.serviceEnabled || !m.serviceStartNow {
		return nil
	}

	if err := exec.Command("systemctl", "start", "jellywatchd.service").Run(); err != nil {
		return fmt.Errorf("failed to start service")
	}
	return nil
}

func stopDaemon(m *model) error {
	exec.Command("systemctl", "stop", "jellywatchd.service").Run()
	return nil
}

func disableService(m *model) error {
	exec.Command("systemctl", "disable", "jellywatchd.service").Run()
	os.Remove("/etc/systemd/system/jellywatchd.service")
	exec.Command("systemctl", "daemon-reload").Run()
	return nil
}

func removeBinaries(m *model) error {
	binaries := []string{
		"/usr/local/bin/jellywatch",
		"/usr/local/bin/jellywatchd",
		"/usr/local/bin/jellywatch-installer",
	}
	for _, bin := range binaries {
		os.Remove(bin)
	}
	return nil
}

func formatPathList(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	quoted := make([]string, len(paths))
	for i, p := range paths {
		quoted[i] = fmt.Sprintf(`"%s"`, p)
	}
	return strings.Join(quoted, ", ")
}

func scanFrequencyToString(freq int) string {
	switch freq {
	case 0:
		return "5m"
	case 1:
		return "10m"
	case 2:
		return "30m"
	case 3:
		return "1h"
	case 4:
		return "24h"
	default:
		return "5m"
	}
}
