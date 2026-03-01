package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	configpkg "github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/radarr"
	"github.com/Nomadcxx/jellywatch/internal/scanner"
	"github.com/Nomadcxx/jellywatch/internal/service"
	"github.com/Nomadcxx/jellywatch/internal/sonarr"
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
		// Handle config/database removal based on user choice
		if !m.keepConfig {
			// Delete everything (config + database)
			m.tasks = append(m.tasks, installTask{
				name:        "Remove config",
				description: "Removing configuration and database",
				execute:     removeConfig,
				status:      statusPending,
			})
		} else if !m.keepDatabase {
			// Keep config but delete database only
			m.tasks = append(m.tasks, installTask{
				name:        "Remove database",
				description: "Removing database (keeping config)",
				execute:     removeDatabase,
				status:      statusPending,
			})
		}
	} else {
		m.tasks = []installTask{
			{name: "Check privileges", description: "Checking root access", execute: checkPrivileges, status: statusPending},
			{name: "Check dependencies", description: "Verifying Go installation", execute: checkDependencies, status: statusPending},
			{name: "Build binaries", description: "Building jellywatch and jellywatchd", execute: buildBinaries, status: statusPending},
			{name: "Install binaries", description: "Installing to /usr/local/bin", execute: installBinaries, status: statusPending},
			{name: "Write config", description: "Writing configuration file", execute: writeConfig, status: statusPending},
			// Scan happens here as a separate step (stepScanning) before systemd setup
		}
		// Add systemd tasks only - scan triggers separately before these
		m.postScanTasks = []installTask{
			{name: "Setup systemd", description: "Installing systemd service", execute: setupSystemd, status: statusPending},
			{name: "Start service", description: "Starting jellywatchd", execute: startService, optional: true, status: statusPending},
		}
		if m.webEnabled {
			m.postScanTasks = append(m.postScanTasks,
				installTask{name: "Setup web service", description: "Installing web UI service", execute: setupWebSystemd, status: statusPending},
				installTask{name: "Start web service", description: "Starting jellyweb", execute: startWebService, optional: true, status: statusPending},
			)
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
	projectRoot, err := resolveInstallerProjectRoot()
	if err != nil {
		return err
	}

	cmds := []struct {
		args []string
		name string
	}{
		{[]string{"go", "build", "-o", "jellywatch", "./cmd/jellywatch"}, "jellywatch"},
		{[]string{"go", "build", "-o", "jellywatchd", "./cmd/jellywatchd"}, "jellywatchd"},
		{[]string{"go", "build", "-o", "jellyweb", "./cmd/jellyweb"}, "jellyweb"},
		{[]string{"go", "build", "-o", "jellywatch-installer", "./cmd/installer"}, "installer"},
	}

	for _, c := range cmds {
		cmd := exec.Command(c.args[0], c.args[1:]...)
		cmd.Dir = projectRoot
		if err := runCommand(c.name, cmd, m.logFile); err != nil {
			return err
		}
	}
	return nil
}

func installBinaries(m *model) error {
	projectRoot, err := resolveInstallerProjectRoot()
	if err != nil {
		return err
	}

	binaries := []string{"jellywatch", "jellywatchd", "jellyweb", "jellywatch-installer"}
	for _, bin := range binaries {
		srcBin := filepath.Join(projectRoot, bin)
		if _, err := os.Stat(srcBin); os.IsNotExist(err) {
			continue
		}
		cmd := exec.Command("install", "-Dm755", srcBin, filepath.Join("/usr/local/bin", bin))
		if err := runCommand("install "+bin, cmd, m.logFile); err != nil {
			return err
		}
		_ = os.Remove(srcBin)
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

	configStr, err := m.generateConfigString()
	if err != nil {
		return err
	}

	configPath := filepath.Join(jellywatchDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(configStr), 0644); err != nil {
		return err
	}

	// Set ownership using actual user and configured group
	actualUser := getActualUser()
	if actualUser != "root" && actualUser != "" {
		group := actualUser
		if m.permGroup != "" {
			group = m.permGroup
		}
		exec.Command("chown", "-R", actualUser+":"+group, jellywatchDir).Run()
	}

	return nil
}

func (m *model) generateConfigString() (string, error) {
	webhookSecret := strings.TrimSpace(m.webhookSecret)
	if m.jellyfinEnabled && webhookSecret == "" {
		generated, err := configpkg.GenerateWebhookSecret()
		if err != nil {
			return "", fmt.Errorf("generating webhook secret: %w", err)
		}
		webhookSecret = generated
		m.webhookSecret = generated
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

	configStr := fmt.Sprintf(`[watch]
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
		configStr += fmt.Sprintf(`
[sonarr]
enabled = true
url = "%s"
api_key = "%s"
`, m.sonarrURL, m.sonarrAPIKey)
	}

	if m.radarrEnabled {
		configStr += fmt.Sprintf(`
[radarr]
enabled = true
url = "%s"
api_key = "%s"
`, m.radarrURL, m.radarrAPIKey)
	}

	if m.jellyfinEnabled {
		configStr += fmt.Sprintf(`
[jellyfin]
enabled = true
url = "%s"
api_key = "%s"
webhook_secret = "%s"
notify_on_import = true
playback_safety = true
verify_after_refresh = false
`, m.jellyfinURL, m.jellyfinAPIKey, webhookSecret)
	}

	if m.aiEnabled && m.aiModel != "" {
		configStr += fmt.Sprintf(`
[ai]
enabled = true
ollama_url = "%s"
model = "%s"
`, m.aiOllamaURL, m.aiModel)
		if m.aiFallbackModel != "" {
			configStr += fmt.Sprintf("fallback_model = \"%s\"\n", m.aiFallbackModel)
		}
	}

	return configStr, nil
}

func setupSystemd(m *model) error {
	if !m.serviceEnabled {
		return nil
	}

	// Get the actual user for SUDO_USER environment variable
	// This ensures paths.UserConfigDir() returns the correct user's config
	// when running as a systemd service (which doesn't set SUDO_USER)
	actualUser := getActualUser()
	if actualUser == "" || actualUser == "root" {
		actualUser = "root" // fallback, though this shouldn't happen in normal install
	}

	serviceContent := fmt.Sprintf(`[Unit]
Description=JellyWatch Media Organizer Daemon
After=network.target

[Service]
Type=simple
User=root
Group=root
Environment=SUDO_USER=%s
ExecStart=/usr/local/bin/jellywatchd
Restart=on-failure
RestartSec=5

# Security settings
PrivateTmp=true

# Restrict capabilities to minimum needed for file ownership changes
CapabilityBoundingSet=CAP_CHOWN CAP_FOWNER CAP_DAC_OVERRIDE
AmbientCapabilities=CAP_CHOWN CAP_FOWNER CAP_DAC_OVERRIDE

[Install]
WantedBy=multi-user.target
`, actualUser)

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

func setupWebSystemd(m *model) error {
	if !m.serviceEnabled || !m.webEnabled {
		return nil
	}

	// Get the actual user for SUDO_USER environment variable
	// This ensures paths.UserConfigDir() returns the correct user's config
	// when running as a systemd service (which doesn't set SUDO_USER)
	actualUser := getActualUser()
	if actualUser == "" || actualUser == "root" {
		actualUser = "root" // fallback, though this shouldn't happen in normal install
	}

	serviceContent := buildWebServiceUnit(actualUser, normalizedWebPort(m.webPort))

	servicePath := "/etc/systemd/system/jellyweb.service"
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return err
	}

	exec.Command("systemctl", "daemon-reload").Run()

	if err := exec.Command("systemctl", "enable", "jellyweb.service").Run(); err != nil {
		return fmt.Errorf("failed to enable web service")
	}

	return nil
}

func startWebService(m *model) error {
	if !m.serviceEnabled || !m.webEnabled || !m.webStartNow {
		return nil
	}

	if err := exec.Command("systemctl", "start", "jellyweb.service").Run(); err != nil {
		return fmt.Errorf("failed to start web service")
	}
	return nil
}

func normalizedWebPort(port string) string {
	p := strings.TrimSpace(port)
	if p == "" {
		return "5522"
	}
	valid := true
	for _, ch := range p {
		if ch < '0' || ch > '9' {
			valid = false
			break
		}
	}
	if !valid {
		return "5522"
	}
	return p
}

func buildWebServiceUnit(actualUser, port string) string {
	return fmt.Sprintf(`[Unit]
Description=JellyWatch Web UI Server
Documentation=https://github.com/Nomadcxx/jellywatch
After=network.target jellywatchd.service
Wants=jellywatchd.service

[Service]
Type=simple
User=root
Group=root
Environment=SUDO_USER=%s
ExecStart=/usr/local/bin/jellyweb --host 0.0.0.0 --port %s
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=jellyweb

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=read-only
ReadWritePaths=/home
PrivateTmp=true

[Install]
WantedBy=multi-user.target
`, actualUser, port)
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

func removeConfig(m *model) error {
	configDir, err := getConfigDir()
	if err != nil {
		return err
	}

	jellywatchDir := filepath.Join(configDir, "jellywatch")

	// Remove the entire jellywatch config directory
	if err := os.RemoveAll(jellywatchDir); err != nil {
		return fmt.Errorf("failed to remove config directory: %v", err)
	}

	return nil
}

func removeDatabase(m *model) error {
	configDir, err := getConfigDir()
	if err != nil {
		return err
	}

	dbPath := filepath.Join(configDir, "jellywatch", "media.db")

	// Remove only the database file
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove database: %v", err)
	}

	// Also remove WAL and SHM files if they exist (SQLite journal files)
	os.Remove(dbPath + "-wal")
	os.Remove(dbPath + "-shm")

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

// runInitialScan runs the library scan with progress updates
func (m model) runInitialScan() tea.Cmd {
	// Capture values before returning the command (closures capture by reference)
	tvLibs := splitPaths(m.tvLibraryPaths)
	movieLibs := splitPaths(m.movieLibraryPaths)
	permGroup := m.permGroup // Configured group for file ownership

	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Send cancel function to model via global program
		if globalProgram != nil {
			globalProgram.Send(scanStartMsg{cancel: cancel})
		}

		// Get database path respecting SUDO_USER
		configDir, err := getConfigDir()
		if err != nil {
			return scanCompleteMsg{err: fmt.Errorf("failed to get config dir: %w", err)}
		}
		dbPath := filepath.Join(configDir, "jellywatch", "media.db")

		db, err := database.OpenPath(dbPath)
		if err != nil {
			return scanCompleteMsg{err: fmt.Errorf("failed to open database: %w", err)}
		}
		defer db.Close()

		fileScanner := scanner.NewFileScanner(db)

		// Run scan with progress callback
		result, err := fileScanner.ScanWithOptions(ctx, scanner.ScanOptions{
			TVLibraries:    tvLibs,
			MovieLibraries: movieLibs,
			OnProgress: func(p scanner.ScanProgress) {
				if globalProgram != nil {
					globalProgram.Send(scanProgressMsg{
						progress: ScanProgress{
							FilesScanned:   p.FilesScanned,
							CurrentPath:    p.CurrentPath,
							LibrariesDone:  p.LibrariesDone,
							LibrariesTotal: p.LibrariesTotal,
						},
					})
				}
			},
		})

		// Fix ownership BEFORE checking scan error - database was already created
		// and we need to ensure the user can access it even if scan failed
		actualUser := getActualUser()
		if actualUser != "root" && actualUser != "" {
			jellywatchDir := filepath.Dir(dbPath)

			// Use configured group if set, otherwise fall back to user's primary group
			group := actualUser
			if permGroup != "" {
				group = permGroup
			}

			ownership := actualUser + ":" + group
			exec.Command("chown", "-R", ownership, jellywatchDir).Run()
		}

		if err != nil {
			return scanCompleteMsg{err: err}
		}

		// Get stats from database
		var stats *ScanStats
		tvCount, _ := db.CountMediaFilesByType("episode")
		movieCount, _ := db.CountMediaFilesByType("movie")

		// Count duplicates
		movieDupes, _ := db.FindDuplicateMovies()
		episodeDupes, _ := db.FindDuplicateEpisodes()
		dupeCount := len(movieDupes) + len(episodeDupes)

		stats = &ScanStats{
			TVShows:         tvCount,
			Movies:          movieCount,
			DuplicateGroups: dupeCount,
		}

		return scanCompleteMsg{
			result: &ScanResult{
				FilesScanned: result.FilesScanned,
				FilesAdded:   result.FilesAdded,
				Duration:     result.Duration,
				Errors:       result.Errors,
			},
			stats: stats,
		}
	}
}

// validateArrSettings checks Sonarr/Radarr configuration for jellywatch compatibility.
func (m model) validateArrSettings() tea.Cmd {
	sonarrEnabled := m.sonarrEnabled
	sonarrURL := m.sonarrURL
	sonarrAPIKey := m.sonarrAPIKey
	radarrEnabled := m.radarrEnabled
	radarrURL := m.radarrURL
	radarrAPIKey := m.radarrAPIKey

	return func() tea.Msg {
		var issues []ArrIssue

		if sonarrEnabled && sonarrURL != "" && sonarrAPIKey != "" {
			client := sonarr.NewClient(sonarr.Config{
				URL:     sonarrURL,
				APIKey:  sonarrAPIKey,
				Timeout: 30 * time.Second,
			})
			svcIssues, err := service.CheckSonarrConfig(client)
			if err == nil {
				for _, i := range svcIssues {
					issues = append(issues, ArrIssue{
						Service:  i.Service,
						Setting:  i.Setting,
						Current:  i.Current,
						Expected: i.Expected,
						Severity: i.Severity,
					})
				}
			}
		}

		if radarrEnabled && radarrURL != "" && radarrAPIKey != "" {
			client := radarr.NewClient(radarr.Config{
				URL:     radarrURL,
				APIKey:  radarrAPIKey,
				Timeout: 30 * time.Second,
			})
			svcIssues, err := service.CheckRadarrConfig(client)
			if err == nil {
				for _, i := range svcIssues {
					issues = append(issues, ArrIssue{
						Service:  i.Service,
						Setting:  i.Setting,
						Current:  i.Current,
						Expected: i.Expected,
						Severity: i.Severity,
					})
				}
			}
		}

		return arrIssuesMsg{issues: issues}
	}
}

// fixArrSettings fixes detected arr configuration issues.
func (m model) fixArrSettings() tea.Cmd {
	sonarrEnabled := m.sonarrEnabled
	sonarrURL := m.sonarrURL
	sonarrAPIKey := m.sonarrAPIKey
	radarrEnabled := m.radarrEnabled
	radarrURL := m.radarrURL
	radarrAPIKey := m.radarrAPIKey
	issues := m.arrIssues

	return func() tea.Msg {
		var fixedCount int

		// Convert ArrIssue to service.HealthIssue
		var sonarrIssues, radarrIssues []service.HealthIssue
		for _, i := range issues {
			hi := service.HealthIssue{
				Service:  i.Service,
				Setting:  i.Setting,
				Current:  i.Current,
				Expected: i.Expected,
				Severity: i.Severity,
			}
			if i.Service == "sonarr" {
				sonarrIssues = append(sonarrIssues, hi)
			} else {
				radarrIssues = append(radarrIssues, hi)
			}
		}

		if sonarrEnabled && len(sonarrIssues) > 0 {
			client := sonarr.NewClient(sonarr.Config{
				URL:     sonarrURL,
				APIKey:  sonarrAPIKey,
				Timeout: 30 * time.Second,
			})
			fixed, _ := service.FixSonarrIssues(client, sonarrIssues, false)
			fixedCount += len(fixed)
		}

		if radarrEnabled && len(radarrIssues) > 0 {
			client := radarr.NewClient(radarr.Config{
				URL:     radarrURL,
				APIKey:  radarrAPIKey,
				Timeout: 30 * time.Second,
			})
			fixed, _ := service.FixRadarrIssues(client, radarrIssues, false)
			fixedCount += len(fixed)
		}

		return arrFixMsg{fixed: fixedCount}
	}
}
