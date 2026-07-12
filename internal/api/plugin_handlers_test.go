package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/api"
	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin/plugininstall"
)

func TestGetJellyfinPluginStatusUnconfigured(t *testing.T) {
	s := &Server{cfg: config.DefaultConfig()}
	w := httptest.NewRecorder()
	s.GetJellyfinPluginStatus(w, httptest.NewRequest(http.MethodGet, "/jellyfin/plugin/status", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var got api.JellyfinPluginStatus
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Healthy == nil || *got.Healthy || got.Message == nil || !strings.Contains(*got.Message, "not configured") {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestGetJellyfinPluginStatusInspects(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/System/Info":
			_, _ = w.Write([]byte(`{"Version":"10.11.0"}`))
		case "/Repositories":
			_, _ = w.Write([]byte(`[{"Name":"x","Url":"` + plugininstall.ManifestURL + `","Enabled":true}]`))
		case "/Plugins":
			_, _ = w.Write([]byte(`[{"Id":"` + plugininstall.PluginGUID + `","Version":"1.2.3"}]`))
		case "/plex2jellyfin/health":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer mock.Close()

	cfg := config.DefaultConfig()
	cfg.Jellyfin.Enabled = true
	cfg.Jellyfin.URL = mock.URL
	cfg.Jellyfin.APIKey = "key"
	s := &Server{cfg: cfg}

	w := httptest.NewRecorder()
	s.GetJellyfinPluginStatus(w, httptest.NewRequest(http.MethodGet, "/jellyfin/plugin/status", nil))
	var got api.JellyfinPluginStatus
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Healthy == nil || !*got.Healthy || got.Installed == nil || !*got.Installed {
		t.Fatalf("expected healthy installed plugin: %+v body=%s", got, w.Body.String())
	}
	if got.Version == nil || *got.Version != "1.2.3" {
		t.Fatalf("version: %+v", got)
	}
}

func TestVerifyJellyfinPluginSoftFailWhenNotResponding(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/System/Info":
			_, _ = w.Write([]byte(`{"Version":"10.11.0"}`))
		case "/Repositories":
			_, _ = w.Write([]byte(`[]`))
		case "/Plugins":
			_, _ = w.Write([]byte(`[]`))
		case "/plex2jellyfin/health":
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer mock.Close()

	cfg := config.DefaultConfig()
	cfg.Jellyfin.Enabled = true
	cfg.Jellyfin.URL = mock.URL
	cfg.Jellyfin.APIKey = "key"
	s := &Server{cfg: cfg}

	w := httptest.NewRecorder()
	s.VerifyJellyfinPlugin(w, httptest.NewRequest(http.MethodPost, "/jellyfin/plugin/verify", nil))
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["ok"] != false {
		t.Fatalf("expected soft fail: %v", got)
	}
}
