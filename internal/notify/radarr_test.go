package notify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/radarr"
)

func TestRadarrNotifierAdoptsP2JPathWithoutImport(t *testing.T) {
	var updatedPath string
	var commands []radarr.Command

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/movie":
			_ = json.NewEncoder(w).Encode([]radarr.Movie{{
				ID: 42, Title: "Truthers", Year: 2026, Path: "/mnt/STORAGE5/MOVIES/Truthers (2026)",
			}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/movie/42":
			_ = json.NewEncoder(w).Encode(radarr.Movie{
				ID: 42, Title: "Truthers", Year: 2026, Path: "/mnt/STORAGE5/MOVIES/Truthers (2026)",
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v3/movie/42":
			var movie radarr.Movie
			if err := json.NewDecoder(r.Body).Decode(&movie); err != nil {
				t.Fatalf("decode movie update: %v", err)
			}
			updatedPath = movie.Path
			_ = json.NewEncoder(w).Encode(movie)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v3/command":
			var command radarr.Command
			if err := json.NewDecoder(r.Body).Decode(&command); err != nil {
				t.Fatalf("decode command: %v", err)
			}
			commands = append(commands, command)
			_ = json.NewEncoder(w).Encode(radarr.CommandResponse{ID: 99})
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
	if _, err := db.UpsertMovie(&database.Movie{
		Title: "Truthers", Year: 2026,
		CanonicalPath: "/mnt/STORAGE1/MOVIES/Truthers (2026)",
		LibraryRoot:   "/mnt/STORAGE1/MOVIES",
		Source:        "plex2jellyfin", SourcePriority: 100,
	}); err != nil {
		t.Fatal(err)
	}

	client := radarr.NewClient(radarr.Config{URL: server.URL, APIKey: "key"})
	target := "/mnt/STORAGE1/MOVIES/Truthers (2026)/Truthers (2026).mkv"
	result := NewRadarrNotifier(client, db, true).Notify(OrganizationEvent{
		MediaType:  MediaTypeMovie,
		Title:      "Truthers",
		Year:       "2026",
		TargetPath: target,
	})

	if !result.Success {
		t.Fatalf("notification failed: %v", result.Error)
	}
	if want := filepath.Dir(target); updatedPath != want {
		t.Fatalf("Radarr path = %q, want P2J path %q", updatedPath, want)
	}
	if len(commands) != 1 || commands[0].Name != "RescanMovie" || commands[0].MovieID != 42 {
		t.Fatalf("commands = %#v, want one RescanMovie for movie 42", commands)
	}
	if commands[0].ImportMode != "" || commands[0].Path != "" {
		t.Fatalf("rescan must not carry import/move fields: %#v", commands[0])
	}
	record, err := db.GetMovieByTitle("Truthers", 2026)
	if err != nil {
		t.Fatal(err)
	}
	if record.RadarrID == nil || *record.RadarrID != 42 {
		t.Fatalf("P2J movie Radarr ID = %v, want 42", record.RadarrID)
	}
}

func TestMatchRadarrMovieFailsClosedOnAmbiguousTitle(t *testing.T) {
	_, err := matchRadarrMovie([]radarr.Movie{
		{ID: 1, Title: "Crash", Year: 1996},
		{ID: 2, Title: "Crash", Year: 2004},
	}, "Crash", 0)
	if err == nil {
		t.Fatal("expected ambiguous title to fail")
	}
}
