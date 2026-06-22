package service

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/database"
)

func TestRebuildMoviesFromMediaFilesUsesMovieLibrariesOnly(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	root := t.TempDir()
	movieRoot := filepath.Join(root, "MOVIES")
	tvRoot := filepath.Join(root, "TVSHOWS")
	year := 2026
	movieFile := &database.MediaFile{
		Path:            filepath.Join(movieRoot, "Example Movie (2026)", "Example Movie (2026).mkv"),
		Size:            200,
		ModifiedAt:      time.Now(),
		MediaType:       "movie",
		NormalizedTitle: "examplemovie",
		Year:            &year,
		QualityScore:    10,
		LibraryRoot:     movieRoot,
	}
	tvMisclassified := &database.MediaFile{
		Path:            filepath.Join(tvRoot, "Season 01", "episode.mkv"),
		Size:            300,
		ModifiedAt:      time.Now(),
		MediaType:       "movie",
		NormalizedTitle: "season01",
		Year:            &year,
		QualityScore:    99,
		LibraryRoot:     tvRoot,
	}
	if err := db.UpsertMediaFile(movieFile); err != nil {
		t.Fatalf("failed to insert movie file: %v", err)
	}
	if err := db.UpsertMediaFile(tvMisclassified); err != nil {
		t.Fatalf("failed to insert misclassified TV file: %v", err)
	}

	result, err := NewCleanupService(db).RebuildMoviesFromMediaFiles([]string{movieRoot})
	if err != nil {
		t.Fatalf("RebuildMoviesFromMediaFiles failed: %v", err)
	}
	if result.FilesConsidered != 1 {
		t.Fatalf("FilesConsidered = %d, want 1", result.FilesConsidered)
	}
	if result.MoviesRebuilt != 1 {
		t.Fatalf("MoviesRebuilt = %d, want 1", result.MoviesRebuilt)
	}

	rebuilt, err := db.GetMovieByTitle("Example Movie", 2026)
	if err != nil {
		t.Fatalf("GetMovieByTitle failed: %v", err)
	}
	if rebuilt == nil {
		t.Fatalf("expected movie row to be rebuilt")
	}
	var bestFileID int64
	if err := db.DB().QueryRow(`SELECT best_file_id FROM movies WHERE id = ?`, rebuilt.ID).Scan(&bestFileID); err != nil {
		t.Fatalf("failed to query best_file_id: %v", err)
	}
	if bestFileID != movieFile.ID {
		t.Fatalf("best_file_id = %d, want %d", bestFileID, movieFile.ID)
	}
	if got, err := db.GetMovieByTitle("Season 01", 2026); err != nil {
		t.Fatalf("GetMovieByTitle for TV artefact failed: %v", err)
	} else if got != nil {
		t.Fatalf("TV library artefact should not be rebuilt as a movie")
	}
}
