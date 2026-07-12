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
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/daemon/ipc"
	"github.com/Nomadcxx/plex2jellyfin/internal/scanner"
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
	called       bool
	enableCalled bool
	err          error
	enableErr    error
}

func (l *setupTestLauncher) Start() error {
	l.called = true
	return l.err
}

func (l *setupTestLauncher) Enable() error {
	l.enableCalled = true
	return l.enableErr
}

func TestSetupStatusMasksSecretsAndDetectsFreshInstall(t *testing.T) {
	setupTestHome(t)
	cfg := config.DefaultConfig()
	cfg.Sonarr.APIKey = "abcdef123456"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
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
	if !bytes.Contains(w.Body.Bytes(), []byte(`"tv":[]`)) || !bytes.Contains(w.Body.Bytes(), []byte(`"path_mappings":[]`)) {
		t.Fatalf("empty setup collections must be arrays: %s", w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"permissions":{"user":"","group":"","file_mode":"","dir_mode":""}`)) {
		t.Fatalf("permissions must use the documented wire names: %s", w.Body.String())
	}
}

func TestSetupStatusBypassesLegacyConfiguredInstall(t *testing.T) {
	setupTestHome(t)
	cfg := config.DefaultConfig()
	cfg.Watch.Movies = []string{"/watch/movies"}
	cfg.Libraries.Movies = []string{"/library/movies"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
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

func TestSetupStatusPicksUpExternalCompletedStamp(t *testing.T) {
	setupTestHome(t)
	cfg := config.DefaultConfig()
	cfg.Setup.Version = setupdomain.CurrentVersion
	cfg.Setup.Completed = false
	cfg.Watch.TV = []string{t.TempDir()}
	cfg.Libraries.TV = []string{t.TempDir()}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	// Web booted from writeConfig (completed=false) before markSetupComplete.
	mem := *cfg
	s := NewServer(nil, &mem)
	s.ipc = &setupTestIPC{}

	_, err := config.UpdateWithLock(func(c *config.Config) bool {
		c.Setup.Completed = true
		return true
	})
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	s.GetSetupStatus(w, httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil))
	var got SetupStatusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Required || !got.Complete {
		t.Fatalf("disk completed=true must bypass setup after TUI stamp: %+v mem=%+v", got, s.cfg.Setup)
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
	s.setupIndexLibraries = func(context.Context, setupdomain.Draft, func(scanner.ScanProgress)) (*scanner.ScanResult, error) {
		return &scanner.ScanResult{}, nil
	}
	s.setupChownConfig = func(string, string) error { return nil }

	w := applySetupRequest(t, s, validSetupDraft(t.TempDir(), t.TempDir()))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if !launcher.called {
		t.Fatal("stopped daemon was not launched")
	}
	if !launcher.enableCalled {
		t.Fatal("daemon unit was not enabled for boot")
	}
	var resp setupApplyResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Indexing || resp.Complete {
		t.Fatalf("apply with libraries should defer completion: %+v", resp)
	}

	sw := httptest.NewRecorder()
	s.SetupIndexStream(sw, httptest.NewRequest(http.MethodGet, "/api/v1/setup/index/stream", nil))
	if sw.Code != http.StatusOK {
		t.Fatalf("index stream status %d body %s", sw.Code, sw.Body.String())
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
	if !launcher.enableCalled {
		t.Fatal("running daemon should still be enabled for boot")
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

func TestApplySetupDefersIndexingWhenLibrariesConfigured(t *testing.T) {
	setupTestHome(t)
	cfg := config.DefaultConfig()
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	s := NewServer(nil, cfg)
	s.ipc = &setupTestIPC{}
	s.launcher = &setupTestLauncher{}
	s.setupPollInterval = time.Millisecond
	s.setupTimeout = 50 * time.Millisecond

	var indexed bool
	s.setupIndexLibraries = func(context.Context, setupdomain.Draft, func(scanner.ScanProgress)) (*scanner.ScanResult, error) {
		indexed = true
		return &scanner.ScanResult{}, nil
	}

	w := applySetupRequest(t, s, validSetupDraft(t.TempDir(), t.TempDir()))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if indexed {
		t.Fatal("apply must not index synchronously; that belongs to /setup/index/stream")
	}
	var resp setupApplyResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Applied || resp.Complete || !resp.Indexing {
		t.Fatalf("response = %+v", resp)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Setup.Completed {
		t.Fatal("setup must stay incomplete until index stream finishes")
	}
}

func TestSetupIndexStreamIndexesThenChowns(t *testing.T) {
	setupTestHome(t)
	cfg := config.DefaultConfig()
	tv := t.TempDir()
	movies := t.TempDir()
	cfg.Libraries.TV = []string{tv}
	cfg.Libraries.Movies = []string{movies}
	cfg.Permissions.User = "nomadx"
	cfg.Permissions.Group = "media"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	s := NewServer(nil, cfg)

	var indexed, chowned bool
	var gotUser, gotGroup string
	var progressHits int
	s.setupIndexLibraries = func(_ context.Context, d setupdomain.Draft, onProgress func(scanner.ScanProgress)) (*scanner.ScanResult, error) {
		indexed = true
		if len(d.Libraries.TV) == 0 {
			t.Fatal("index called without library paths")
		}
		if onProgress != nil {
			onProgress(scanner.ScanProgress{Library: tv, LibrariesDone: 0, LibrariesTotal: 2, FilesScanned: 3})
			progressHits++
		}
		return &scanner.ScanResult{FilesScanned: 3, FilesAdded: 3, Duration: time.Second}, nil
	}
	s.setupChownConfig = func(user, group string) error {
		chowned = true
		gotUser, gotGroup = user, group
		return nil
	}

	w := httptest.NewRecorder()
	s.SetupIndexStream(w, httptest.NewRequest(http.MethodGet, "/api/v1/setup/index/stream", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if !indexed || !chowned {
		t.Fatalf("indexed=%v chowned=%v", indexed, chowned)
	}
	if progressHits == 0 {
		t.Fatal("expected progress frames")
	}
	if gotUser != "nomadx" || gotGroup != "media" {
		t.Fatalf("chown args = %q:%q", gotUser, gotGroup)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"type":"progress"`) || !strings.Contains(body, `"type":"done"`) {
		t.Fatalf("stream body missing events:\n%s", body)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Setup.Completed {
		t.Fatal("index stream must mark setup complete")
	}
}

func TestSetupIndexStreamAbortsWithoutCompletingOnCancel(t *testing.T) {
	setupTestHome(t)
	cfg := config.DefaultConfig()
	cfg.Libraries.TV = []string{t.TempDir()}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	s := NewServer(nil, cfg)

	started := make(chan struct{})
	s.setupIndexLibraries = func(ctx context.Context, _ setupdomain.Draft, _ func(scanner.ScanProgress)) (*scanner.ScanResult, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	s.setupChownConfig = func(string, string) error {
		t.Fatal("chown must not run when the client cancels mid-index")
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/setup/index/stream", nil)
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.SetupIndexStream(w, req)
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("index never started")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after cancel")
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Setup.Completed {
		t.Fatal("cancelled index must leave setup incomplete")
	}
}

func TestSetupIndexStreamChownsEvenWhenIndexFails(t *testing.T) {
	setupTestHome(t)
	cfg := config.DefaultConfig()
	cfg.Libraries.TV = []string{t.TempDir()}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	s := NewServer(nil, cfg)

	var chowned bool
	s.setupIndexLibraries = func(context.Context, setupdomain.Draft, func(scanner.ScanProgress)) (*scanner.ScanResult, error) {
		return nil, errors.New("scan boom")
	}
	s.setupChownConfig = func(string, string) error {
		chowned = true
		return nil
	}

	w := httptest.NewRecorder()
	s.SetupIndexStream(w, httptest.NewRequest(http.MethodGet, "/api/v1/setup/index/stream", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if !chowned {
		t.Fatal("chown must run even when index fails")
	}
	if !strings.Contains(w.Body.String(), "scan boom") {
		t.Fatalf("expected scan warning in stream:\n%s", w.Body.String())
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Setup.Completed {
		t.Fatal("setup must still complete when index soft-fails")
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
