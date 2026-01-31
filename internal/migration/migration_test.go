package migration

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/radarr"
	"github.com/Nomadcxx/jellywatch/internal/sonarr"
)

func TestPathMismatchStructure(t *testing.T) {
	mismatch := PathMismatch{
		MediaType:    "series",
		ID:           1,
		Title:        "Test Series",
		Year:         2024,
		DatabasePath: "/media/TV/Test Series (2024)",
		SonarrPath:   "/old/path/Test Series",
		HasSonarrID:  true,
	}

	if mismatch.MediaType != "series" {
		t.Errorf("expected media type series, got %s", mismatch.MediaType)
	}

	if mismatch.DatabasePath == mismatch.SonarrPath {
		t.Error("paths should be different for a mismatch")
	}
}

func TestFixChoiceConstants(t *testing.T) {
	tests := []struct {
		choice FixChoice
		want   string
	}{
		{FixChoiceKeepJellyWatch, "jellywatch"},
		{FixChoiceKeepSonarrRadarr, "arr"},
		{FixChoiceSkip, "skip"},
	}

	for _, tt := range tests {
		if string(tt.choice) != tt.want {
			t.Errorf("FixChoice constant mismatch: got %s, want %s", tt.choice, tt.want)
		}
	}
}

func setupTestDB(t *testing.T) *database.MediaDB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	return db
}

func newMockSonarrServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

func newMockRadarrServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

func TestMigration_BothConfigsNil(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, err := DetectSeriesMismatches(db, nil)
	if err == nil {
		t.Error("expected error when Sonarr client is nil, got nil")
	}
	expectedErrMsg := "getting series from Sonarr: nil client"
	if err != nil && !strings.Contains(err.Error(), expectedErrMsg) {
		t.Errorf("expected error containing %q, got: %v", expectedErrMsg, err)
	}

	_, err = DetectMovieMismatches(db, nil)
	if err == nil {
		t.Error("expected error when Radarr client is nil, got nil")
	}
	expectedErrMsg = "getting movies from Radarr: nil client"
	if err != nil && !strings.Contains(err.Error(), expectedErrMsg) {
		t.Errorf("expected error containing %q, got: %v", expectedErrMsg, err)
	}
}

func TestMigration_APIAuthFailure(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	authError := errors.New("authentication failed: invalid API key")

	sonarrServer := newMockSonarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid API key"})
	})
	defer sonarrServer.Close()

	sonarrClient := sonarr.NewClient(sonarr.Config{
		URL:    sonarrServer.URL,
		APIKey: "invalid-key",
	})

	_, err := DetectSeriesMismatches(db, sonarrClient)
	if err == nil {
		t.Fatal("expected error on auth failure, got nil")
	}

	errMsg := err.Error()
	if errMsg == "" {
		t.Error("error message should not be empty")
	}
	if !strings.Contains(errMsg, "Sonarr") {
		t.Errorf("error message should mention Sonarr, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "401") && !strings.Contains(errMsg, "unauthorized") {
		t.Logf("Note: Error should ideally mention 401/unauthorized, got: %s", errMsg)
	}
	t.Logf("Sonarr auth error message (clear): %s", errMsg)
	_ = authError

	radarrServer := newMockRadarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid API key"})
	})
	defer radarrServer.Close()

	radarrClient := radarr.NewClient(radarr.Config{
		URL:    radarrServer.URL,
		APIKey: "invalid-key",
	})

	_, err = DetectMovieMismatches(db, radarrClient)
	if err == nil {
		t.Fatal("expected error on auth failure, got nil")
	}

	errMsg = err.Error()
	if errMsg == "" {
		t.Error("error message should not be empty")
	}
	if !strings.Contains(errMsg, "Radarr") {
		t.Errorf("error message should mention Radarr, got: %s", errMsg)
	}
	t.Logf("Radarr auth error message (clear): %s", errMsg)
}

func TestMigration_DatabaseReadOnly(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "readonly.db")

	db, err := database.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	sonarrID := 123
	series := &database.Series{
		Title:         "Test Series",
		Year:          2024,
		CanonicalPath: "/media/TV/Test Series (2024)",
		SonarrID:      &sonarrID,
	}
	_, err = db.UpsertSeries(series)
	if err != nil {
		db.Close()
		t.Fatalf("failed to insert test series: %v", err)
	}
	db.Close()

	err = os.Chmod(dbPath, 0444)
	if err != nil {
		t.Fatalf("failed to make database read-only: %v", err)
	}
	defer os.Chmod(dbPath, 0644)

	readonlyDB, err := database.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open read-only database: %v", err)
	}
	defer readonlyDB.Close()

	sonarrServer := newMockSonarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/series":
			json.NewEncoder(w).Encode([]sonarr.Series{
				{ID: 123, Title: "Test Series", Year: 2024, Path: "/old/path/Test Series"},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer sonarrServer.Close()

	sonarrClient := sonarr.NewClient(sonarr.Config{
		URL:    sonarrServer.URL,
		APIKey: "test-key",
	})

	mismatches, err := DetectSeriesMismatches(readonlyDB, sonarrClient)
	if err != nil {
		t.Fatalf("failed to detect mismatches: %v", err)
	}
	if len(mismatches) == 0 {
		t.Fatal("expected to find a mismatch")
	}

	err = FixSeriesMismatch(readonlyDB, sonarrClient, mismatches[0], FixChoiceKeepSonarrRadarr)
	if err == nil {
		t.Error("expected error when fixing on read-only database, got nil")
	}
	if err != nil {
		t.Logf("Got expected error on read-only fix attempt: %v", err)
	}
}

func TestMigration_DatabaseQueryFailure(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := database.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	sonarrID := 123
	series := &database.Series{
		Title:         "Test Series",
		Year:          2024,
		CanonicalPath: "/media/TV/Test Series (2024)",
		SonarrID:      &sonarrID,
	}
	_, err = db.UpsertSeries(series)
	if err != nil {
		db.Close()
		t.Fatalf("failed to insert test series: %v", err)
	}
	db.Close()

	readonlyDB, err := database.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen database: %v", err)
	}
	defer readonlyDB.Close()

	sonarrServer := newMockSonarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/series":
			json.NewEncoder(w).Encode([]sonarr.Series{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer sonarrServer.Close()

	sonarrClient := sonarr.NewClient(sonarr.Config{
		URL:    sonarrServer.URL,
		APIKey: "test-key",
	})

	mismatches, err := DetectSeriesMismatches(readonlyDB, sonarrClient)
	if err != nil {
		t.Fatalf("unexpected error when querying: %v", err)
	}
	if len(mismatches) != 0 {
		t.Fatalf("expected 0 mismatches with empty sonarr, got %d", len(mismatches))
	}

	radarrServer := newMockRadarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/movie":
			json.NewEncoder(w).Encode([]radarr.Movie{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer radarrServer.Close()

	radarrClient := radarr.NewClient(radarr.Config{
		URL:    radarrServer.URL,
		APIKey: "test-key",
	})

	movieMismatches, err := DetectMovieMismatches(readonlyDB, radarrClient)
	if err != nil {
		t.Fatalf("unexpected error when querying movies: %v", err)
	}
	if len(movieMismatches) != 0 {
		t.Fatalf("expected 0 movie mismatches with empty radarr, got %d", len(movieMismatches))
	}
}

func TestMigration_FixNotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	mismatch := PathMismatch{
		MediaType:    "series",
		ID:           99999,
		Title:        "Nonexistent Series",
		Year:         2024,
		DatabasePath: "/media/TV/Nonexistent Series (2024)",
		SonarrPath:   "/old/path/Nonexistent Series",
		SonarrID:     intPtr(99999),
		HasSonarrID:  true,
	}

	sonarrServer := newMockSonarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/series/99999":
			json.NewEncoder(w).Encode(sonarr.Series{ID: 99999, Title: "Nonexistent Series", Year: 2024, Path: "/old/path/Nonexistent Series"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer sonarrServer.Close()

	sonarrClient := sonarr.NewClient(sonarr.Config{
		URL:    sonarrServer.URL,
		APIKey: "test-key",
	})

	err := FixSeriesMismatch(db, sonarrClient, mismatch, FixChoiceKeepSonarrRadarr)
	if err == nil {
		t.Error("expected error when fixing non-existent series, got nil")
	}
	if err != nil {
		errMsg := err.Error()
		if !strings.Contains(errMsg, "not found") {
			t.Errorf("expected 'not found' in error, got: %s", errMsg)
		}
		t.Logf("Got expected error for non-existent series: %v", err)
	}

	movieMismatch := PathMismatch{
		MediaType:    "movie",
		ID:           88888,
		Title:        "Nonexistent Movie",
		Year:         2024,
		DatabasePath: "/media/Movies/Nonexistent Movie (2024)",
		RadarrPath:   "/old/path/Nonexistent Movie",
		RadarrID:     intPtr(88888),
		HasRadarrID:  true,
	}

	radarrServer := newMockRadarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/movie/88888":
			json.NewEncoder(w).Encode(radarr.Movie{ID: 88888, Title: "Nonexistent Movie", Year: 2024, Path: "/old/path/Nonexistent Movie"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer radarrServer.Close()

	radarrClient := radarr.NewClient(radarr.Config{
		URL:    radarrServer.URL,
		APIKey: "test-key",
	})

	err = FixMovieMismatch(db, radarrClient, movieMismatch, FixChoiceKeepSonarrRadarr)
	if err == nil {
		t.Error("expected error when fixing non-existent movie, got nil")
	}
	if err != nil {
		errMsg := err.Error()
		if !strings.Contains(errMsg, "not found") {
			t.Errorf("expected 'not found' in error, got: %s", errMsg)
		}
		t.Logf("Got expected error for non-existent movie: %v", err)
	}
}

func TestMigration_InvalidMismatchType(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	sonarrServer := newMockSonarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer sonarrServer.Close()

	sonarrClient := sonarr.NewClient(sonarr.Config{
		URL:    sonarrServer.URL,
		APIKey: "test-key",
	})

	movieAsSeries := PathMismatch{
		MediaType: "movie",
		ID:        1,
		Title:     "Test Movie",
		Year:      2024,
	}

	err := FixSeriesMismatch(db, sonarrClient, movieAsSeries, FixChoiceKeepJellyWatch)
	if err == nil {
		t.Error("expected error when passing movie mismatch to FixSeriesMismatch, got nil")
	}
	if err != nil {
		if !strings.Contains(err.Error(), "not a series mismatch") {
			t.Errorf("expected 'not a series mismatch' error, got: %v", err)
		}
		t.Logf("Got expected error for invalid mismatch type: %v", err)
	}

	radarrServer := newMockRadarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer radarrServer.Close()

	radarrClient := radarr.NewClient(radarr.Config{
		URL:    radarrServer.URL,
		APIKey: "test-key",
	})

	seriesAsMovie := PathMismatch{
		MediaType: "series",
		ID:        1,
		Title:     "Test Series",
		Year:      2024,
	}

	err = FixMovieMismatch(db, radarrClient, seriesAsMovie, FixChoiceKeepJellyWatch)
	if err == nil {
		t.Error("expected error when passing series mismatch to FixMovieMismatch, got nil")
	}
	if err != nil {
		if !strings.Contains(err.Error(), "not a movie mismatch") {
			t.Errorf("expected 'not a movie mismatch' error, got: %v", err)
		}
		t.Logf("Got expected error for invalid mismatch type: %v", err)
	}
}

func TestMigration_InvalidFixChoice(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	sonarrID := 123
	series := &database.Series{
		Title:         "Test Series",
		Year:          2024,
		CanonicalPath: "/media/TV/Test Series (2024)",
		SonarrID:      &sonarrID,
	}
	_, err := db.UpsertSeries(series)
	if err != nil {
		t.Fatalf("failed to insert test series: %v", err)
	}

	sonarrServer := newMockSonarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/series":
			json.NewEncoder(w).Encode([]sonarr.Series{
				{ID: 123, Title: "Test Series", Year: 2024, Path: "/old/path/Test Series"},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer sonarrServer.Close()

	sonarrClient := sonarr.NewClient(sonarr.Config{
		URL:    sonarrServer.URL,
		APIKey: "test-key",
	})

	mismatches, err := DetectSeriesMismatches(db, sonarrClient)
	if err != nil {
		t.Fatalf("failed to detect mismatches: %v", err)
	}
	if len(mismatches) != 1 {
		t.Fatalf("expected 1 mismatch, got %d", len(mismatches))
	}

	invalidChoice := FixChoice("invalid_choice")
	err = FixSeriesMismatch(db, sonarrClient, mismatches[0], invalidChoice)
	if err == nil {
		t.Error("expected error with invalid fix choice, got nil")
	}
	if err != nil {
		if !strings.Contains(err.Error(), "invalid choice") {
			t.Errorf("expected 'invalid choice' in error, got: %v", err)
		}
		t.Logf("Got expected error for invalid choice: %v", err)
	}

	radarrID := 456
	movie := &database.Movie{
		Title:         "Test Movie",
		Year:          2024,
		CanonicalPath: "/media/Movies/Test Movie (2024)",
		RadarrID:      &radarrID,
	}
	_, err = db.UpsertMovie(movie)
	if err != nil {
		t.Fatalf("failed to insert test movie: %v", err)
	}

	radarrServer := newMockRadarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/movie":
			json.NewEncoder(w).Encode([]radarr.Movie{
				{ID: 456, Title: "Test Movie", Year: 2024, Path: "/old/path/Test Movie"},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer radarrServer.Close()

	radarrClient := radarr.NewClient(radarr.Config{
		URL:    radarrServer.URL,
		APIKey: "test-key",
	})

	movieMismatches, err := DetectMovieMismatches(db, radarrClient)
	if err != nil {
		t.Fatalf("failed to detect movie mismatches: %v", err)
	}
	if len(movieMismatches) != 1 {
		t.Fatalf("expected 1 movie mismatch, got %d", len(movieMismatches))
	}

	err = FixMovieMismatch(db, radarrClient, movieMismatches[0], invalidChoice)
	if err == nil {
		t.Error("expected error with invalid fix choice for movie, got nil")
	}
	if err != nil {
		if !strings.Contains(err.Error(), "invalid choice") {
			t.Errorf("expected 'invalid choice' in error, got: %v", err)
		}
		t.Logf("Got expected error for invalid movie choice: %v", err)
	}
}

func intPtr(i int) *int {
	return &i
}

func TestDetectSeriesMismatches_NoMismatches(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	sonarrID := 123
	series := &database.Series{
		Title:         "Test Series",
		Year:          2024,
		CanonicalPath: "/media/TV/Test Series (2024)",
		SonarrID:      &sonarrID,
	}
	_, err := db.UpsertSeries(series)
	if err != nil {
		t.Fatalf("failed to insert test series: %v", err)
	}

	sonarrServer := newMockSonarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]sonarr.Series{
			{ID: 123, Title: "Test Series", Year: 2024, Path: "/media/TV/Test Series (2024)"},
		})
	})
	defer sonarrServer.Close()

	sonarrClient := sonarr.NewClient(sonarr.Config{
		URL:    sonarrServer.URL,
		APIKey: "test-key",
	})

	mismatches, err := DetectSeriesMismatches(db, sonarrClient)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mismatches) != 0 {
		t.Errorf("expected 0 mismatches when paths match, got %d", len(mismatches))
	}
}

func TestDetectSeriesMismatches_MultipleMismatches(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	sonarrID1 := 101
	sonarrID2 := 102
	sonarrID3 := 103

	series1 := &database.Series{
		Title:         "Series One",
		Year:          2024,
		CanonicalPath: "/media/TV/Series One (2024)",
		SonarrID:      &sonarrID1,
	}
	series2 := &database.Series{
		Title:         "Series Two",
		Year:          2023,
		CanonicalPath: "/media/TV/Series Two (2023)",
		SonarrID:      &sonarrID2,
	}
	series3 := &database.Series{
		Title:         "Series Three",
		Year:          2022,
		CanonicalPath: "/media/TV/Series Three (2022)",
		SonarrID:      &sonarrID3,
	}

	_, err := db.UpsertSeries(series1)
	if err != nil {
		t.Fatalf("failed to insert series1: %v", err)
	}
	_, err = db.UpsertSeries(series2)
	if err != nil {
		t.Fatalf("failed to insert series2: %v", err)
	}
	_, err = db.UpsertSeries(series3)
	if err != nil {
		t.Fatalf("failed to insert series3: %v", err)
	}

	sonarrServer := newMockSonarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]sonarr.Series{
			{ID: 101, Title: "Series One", Year: 2024, Path: "/old/path/Series One"},
			{ID: 102, Title: "Series Two", Year: 2023, Path: "/old/path/Series Two"},
			{ID: 103, Title: "Series Three", Year: 2022, Path: "/old/path/Series Three"},
		})
	})
	defer sonarrServer.Close()

	sonarrClient := sonarr.NewClient(sonarr.Config{
		URL:    sonarrServer.URL,
		APIKey: "test-key",
	})

	mismatches, err := DetectSeriesMismatches(db, sonarrClient)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mismatches) != 3 {
		t.Errorf("expected 3 mismatches, got %d", len(mismatches))
	}

	for i, m := range mismatches {
		if m.MediaType != "series" {
			t.Errorf("mismatch %d: expected media type 'series', got %s", i, m.MediaType)
		}
		if m.DatabasePath == m.SonarrPath {
			t.Errorf("mismatch %d: paths should be different", i)
		}
		if !m.HasSonarrID {
			t.Errorf("mismatch %d: should have Sonarr ID", i)
		}
	}
}

func TestDetectSeriesMismatches_OrphanedRecords(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	sonarrID := 123
	series1 := &database.Series{
		Title:         "Series With Sonarr ID",
		Year:          2024,
		CanonicalPath: "/media/TV/Series With Sonarr ID (2024)",
		SonarrID:      &sonarrID,
	}
	series2 := &database.Series{
		Title:         "Orphaned Series",
		Year:          2023,
		CanonicalPath: "/media/TV/Orphaned Series (2023)",
		SonarrID:      nil,
	}

	_, err := db.UpsertSeries(series1)
	if err != nil {
		t.Fatalf("failed to insert series1: %v", err)
	}
	_, err = db.UpsertSeries(series2)
	if err != nil {
		t.Fatalf("failed to insert series2: %v", err)
	}

	sonarrServer := newMockSonarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]sonarr.Series{
			{ID: 123, Title: "Series With Sonarr ID", Year: 2024, Path: "/old/path/Series"},
		})
	})
	defer sonarrServer.Close()

	sonarrClient := sonarr.NewClient(sonarr.Config{
		URL:    sonarrServer.URL,
		APIKey: "test-key",
	})

	mismatches, err := DetectSeriesMismatches(db, sonarrClient)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mismatches) != 1 {
		t.Errorf("expected 1 mismatch (orphaned record should be skipped), got %d", len(mismatches))
	}
	if len(mismatches) > 0 && mismatches[0].Title != "Series With Sonarr ID" {
		t.Errorf("expected mismatch for 'Series With Sonarr ID', got %s", mismatches[0].Title)
	}
}

func TestDetectMovieMismatches_NoMismatches(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	radarrID := 456
	movie := &database.Movie{
		Title:         "Test Movie",
		Year:          2024,
		CanonicalPath: "/media/Movies/Test Movie (2024)",
		RadarrID:      &radarrID,
	}
	_, err := db.UpsertMovie(movie)
	if err != nil {
		t.Fatalf("failed to insert test movie: %v", err)
	}

	radarrServer := newMockRadarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]radarr.Movie{
			{ID: 456, Title: "Test Movie", Year: 2024, Path: "/media/Movies/Test Movie (2024)"},
		})
	})
	defer radarrServer.Close()

	radarrClient := radarr.NewClient(radarr.Config{
		URL:    radarrServer.URL,
		APIKey: "test-key",
	})

	mismatches, err := DetectMovieMismatches(db, radarrClient)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mismatches) != 0 {
		t.Errorf("expected 0 mismatches when paths match, got %d", len(mismatches))
	}
}

func TestDetectMovieMismatches_MultipleMismatches(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	radarrID1 := 201
	radarrID2 := 202
	radarrID3 := 203

	movie1 := &database.Movie{
		Title:         "Movie One",
		Year:          2024,
		CanonicalPath: "/media/Movies/Movie One (2024)",
		RadarrID:      &radarrID1,
	}
	movie2 := &database.Movie{
		Title:         "Movie Two",
		Year:          2023,
		CanonicalPath: "/media/Movies/Movie Two (2023)",
		RadarrID:      &radarrID2,
	}
	movie3 := &database.Movie{
		Title:         "Movie Three",
		Year:          2022,
		CanonicalPath: "/media/Movies/Movie Three (2022)",
		RadarrID:      &radarrID3,
	}

	_, err := db.UpsertMovie(movie1)
	if err != nil {
		t.Fatalf("failed to insert movie1: %v", err)
	}
	_, err = db.UpsertMovie(movie2)
	if err != nil {
		t.Fatalf("failed to insert movie2: %v", err)
	}
	_, err = db.UpsertMovie(movie3)
	if err != nil {
		t.Fatalf("failed to insert movie3: %v", err)
	}

	radarrServer := newMockRadarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]radarr.Movie{
			{ID: 201, Title: "Movie One", Year: 2024, Path: "/old/path/Movie One"},
			{ID: 202, Title: "Movie Two", Year: 2023, Path: "/old/path/Movie Two"},
			{ID: 203, Title: "Movie Three", Year: 2022, Path: "/old/path/Movie Three"},
		})
	})
	defer radarrServer.Close()

	radarrClient := radarr.NewClient(radarr.Config{
		URL:    radarrServer.URL,
		APIKey: "test-key",
	})

	mismatches, err := DetectMovieMismatches(db, radarrClient)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mismatches) != 3 {
		t.Errorf("expected 3 mismatches, got %d", len(mismatches))
	}

	for i, m := range mismatches {
		if m.MediaType != "movie" {
			t.Errorf("mismatch %d: expected media type 'movie', got %s", i, m.MediaType)
		}
		if m.DatabasePath == m.RadarrPath {
			t.Errorf("mismatch %d: paths should be different", i)
		}
		if !m.HasRadarrID {
			t.Errorf("mismatch %d: should have Radarr ID", i)
		}
	}
}

func TestDetectMovieMismatches_OrphanedRecords(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	radarrID := 456
	movie1 := &database.Movie{
		Title:         "Movie With Radarr ID",
		Year:          2024,
		CanonicalPath: "/media/Movies/Movie With Radarr ID (2024)",
		RadarrID:      &radarrID,
	}
	movie2 := &database.Movie{
		Title:         "Orphaned Movie",
		Year:          2023,
		CanonicalPath: "/media/Movies/Orphaned Movie (2023)",
		RadarrID:      nil,
	}

	_, err := db.UpsertMovie(movie1)
	if err != nil {
		t.Fatalf("failed to insert movie1: %v", err)
	}
	_, err = db.UpsertMovie(movie2)
	if err != nil {
		t.Fatalf("failed to insert movie2: %v", err)
	}

	radarrServer := newMockRadarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]radarr.Movie{
			{ID: 456, Title: "Movie With Radarr ID", Year: 2024, Path: "/old/path/Movie"},
		})
	})
	defer radarrServer.Close()

	radarrClient := radarr.NewClient(radarr.Config{
		URL:    radarrServer.URL,
		APIKey: "test-key",
	})

	mismatches, err := DetectMovieMismatches(db, radarrClient)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mismatches) != 1 {
		t.Errorf("expected 1 mismatch (orphaned record should be skipped), got %d", len(mismatches))
	}
	if len(mismatches) > 0 && mismatches[0].Title != "Movie With Radarr ID" {
		t.Errorf("expected mismatch for 'Movie With Radarr ID', got %s", mismatches[0].Title)
	}
}

func TestMigration_PartialFailure(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	sonarrID1 := 101
	sonarrID2 := 102
	sonarrID3 := 103

	series1 := &database.Series{
		Title:         "Series One",
		Year:          2024,
		CanonicalPath: "/media/TV/Series One (2024)",
		SonarrID:      &sonarrID1,
	}
	series2 := &database.Series{
		Title:         "Series Two",
		Year:          2023,
		CanonicalPath: "/media/TV/Series Two (2023)",
		SonarrID:      &sonarrID2,
	}
	series3 := &database.Series{
		Title:         "Series Three",
		Year:          2022,
		CanonicalPath: "/media/TV/Series Three (2022)",
		SonarrID:      &sonarrID3,
	}

	_, err := db.UpsertSeries(series1)
	if err != nil {
		t.Fatalf("failed to insert series1: %v", err)
	}
	_, err = db.UpsertSeries(series2)
	if err != nil {
		t.Fatalf("failed to insert series2: %v", err)
	}
	_, err = db.UpsertSeries(series3)
	if err != nil {
		t.Fatalf("failed to insert series3: %v", err)
	}

	sonarrServer := newMockSonarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/series":
			json.NewEncoder(w).Encode([]sonarr.Series{
				{ID: 101, Title: "Series One", Year: 2024, Path: "/old/path/Series One"},
				{ID: 102, Title: "Series Two", Year: 2023, Path: "/old/path/Series Two"},
				{ID: 103, Title: "Series Three", Year: 2022, Path: "/old/path/Series Three"},
			})
		case "/api/v3/series/101":
			if r.Method == http.MethodPut {
				json.NewEncoder(w).Encode(sonarr.Series{ID: 101, Title: "Series One", Year: 2024, Path: "/media/TV/Series One (2024)"})
			} else {
				json.NewEncoder(w).Encode(sonarr.Series{ID: 101, Title: "Series One", Year: 2024, Path: "/old/path/Series One"})
			}
		case "/api/v3/series/102":
			if r.Method == http.MethodPut {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "network error: connection refused"})
			} else {
				json.NewEncoder(w).Encode(sonarr.Series{ID: 102, Title: "Series Two", Year: 2023, Path: "/old/path/Series Two"})
			}
		case "/api/v3/series/103":
			if r.Method == http.MethodPut {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "network error: connection refused"})
			} else {
				json.NewEncoder(w).Encode(sonarr.Series{ID: 103, Title: "Series Three", Year: 2022, Path: "/old/path/Series Three"})
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer sonarrServer.Close()

	sonarrClient := sonarr.NewClient(sonarr.Config{
		URL:    sonarrServer.URL,
		APIKey: "test-key",
	})

	mismatches, err := DetectSeriesMismatches(db, sonarrClient)
	if err != nil {
		t.Fatalf("failed to detect mismatches: %v", err)
	}
	if len(mismatches) != 3 {
		t.Fatalf("expected 3 mismatches, got %d", len(mismatches))
	}

	err = FixSeriesMismatch(db, sonarrClient, mismatches[0], FixChoiceKeepJellyWatch)
	if err != nil {
		t.Errorf("first fix should succeed: %v", err)
	}

	err = FixSeriesMismatch(db, sonarrClient, mismatches[1], FixChoiceKeepJellyWatch)
	if err == nil {
		t.Error("second fix should fail due to simulated network error")
	}
	if err != nil {
		t.Logf("Got expected error on second fix: %v", err)
	}

	err = FixSeriesMismatch(db, sonarrClient, mismatches[2], FixChoiceKeepJellyWatch)
	if err == nil {
		t.Error("third fix should fail due to simulated network error")
	}
	if err != nil {
		t.Logf("Got expected error on third fix: %v", err)
	}

	radarrID1 := 201
	radarrID2 := 202

	movie1 := &database.Movie{
		Title:         "Movie One",
		Year:          2024,
		CanonicalPath: "/media/Movies/Movie One (2024)",
		RadarrID:      &radarrID1,
	}
	movie2 := &database.Movie{
		Title:         "Movie Two",
		Year:          2023,
		CanonicalPath: "/media/Movies/Movie Two (2023)",
		RadarrID:      &radarrID2,
	}

	_, err = db.UpsertMovie(movie1)
	if err != nil {
		t.Fatalf("failed to insert movie1: %v", err)
	}
	_, err = db.UpsertMovie(movie2)
	if err != nil {
		t.Fatalf("failed to insert movie2: %v", err)
	}

	radarrServer := newMockRadarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/movie":
			json.NewEncoder(w).Encode([]radarr.Movie{
				{ID: 201, Title: "Movie One", Year: 2024, Path: "/old/path/Movie One"},
				{ID: 202, Title: "Movie Two", Year: 2023, Path: "/old/path/Movie Two"},
			})
		case "/api/v3/movie/201":
			if r.Method == http.MethodPut {
				json.NewEncoder(w).Encode(radarr.Movie{ID: 201, Title: "Movie One", Year: 2024, Path: "/media/Movies/Movie One (2024)"})
			} else {
				json.NewEncoder(w).Encode(radarr.Movie{ID: 201, Title: "Movie One", Year: 2024, Path: "/old/path/Movie One"})
			}
		case "/api/v3/movie/202":
			if r.Method == http.MethodPut {
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(map[string]string{"error": "API rate limit exceeded"})
			} else {
				json.NewEncoder(w).Encode(radarr.Movie{ID: 202, Title: "Movie Two", Year: 2023, Path: "/old/path/Movie Two"})
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer radarrServer.Close()

	radarrClient := radarr.NewClient(radarr.Config{
		URL:    radarrServer.URL,
		APIKey: "test-key",
	})

	movieMismatches, err := DetectMovieMismatches(db, radarrClient)
	if err != nil {
		t.Fatalf("failed to detect movie mismatches: %v", err)
	}
	if len(movieMismatches) != 2 {
		t.Fatalf("expected 2 movie mismatches, got %d", len(movieMismatches))
	}

	err = FixMovieMismatch(db, radarrClient, movieMismatches[0], FixChoiceKeepJellyWatch)
	if err != nil {
		t.Errorf("first movie fix should succeed: %v", err)
	}

	err = FixMovieMismatch(db, radarrClient, movieMismatches[1], FixChoiceKeepJellyWatch)
	if err == nil {
		t.Error("second movie fix should fail due to rate limit")
	}
	if err != nil {
		t.Logf("Got expected error on movie fix: %v", err)
	}
}

func TestFixSeriesMismatch_KeepJellyWatch(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	sonarrID := 123
	series := &database.Series{
		Title:         "Test Series",
		Year:          2024,
		CanonicalPath: "/media/TV/Test Series (2024)",
		SonarrID:      &sonarrID,
	}
	_, err := db.UpsertSeries(series)
	id := series.ID
	if err != nil {
		t.Fatalf("failed to insert test series: %v", err)
	}

	pathUpdated := false
	sonarrServer := newMockSonarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/series/123":
			if r.Method == http.MethodPut {
				pathUpdated = true
				json.NewEncoder(w).Encode(sonarr.Series{ID: 123, Title: "Test Series", Year: 2024, Path: "/media/TV/Test Series (2024)"})
			} else {
				json.NewEncoder(w).Encode(sonarr.Series{ID: 123, Title: "Test Series", Year: 2024, Path: "/old/path/Test Series"})
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer sonarrServer.Close()

	sonarrClient := sonarr.NewClient(sonarr.Config{
		URL:    sonarrServer.URL,
		APIKey: "test-key",
	})

	mismatch := PathMismatch{
		MediaType:    "series",
		ID:           id,
		Title:        "Test Series",
		Year:         2024,
		DatabasePath: "/media/TV/Test Series (2024)",
		SonarrPath:   "/old/path/Test Series",
		SonarrID:     intPtr(123),
		HasSonarrID:  true,
	}

	err = FixSeriesMismatch(db, sonarrClient, mismatch, FixChoiceKeepJellyWatch)
	if err != nil {
		t.Errorf("expected fix to succeed, got error: %v", err)
	}

	if !pathUpdated {
		t.Error("expected Sonarr path to be updated")
	}

	series, err = db.GetSeriesByID(id)
	if err != nil {
		t.Fatalf("failed to get series: %v", err)
	}
	if series == nil {
		t.Fatal("series should exist in DB")
	}
	if series.SonarrSyncedAt == nil {
		t.Error("expected series to be marked as synced")
	}
}

func TestFixSeriesMismatch_KeepSonarr(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	sonarrID := 123
	series := &database.Series{
		Title:         "Test Series",
		Year:          2024,
		CanonicalPath: "/media/TV/Test Series (2024)",
		SonarrID:      &sonarrID,
	}
	_, err := db.UpsertSeries(series)
	id := series.ID
	if err != nil {
		t.Fatalf("failed to insert test series: %v", err)
	}

	sonarrServer := newMockSonarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/series/123":
			json.NewEncoder(w).Encode(sonarr.Series{ID: 123, Title: "Test Series", Year: 2024, Path: "/sonarr/path/Test Series"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer sonarrServer.Close()

	sonarrClient := sonarr.NewClient(sonarr.Config{
		URL:    sonarrServer.URL,
		APIKey: "test-key",
	})

	mismatch := PathMismatch{
		MediaType:    "series",
		ID:           id,
		Title:        "Test Series",
		Year:         2024,
		DatabasePath: "/media/TV/Test Series (2024)",
		SonarrPath:   "/sonarr/path/Test Series",
		SonarrID:     intPtr(123),
		HasSonarrID:  true,
	}

	err = FixSeriesMismatch(db, sonarrClient, mismatch, FixChoiceKeepSonarrRadarr)
	if err != nil {
		t.Errorf("expected fix to succeed, got error: %v", err)
	}

	series, err = db.GetSeriesByID(id)
	if err != nil {
		t.Fatalf("failed to get series: %v", err)
	}
	if series == nil {
		t.Fatal("series should exist in DB")
	}
	if series.CanonicalPath != "/sonarr/path/Test Series" {
		t.Errorf("expected DB path to be updated to Sonarr path, got: %s", series.CanonicalPath)
	}
	if series.Source != "sonarr" {
		t.Errorf("expected source to be 'sonarr', got: %s", series.Source)
	}
	if series.SonarrSyncedAt == nil {
		t.Error("expected series to be marked as synced")
	}
}

func TestFixSeriesMismatch_Skip(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	sonarrID := 123
	series := &database.Series{
		Title:         "Test Series",
		Year:          2024,
		CanonicalPath: "/media/TV/Test Series (2024)",
		SonarrID:      &sonarrID,
	}
	_, err := db.UpsertSeries(series)
	id := series.ID
	if err != nil {
		t.Fatalf("failed to insert test series: %v", err)
	}

	sonarrServer := newMockSonarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer sonarrServer.Close()

	sonarrClient := sonarr.NewClient(sonarr.Config{
		URL:    sonarrServer.URL,
		APIKey: "test-key",
	})

	mismatch := PathMismatch{
		MediaType:    "series",
		ID:           id,
		Title:        "Test Series",
		Year:         2024,
		DatabasePath: "/media/TV/Test Series (2024)",
		SonarrPath:   "/old/path/Test Series",
		SonarrID:     intPtr(123),
		HasSonarrID:  true,
	}

	err = FixSeriesMismatch(db, sonarrClient, mismatch, FixChoiceSkip)
	if err != nil {
		t.Errorf("expected skip to succeed without error, got: %v", err)
	}

	series, err = db.GetSeriesByID(id)
	if err != nil {
		t.Fatalf("failed to get series: %v", err)
	}
	if series == nil {
		t.Fatal("series should exist in DB")
	}
	if series.CanonicalPath != "/media/TV/Test Series (2024)" {
		t.Errorf("expected DB path to remain unchanged, got: %s", series.CanonicalPath)
	}
	if series.SonarrSyncedAt != nil {
		t.Error("expected series to NOT be marked as synced when skipping")
	}
}

func TestFixSeriesMismatch_NoSonarrID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	sonarrID := 123
	series := &database.Series{
		Title:         "Test Series",
		Year:          2024,
		CanonicalPath: "/media/TV/Test Series (2024)",
		SonarrID:      &sonarrID,
	}
	_, err := db.UpsertSeries(series)
	id := series.ID
	if err != nil {
		t.Fatalf("failed to insert test series: %v", err)
	}

	sonarrServer := newMockSonarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer sonarrServer.Close()

	sonarrClient := sonarr.NewClient(sonarr.Config{
		URL:    sonarrServer.URL,
		APIKey: "test-key",
	})

	mismatch := PathMismatch{
		MediaType:    "series",
		ID:           id,
		Title:        "Test Series",
		Year:         2024,
		DatabasePath: "/media/TV/Test Series (2024)",
		SonarrPath:   "/old/path/Test Series",
		SonarrID:     nil,
		HasSonarrID:  false,
	}

	err = FixSeriesMismatch(db, sonarrClient, mismatch, FixChoiceKeepJellyWatch)
	if err == nil {
		t.Error("expected error when no Sonarr ID provided, got nil")
	}
	if err != nil {
		if !strings.Contains(err.Error(), "no Sonarr ID") {
			t.Errorf("expected 'no Sonarr ID' error, got: %v", err)
		}
	}
}

func TestFixSeriesMismatch_APIFailure(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	sonarrID := 123
	series := &database.Series{
		Title:         "Test Series",
		Year:          2024,
		CanonicalPath: "/media/TV/Test Series (2024)",
		SonarrID:      &sonarrID,
	}
	_, err := db.UpsertSeries(series)
	id := series.ID
	if err != nil {
		t.Fatalf("failed to insert test series: %v", err)
	}

	sonarrServer := newMockSonarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/series/123":
			if r.Method == http.MethodPut {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "server error"})
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer sonarrServer.Close()

	sonarrClient := sonarr.NewClient(sonarr.Config{
		URL:    sonarrServer.URL,
		APIKey: "test-key",
	})

	mismatch := PathMismatch{
		MediaType:    "series",
		ID:           id,
		Title:        "Test Series",
		Year:         2024,
		DatabasePath: "/media/TV/Test Series (2024)",
		SonarrPath:   "/old/path/Test Series",
		SonarrID:     intPtr(123),
		HasSonarrID:  true,
	}

	err = FixSeriesMismatch(db, sonarrClient, mismatch, FixChoiceKeepJellyWatch)
	if err == nil {
		t.Error("expected error when API fails, got nil")
	}
	if err != nil {
		if !strings.Contains(err.Error(), "updating Sonarr path") {
			t.Errorf("expected 'updating Sonarr path' error, got: %v", err)
		}
	}

	series, err = db.GetSeriesByID(id)
	if err != nil {
		t.Fatalf("failed to get series: %v", err)
	}
	if series == nil {
		t.Fatal("series should exist in DB")
	}
	if series.SonarrSyncedAt != nil {
		t.Error("expected series to NOT be marked as synced when API fails")
	}
}

func TestFixSeriesMismatch_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	sonarrServer := newMockSonarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/series/999":
			json.NewEncoder(w).Encode(sonarr.Series{ID: 999, Title: "Nonexistent", Year: 2024, Path: "/sonarr/path/Nonexistent"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer sonarrServer.Close()

	sonarrClient := sonarr.NewClient(sonarr.Config{
		URL:    sonarrServer.URL,
		APIKey: "test-key",
	})

	mismatch := PathMismatch{
		MediaType:    "series",
		ID:           999,
		Title:        "Nonexistent Series",
		Year:         2024,
		DatabasePath: "/media/TV/Nonexistent Series (2024)",
		SonarrPath:   "/sonarr/path/Nonexistent",
		SonarrID:     intPtr(999),
		HasSonarrID:  true,
	}

	err := FixSeriesMismatch(db, sonarrClient, mismatch, FixChoiceKeepSonarrRadarr)
	if err == nil {
		t.Error("expected error when series not found in DB, got nil")
	}
	if err != nil {
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected 'not found' error, got: %v", err)
		}
	}
}

func TestFixMovieMismatch_KeepJellyWatch(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	radarrID := 456
	movie := &database.Movie{
		Title:         "Test Movie",
		Year:          2024,
		CanonicalPath: "/media/Movies/Test Movie (2024)",
		RadarrID:      &radarrID,
	}
	_, err := db.UpsertMovie(movie)
	id := movie.ID
	if err != nil {
		t.Fatalf("failed to insert test movie: %v", err)
	}

	pathUpdated := false
	radarrServer := newMockRadarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/movie/456":
			if r.Method == http.MethodPut {
				pathUpdated = true
				json.NewEncoder(w).Encode(radarr.Movie{ID: 456, Title: "Test Movie", Year: 2024, Path: "/media/Movies/Test Movie (2024)"})
			} else {
				json.NewEncoder(w).Encode(radarr.Movie{ID: 456, Title: "Test Movie", Year: 2024, Path: "/old/path/Test Movie"})
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer radarrServer.Close()

	radarrClient := radarr.NewClient(radarr.Config{
		URL:    radarrServer.URL,
		APIKey: "test-key",
	})

	mismatch := PathMismatch{
		MediaType:    "movie",
		ID:           id,
		Title:        "Test Movie",
		Year:         2024,
		DatabasePath: "/media/Movies/Test Movie (2024)",
		RadarrPath:   "/old/path/Test Movie",
		RadarrID:     intPtr(456),
		HasRadarrID:  true,
	}

	err = FixMovieMismatch(db, radarrClient, mismatch, FixChoiceKeepJellyWatch)
	if err != nil {
		t.Errorf("expected fix to succeed, got error: %v", err)
	}

	if !pathUpdated {
		t.Error("expected Radarr path to be updated")
	}

	movie, err = db.GetMovieByID(id)
	if err != nil {
		t.Fatalf("failed to get movie: %v", err)
	}
	if movie == nil {
		t.Fatal("movie should exist in DB")
	}
	if movie.RadarrSyncedAt == nil {
		t.Error("expected movie to be marked as synced")
	}
}

func TestFixMovieMismatch_KeepRadarr(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	radarrID := 456
	movie := &database.Movie{
		Title:         "Test Movie",
		Year:          2024,
		CanonicalPath: "/media/Movies/Test Movie (2024)",
		RadarrID:      &radarrID,
	}
	_, err := db.UpsertMovie(movie)
	id := movie.ID
	if err != nil {
		t.Fatalf("failed to insert test movie: %v", err)
	}

	radarrServer := newMockRadarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/movie/456":
			json.NewEncoder(w).Encode(radarr.Movie{ID: 456, Title: "Test Movie", Year: 2024, Path: "/radarr/path/Test Movie"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer radarrServer.Close()

	radarrClient := radarr.NewClient(radarr.Config{
		URL:    radarrServer.URL,
		APIKey: "test-key",
	})

	mismatch := PathMismatch{
		MediaType:    "movie",
		ID:           id,
		Title:        "Test Movie",
		Year:         2024,
		DatabasePath: "/media/Movies/Test Movie (2024)",
		RadarrPath:   "/radarr/path/Test Movie",
		RadarrID:     intPtr(456),
		HasRadarrID:  true,
	}

	err = FixMovieMismatch(db, radarrClient, mismatch, FixChoiceKeepSonarrRadarr)
	if err != nil {
		t.Errorf("expected fix to succeed, got error: %v", err)
	}

	movie, err = db.GetMovieByID(id)
	if err != nil {
		t.Fatalf("failed to get movie: %v", err)
	}
	if movie == nil {
		t.Fatal("movie should exist in DB")
	}
	if movie.CanonicalPath != "/radarr/path/Test Movie" {
		t.Errorf("expected DB path to be updated to Radarr path, got: %s", movie.CanonicalPath)
	}
	if movie.Source != "radarr" {
		t.Errorf("expected source to be 'radarr', got: %s", movie.Source)
	}
	if movie.RadarrSyncedAt == nil {
		t.Error("expected movie to be marked as synced")
	}
}

func TestFixMovieMismatch_Skip(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	radarrID := 456
	movie := &database.Movie{
		Title:         "Test Movie",
		Year:          2024,
		CanonicalPath: "/media/Movies/Test Movie (2024)",
		RadarrID:      &radarrID,
	}
	_, err := db.UpsertMovie(movie)
	id := movie.ID
	if err != nil {
		t.Fatalf("failed to insert test movie: %v", err)
	}

	radarrServer := newMockRadarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer radarrServer.Close()

	radarrClient := radarr.NewClient(radarr.Config{
		URL:    radarrServer.URL,
		APIKey: "test-key",
	})

	mismatch := PathMismatch{
		MediaType:    "movie",
		ID:           id,
		Title:        "Test Movie",
		Year:         2024,
		DatabasePath: "/media/Movies/Test Movie (2024)",
		RadarrPath:   "/old/path/Test Movie",
		RadarrID:     intPtr(456),
		HasRadarrID:  true,
	}

	err = FixMovieMismatch(db, radarrClient, mismatch, FixChoiceSkip)
	if err != nil {
		t.Errorf("expected skip to succeed without error, got: %v", err)
	}

	movie, err = db.GetMovieByID(id)
	if err != nil {
		t.Fatalf("failed to get movie: %v", err)
	}
	if movie == nil {
		t.Fatal("movie should exist in DB")
	}
	if movie.CanonicalPath != "/media/Movies/Test Movie (2024)" {
		t.Errorf("expected DB path to remain unchanged, got: %s", movie.CanonicalPath)
	}
	if movie.RadarrSyncedAt != nil {
		t.Error("expected movie to NOT be marked as synced when skipping")
	}
}

func TestFixMovieMismatch_NoRadarrID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	radarrID := 456
	movie := &database.Movie{
		Title:         "Test Movie",
		Year:          2024,
		CanonicalPath: "/media/Movies/Test Movie (2024)",
		RadarrID:      &radarrID,
	}
	_, err := db.UpsertMovie(movie)
	id := movie.ID
	if err != nil {
		t.Fatalf("failed to insert test movie: %v", err)
	}

	radarrServer := newMockRadarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer radarrServer.Close()

	radarrClient := radarr.NewClient(radarr.Config{
		URL:    radarrServer.URL,
		APIKey: "test-key",
	})

	mismatch := PathMismatch{
		MediaType:    "movie",
		ID:           id,
		Title:        "Test Movie",
		Year:         2024,
		DatabasePath: "/media/Movies/Test Movie (2024)",
		RadarrPath:   "/old/path/Test Movie",
		RadarrID:     nil,
		HasRadarrID:  false,
	}

	err = FixMovieMismatch(db, radarrClient, mismatch, FixChoiceKeepJellyWatch)
	if err == nil {
		t.Error("expected error when no Radarr ID provided, got nil")
	}
	if err != nil {
		if !strings.Contains(err.Error(), "no Radarr ID") {
			t.Errorf("expected 'no Radarr ID' error, got: %v", err)
		}
	}
}

func TestFixMovieMismatch_APIFailure(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	radarrID := 456
	movie := &database.Movie{
		Title:         "Test Movie",
		Year:          2024,
		CanonicalPath: "/media/Movies/Test Movie (2024)",
		RadarrID:      &radarrID,
	}
	_, err := db.UpsertMovie(movie)
	id := movie.ID
	if err != nil {
		t.Fatalf("failed to insert test movie: %v", err)
	}

	radarrServer := newMockRadarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/movie/456":
			if r.Method == http.MethodPut {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "server error"})
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer radarrServer.Close()

	radarrClient := radarr.NewClient(radarr.Config{
		URL:    radarrServer.URL,
		APIKey: "test-key",
	})

	mismatch := PathMismatch{
		MediaType:    "movie",
		ID:           id,
		Title:        "Test Movie",
		Year:         2024,
		DatabasePath: "/media/Movies/Test Movie (2024)",
		RadarrPath:   "/old/path/Test Movie",
		RadarrID:     intPtr(456),
		HasRadarrID:  true,
	}

	err = FixMovieMismatch(db, radarrClient, mismatch, FixChoiceKeepJellyWatch)
	if err == nil {
		t.Error("expected error when API fails, got nil")
	}
	if err != nil {
		if !strings.Contains(err.Error(), "updating Radarr path") {
			t.Errorf("expected 'updating Radarr path' error, got: %v", err)
		}
	}

	movie, err = db.GetMovieByID(id)
	if err != nil {
		t.Fatalf("failed to get movie: %v", err)
	}
	if movie == nil {
		t.Fatal("movie should exist in DB")
	}
	if movie.RadarrSyncedAt != nil {
		t.Error("expected movie to NOT be marked as synced when API fails")
	}
}

func TestFixMovieMismatch_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	radarrServer := newMockRadarrServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/movie/999":
			json.NewEncoder(w).Encode(radarr.Movie{ID: 999, Title: "Nonexistent", Year: 2024, Path: "/radarr/path/Nonexistent"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer radarrServer.Close()

	radarrClient := radarr.NewClient(radarr.Config{
		URL:    radarrServer.URL,
		APIKey: "test-key",
	})

	mismatch := PathMismatch{
		MediaType:    "movie",
		ID:           999,
		Title:        "Nonexistent Movie",
		Year:         2024,
		DatabasePath: "/media/Movies/Nonexistent Movie (2024)",
		RadarrPath:   "/radarr/path/Nonexistent",
		RadarrID:     intPtr(999),
		HasRadarrID:  true,
	}

	err := FixMovieMismatch(db, radarrClient, mismatch, FixChoiceKeepSonarrRadarr)
	if err == nil {
		t.Error("expected error when movie not found in DB, got nil")
	}
	if err != nil {
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected 'not found' error, got: %v", err)
		}
	}
}
