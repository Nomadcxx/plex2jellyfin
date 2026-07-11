package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	configpkg "github.com/Nomadcxx/plex2jellyfin/internal/config"
	setuppkg "github.com/Nomadcxx/plex2jellyfin/internal/setup"
)

func taskNames(tasks []installTask) []string {
	names := make([]string, 0, len(tasks))
	for _, task := range tasks {
		names = append(names, task.name)
	}
	return names
}

func hasTask(tasks []installTask, name string) bool {
	for _, task := range tasks {
		if task.name == name {
			return true
		}
	}
	return false
}

func TestStartInstallation_PluginTasksGatedByConsent(t *testing.T) {
	cases := []struct {
		name            string
		jellyfin        bool
		install         bool
		restart         bool
		serviceStartNow bool
		wantInstall     bool
		wantRestart     bool
		wantConfigure   bool
	}{
		{"full consent", true, true, true, true, true, true, true},
		{"jellyfin disabled", false, true, true, true, false, false, false},
		{"install declined", true, false, true, true, false, false, false},
		{"restart declined", true, true, false, true, true, false, false},
		{"no listener started", true, true, true, false, true, true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := model{
				serviceEnabled:  true,
				serviceStartNow: tc.serviceStartNow,
				webEnabled:      false,
				jellyfinEnabled: tc.jellyfin,
				jellyfinURL:     "http://localhost:8096",
				jellyfinAPIKey:  "k",
				webhookSecret:   "s",
				pluginInstall:   tc.install,
				pluginRestart:   tc.restart,
				pluginDaemonURL: "http://10.0.0.5:5522",
			}

			next, _ := m.startInstallation()
			got := next.(model)

			if hasTask(got.tasks, "Install Jellyfin plugin") != tc.wantInstall {
				t.Errorf("install task presence = %v, want %v (tasks: %v)",
					!tc.wantInstall, tc.wantInstall, taskNames(got.tasks))
			}
			if hasTask(got.tasks, "Restart Jellyfin") != tc.wantRestart {
				t.Errorf("restart task presence = %v, want %v", !tc.wantRestart, tc.wantRestart)
			}
			if hasTask(got.postScanTasks, "Configure plugin feedback loop") != tc.wantConfigure {
				t.Errorf("configure task presence = %v, want %v (postScan: %v)",
					!tc.wantConfigure, tc.wantConfigure, taskNames(got.postScanTasks))
			}
			if got.pluginState == nil {
				t.Fatal("pluginState must always be initialized on fresh install")
			}
		})
	}
}

func TestStartInstallation_ResolvesPluginPrerequisitesUpFront(t *testing.T) {
	m := model{
		serviceEnabled:  true,
		jellyfinEnabled: true,
		jellyfinURL:     "http://localhost:8096",
		jellyfinAPIKey:  "k",
		pluginInstall:   true,
		pluginRestart:   true,
	}

	next, _ := m.startInstallation()
	got := next.(model)

	if got.webhookSecret == "" {
		t.Error("webhook secret must be generated before the pipeline forks goroutines")
	}
	if got.pluginDaemonURL == "" {
		t.Error("pluginDaemonURL must be derived before the pipeline forks goroutines")
	}
	if got.pluginState.outcome != "needs-restart" {
		t.Errorf("initial outcome = %q, want needs-restart", got.pluginState.outcome)
	}
}

func TestStartInstallation_PluginSkippedOutcomeWhenDeclined(t *testing.T) {
	m := model{serviceEnabled: true, jellyfinEnabled: true, pluginInstall: false}
	next, _ := m.startInstallation()
	if got := next.(model); got.pluginState.outcome != "skipped" {
		t.Errorf("outcome = %q, want skipped", got.pluginState.outcome)
	}
}

func TestInstallJellyfinPlugin_ServerErrorMarksFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	m := &model{
		jellyfinURL:    srv.URL,
		jellyfinAPIKey: "k",
		pluginState:    &pluginRunState{outcome: "needs-restart"},
	}

	if err := installJellyfinPlugin(m); err == nil {
		t.Fatal("expected error from erroring server")
	}
	if m.pluginState.outcome != "failed" {
		t.Errorf("outcome = %q, want failed", m.pluginState.outcome)
	}
}

func TestGenerateConfigString_IncludesSetupMarker(t *testing.T) {
	m := &model{
		watchFolders: []WatchFolder{
			{Type: "movies", Paths: "/watch/movies"},
			{Type: "tv", Paths: "/watch/tv"},
		},
		movieLibraryPaths: "/lib/movies",
		tvLibraryPaths:    "/lib/tv",
		serviceEnabled:    true,
	}

	configStr, err := m.generateConfigString()
	if err != nil {
		t.Fatalf("generateConfigString() error = %v", err)
	}

	if !strings.Contains(configStr, "[setup]") {
		t.Fatalf("expected [setup] block, got:\n%s", configStr)
	}
	if !strings.Contains(configStr, fmt.Sprintf("version = %d", setuppkg.CurrentVersion)) {
		t.Fatalf("expected setup version %d, got:\n%s", setuppkg.CurrentVersion, configStr)
	}
	if !strings.Contains(configStr, "completed = true") {
		t.Fatalf("expected completed = true, got:\n%s", configStr)
	}
}

func TestGenerateConfigString_IncludesPluginFields(t *testing.T) {
	m := &model{
		watchFolders:    []WatchFolder{{Type: "tv", Paths: "/watch/tv"}},
		tvLibraryPaths:  "/lib/tv",
		jellyfinEnabled: true,
		jellyfinURL:     "http://localhost:8096",
		jellyfinAPIKey:  "jf-api",
		webhookSecret:   "secret-1",
		pluginInstall:   true,
		pluginDaemonURL: "http://192.168.0.10:5522",
	}

	configStr, err := m.generateConfigString()
	if err != nil {
		t.Fatalf("generateConfigString() error = %v", err)
	}

	if !strings.Contains(configStr, "plugin_enabled = true") {
		t.Fatalf("expected plugin_enabled = true, got:\n%s", configStr)
	}
	if !strings.Contains(configStr, `plugin_daemon_url = "http://192.168.0.10:5522"`) {
		t.Fatalf("expected plugin_daemon_url, got:\n%s", configStr)
	}
}

// The TUI writes TOML by hand; this guards the silent-field-loss trap by
// parsing the emitted string through the real config loader.
func TestGenerateConfigString_RoundTripsThroughConfigLoad(t *testing.T) {
	m := &model{
		watchFolders:    []WatchFolder{{Type: "tv", Paths: "/watch/tv"}},
		tvLibraryPaths:  "/lib/tv",
		jellyfinEnabled: true,
		jellyfinURL:     "http://localhost:8096",
		jellyfinAPIKey:  "jf-api",
		webhookSecret:   "secret-1",
		pluginInstall:   true,
		pluginDaemonURL: "http://192.168.0.10:5522",
	}

	configStr, err := m.generateConfigString()
	if err != nil {
		t.Fatalf("generateConfigString() error = %v", err)
	}

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, ".config"))
	t.Setenv("SUDO_USER", "")
	dir := filepath.Join(tmp, ".config", "plex2jellyfin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(configStr), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := configpkg.Load()
	if err != nil {
		t.Fatalf("config.Load() on generated config: %v", err)
	}
	if cfg.Setup.Version != setuppkg.CurrentVersion || !cfg.Setup.Completed {
		t.Errorf("setup marker did not round-trip: %+v", cfg.Setup)
	}
	if !cfg.Jellyfin.PluginEnabled {
		t.Error("plugin_enabled did not round-trip")
	}
	if cfg.Jellyfin.PluginDaemonURL != "http://192.168.0.10:5522" {
		t.Errorf("plugin_daemon_url did not round-trip: %q", cfg.Jellyfin.PluginDaemonURL)
	}
}

func TestStartInstallation_WebUIDisabledSkipsWebTasks(t *testing.T) {
	m := model{
		serviceEnabled: true,
		webEnabled:     false,
	}

	next, _ := m.startInstallation()
	got := next.(model)

	for _, task := range got.postScanTasks {
		if strings.Contains(strings.ToLower(task.name), "web service") {
			t.Fatalf("expected no web service tasks when web UI disabled, found task %q", task.name)
		}
	}
}

func TestStartInstallation_UpdateModeRefreshesSystemdUnits(t *testing.T) {
	m := model{
		updateMode: true,
	}

	next, _ := m.startInstallation()
	got := next.(model)

	found := false
	for _, task := range got.tasks {
		if task.name == "Refresh systemd" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected update mode to refresh installed systemd units, got tasks: %#v", got.tasks)
	}
}

func TestBuildWebServiceUnit_UsesConfiguredPort(t *testing.T) {
	unit := buildWebServiceUnit("nomadx", "18080")
	if !strings.Contains(unit, "ExecStart=/usr/bin/plex2jellyfin-web --host 0.0.0.0 --port 18080") {
		t.Fatalf("expected configured port in service unit, got:\n%s", unit)
	}
	if !strings.Contains(unit, "Environment=SUDO_USER=nomadx") {
		t.Fatalf("expected SUDO_USER in service unit, got:\n%s", unit)
	}
}

func TestBuildWebServiceUnit_AllowsWebToWriteLibraryMounts(t *testing.T) {
	unit := buildWebServiceUnit("nomadx", "5522")
	if !strings.Contains(unit, "ProtectSystem=full") {
		t.Fatalf("expected service unit to protect system paths without making media mounts read-only, got:\n%s", unit)
	}
	if strings.Contains(unit, "ReadWritePaths=/mnt") || strings.Contains(unit, "ReadWritePaths=/home /mnt") {
		t.Fatalf("expected service unit not to hardcode media mount roots, got:\n%s", unit)
	}
	if regexp.MustCompile(`(?m)^ProtectHome=`).MatchString(unit) {
		t.Fatalf("expected ProtectHome to remain unset so config/database writes work, got:\n%s", unit)
	}
}

func TestExistingWebServicePortPreservesCustomPort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plex2jellyfin-web.service")
	unit := buildWebServiceUnit("nomadx", "18080")
	if err := os.WriteFile(path, []byte(unit), 0600); err != nil {
		t.Fatalf("failed to write test unit: %v", err)
	}

	if got := existingWebServicePort(path, "5522"); got != "18080" {
		t.Fatalf("existingWebServicePort = %q, want 18080", got)
	}
}

func TestExistingWebServicePortFallsBackWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.service")

	if got := existingWebServicePort(path, "5522"); got != "5522" {
		t.Fatalf("existingWebServicePort = %q, want fallback 5522", got)
	}
}

func TestGenerateConfigString_IncludesJellyfinWebhookSecret(t *testing.T) {
	m := &model{
		watchFolders: []WatchFolder{
			{Type: "movies", Paths: "/watch/movies"},
			{Type: "tv", Paths: "/watch/tv"},
		},
		movieLibraryPaths: "/lib/movies",
		tvLibraryPaths:    "/lib/tv",
		serviceEnabled:    true,
		scanFrequency:     0,
		permUser:          "nomadx",
		permGroup:         "media",
		permFileMode:      "0664",
		permDirMode:       "0775",
		jellyfinEnabled:   true,
		jellyfinURL:       "http://localhost:8096",
		jellyfinAPIKey:    "jf-api",
		webhookSecret:     "my-explicit-secret",
	}

	configStr, err := m.generateConfigString()
	if err != nil {
		t.Fatalf("generateConfigString() error = %v", err)
	}

	if !strings.Contains(configStr, `webhook_secret = "my-explicit-secret"`) {
		t.Fatalf("expected explicit webhook secret in config, got:\n%s", configStr)
	}
	if !strings.Contains(configStr, `plugin_shared_secret = "my-explicit-secret"`) {
		t.Fatalf("expected companion plugin secret to match webhook secret in config, got:\n%s", configStr)
	}
	if !strings.Contains(configStr, "same value used for X-Plex2Jellyfin-Webhook-Secret") {
		t.Fatalf("expected config comments to document webhook header contract, got:\n%s", configStr)
	}
}

func TestGenerateConfigString_AutoGeneratesWebhookSecret(t *testing.T) {
	m := &model{
		watchFolders: []WatchFolder{
			{Type: "movies", Paths: "/watch/movies"},
			{Type: "tv", Paths: "/watch/tv"},
		},
		movieLibraryPaths: "/lib/movies",
		tvLibraryPaths:    "/lib/tv",
		serviceEnabled:    true,
		scanFrequency:     0,
		permUser:          "nomadx",
		permGroup:         "media",
		permFileMode:      "0664",
		permDirMode:       "0775",
		jellyfinEnabled:   true,
		jellyfinURL:       "http://localhost:8096",
		jellyfinAPIKey:    "jf-api",
		webhookSecret:     "",
	}

	configStr, err := m.generateConfigString()
	if err != nil {
		t.Fatalf("generateConfigString() error = %v", err)
	}

	if strings.Contains(configStr, `webhook_secret = ""`) {
		t.Fatalf("expected generated webhook secret, got empty value:\n%s", configStr)
	}

	re := regexp.MustCompile(`webhook_secret = "([a-f0-9]{64})"`)
	matches := re.FindStringSubmatch(configStr)
	if len(matches) != 2 {
		t.Fatalf("expected generated 64-char hex webhook secret, got:\n%s", configStr)
	}
	if m.webhookSecret != matches[1] {
		t.Fatalf("expected model webhookSecret to match generated value")
	}
	if !strings.Contains(configStr, `plugin_shared_secret = "`+matches[1]+`"`) {
		t.Fatalf("expected companion plugin secret to match generated webhook secret, got:\n%s", configStr)
	}
}

func TestGenerateConfigString_IncludesMetadataRecoveryBlock(t *testing.T) {
	m := &model{
		watchFolders: []WatchFolder{
			{Type: "movies", Paths: "/watch/movies"},
			{Type: "tv", Paths: "/watch/tv"},
		},
		movieLibraryPaths: "/lib/movies",
		tvLibraryPaths:    "/lib/tv",
		serviceEnabled:    true,
		scanFrequency:     0,
		permUser:          "nomadx",
		permGroup:         "media",
		permFileMode:      "0664",
		permDirMode:       "0775",
	}

	configStr, err := m.generateConfigString()
	if err != nil {
		t.Fatalf("generateConfigString() error = %v", err)
	}

	for _, want := range []string{
		"[metadata_recovery]",
		"passive_enabled = true",
		"repair_enabled = false",
		"passive_interval_minutes = 60",
		"passive_batch_size = 25",
		"repair_batch_size = 5",
		"repair_cooldown_hours = 6",
		"needs_review_after = 4",
	} {
		if !strings.Contains(configStr, want) {
			t.Fatalf("expected generated config to contain %q, got:\n%s", want, configStr)
		}
	}
}

func TestDefaultConfig_MetadataRecoveryDefaults(t *testing.T) {
	cfg := configpkg.DefaultConfig()

	if !cfg.MetadataRecovery.PassiveEnabled {
		t.Error("expected metadata recovery passive_enabled default true")
	}
	if cfg.MetadataRecovery.RepairEnabled {
		t.Error("expected metadata recovery repair_enabled default false")
	}
	if cfg.MetadataRecovery.PassiveIntervalMinutes != 60 {
		t.Errorf("expected passive_interval_minutes 60, got %d", cfg.MetadataRecovery.PassiveIntervalMinutes)
	}
	if cfg.MetadataRecovery.PassiveBatchSize != 25 {
		t.Errorf("expected passive_batch_size 25, got %d", cfg.MetadataRecovery.PassiveBatchSize)
	}
	if cfg.MetadataRecovery.RepairBatchSize != 5 {
		t.Errorf("expected repair_batch_size 5, got %d", cfg.MetadataRecovery.RepairBatchSize)
	}
	if cfg.MetadataRecovery.RepairCooldownHours != 6 {
		t.Errorf("expected repair_cooldown_hours 6, got %d", cfg.MetadataRecovery.RepairCooldownHours)
	}
	if cfg.MetadataRecovery.NeedsReviewAfter != 4 {
		t.Errorf("expected needs_review_after 4, got %d", cfg.MetadataRecovery.NeedsReviewAfter)
	}
}
