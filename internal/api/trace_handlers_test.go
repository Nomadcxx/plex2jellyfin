package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/database"
)

func traceTestServer(t *testing.T) (*Server, *database.MediaDB) {
	t.Helper()
	db, err := database.OpenPath(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &Server{cfg: &config.Config{}, db: db}, db
}

func TestGetFileTrace(t *testing.T) {
	server, db := traceTestServer(t)

	year := 2019
	if _, err := db.InsertDecision(database.ParseDecision{
		SourcePath:      "/downloads/tv/Show.S01E01.mkv",
		SourceFilename:  "Show.S01E01.mkv",
		EventAt:         time.Now().UTC(),
		ParseMethod:     "regex",
		ParsedTitle:     "Show",
		ParsedYear:      &year,
		TargetPath:      "/media/TV Shows/Show (2019)/Season 01/Show (2019) S01E01.mkv",
		OrganizeOutcome: "SUCCESS",
		JellyfinItemID:  "jf-1",
	}); err != nil {
		t.Fatalf("InsertDecision: %v", err)
	}
	if _, err := db.InsertDecision(database.ParseDecision{
		SourcePath:      "/downloads/movies/Other.Movie.mkv",
		SourceFilename:  "Other.Movie.mkv",
		EventAt:         time.Now().UTC(),
		OrganizeOutcome: "FAIL",
	}); err != nil {
		t.Fatalf("InsertDecision: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/files/trace", nil)
	w := httptest.NewRecorder()
	server.apiRouter().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body %s", w.Code, w.Body.String())
	}

	var resp struct {
		Items []TraceItem `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(resp.Items))
	}
	// Newest first.
	if resp.Items[0].SourceFilename != "Other.Movie.mkv" {
		t.Errorf("first item = %s, want newest", resp.Items[0].SourceFilename)
	}
	if resp.Items[1].Jellyfin == nil || resp.Items[1].Jellyfin.ItemID != "jf-1" {
		t.Errorf("jellyfin resolution missing: %+v", resp.Items[1].Jellyfin)
	}

	// Filtered query.
	req = httptest.NewRequest(http.MethodGet, "/files/trace?q=Other", nil)
	w = httptest.NewRecorder()
	server.apiRouter().ServeHTTP(w, req)
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal filtered: %v", err)
	}
	if len(resp.Items) != 1 || resp.Items[0].SourceFilename != "Other.Movie.mkv" {
		t.Errorf("filtered items = %+v", resp.Items)
	}
}
