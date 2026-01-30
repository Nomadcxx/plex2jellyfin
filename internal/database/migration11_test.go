package database

import (
	"testing"
)

func TestMigration11_DirtyFlags(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	series := &Series{
		Title:         "Test Show",
		Year:          2020,
		CanonicalPath: "/tv/Test Show (2020)",
		LibraryRoot:   "/tv",
		Source:        "jellywatch",
	}

	_, err := db.UpsertSeries(series)
	if err != nil {
		t.Fatalf("UpsertSeries failed: %v", err)
	}

	retrieved, err := db.GetSeriesByTitle("Test Show", 2020)
	if err != nil {
		t.Fatalf("GetSeriesByTitle failed: %v", err)
	}

	if retrieved.SonarrSyncedAt != nil {
		t.Errorf("expected SonarrSyncedAt to be nil (default), got %v", retrieved.SonarrSyncedAt)
	}
	if retrieved.SonarrPathDirty {
		t.Errorf("expected SonarrPathDirty to be false (default), got true")
	}
	if retrieved.RadarrSyncedAt != nil {
		t.Errorf("expected RadarrSyncedAt to be nil (default), got %v", retrieved.RadarrSyncedAt)
	}
	if retrieved.RadarrPathDirty {
		t.Errorf("expected RadarrPathDirty to be false (default), got true")
	}

	movie := &Movie{
		Title:         "Test Movie",
		Year:          2021,
		CanonicalPath: "/movies/Test Movie (2021)",
		LibraryRoot:   "/movies",
		Source:        "jellywatch",
	}

	_, err = db.UpsertMovie(movie)
	if err != nil {
		t.Fatalf("UpsertMovie failed: %v", err)
	}

	retrievedMovie, err := db.GetMovieByTitle("Test Movie", 2021)
	if err != nil {
		t.Fatalf("GetMovieByTitle failed: %v", err)
	}

	if retrievedMovie.RadarrSyncedAt != nil {
		t.Errorf("expected RadarrSyncedAt to be nil (default), got %v", retrievedMovie.RadarrSyncedAt)
	}
	if retrievedMovie.RadarrPathDirty {
		t.Errorf("expected RadarrPathDirty to be false (default), got true")
	}
}
