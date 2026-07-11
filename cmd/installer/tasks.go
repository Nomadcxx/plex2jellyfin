package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	configpkg "github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin/plugininstall"
	"github.com/Nomadcxx/plex2jellyfin/internal/radarr"
	"github.com/Nomadcxx/plex2jellyfin/internal/scanner"
	"github.com/Nomadcxx/plex2jellyfin/internal/service"
	setuppkg "github.com/Nomadcxx/plex2jellyfin/internal/setup"
	"github.com/Nomadcxx/plex2jellyfin/internal/sonarr"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	daemonServicePath = "/etc/systemd/system/plex2jellyfin-daemon.service"
	webServicePath    = "/etc/systemd/system/plex2jellyfin-web.service"
)

func (m model) startInstallation() (tea.Model, tea.Cmd) {
	m.step = stepInstalling

	if m.uninstallMode {
		m.tasks = []installTask{
			{name: "Check privileges", description: "Checking root access", execute: checkPrivileges, status: statusPending},
			{name: "Stop daemon", description: "Stopping plex2jellyfin-daemon service", execute: stopDaemon, status: statusPending},
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
	} else if m.updateMode {
		// Update mode: rebuild binaries and restart services, preserve config
		m.tasks = []installTask{
			{name: "Check privileges", description: "Checking root access", execute: checkPrivileges, status: statusPending},
			{name: "Check dependencies", description: "Verifying Go installation", execute: checkDependencies, status: statusPending},
			{name: "Stop services", description: "Stopping running services", execute: stopRunningServices, status: statusPending},
			{name: "Build binaries", description: "Building plex2jellyfin and plex2jellyfin-daemon", execute: buildBinaries, status: statusPending},
			{name: "Install binaries", description: "Installing to /usr/bin", execute: installBinaries, status: statusPending},
			{name: "Refresh systemd", description: "Updating installed systemd units", execute: refreshSystemdUnits, status: statusPending},
			{name: "Start services", description: "Restarting services", execute: restartServices, status: statusPending},
		}
	} else {
		// Fresh install: full wizard flow
		m.tasks = []installTask{
			{name: "Check privileges", description: "Checking root access", execute: checkPrivileges, status: statusPending},
			{name: "Check dependencies", description: "Verifying Go installation", execute: checkDependencies, status: statusPending},
			{name: "Build binaries", description: "Building plex2jellyfin and plex2jellyfin-daemon", execute: buildBinaries, status: statusPending},
			{name: "Install binaries", description: "Installing to /usr/bin", execute: installBinaries, status: statusPending},
			{name: "Write config", description: "Writing configuration file", execute: writeConfig, status: statusPending},
			// Scan happens here as a separate step (stepScanning) before systemd setup
		}
		// Add systemd tasks only - scan triggers separately before these
		m.postScanTasks = []installTask{
			{name: "Setup systemd", description: "Installing systemd service", execute: setupSystemd, status: statusPending},
			{name: "Start service", description: "Starting plex2jellyfin-daemon", execute: startService, optional: true, status: statusPending},
		}
		if m.webEnabled {
			m.postScanTasks = append(m.postScanTasks,
				installTask{name: "Setup web service", description: "Installing web UI service", execute: setupWebSystemd, status: statusPending},
				installTask{name: "Start web service", description: "Starting plex2jellyfin-web", execute: startWebService, optional: true, status: statusPending},
			)
		}
		m.serviceState = &serviceRunState{}

		// Companion plugin: resolve everything the async pipeline reads up
		// front, in this goroutine, before model copies diverge (see the
		// pluginRunState comment in types.go).
		m.pluginState = &pluginRunState{outcome: "skipped"}
		if m.jellyfinEnabled && m.pluginInstall {
			m.pluginState.outcome = "needs-restart" // upgraded as tasks succeed
			if strings.TrimSpace(m.webhookSecret) == "" {
				if generated, err := configpkg.GenerateWebhookSecret(); err == nil {
					m.webhookSecret = generated
				}
			}
			if strings.TrimSpace(m.pluginDaemonURL) == "" {
				m.pluginDaemonURL = m.defaultCallbackURL()
			}

			m.tasks = append(m.tasks, installTask{
				name:        "Install Jellyfin plugin",
				description: "Registering the plugin repository and installing the companion plugin",
				execute:     installJellyfinPlugin,
				optional:    true,
				status:      statusPending,
			})
			if m.pluginRestart {
				m.tasks = append(m.tasks, installTask{
					name:        "Restart Jellyfin",
					description: "Restarting Jellyfin and waiting for the plugin to load (up to 60s)",
					execute:     restartJellyfinForPlugin,
					optional:    true,
					status:      statusPending,
				})
				// ponytail: custom callback URLs follow the selected web/daemon service;
				// upgrade to probing arbitrary endpoints if external listeners are supported.
				listenerStarts := m.serviceEnabled && ((m.webEnabled && m.webStartNow) ||
					(!m.webEnabled && m.serviceStartNow))
				if listenerStarts {
					m.postScanTasks = append(m.postScanTasks, installTask{
						name:        "Configure plugin feedback loop",
						description: "Pushing the callback URL and secret to the plugin, then verifying a signed test event",
						execute:     configurePluginFeedback,
						optional:    true,
						status:      statusPending,
					})
				}
			}
		}
		// After listeners (and optional plugin configure): stamp setup.completed.
		m.postScanTasks = append(m.postScanTasks, installTask{
			name:        "Mark setup complete",
			description: "Recording setup completion after services start",
			execute:     markSetupComplete,
			optional:    true,
			status:      statusPending,
		})
	}

	m.currentTaskIndex = 0
	m.tasks[0].status = statusRunning
	return m, tea.Batch(m.spinner.Tick, executeTaskCmd(0, &m))
}

const pluginRestartWaitTimeout = 60 * time.Second

func newPluginEngine(m *model) *plugininstall.Engine {
	return plugininstall.New(m.jellyfinURL, m.jellyfinAPIKey, &http.Client{Timeout: 15 * time.Second})
}

func pluginTaskContext(m *model) context.Context {
	if m.ctx != nil {
		return m.ctx
	}
	return context.Background()
}

func installJellyfinPlugin(m *model) error {
	st := m.pluginState
	if st == nil {
		return fmt.Errorf("plugin state missing")
	}
	engine := newPluginEngine(m)
	ctx := pluginTaskContext(m)

	insp, err := engine.Inspect(ctx)
	if err != nil {
		st.outcome = "failed"
		return fmt.Errorf("inspect jellyfin: %w", err)
	}
	if !insp.ABISupported {
		st.outcome = "failed"
		return fmt.Errorf("jellyfin %s does not support the plugin (needs 10.11.x)", insp.ServerVersion)
	}
	if insp.InstalledVersion != "" && insp.PluginResponding {
		st.loaded = true
		st.outcome = "unverified"
		return nil
	}
	if _, err := engine.RegisterRepo(ctx); err != nil {
		st.outcome = "failed"
		return fmt.Errorf("register plugin repository: %w", err)
	}
	if insp.InstalledVersion == "" {
		if err := engine.Install(ctx); err != nil {
			st.outcome = "failed"
			return fmt.Errorf("install plugin: %w", err)
		}
	}
	return nil
}

func restartJellyfinForPlugin(m *model) error {
	st := m.pluginState
	if st == nil {
		return fmt.Errorf("plugin state missing")
	}
	if st.outcome == "failed" {
		return fmt.Errorf("skipping restart: plugin install did not succeed")
	}
	if st.loaded {
		return nil
	}
	engine := newPluginEngine(m)
	ctx := pluginTaskContext(m)

	if err := engine.Restart(ctx); err != nil {
		st.outcome = "failed"
		return fmt.Errorf("restart jellyfin: %w", err)
	}
	if err := engine.WaitReady(ctx, pluginRestartWaitTimeout); err != nil {
		st.outcome = "failed"
		return fmt.Errorf("plugin did not come back after restart: %w", err)
	}
	st.loaded = true
	st.outcome = "unverified"
	return nil
}

func configurePluginFeedback(m *model) error {
	st := m.pluginState
	if st == nil {
		return fmt.Errorf("plugin state missing")
	}
	if !st.listenerReady {
		return nil
	}
	if !st.loaded {
		return fmt.Errorf("plugin not loaded; restart Jellyfin, then run: plex2jellyfin plugin verify")
	}
	engine := newPluginEngine(m)
	ctx := pluginTaskContext(m)

	if err := engine.Configure(ctx, m.pluginDaemonURL, m.webhookSecret); err != nil {
		st.outcome = "failed"
		return fmt.Errorf("configure plugin: %w", err)
	}
	res, err := engine.Verify(ctx)
	if err != nil {
		st.outcome = "failed"
		return fmt.Errorf("verify feedback loop: %w", err)
	}
	if !res.Sent || !res.Authenticated {
		st.outcome = "failed"
		detail := res.Error
		if detail == "" {
			detail = fmt.Sprintf("daemon responded %d", res.DaemonStatusCode)
		}
		return fmt.Errorf("test event did not round-trip: %s", detail)
	}
	st.outcome = "verified"
	return nil
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
		{[]string{"go", "build", "-o", "plex2jellyfin", "./cmd/plex2jellyfin"}, "plex2jellyfin"},
		{[]string{"go", "build", "-o", "plex2jellyfin-daemon", "./cmd/plex2jellyfin-daemon"}, "plex2jellyfin-daemon"},
		{[]string{"go", "build", "-o", "plex2jellyfin-web", "./cmd/plex2jellyfin-web"}, "plex2jellyfin-web"},
		{[]string{"go", "build", "-o", "plex2jellyfin-installer", "./cmd/installer"}, "installer"},
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

	binaries := []string{"plex2jellyfin", "plex2jellyfin-daemon", "plex2jellyfin-web", "plex2jellyfin-installer"}
	for _, bin := range binaries {
		srcBin := filepath.Join(projectRoot, bin)
		if _, err := os.Stat(srcBin); os.IsNotExist(err) {
			continue
		}
		cmd := exec.Command("install", "-Dm755", srcBin, filepath.Join("/usr/bin", bin))
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

	plex2jellyfinDir := filepath.Join(configDir, "plex2jellyfin")
	if err := os.MkdirAll(plex2jellyfinDir, 0700); err != nil {
		return err
	}

	configStr, err := m.generateConfigString()
	if err != nil {
		return err
	}

	configPath := filepath.Join(plex2jellyfinDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(configStr), 0600); err != nil {
		return err
	}

	// Set ownership using actual user and configured group
	actualUser := getActualUser()
	if actualUser != "root" && actualUser != "" {
		group := actualUser
		if m.permGroup != "" {
			group = m.permGroup
		}
		if err := exec.Command("chown", "-R", actualUser+":"+group, plex2jellyfinDir).Run(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to chown config dir %s: %v\n", plex2jellyfinDir, err)
		}
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

	configStr := fmt.Sprintf(`[setup]
# Managed by the setup wizards. completed = true tells the web UI to skip
# its guided setup and only ask for a password. Written false until the
# selected listeners start successfully (mirrors CLI/web two-phase setup).
version = %d
completed = false

[watch]
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

[metadata_recovery]
# Passive recovery checks Jellyfin for metadata that arrives after import.
passive_enabled = true
# Active repair is disabled by default because it asks Jellyfin to refresh items.
repair_enabled = false
passive_interval_minutes = 60
passive_batch_size = 25
repair_batch_size = 5
repair_cooldown_hours = 6
needs_review_after = 4
`,
		setuppkg.CurrentVersion,
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
# Jellyfin webhook requests must send this same value in the
# X-Plex2Jellyfin-Webhook-Secret header.
webhook_secret = "%s"
# Companion plugin shared secret; keep this the same value used for X-Plex2Jellyfin-Webhook-Secret.
plugin_shared_secret = "%s"
plugin_enabled = %t
# Base URL the companion plugin calls back to (the plugin appends
# /api/v1/webhooks/jellyfin). From Jellyfin's point of view - never
# localhost when Jellyfin runs in a container.
plugin_daemon_url = %s
notify_on_import = true
playback_safety = true
verify_after_refresh = false

# If Jellyfin reports paths from a different mount namespace than Plex2Jellyfin,
# add one mapping per mounted library root.
#
# [[jellyfin.path_mappings]]
# jellyfin = "/path/as/jellyfin/sees/it"
# daemon = "/path/as/plex2jellyfin/sees/it"
#
# [[jellyfin.path_mappings]]
# jellyfin = "/another/jellyfin/root"
# daemon = "/another/plex2jellyfin/root"
`, m.jellyfinURL, m.jellyfinAPIKey, webhookSecret, webhookSecret, m.pluginInstall, strconv.Quote(m.pluginDaemonURL))
	}

	if m.aiEnabled && m.aiModel != "" {
		configStr += fmt.Sprintf(`
[ai]
enabled = true
ollama_endpoint = "%s"
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

	serviceContent := buildDaemonServiceUnit(actualUser)

	if err := os.WriteFile(daemonServicePath, []byte(serviceContent), 0600); err != nil {
		return err
	}
	if m.serviceState != nil {
		m.serviceState.daemonUnitWritten = true
	}

	exec.Command("systemctl", "daemon-reload").Run()

	if err := exec.Command("systemctl", "enable", "plex2jellyfin-daemon.service").Run(); err != nil {
		return fmt.Errorf("failed to enable service")
	}

	return nil
}

func buildDaemonServiceUnit(actualUser string) string {
	return fmt.Sprintf(`[Unit]
Description=Plex2Jellyfin Media Organizer Daemon
After=network.target

[Service]
Type=simple
User=root
Group=root
Environment=SUDO_USER=%s
ExecStart=/usr/bin/plex2jellyfin-daemon
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
}

func startService(m *model) error {
	if !m.serviceEnabled || !m.serviceStartNow {
		return nil
	}

	if err := exec.Command("systemctl", "start", "plex2jellyfin-daemon.service").Run(); err != nil {
		return fmt.Errorf("failed to start service")
	}
	if m.serviceState != nil {
		m.serviceState.daemonStarted = true
	}
	if m.pluginState != nil && !m.webEnabled {
		m.pluginState.listenerReady = true
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

	if err := os.WriteFile(webServicePath, []byte(serviceContent), 0600); err != nil {
		return err
	}
	if m.serviceState != nil {
		m.serviceState.webUnitWritten = true
	}

	exec.Command("systemctl", "daemon-reload").Run()

	if err := exec.Command("systemctl", "enable", "plex2jellyfin-web.service").Run(); err != nil {
		return fmt.Errorf("failed to enable web service")
	}

	return nil
}

func startWebService(m *model) error {
	if !m.serviceEnabled || !m.webEnabled || !m.webStartNow {
		return nil
	}

	if err := exec.Command("systemctl", "start", "plex2jellyfin-web.service").Run(); err != nil {
		return fmt.Errorf("failed to start web service")
	}
	if m.serviceState != nil {
		m.serviceState.webStarted = true
	}
	if m.pluginState != nil {
		m.pluginState.listenerReady = true
	}
	return nil
}

// markSetupComplete stamps [setup].completed after selected listeners started
// (or were intentionally left stopped). Failure leaves completed=false so the
// web wizard can recover.
func markSetupComplete(m *model) error {
	st := m.serviceState
	if st == nil {
		return fmt.Errorf("service state missing")
	}
	if m.serviceEnabled && m.serviceStartNow && !st.daemonStarted {
		return fmt.Errorf("daemon did not start; leaving setup incomplete")
	}
	if m.serviceEnabled && m.webEnabled && m.webStartNow && !st.webStarted {
		return fmt.Errorf("web service did not start; leaving setup incomplete")
	}

	_, err := configpkg.UpdateWithLock(func(cfg *configpkg.Config) bool {
		cfg.Setup.Version = setuppkg.CurrentVersion
		cfg.Setup.Completed = true
		return true
	})
	if err != nil {
		return err
	}
	st.setupCompleted = true
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
Description=Plex2Jellyfin Web UI Server
Documentation=https://github.com/Nomadcxx/plex2jellyfin
After=network.target plex2jellyfin-daemon.service
Wants=plex2jellyfin-daemon.service

[Service]
Type=simple
User=root
Group=root
Environment=SUDO_USER=%s
ExecStart=/usr/bin/plex2jellyfin-web --host 0.0.0.0 --port %s
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=plex2jellyfin-web

# Security hardening
NoNewPrivileges=true
ProtectSystem=full
# ProtectHome left unset: plex2jellyfin-web needs RW on the user's config dir
# (config.toml + lock files + media.db live under ~/.config/plex2jellyfin).
# Plex2Jellyfin web also executes user-triggered library maintenance actions such as
# consolidation, so non-system media mounts must remain writable according to
# their normal filesystem permissions rather than a hardcoded allow-list.
PrivateTmp=true

[Install]
WantedBy=multi-user.target
`, actualUser, port)
}

func refreshSystemdUnits(m *model) error {
	daemonExists := pathExists(daemonServicePath)
	webExists := pathExists(webServicePath)
	if !daemonExists && !webExists {
		return nil
	}

	actualUser := getActualUser()
	if actualUser == "" || actualUser == "root" {
		actualUser = "root"
	}

	if daemonExists {
		if err := os.WriteFile(daemonServicePath, []byte(buildDaemonServiceUnit(actualUser)), 0600); err != nil {
			return fmt.Errorf("failed to refresh daemon service unit: %w", err)
		}
	}

	if webExists {
		port := existingWebServicePort(webServicePath, normalizedWebPort(m.webPort))
		if err := os.WriteFile(webServicePath, []byte(buildWebServiceUnit(actualUser, port)), 0600); err != nil {
			return fmt.Errorf("failed to refresh web service unit: %w", err)
		}
	}

	exec.Command("systemctl", "daemon-reload").Run()
	return nil
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func existingWebServicePort(path, fallback string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return fallback
	}
	match := regexp.MustCompile(`--port\s+([0-9]+)`).FindSubmatch(data)
	if len(match) != 2 {
		return fallback
	}
	return string(match[1])
}

// stopRunningServices stops plex2jellyfin-daemon and plex2jellyfin-web before an update
func stopRunningServices(m *model) error {
	// Track what was running so we can restart only those
	m.daemonWasRunning = exec.Command("systemctl", "is-active", "--quiet", "plex2jellyfin-daemon.service").Run() == nil
	m.webEnabled = exec.Command("systemctl", "is-active", "--quiet", "plex2jellyfin-web.service").Run() == nil

	if m.daemonWasRunning {
		if err := exec.Command("systemctl", "stop", "plex2jellyfin-daemon.service").Run(); err != nil {
			return fmt.Errorf("failed to stop plex2jellyfin-daemon: %w", err)
		}
	}
	if m.webEnabled {
		if err := exec.Command("systemctl", "stop", "plex2jellyfin-web.service").Run(); err != nil {
			return fmt.Errorf("failed to stop plex2jellyfin-web: %w", err)
		}
	}
	return nil
}

// restartServices restarts services that were running before the update
func restartServices(m *model) error {
	if m.daemonWasRunning {
		if err := exec.Command("systemctl", "start", "plex2jellyfin-daemon.service").Run(); err != nil {
			return fmt.Errorf("failed to start plex2jellyfin-daemon: %w", err)
		}
	}
	if m.webEnabled {
		if err := exec.Command("systemctl", "start", "plex2jellyfin-web.service").Run(); err != nil {
			return fmt.Errorf("failed to start plex2jellyfin-web: %w", err)
		}
	}
	return nil
}

func stopDaemon(m *model) error {
	exec.Command("systemctl", "stop", "plex2jellyfin-daemon.service").Run()
	return nil
}

func disableService(m *model) error {
	exec.Command("systemctl", "stop", "plex2jellyfin-daemon.service").Run()
	exec.Command("systemctl", "disable", "plex2jellyfin-daemon.service").Run()
	os.Remove("/etc/systemd/system/plex2jellyfin-daemon.service")
	exec.Command("systemctl", "stop", "plex2jellyfin-web.service").Run()
	exec.Command("systemctl", "disable", "plex2jellyfin-web.service").Run()
	os.Remove("/etc/systemd/system/plex2jellyfin-web.service")
	exec.Command("systemctl", "daemon-reload").Run()
	return nil
}

func removeBinaries(m *model) error {
	binaries := []string{
		"/usr/bin/plex2jellyfin",
		"/usr/bin/plex2jellyfin-daemon",
		"/usr/bin/plex2jellyfin-web",
		"/usr/bin/plex2jellyfin-installer",
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

	plex2jellyfinDir := filepath.Join(configDir, "plex2jellyfin")

	// Remove the entire plex2jellyfin config directory
	if err := os.RemoveAll(plex2jellyfinDir); err != nil {
		return fmt.Errorf("failed to remove config directory: %v", err)
	}

	return nil
}

func removeDatabase(m *model) error {
	configDir, err := getConfigDir()
	if err != nil {
		return err
	}

	dbPath := filepath.Join(configDir, "plex2jellyfin", "media.db")

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
		dbPath := filepath.Join(configDir, "plex2jellyfin", "media.db")

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
			plex2jellyfinDir := filepath.Dir(dbPath)

			// Use configured group if set, otherwise fall back to user's primary group
			group := actualUser
			if permGroup != "" {
				group = permGroup
			}

			ownership := actualUser + ":" + group
			if err := exec.Command("chown", "-R", ownership, plex2jellyfinDir).Run(); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to chown database dir %s: %v\n", plex2jellyfinDir, err)
			}
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

// validateArrSettings checks Sonarr/Radarr configuration for plex2jellyfin compatibility.
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
