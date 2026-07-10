package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/daemon/ipc"
	setupdomain "github.com/Nomadcxx/plex2jellyfin/internal/setup"
)

type setupTestIPC struct {
	calls           []ipc.Command
	failStatusCalls int
	alwaysFail      bool
}

func (s *setupTestIPC) Call(_ context.Context, cmd ipc.Command, _ any) (json.RawMessage, error) {
	s.calls = append(s.calls, cmd)
	if cmd == ipc.CmdStatus {
		if s.alwaysFail || s.failStatusCalls > 0 {
			s.failStatusCalls--
			return nil, errors.New("daemon unavailable")
		}
		return json.RawMessage(`{"state":"running"}`), nil
	}
	if cmd == ipc.CmdReload {
		return json.RawMessage(`{"ok":true,"reloaded":["scanner"]}`), nil
	}
	return nil, nil
}

func (s *setupTestIPC) StreamWithID(context.Context, ipc.Command, any, string) error { return nil }

type setupTestLauncher struct {
	called bool
	err    error
}

func (l *setupTestLauncher) Start() error {
	l.called = true
	return l.err
}

func TestSetupStatusMasksSecretsAndDetectsFreshInstall(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Sonarr.APIKey = "abcdef123456"
	s := NewServer(nil, cfg)
	s.ipc = &setupTestIPC{alwaysFail: true}

	w := httptest.NewRecorder()
	s.GetSetupStatus(w, httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	var got SetupStatusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Required || got.Complete || got.DaemonState != "stopped" {
		t.Fatalf("unexpected setup status: %+v", got)
	}
	if got.Draft.Sonarr.APIKey != "****3456" {
		t.Fatalf("secret was not masked: %q", got.Draft.Sonarr.APIKey)
	}
}

func TestSetupStatusBypassesLegacyConfiguredInstall(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Watch.Movies = []string{"/watch/movies"}
	cfg.Libraries.Movies = []string{"/library/movies"}
	s := NewServer(nil, cfg)
	s.ipc = &setupTestIPC{}

	w := httptest.NewRecorder()
	s.GetSetupStatus(w, httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil))
	var got SetupStatusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Required || !got.Complete {
		t.Fatalf("legacy configured install should bypass setup: %+v", got)
	}
}

func TestApplySetupRejectsUnknownFields(t *testing.T) {
	s := NewServer(nil, config.DefaultConfig())
	w := httptest.NewRecorder()
	s.ApplySetup(w, httptest.NewRequest(http.MethodPost, "/api/v1/setup/apply", bytes.NewBufferString(`{"unknown":true}`)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
}

func TestApplySetupRejectsMissingPathBeforeSaving(t *testing.T) {
	setupTestHome(t)
	cfg := config.DefaultConfig()
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	s := NewServer(nil, cfg)
	draft := validSetupDraft("/missing/watch", t.TempDir())

	w := applySetupRequest(t, s, draft)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Setup.Version != 0 {
		t.Fatalf("invalid draft was saved: %+v", loaded.Setup)
	}
}

func TestApplySetupLaunchesStoppedDaemonAndMarksComplete(t *testing.T) {
	setupTestHome(t)
	cfg := config.DefaultConfig()
	cfg.PasswordHash = "password-hash"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	ipcClient := &setupTestIPC{failStatusCalls: 1}
	launcher := &setupTestLauncher{}
	s := NewServer(nil, cfg)
	s.ipc = ipcClient
	s.launcher = launcher
	s.setupPollInterval = time.Millisecond
	s.setupTimeout = 50 * time.Millisecond

	w := applySetupRequest(t, s, validSetupDraft(t.TempDir(), t.TempDir()))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if !launcher.called {
		t.Fatal("stopped daemon was not launched")
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Setup.Completed || loaded.Setup.Version != setupdomain.CurrentVersion || !loaded.Daemon.Enabled {
		t.Fatalf("setup was not completed: setup=%+v daemon=%+v", loaded.Setup, loaded.Daemon)
	}
	if loaded.PasswordHash != "password-hash" {
		t.Fatalf("password hash was not preserved: %q", loaded.PasswordHash)
	}
}

func TestApplySetupReloadsRunningDaemon(t *testing.T) {
	setupTestHome(t)
	cfg := config.DefaultConfig()
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	ipcClient := &setupTestIPC{}
	launcher := &setupTestLauncher{}
	s := NewServer(nil, cfg)
	s.ipc = ipcClient
	s.launcher = launcher
	s.setupPollInterval = time.Millisecond
	s.setupTimeout = 50 * time.Millisecond

	w := applySetupRequest(t, s, validSetupDraft(t.TempDir(), t.TempDir()))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if launcher.called {
		t.Fatal("running daemon should be reloaded, not launched")
	}
	foundReload := false
	for _, call := range ipcClient.calls {
		foundReload = foundReload || call == ipc.CmdReload
	}
	if !foundReload {
		t.Fatalf("reload was not called: %v", ipcClient.calls)
	}
}

func TestApplySetupActivationFailureLeavesIncompleteMarker(t *testing.T) {
	setupTestHome(t)
	cfg := config.DefaultConfig()
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	s := NewServer(nil, cfg)
	s.ipc = &setupTestIPC{alwaysFail: true}
	s.launcher = &setupTestLauncher{}
	s.setupPollInterval = time.Millisecond
	s.setupTimeout = 5 * time.Millisecond

	w := applySetupRequest(t, s, validSetupDraft(t.TempDir(), t.TempDir()))
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Setup.Version != setupdomain.CurrentVersion || loaded.Setup.Completed {
		t.Fatalf("activation failure should remain incomplete: %+v", loaded.Setup)
	}
}

func applySetupRequest(t *testing.T, s *Server, draft setupdomain.Draft) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.ApplySetup(w, httptest.NewRequest(http.MethodPost, "/api/v1/setup/apply", bytes.NewReader(body)))
	return w
}

func validSetupDraft(watch, library string) setupdomain.Draft {
	return setupdomain.Draft{
		Watch:     setupdomain.PathsDraft{TV: []string{watch}},
		Libraries: setupdomain.PathsDraft{TV: []string{library}},
		Runtime:   setupdomain.RuntimeDraft{ScanFrequency: "5m", DeleteSource: true},
	}
}

func setupTestHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SUDO_USER", "")
	t.Setenv("container", "")
	// Prevent a developer machine's PUID/PGID from affecting native test state.
	t.Setenv("PUID", "")
	t.Setenv("PGID", "")
	if err := os.MkdirAll(filepath.Join(home, ".config", "plex2jellyfin"), 0o700); err != nil {
		t.Fatal(err)
	}
}
