package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin/plugininstall"
	"github.com/Nomadcxx/plex2jellyfin/internal/service"
	setupdomain "github.com/Nomadcxx/plex2jellyfin/internal/setup"
)

// wizardState is the mutable record scripted fakes write into so tests can
// assert on what the wizard actually did.
type wizardState struct {
	activated        bool
	webStarted       bool
	scanned          bool
	sonarrFixed      bool
	pluginInstalled  bool
	pluginRestarted  bool
	pluginConfigured []string
}

func wizardTestDeps(t *testing.T) (setupDeps, *wizardState) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SUDO_USER", "")

	state := &wizardState{}

	deps := setupDeps{
		loadConfig: config.Load,
		preflight:  func(setupdomain.Draft) []setupdomain.FieldError { return nil },
		testSonarr: func(url, apiKey string) error { return nil },
		testRadarr: func(url, apiKey string) error { return nil },
		testJellyfin: func(url, apiKey string) error {
			return nil
		},
		checkSonarr: func(url, apiKey string) ([]service.HealthIssue, error) {
			return []service.HealthIssue{{
				Service: "sonarr", Setting: "enableCompletedDownloadHandling",
				Current: "true", Expected: "false", Severity: "critical",
			}}, nil
		},
		fixSonarr: func(url, apiKey string, issues []service.HealthIssue) error {
			state.sonarrFixed = true
			return nil
		},
		checkRadarr: func(url, apiKey string) ([]service.HealthIssue, error) { return nil, nil },
		fixRadarr:   func(url, apiKey string, issues []service.HealthIssue) error { return nil },
		listModels:  func(endpoint string) ([]string, error) { return []string{"llama3", "qwen"}, nil },
		activate: func(ctx context.Context) error {
			state.activated = true
			return nil
		},
		activateWeb: func() error {
			state.webStarted = true
			return nil
		},
		webUnitOK: func() bool { return true },
		initialScan: func(ctx context.Context, out io.Writer, draft setupdomain.Draft) error {
			state.scanned = true
			fmt.Fprintln(out, "Initial library scan")
			return nil
		},
		generateToken: func() (string, error) { return "generated-secret", nil },
		runtime:       setupdomain.RuntimeInfo{Kind: setupdomain.RuntimeNative, UID: 1000, GID: 1000},
		pluginEngine: func(url, apiKey string) pluginEngine {
			return &wizardFakeEngine{state: state}
		},
		advertiseIP: func() string { return "10.0.0.5" },
		saveConfig:  func(c *config.Config) error { return c.Save() },
		permDefaults: func() config.PermissionsConfig {
			return config.PermissionsConfig{Group: "media", FileMode: "0664", DirMode: "0775"}
		},
	}
	return deps, state
}

// wizardFakeEngine scripts the companion-plugin engine for wizard tests: it
// records install/restart/configure calls into the shared wizardState.
type wizardFakeEngine struct {
	state *wizardState
}

func (w *wizardFakeEngine) Inspect(ctx context.Context) (*plugininstall.Inspection, error) {
	return &plugininstall.Inspection{
		ServerVersion:    "10.11.6",
		ABISupported:     true,
		InstalledVersion: "",
		PluginResponding: w.state.pluginRestarted,
	}, nil
}
func (w *wizardFakeEngine) RegisterRepo(ctx context.Context) (bool, error) { return true, nil }
func (w *wizardFakeEngine) Install(ctx context.Context) error {
	w.state.pluginInstalled = true
	return nil
}
func (w *wizardFakeEngine) Restart(ctx context.Context) error {
	w.state.pluginRestarted = true
	return nil
}
func (w *wizardFakeEngine) WaitReady(ctx context.Context, d time.Duration) error { return nil }
func (w *wizardFakeEngine) Configure(ctx context.Context, daemonURL, secret string) error {
	w.state.pluginConfigured = []string{daemonURL, secret}
	return nil
}
func (w *wizardFakeEngine) Verify(ctx context.Context) (*plugininstall.VerifyResult, error) {
	return &plugininstall.VerifyResult{Sent: true, DaemonStatusCode: 200, Authenticated: true}, nil
}

// failingEngine makes every companion-plugin engine call fail, to prove the
// wizard degrades to a printed recovery command instead of aborting setup.
type failingEngine struct{}

func (f *failingEngine) Inspect(ctx context.Context) (*plugininstall.Inspection, error) {
	return nil, errors.New("jellyfin exploded")
}
func (f *failingEngine) RegisterRepo(ctx context.Context) (bool, error) { return false, errors.New("no") }
func (f *failingEngine) Install(ctx context.Context) error              { return errors.New("no") }
func (f *failingEngine) Restart(ctx context.Context) error              { return errors.New("no") }
func (f *failingEngine) WaitReady(ctx context.Context, d time.Duration) error {
	return errors.New("no")
}
func (f *failingEngine) Configure(ctx context.Context, u, s string) error { return errors.New("no") }
func (f *failingEngine) Verify(ctx context.Context) (*plugininstall.VerifyResult, error) {
	return nil, errors.New("no")
}

func runWizard(t *testing.T, deps setupDeps, answers []string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := runSetupWizard(context.Background(), deps, strings.NewReader(strings.Join(answers, "\n")+"\n"), &out)
	return out.String(), err
}

func TestSetupWizardShowsBannerAndIntro(t *testing.T) {
	deps, _ := wizardTestDeps(t)
	answers := []string{
		"/downloads/tv", "/media/tv", "", "",
		"n", "n", "n",
		"n",
		"5m", "n", "n",
		"", "", "", "", // permissions (defaults)
		"y", // confirm
		"y", // start web UI
	}
	out, err := runWizard(t, deps, answers)
	if err != nil {
		t.Fatalf("setup: %v\n%s", err, out)
	}
	if !strings.Contains(out, "▄▄▄▄▄") {
		t.Fatalf("expected ASCII banner in setup output:\n%s", out)
	}
	if !strings.Contains(out, "Interactive first-run setup") {
		t.Fatalf("expected intro blurb in setup output:\n%s", out)
	}
	if !strings.Contains(out, "Media paths") {
		t.Fatalf("expected Media paths section:\n%s", out)
	}
	if !strings.Contains(out, "low-confidence titles") {
		t.Fatalf("expected AI guidance text:\n%s", out)
	}
	if !strings.Contains(out, "periodic catch-up") {
		t.Fatalf("expected runtime guidance text:\n%s", out)
	}
	if !strings.Contains(out, "Group is the critical setting") {
		t.Fatalf("expected permissions group guidance:\n%s", out)
	}
	if !strings.Contains(out, "Initial library scan") && !strings.Contains(out, "Setup complete") {
		t.Fatalf("expected setup completion:\n%s", out)
	}
	if strings.Contains(out, "💡") || strings.Contains(out, "✅") {
		t.Fatalf("emoji leaked into setup output:\n%s", out)
	}
}

func TestSetupWizardTVOnlyHappyPath(t *testing.T) {
	deps, state := wizardTestDeps(t)

	answers := []string{
		"/downloads/tv",  // TV incoming
		"/media/tv",      // TV library
		"",               // Movie incoming (skip)
		"",               // Movie library (skip)
		"y",              // connect Sonarr?
		"",               // Sonarr URL (default)
		"sonarr-key",     // Sonarr API key
		"y",              // apply sonarr fixes?
		"n",              // connect Radarr?
		"y",              // connect Jellyfin?
		"",               // Jellyfin URL (default)
		"jf-key",         // Jellyfin API key
		"y",              // install companion plugin?
		"y",              // restart Jellyfin?
		"n",              // use Ollama?
		"10m",            // scan frequency
		"y",              // move files?
		"n",              // verify checksums?
		"",               // owner user
		"",               // owner group (default media)
		"",               // file mode
		"",               // dir mode
		"y",              // confirm write
		"y",              // start web UI
		"",               // webhook URL (accept detected default)
	}
	out, err := runWizard(t, deps, answers)
	if err != nil {
		t.Fatalf("wizard error: %v\n---\n%s", err, out)
	}

	if !state.activated {
		t.Error("daemon was not activated")
	}
	if !state.webStarted {
		t.Error("web UI was not started")
	}
	if !state.scanned {
		t.Error("initial scan was not run")
	}
	if !state.sonarrFixed {
		t.Error("sonarr fix was confirmed but not applied")
	}
	if !state.pluginInstalled || !state.pluginRestarted {
		t.Error("plugin was not installed+restarted during the Jellyfin step")
	}
	if len(state.pluginConfigured) != 2 ||
		state.pluginConfigured[0] != "http://10.0.0.5:5522" ||
		state.pluginConfigured[1] != "generated-secret" {
		t.Errorf("plugin configure args: %v", state.pluginConfigured)
	}

	saved, err := config.Load()
	if err != nil {
		t.Fatalf("load saved config: %v", err)
	}
	if got := saved.Watch.TV; len(got) != 1 || got[0] != "/downloads/tv" {
		t.Errorf("watch.tv = %v", got)
	}
	if len(saved.Watch.Movies) != 0 {
		t.Errorf("watch.movies should be empty, got %v", saved.Watch.Movies)
	}
	if !saved.Sonarr.Enabled || saved.Sonarr.APIKey != "sonarr-key" {
		t.Errorf("sonarr not saved: %+v", saved.Sonarr)
	}
	if saved.Radarr.Enabled {
		t.Error("radarr should stay disabled")
	}
	if !saved.Jellyfin.Enabled || saved.Jellyfin.WebhookSecret != "generated-secret" {
		t.Errorf("jellyfin webhook secret not generated: %+v", saved.Jellyfin)
	}
	if saved.Jellyfin.PluginDaemonURL != "http://10.0.0.5:5522" || !saved.Jellyfin.PluginEnabled {
		t.Errorf("plugin daemon URL not persisted: %+v", saved.Jellyfin)
	}
	if saved.Daemon.ScanFrequency != "10m" || !saved.Options.DeleteSource {
		t.Errorf("runtime settings not saved: %+v %+v", saved.Daemon, saved.Options)
	}
	if saved.Setup.Version != setupdomain.CurrentVersion || !saved.Setup.Completed {
		t.Errorf("setup marker = %+v", saved.Setup)
	}
	if saved.Permissions.User != "" || saved.Permissions.Group != "media" ||
		saved.Permissions.FileMode != "0664" || saved.Permissions.DirMode != "0775" {
		t.Errorf("permissions = %+v, want empty:media 0664/0775", saved.Permissions)
	}
}

func TestSetupWizardDecliningFixLeavesArrAlone(t *testing.T) {
	deps, state := wizardTestDeps(t)

	answers := []string{
		"/downloads/tv", "/media/tv", "", "", // paths
		"y", "", "sonarr-key", // sonarr
		"n",      // decline fixes
		"n", "n", // radarr, jellyfin off
		"n",             // no ollama
		"5m", "n", "n",  // runtime
		"", "", "", "",  // permissions (defaults)
		"y",             // confirm
		"y",             // start web UI
	}
	out, err := runWizard(t, deps, answers)
	if err != nil {
		t.Fatalf("wizard error: %v\n---\n%s", err, out)
	}
	if state.sonarrFixed {
		t.Error("sonarr fix ran without consent")
	}
	if !strings.Contains(out, "health --fix") {
		t.Errorf("expected pointer to health --fix, got:\n%s", out)
	}
}

func TestSetupWizardDecliningWebUIPrintsServiceHints(t *testing.T) {
	deps, state := wizardTestDeps(t)
	answers := []string{
		"/downloads/tv", "/media/tv", "", "",
		"n", "n", "n",
		"n",
		"5m", "n", "n",
		"", "", "", "",
		"y", // confirm
		"n", // decline web UI
	}
	out, err := runWizard(t, deps, answers)
	if err != nil {
		t.Fatalf("setup: %v\n%s", err, out)
	}
	if state.webStarted {
		t.Error("web UI started despite decline")
	}
	if !strings.Contains(out, "systemctl enable --now plex2jellyfin-web") {
		t.Fatalf("expected web start hint:\n%s", out)
	}
	if !strings.Contains(out, "systemctl status plex2jellyfin-daemon plex2jellyfin-web") {
		t.Fatalf("expected status check hint:\n%s", out)
	}
}

func TestSetupWizardFailedActivationLeavesSetupIncomplete(t *testing.T) {
	deps, _ := wizardTestDeps(t)
	deps.activate = func(ctx context.Context) error { return errors.New("no daemon") }

	answers := []string{
		"/downloads/tv", "/media/tv", "", "",
		"n", "n", "n", // no services
		"n",            // no ollama
		"5m", "n", "n", // runtime
		"", "", "", "", // permissions (defaults)
		"y", // confirm
		"y", // start web UI
	}
	out, err := runWizard(t, deps, answers)
	if err == nil {
		t.Fatalf("expected activation error, got success:\n%s", out)
	}

	saved, err := config.Load()
	if err != nil {
		t.Fatalf("load saved config: %v", err)
	}
	if saved.Setup.Version != setupdomain.CurrentVersion || saved.Setup.Completed {
		t.Errorf("setup marker after failed activation = %+v (want version=1 completed=false)", saved.Setup)
	}
}

func TestSetupWizardRejectsHalfConfiguredMedia(t *testing.T) {
	deps, state := wizardTestDeps(t)

	answers := []string{
		"/downloads/tv", "", "", "", // TV incoming without library
		"n", "n", "n",
		"n",
		"5m", "n", "n",
		"", "", "", "", // permissions (still prompted before validate)
	}
	out, err := runWizard(t, deps, answers)
	if err == nil {
		t.Fatalf("expected validation error, got success:\n%s", out)
	}
	if !strings.Contains(out, "incoming and library paths must both be configured") {
		t.Errorf("missing pair validation message:\n%s", out)
	}
	if state.activated {
		t.Error("daemon activated despite invalid draft")
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".config", "plex2jellyfin", "config.toml")); !os.IsNotExist(err) {
		t.Error("config was written despite invalid draft")
	}
}

func TestSetupWizardDeclinedPluginRestartStillCompletesSetup(t *testing.T) {
	deps, state := wizardTestDeps(t)

	answers := []string{
		"/downloads/tv", "/media/tv", "", "",
		"n", "n", // sonarr, radarr off
		"y", "", "jf-key", // jellyfin
		"y",            // install plugin?
		"n",            // restart Jellyfin? (declined)
		"n",            // no ollama
		"5m", "n", "n", // runtime
		"", "", "", "", // permissions (defaults)
		"y", // confirm
		"y", // start web UI
	}
	out, err := runWizard(t, deps, answers)
	if err != nil {
		t.Fatalf("wizard error: %v\n---\n%s", err, out)
	}
	if state.pluginRestarted {
		t.Error("restarted Jellyfin without consent")
	}
	saved, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !saved.Setup.Completed {
		t.Error("setup must complete regardless of the plugin step")
	}
	if !strings.Contains(out, "plex2jellyfin plugin verify") {
		t.Errorf("expected recovery pointer in output:\n%s", out)
	}
}

func TestSetupWizardPluginFailureDoesNotFailSetup(t *testing.T) {
	deps, state := wizardTestDeps(t)
	deps.pluginEngine = func(url, apiKey string) pluginEngine {
		return &failingEngine{}
	}

	answers := []string{
		"/downloads/tv", "/media/tv", "", "",
		"n", "n",
		"y", "", "jf-key",
		"n",            // no ollama
		"5m", "n", "n", // runtime
		"", "", "", "", // permissions (defaults)
		"y", // confirm
		"y", // start web UI
	}
	out, err := runWizard(t, deps, answers)
	if err != nil {
		t.Fatalf("plugin failure must not fail setup: %v\n%s", err, out)
	}
	_ = state
	saved, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !saved.Setup.Completed {
		t.Error("setup must complete when the plugin step errors")
	}
	if !strings.Contains(out, "plex2jellyfin plugin install") {
		t.Errorf("expected install recovery pointer:\n%s", out)
	}
}

func TestSetupWizardAINumberedModelPick(t *testing.T) {
	deps, _ := wizardTestDeps(t)
	answers := []string{
		"/downloads/tv", "/media/tv", "", "",
		"n", "n", "n", // no arr/jellyfin
		"y",            // enable ollama
		"",             // endpoint default
		"2",            // primary = qwen
		"",             // fallback empty
		"5m", "n", "n", // runtime
		"", "", "", "", // permissions (defaults)
		"y",            // confirm
		"y",            // start web UI
	}
	out, err := runWizard(t, deps, answers)
	if err != nil {
		t.Fatalf("setup: %v\n%s", err, out)
	}
	if !strings.Contains(out, "1) llama3") || !strings.Contains(out, "2) qwen") {
		t.Fatalf("expected numbered model list:\n%s", out)
	}
	saved, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !saved.AI.Enabled {
		t.Fatal("AI should be enabled")
	}
	if saved.AI.Model != "qwen" {
		t.Fatalf("primary model = %q, want qwen", saved.AI.Model)
	}
	if saved.AI.FallbackModel != "" {
		t.Fatalf("fallback = %q, want empty", saved.AI.FallbackModel)
	}
}
