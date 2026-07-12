package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/go-chi/chi/v5"
)

func TestGetJellyfinRecentlyAddedDisabled(t *testing.T) {
	s := &Server{cfg: config.DefaultConfig()}
	w := httptest.NewRecorder()
	s.GetJellyfinRecentlyAdded(w, httptest.NewRequest(http.MethodGet, "/jellyfin/recently-added", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["enabled"] != false {
		t.Fatalf("expected enabled=false: %v", got)
	}
}

func TestGetJellyfinRecentlyAddedReturnsItems(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/Users":
			_, _ = w.Write([]byte(`[{"Id":"u1","Name":"Admin","Policy":{"IsAdministrator":true}}]`))
		case strings.HasSuffix(r.URL.Path, "/Items/Latest"):
			_, _ = w.Write([]byte(`[
				{"Id":"m1","Name":"Film","Type":"Movie","DateCreated":"2026-01-01T00:00:00Z","LocationType":"FileSystem"},
				{"Id":"e1","Name":"Ep1","Type":"Episode","SeriesId":"s1","SeriesName":"Show","DateCreated":"2026-01-02T00:00:00Z","LocationType":"FileSystem"},
				{"Id":"v1","Name":"Ghost","Type":"Movie","LocationType":"Virtual"}
			]`))
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
	req := httptest.NewRequest(http.MethodGet, "/jellyfin/recently-added?limit=24", nil)
	s.GetJellyfinRecentlyAdded(w, req)

	var got struct {
		Enabled bool `json:"enabled"`
		Items   []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Type        string `json:"type"`
			SeriesName  string `json:"series_name"`
			ImageItemID string `json:"image_item_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || len(got.Items) != 2 {
		t.Fatalf("got %+v body=%s", got, w.Body.String())
	}
	if got.Items[0].ID != "m1" || got.Items[0].ImageItemID != "m1" {
		t.Fatalf("movie image id: %+v", got.Items[0])
	}
	if got.Items[1].ID != "e1" || got.Items[1].ImageItemID != "s1" || got.Items[1].SeriesName != "Show" {
		t.Fatalf("episode should use series poster: %+v", got.Items[1])
	}
}

func TestProxyJellyfinPrimaryImage(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Items/abc-123/Images/Primary" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("JPEGDATA"))
	}))
	defer mock.Close()

	cfg := config.DefaultConfig()
	cfg.Jellyfin.Enabled = true
	cfg.Jellyfin.URL = mock.URL
	cfg.Jellyfin.APIKey = "key"
	s := &Server{cfg: cfg}

	r := chi.NewRouter()
	r.Get("/jellyfin/items/{id}/image/primary", s.ProxyJellyfinPrimaryImage)
	req := httptest.NewRequest(http.MethodGet, "/jellyfin/items/abc-123/image/primary", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", w.Code, w.Body.String())
	}
	if w.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("content-type %q", w.Header().Get("Content-Type"))
	}
	if w.Body.String() != "JPEGDATA" {
		t.Fatalf("body %q", w.Body.String())
	}
}
