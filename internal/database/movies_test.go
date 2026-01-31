package database

import (
	"testing"
)

func TestUpsertMovie_SetsDirtyFlag(t *testing.T) {
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
		t.Error("movie should be dirty after SetMovieDirty")
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
		t.Error("movie should not be dirty after MarkMovieSynced")
	}
}

func TestUpsertMovie_ScansNewColumns(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	movie := &Movie{
		Title:          "Test Movie",
		Year:           2020,
		CanonicalPath:  "/movies/Test Movie (2020)",
		LibraryRoot:    "/movies",
		Source:         "jellywatch",
		SourcePriority: 75,
	}

	_, err := db.UpsertMovie(movie)
	if err != nil {
		t.Fatalf("UpsertMovie with new columns failed: %v", err)
	}

	retrieved, err := db.GetMovieByID(movie.ID)
	if err != nil {
		t.Fatalf("GetMovieByID failed: %v", err)
	}

	if retrieved.SourcePriority != 75 {
		t.Errorf("expected SourcePriority 75, got %d", retrieved.SourcePriority)
	}

	if retrieved.RadarrPathDirty {
		t.Error("RadarrPathDirty should default to false")
	}
	if retrieved.RadarrSyncedAt != nil {
		t.Error("RadarrSyncedAt should default to nil")
	}
}

func TestGetAllMovies_IncludesDirtyFlags(t *testing.T) {
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

	allMovies, err := db.GetAllMovies()
	if err != nil {
		t.Fatalf("GetAllMovies failed: %v", err)
	}

	if len(allMovies) != 1 {
		t.Fatalf("expected 1 movie, got %d", len(allMovies))
	}

	retrieved := allMovies[0]
	if !retrieved.RadarrPathDirty {
		t.Error("dirty flag not included in GetAllMovies result")
	}
	if retrieved.RadarrSyncedAt != nil {
		t.Error("RadarrSyncedAt should be nil before syncing")
	}
}

func TestGetAllMovies_Empty(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	allMovies, err := db.GetAllMovies()
	if err != nil {
		t.Fatalf("GetAllMovies failed on empty DB: %v", err)
	}

	if len(allMovies) != 0 {
		t.Errorf("expected 0 movies, got %d", len(allMovies))
	}
}
