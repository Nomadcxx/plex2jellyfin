package database

import (
	"testing"
)

func TestGetDirtySeries(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	series1 := &Series{
		Title:         "Test Show 1",
		Year:          2020,
		CanonicalPath: "/tv/Test Show 1 (2020)",
		LibraryRoot:   "/tv",
		Source:        "jellywatch",
	}
	_, err := db.UpsertSeries(series1)
	if err != nil {
		t.Fatalf("UpsertSeries failed: %v", err)
	}

	err = db.SetSeriesDirty(series1.ID)
	if err != nil {
		t.Fatalf("SetSeriesDirty failed: %v", err)
	}

	series2 := &Series{
		Title:         "Test Show 2",
		Year:          2021,
		CanonicalPath: "/tv/Test Show 2 (2021)",
		LibraryRoot:   "/tv",
		Source:        "jellywatch",
	}
	_, err = db.UpsertSeries(series2)
	if err != nil {
		t.Fatalf("UpsertSeries failed: %v", err)
	}

	dirtySeries, err := db.GetDirtySeries()
	if err != nil {
		t.Fatalf("GetDirtySeries failed: %v", err)
	}

	if len(dirtySeries) != 1 {
		t.Errorf("expected 1 dirty series, got %d", len(dirtySeries))
	}

	if len(dirtySeries) > 0 && dirtySeries[0].ID != series1.ID {
		t.Errorf("expected dirty series ID %d, got %d", series1.ID, dirtySeries[0].ID)
	}
}

func TestGetDirtyMovies(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	movie1 := &Movie{
		Title:         "Test Movie 1",
		Year:          2020,
		CanonicalPath: "/movies/Test Movie 1 (2020)",
		LibraryRoot:   "/movies",
		Source:        "jellywatch",
	}
	_, err := db.UpsertMovie(movie1)
	if err != nil {
		t.Fatalf("UpsertMovie failed: %v", err)
	}

	err = db.SetMovieDirty(movie1.ID)
	if err != nil {
		t.Fatalf("SetMovieDirty failed: %v", err)
	}

	movie2 := &Movie{
		Title:         "Test Movie 2",
		Year:          2021,
		CanonicalPath: "/movies/Test Movie 2 (2021)",
		LibraryRoot:   "/movies",
		Source:        "jellywatch",
	}
	_, err = db.UpsertMovie(movie2)
	if err != nil {
		t.Fatalf("UpsertMovie failed: %v", err)
	}

	dirtyMovies, err := db.GetDirtyMovies()
	if err != nil {
		t.Fatalf("GetDirtyMovies failed: %v", err)
	}

	if len(dirtyMovies) != 1 {
		t.Errorf("expected 1 dirty movie, got %d", len(dirtyMovies))
	}

	if len(dirtyMovies) > 0 && dirtyMovies[0].ID != movie1.ID {
		t.Errorf("expected dirty movie ID %d, got %d", movie1.ID, dirtyMovies[0].ID)
	}
}

func TestMarkSeriesSynced(t *testing.T) {
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

	err = db.SetSeriesDirty(series.ID)
	if err != nil {
		t.Fatalf("SetSeriesDirty failed: %v", err)
	}

	retrieved, err := db.GetSeriesByID(series.ID)
	if err != nil {
		t.Fatalf("GetSeriesByID failed: %v", err)
	}
	if !retrieved.SonarrPathDirty {
		t.Error("expected series to be dirty before marking synced")
	}

	err = db.MarkSeriesSynced(series.ID)
	if err != nil {
		t.Fatalf("MarkSeriesSynced failed: %v", err)
	}

	retrieved, err = db.GetSeriesByID(series.ID)
	if err != nil {
		t.Fatalf("GetSeriesByID after sync failed: %v", err)
	}
	if retrieved.SonarrPathDirty {
		t.Error("expected series dirty flag to be cleared after marking synced")
	}
	if retrieved.SonarrSyncedAt == nil {
		t.Error("expected SonarrSyncedAt to be set after marking synced")
	}
}

func TestMarkMovieSynced(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	movie := &Movie{
		Title:         "Test Movie",
		Year:          2020,
		CanonicalPath: "/movies/Test Movie (2020)",
		LibraryRoot:   "/movies",
		Source:        "jellywatch",
	}
	_, err := db.UpsertMovie(movie)
	if err != nil {
		t.Fatalf("UpsertMovie failed: %v", err)
	}

	err = db.SetMovieDirty(movie.ID)
	if err != nil {
		t.Fatalf("SetMovieDirty failed: %v", err)
	}

	retrieved, err := db.GetMovieByID(movie.ID)
	if err != nil {
		t.Fatalf("GetMovieByID failed: %v", err)
	}
	if !retrieved.RadarrPathDirty {
		t.Error("expected movie to be dirty before marking synced")
	}

	err = db.MarkMovieSynced(movie.ID)
	if err != nil {
		t.Fatalf("MarkMovieSynced failed: %v", err)
	}

	retrieved, err = db.GetMovieByID(movie.ID)
	if err != nil {
		t.Fatalf("GetMovieByID after sync failed: %v", err)
	}
	if retrieved.RadarrPathDirty {
		t.Error("expected movie dirty flag to be cleared after marking synced")
	}
	if retrieved.RadarrSyncedAt == nil {
		t.Error("expected RadarrSyncedAt to be set after marking synced")
	}
}

func TestGetSeriesByID(t *testing.T) {
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

	retrieved, err := db.GetSeriesByID(series.ID)
	if err != nil {
		t.Fatalf("GetSeriesByID failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("expected series to be found")
	}

	if retrieved.Title != "Test Show" {
		t.Errorf("expected title 'Test Show', got '%s'", retrieved.Title)
	}

	nonExistent, err := db.GetSeriesByID(999999)
	if err != nil {
		t.Fatalf("GetSeriesByID with non-existent ID failed: %v", err)
	}
	if nonExistent != nil {
		t.Error("expected nil for non-existent series")
	}
}

func TestGetMovieByID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	movie := &Movie{
		Title:         "Test Movie",
		Year:          2020,
		CanonicalPath: "/movies/Test Movie (2020)",
		LibraryRoot:   "/movies",
		Source:        "jellywatch",
	}
	_, err := db.UpsertMovie(movie)
	if err != nil {
		t.Fatalf("UpsertMovie failed: %v", err)
	}

	retrieved, err := db.GetMovieByID(movie.ID)
	if err != nil {
		t.Fatalf("GetMovieByID failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("expected movie to be found")
	}

	if retrieved.Title != "Test Movie" {
		t.Errorf("expected title 'Test Movie', got '%s'", retrieved.Title)
	}

	nonExistent, err := db.GetMovieByID(999999)
	if err != nil {
		t.Fatalf("GetMovieByID with non-existent ID failed: %v", err)
	}
	if nonExistent != nil {
		t.Error("expected nil for non-existent movie")
	}
}
