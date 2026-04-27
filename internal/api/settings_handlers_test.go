package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
	"github.com/go-chi/chi/v5"
)

type stubIPC struct {
	called bool
	result json.RawMessage
}

func (s *stubIPC) Call(ctx context.Context, cmd ipc.Command, args any) (json.RawMessage, error) {
	s.called = true
	if s.result != nil {
		return s.result, nil
	}
	return json.RawMessage(`{"ok":true,"reloaded":["ai"]}`), nil
}

func (s *stubIPC) StreamWithID(ctx context.Context, cmd ipc.Command, args any, opID string) error {
	return nil
}

func newTestSettingsRouter(t *testing.T, cfg *config.Config) (*chi.Mux, *stubIPC) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SUDO_USER", "")
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	ipcStub := &stubIPC{}
	h := &SettingsHandlers{Cfg: cfg, IPC: ipcStub}
	r.Get("/settings/{section}", h.Get)
	r.Put("/settings/{section}", h.Put)
	return r, ipcStub
}

func TestGetSectionMasksSecrets(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Sonarr.APIKey = "abcdef1234567890"
	r, _ := newTestSettingsRouter(t, cfg)

	req := httptest.NewRequest("GET", "/settings/sonarr", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["api_key"] != "****7890" {
		t.Errorf("expected masked key, got %q", got["api_key"])
	}
}

func TestPutSectionTriggersReload(t *testing.T) {
	cfg := config.DefaultConfig()
	r, ipcStub := newTestSettingsRouter(t, cfg)
	body := []byte(`{"enabled":true,"ollama_endpoint":"http://x","model":"m"}`)

	req := httptest.NewRequest("PUT", "/settings/ai", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if !ipcStub.called {
		t.Error("expected IPC reload to be called")
	}
	var resp PutSectionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Saved || !resp.Reload.OK {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestPutSectionRestoresConfigOnReloadFailure(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Logging.Level = "info"
	r, ipcStub := newTestSettingsRouter(t, cfg)
	ipcStub.result = json.RawMessage(`{"ok":false,"failed":[{"name":"logging","error":"bad level"}]}`)

	body := []byte(`{"level":"debug","file":"","max_size_mb":10,"max_backups":5,"max_age_days":0,"compress":false}`)
	req := httptest.NewRequest("PUT", "/settings/logging", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	var resp PutSectionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.RestoredPreviousConfig {
		t.Fatalf("expected restored_previous_config: %+v", resp)
	}
	disk, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".config", "jellywatch", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(disk, []byte(`level = "info"`)) {
		t.Fatalf("previous config not restored:\n%s", string(disk))
	}
}

func TestPutSectionPreservesMaskedSecretPlaceholder(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Sonarr.Enabled = true
	cfg.Sonarr.URL = "http://sonarr"
	cfg.Sonarr.APIKey = "abcdef1234567890"
	r, _ := newTestSettingsRouter(t, cfg)

	body := []byte(`{"enabled":true,"url":"http://sonarr","api_key":"****7890","notify_on_import":true}`)
	req := httptest.NewRequest("PUT", "/settings/sonarr", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	disk, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if disk.Sonarr.APIKey != "abcdef1234567890" {
		t.Fatalf("secret was not preserved: %q", disk.Sonarr.APIKey)
	}
}
