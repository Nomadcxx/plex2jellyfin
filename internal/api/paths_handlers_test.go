package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/go-chi/chi/v5"
)

func newPathsRouter(t *testing.T, cfg *config.Config) *chi.Mux {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SUDO_USER", "")
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	ipcStub := &stubIPC{}
	h := &PathsHandlers{Cfg: cfg, IPC: ipcStub}
	r.Get("/settings/paths/{kind}", h.Get)
	r.Post("/settings/paths/{kind}", h.Add)
	r.Delete("/settings/paths/{kind}/{index}", h.Remove)
	r.Put("/settings/paths/{kind}", h.Replace)
	return r
}

func TestPathWritePreservesLatestSettingsChange(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SUDO_USER", "")
	cfg := config.DefaultConfig()
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	var mu sync.RWMutex
	ipcStub := &stubIPC{}
	settings := &SettingsHandlers{Cfg: cfg, IPC: ipcStub, Mu: &mu}
	paths := &PathsHandlers{Cfg: cfg, IPC: ipcStub, Mu: &mu}

	r := chi.NewRouter()
	r.Put("/settings/{section}", settings.Put)
	r.Put("/settings/paths/{kind}", paths.Replace)

	settingsBody := []byte(`{"level":"debug","file":"","max_size_mb":10,"max_backups":5,"max_age_days":0,"compress":false}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("PUT", "/settings/logging", bytes.NewReader(settingsBody)))
	if w.Code != http.StatusOK {
		t.Fatalf("settings status %d body %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("PUT", "/settings/paths/tv", bytes.NewBufferString(`{"paths":["/watch/tv"]}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("paths status %d body %s", w.Code, w.Body.String())
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Logging.Level != "debug" || len(loaded.Watch.TV) != 1 || loaded.Watch.TV[0] != "/watch/tv" {
		t.Fatalf("later path write lost settings change: logging=%q paths=%v", loaded.Logging.Level, loaded.Watch.TV)
	}
}

func newLibrariesRouter(t *testing.T, cfg *config.Config) *chi.Mux {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SUDO_USER", "")
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	ipcStub := &stubIPC{}
	h := &LibrariesHandlers{Cfg: cfg, IPC: ipcStub}
	r.Get("/settings/libraries/{kind}", h.Get)
	r.Post("/settings/libraries/{kind}", h.Add)
	r.Delete("/settings/libraries/{kind}/{index}", h.Remove)
	r.Put("/settings/libraries/{kind}", h.Replace)
	return r
}

func TestAddTVWatchPath(t *testing.T) {
	cfg := config.DefaultConfig()
	r := newPathsRouter(t, cfg)
	body := []byte(`{"path":"/storage/tv"}`)
	req := httptest.NewRequest("POST", "/settings/paths/tv", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if len(cfg.Watch.TV) != 1 || cfg.Watch.TV[0] != "/storage/tv" {
		t.Errorf("got %v", cfg.Watch.TV)
	}
}

func TestRemoveTVWatchPath(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Watch.TV = []string{"/a", "/b", "/c"}
	r := newPathsRouter(t, cfg)
	req := httptest.NewRequest("DELETE", "/settings/paths/tv/1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status %d", w.Code)
	}
	want := []string{"/a", "/c"}
	for i, p := range cfg.Watch.TV {
		if p != want[i] {
			t.Errorf("got %v", cfg.Watch.TV)
			break
		}
	}
}

func TestReplaceMoviesWatchPaths(t *testing.T) {
	cfg := config.DefaultConfig()
	r := newPathsRouter(t, cfg)
	body, _ := json.Marshal(map[string][]string{"paths": []string{"/x", "/y"}})
	req := httptest.NewRequest("PUT", "/settings/paths/movies", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	if len(cfg.Watch.Movies) != 2 {
		t.Errorf("got %v", cfg.Watch.Movies)
	}
}

func TestAddTVLibrary(t *testing.T) {
	cfg := config.DefaultConfig()
	r := newLibrariesRouter(t, cfg)
	body := []byte(`{"path":"/media/tv"}`)
	req := httptest.NewRequest("POST", "/settings/libraries/tv", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if len(cfg.Libraries.TV) != 1 || cfg.Libraries.TV[0] != "/media/tv" {
		t.Errorf("got %v", cfg.Libraries.TV)
	}
}
