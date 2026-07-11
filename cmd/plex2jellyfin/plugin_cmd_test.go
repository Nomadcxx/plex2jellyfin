package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin/plugininstall"
)

// fakeEngine scripts the engine for command tests.
type fakeEngine struct {
	inspection   plugininstall.Inspection
	inspectErr   error
	repoAdded    bool
	installed    bool
	restarted    bool
	waited       bool
	configured   []string // daemonURL, secret
	verifyResult plugininstall.VerifyResult
	verifyErr    error
}

func (f *fakeEngine) Inspect(ctx context.Context) (*plugininstall.Inspection, error) {
	if f.inspectErr != nil {
		return nil, f.inspectErr
	}
	insp := f.inspection
	return &insp, nil
}
func (f *fakeEngine) RegisterRepo(ctx context.Context) (bool, error) {
	f.repoAdded = true
	return true, nil
}
func (f *fakeEngine) Install(ctx context.Context) error { f.installed = true; return nil }
func (f *fakeEngine) Restart(ctx context.Context) error { f.restarted = true; return nil }
func (f *fakeEngine) WaitReady(ctx context.Context, timeout time.Duration) error {
	f.waited = true
	return nil
}
func (f *fakeEngine) Configure(ctx context.Context, daemonURL, secret string) error {
	f.configured = []string{daemonURL, secret}
	return nil
}
func (f *fakeEngine) Verify(ctx context.Context) (*plugininstall.VerifyResult, error) {
	if f.verifyErr != nil {
		return nil, f.verifyErr
	}
	res := f.verifyResult
	return &res, nil
}

func pluginTestConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Jellyfin.Enabled = true
	cfg.Jellyfin.URL = "http://jellyfin.local:8096"
	cfg.Jellyfin.APIKey = "key"
	cfg.Jellyfin.WebhookSecret = "s3cret"
	cfg.Jellyfin.PluginDaemonURL = "http://10.0.0.5:5522"
	return cfg
}

func pluginTestDeps(engine *fakeEngine, cfg *config.Config) pluginDeps {
	return pluginDeps{
		loadConfig:  func() (*config.Config, error) { return cfg, nil },
		saveConfig:  func(c *config.Config) error { return nil },
		newEngine:   func(baseURL, apiKey string) pluginEngine { return engine },
		advertiseIP: func() string { return "10.0.0.5" },
	}
}

func TestPluginInstallHappyPathWithRestartConsent(t *testing.T) {
	engine := &fakeEngine{
		inspection:   plugininstall.Inspection{ServerVersion: "10.11.6", ABISupported: true},
		verifyResult: plugininstall.VerifyResult{Sent: true, DaemonStatusCode: 200, Authenticated: true},
	}
	var out bytes.Buffer
	err := runPluginInstall(context.Background(), pluginTestDeps(engine, pluginTestConfig()),
		strings.NewReader("y\n"), &out) // single prompt: restart consent
	if err != nil {
		t.Fatalf("install: %v\n%s", err, out.String())
	}
	if !engine.repoAdded || !engine.installed || !engine.restarted || !engine.waited {
		t.Errorf("stage skipped: %+v", engine)
	}
	if len(engine.configured) != 2 || engine.configured[0] != "http://10.0.0.5:5522" || engine.configured[1] != "s3cret" {
		t.Errorf("configure args: %v", engine.configured)
	}
	if !strings.Contains(out.String(), "verified") {
		t.Errorf("expected verified line:\n%s", out.String())
	}
}

func TestPluginInstallDecliningRestartPrintsRecovery(t *testing.T) {
	engine := &fakeEngine{
		inspection: plugininstall.Inspection{ServerVersion: "10.11.6", ABISupported: true},
	}
	var out bytes.Buffer
	err := runPluginInstall(context.Background(), pluginTestDeps(engine, pluginTestConfig()),
		strings.NewReader("n\n"), &out)
	if err != nil {
		t.Fatalf("declining restart must not error: %v", err)
	}
	if engine.restarted {
		t.Error("restarted without consent")
	}
	if !strings.Contains(out.String(), "plex2jellyfin plugin verify") {
		t.Errorf("expected recovery pointer:\n%s", out.String())
	}
}

func TestPluginInstallRefusesUnsupportedABI(t *testing.T) {
	engine := &fakeEngine{
		inspection: plugininstall.Inspection{ServerVersion: "10.10.7", ABISupported: false},
	}
	var out bytes.Buffer
	err := runPluginInstall(context.Background(), pluginTestDeps(engine, pluginTestConfig()),
		strings.NewReader(""), &out)
	if err == nil {
		t.Fatal("expected ABI gate error")
	}
	if engine.installed {
		t.Error("must not install on unsupported servers")
	}
}

func TestPluginVerifyFailsOnUnauthenticated(t *testing.T) {
	engine := &fakeEngine{
		inspection:   plugininstall.Inspection{ServerVersion: "10.11.6", ABISupported: true, PluginResponding: true},
		verifyResult: plugininstall.VerifyResult{Sent: true, DaemonStatusCode: 401, Authenticated: false},
	}
	var out bytes.Buffer
	err := runPluginVerify(context.Background(), pluginTestDeps(engine, pluginTestConfig()), &out)
	if err == nil {
		t.Fatal("verify must fail when the daemon rejects the secret")
	}
}

func TestPluginInstallRequiresJellyfinConfig(t *testing.T) {
	cfg := config.DefaultConfig() // jellyfin disabled
	engine := &fakeEngine{}
	var out bytes.Buffer
	err := runPluginInstall(context.Background(), pluginTestDeps(engine, cfg), strings.NewReader(""), &out)
	if err == nil || !strings.Contains(err.Error(), "setup") {
		t.Fatalf("expected pointer to setup, got %v", err)
	}
	_ = errors.Is // keep errors import if unused otherwise
}
