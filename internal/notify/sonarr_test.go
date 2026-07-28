package notify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/sonarr"
)

func TestSonarrNotifierAdoptsP2JPathWithoutImport(t *testing.T) {
	var updatedPath string
	var commands []sonarr.Command

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/config/downloadClient":
			_ = json.NewEncoder(w).Encode(sonarr.DownloadClientConfig{
				ID: 1, EnableCompletedDownloadHandling: false,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/config/naming":
			_ = json.NewEncoder(w).Encode(sonarr.NamingConfig{ID: 1, RenameEpisodes: false})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/series":
			_ = json.NewEncoder(w).Encode([]sonarr.Series{{
				ID: 7, Title: "Pluribus", Year: 2025, Path: "/mnt/STORAGE10/TVSHOWS/Pluribus (2025)",
			}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/series/7":
			_ = json.NewEncoder(w).Encode(sonarr.Series{
				ID: 7, Title: "Pluribus", Year: 2025, Path: "/mnt/STORAGE10/TVSHOWS/Pluribus (2025)",
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v3/series/7":
			var series sonarr.Series
			if err := json.NewDecoder(r.Body).Decode(&series); err != nil {
				t.Fatalf("decode series update: %v", err)
			}
			updatedPath = series.Path
			_ = json.NewEncoder(w).Encode(series)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v3/command":
			var command sonarr.Command
			if err := json.NewDecoder(r.Body).Decode(&command); err != nil {
				t.Fatalf("decode command: %v", err)
			}
			commands = append(commands, command)
			_ = json.NewEncoder(w).Encode(sonarr.CommandResponse{ID: 88})
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	db, err := database.OpenPath(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.UpsertSeries(&database.Series{
		Title: "Pluribus", Year: 2025,
		CanonicalPath: "/mnt/STORAGE4/TVSHOWS/Pluribus (2025)",
		LibraryRoot:   "/mnt/STORAGE4/TVSHOWS",
		Source:        "plex2jellyfin", SourcePriority: 100,
	}); err != nil {
		t.Fatal(err)
	}

	client := sonarr.NewClient(sonarr.Config{URL: server.URL, APIKey: "key"})
	target := "/mnt/STORAGE4/TVSHOWS/Pluribus (2025)/Season 02/Pluribus (2025) S02E01.mkv"
	result := NewSonarrNotifier(client, db, true).Notify(OrganizationEvent{
		MediaType:  MediaTypeTVEpisode,
		Title:      "Pluribus",
		Year:       "2025",
		Season:     2,
		Episode:    1,
		TargetPath: target,
	})

	if !result.Success {
		t.Fatalf("notification failed: %v", result.Error)
	}
	if want := filepath.Dir(filepath.Dir(target)); updatedPath != want {
		t.Fatalf("Sonarr path = %q, want P2J series path %q", updatedPath, want)
	}
	if len(commands) != 1 || commands[0].Name != "RescanSeries" || commands[0].SeriesID != 7 {
		t.Fatalf("commands = %#v, want one RescanSeries for series 7", commands)
	}
	if commands[0].ImportMode != "" || commands[0].Path != "" {
		t.Fatalf("rescan must not carry import/move fields: %#v", commands[0])
	}
	record, err := db.GetSeriesByTitle("Pluribus", 2025)
	if err != nil {
		t.Fatal(err)
	}
	if record.SonarrID == nil || *record.SonarrID != 7 {
		t.Fatalf("P2J series Sonarr ID = %v, want 7", record.SonarrID)
	}
}

func TestMatchSonarrSeriesFailsClosedOnAmbiguousTitle(t *testing.T) {
	_, err := matchSonarrSeries([]sonarr.Series{
		{ID: 1, Title: "The Office", Year: 2001},
		{ID: 2, Title: "The Office", Year: 2005},
	}, "The Office", 0)
	if err == nil {
		t.Fatal("expected ambiguous title to fail")
	}
}

func TestSonarrNotifierStopsWhenHandsOffPolicyDrifts(t *testing.T) {
	var operationalCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/config/downloadClient":
			_ = json.NewEncoder(w).Encode(sonarr.DownloadClientConfig{
				ID: 1, EnableCompletedDownloadHandling: false,
			})
		case "/api/v3/config/naming":
			_ = json.NewEncoder(w).Encode(sonarr.NamingConfig{ID: 1, RenameEpisodes: true})
		default:
			operationalCalls++
			http.Error(w, "must not reach Sonarr operations", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	db, err := database.OpenPath(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	client := sonarr.NewClient(sonarr.Config{URL: server.URL, APIKey: "key"})
	result := NewSonarrNotifier(client, db, true).Notify(OrganizationEvent{
		MediaType: MediaTypeTVEpisode, Title: "Pluribus", Year: "2025",
	})

	if result.Error == nil || !strings.Contains(result.Error.Error(), "incompatible") {
		t.Fatalf("result error = %v, want incompatible policy", result.Error)
	}
	if operationalCalls != 0 {
		t.Fatalf("made %d operational Sonarr calls after policy drift", operationalCalls)
	}
}
