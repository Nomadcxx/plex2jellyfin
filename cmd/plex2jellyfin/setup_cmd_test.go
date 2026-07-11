package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/service"
	setupdomain "github.com/Nomadcxx/plex2jellyfin/internal/setup"
)

func wizardTestDeps(t *testing.T) (setupDeps, *struct {
	activated   bool
	sonarrFixed bool
}) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SUDO_USER", "")

	state := &struct {
		activated   bool
		sonarrFixed bool
	}{}

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
		generateToken: func() (string, error) { return "generated-secret", nil },
		runtime:       setupdomain.RuntimeInfo{Kind: setupdomain.RuntimeNative, UID: 1000, GID: 1000},
	}
	return deps, state
}

func runWizard(t *testing.T, deps setupDeps, answers []string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := runSetupWizard(context.Background(), deps, strings.NewReader(strings.Join(answers, "\n")+"\n"), &out)
	return out.String(), err
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
		"n",              // use Ollama?
		"10m",            // scan frequency
		"y",              // move files?
		"n",              // verify checksums?
		"",               // chown user (skip)
		"y",              // confirm write
	}
	out, err := runWizard(t, deps, answers)
	if err != nil {
		t.Fatalf("wizard error: %v\n---\n%s", err, out)
	}

	if !state.activated {
		t.Error("daemon was not activated")
	}
	if !state.sonarrFixed {
		t.Error("sonarr fix was confirmed but not applied")
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
	if saved.Daemon.ScanFrequency != "10m" || !saved.Options.DeleteSource {
		t.Errorf("runtime settings not saved: %+v %+v", saved.Daemon, saved.Options)
	}
	if saved.Setup.Version != setupdomain.CurrentVersion || !saved.Setup.Completed {
		t.Errorf("setup marker = %+v", saved.Setup)
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
		"",              // chown skip
		"y",             // confirm
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

func TestSetupWizardFailedActivationLeavesSetupIncomplete(t *testing.T) {
	deps, _ := wizardTestDeps(t)
	deps.activate = func(ctx context.Context) error { return errors.New("no daemon") }

	answers := []string{
		"/downloads/tv", "/media/tv", "", "",
		"n", "n", "n", // no services
		"n",            // no ollama
		"5m", "n", "n", // runtime
		"",  // chown skip
		"y", // confirm
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
		"",
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
