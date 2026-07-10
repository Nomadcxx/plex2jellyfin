package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
)

func TestArrCompatibilityCheckAndFixUseDraftCredentials(t *testing.T) {
	tests := []struct {
		name        string
		renameField string
		check       func(*TestHandlers, http.ResponseWriter, *http.Request)
		fix         func(*TestHandlers, http.ResponseWriter, *http.Request)
	}{
		{name: "sonarr", renameField: "renameEpisodes", check: (*TestHandlers).SonarrCompatibility, fix: (*TestHandlers).FixSonarrCompatibility},
		{name: "radarr", renameField: "renameMovies", check: (*TestHandlers).RadarrCompatibility, fix: (*TestHandlers).FixRadarrCompatibility},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var puts atomic.Int32
			completed, renamed := true, false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Api-Key") != "draft-key" {
					t.Errorf("missing draft API key: %q", r.Header.Get("X-Api-Key"))
				}
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasPrefix(r.URL.Path, "/api/v3/config/downloadClient"):
					if r.Method == http.MethodPut {
						completed = false
						puts.Add(1)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "enableCompletedDownloadHandling": completed})
				case r.URL.Path == "/api/v3/config/naming":
					if r.Method == http.MethodPut {
						renamed = true
						puts.Add(1)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, tt.renameField: renamed})
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			h := &TestHandlers{Cfg: config.DefaultConfig()}
			payload := []byte(`{"url":"` + server.URL + `","api_key":"draft-key"}`)
			w := httptest.NewRecorder()
			tt.check(h, w, httptest.NewRequest(http.MethodPost, "/check", bytes.NewReader(payload)))
			var checked compatibilityResult
			if err := json.Unmarshal(w.Body.Bytes(), &checked); err != nil {
				t.Fatal(err)
			}
			if w.Code != http.StatusOK || !checked.OK || checked.Healthy || len(checked.Issues) != 2 || puts.Load() != 0 {
				t.Fatalf("check result=%+v status=%d puts=%d", checked, w.Code, puts.Load())
			}

			w = httptest.NewRecorder()
			tt.fix(h, w, httptest.NewRequest(http.MethodPost, "/fix", bytes.NewReader(payload)))
			var fixed compatibilityResult
			if err := json.Unmarshal(w.Body.Bytes(), &fixed); err != nil {
				t.Fatal(err)
			}
			if w.Code != http.StatusOK || !fixed.OK || !fixed.Healthy || fixed.Fixed != 2 || puts.Load() != 2 {
				t.Fatalf("fix result=%+v status=%d puts=%d", fixed, w.Code, puts.Load())
			}
		})
	}
}

func TestTestSonarrFailsWithBadURL(t *testing.T) {
	h := &TestHandlers{}
	body := []byte(`{"url":"http://127.0.0.1:1","api_key":"x","enabled":true}`)
	req := httptest.NewRequest("POST", "/settings/sonarr/test", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Sonarr(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"ok":false`)) {
		t.Errorf("expected ok=false; body=%s", w.Body.String())
	}
}

func TestTestSonarrSucceedsAgainstMock(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"appName":"Sonarr","version":"4.0"}`))
	}))
	defer mock.Close()

	h := &TestHandlers{}
	body := []byte(`{"url":"` + mock.URL + `","api_key":"x","enabled":true}`)
	req := httptest.NewRequest("POST", "/settings/sonarr/test", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Sonarr(w, req)
	if !bytes.Contains(w.Body.Bytes(), []byte(`"ok":true`)) {
		t.Errorf("body=%s", w.Body.String())
	}
}

func TestTestSonarrResolvesMaskedAPIKeyFromConfig(t *testing.T) {
	var receivedKey string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedKey = r.Header.Get("X-Api-Key")
		_, _ = w.Write([]byte(`{"appName":"Sonarr","version":"4.0"}`))
	}))
	defer mock.Close()

	cfg := &config.Config{}
	cfg.Sonarr.APIKey = "real-secret-key"
	h := &TestHandlers{Cfg: cfg}
	body := []byte(`{"url":"` + mock.URL + `","api_key":"****key","enabled":true}`)
	req := httptest.NewRequest("POST", "/settings/sonarr/test", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Sonarr(w, req)
	if !bytes.Contains(w.Body.Bytes(), []byte(`"ok":true`)) {
		t.Fatalf("body=%s", w.Body.String())
	}
	if receivedKey != "real-secret-key" {
		t.Errorf("upstream got api key %q; want real-secret-key", receivedKey)
	}
}
