package service

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/database"
)

func setupTestDB(t *testing.T) *database.MediaDB {
	tempFile := filepath.Join(t.TempDir(), "test.db")
	db, err := database.OpenPath(tempFile)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	return db
}

func TestAnalyzeDuplicates_Empty(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	svc := NewCleanupService(db)
	analysis, err := svc.AnalyzeDuplicates()
	if err != nil {
		t.Fatalf("AnalyzeDuplicates failed: %v", err)
	}

	if analysis.TotalGroups != 0 {
		t.Errorf("Expected 0 groups, got %d", analysis.TotalGroups)
	}
}

func TestAnalyzeDuplicates_FindsMovieDuplicates(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert duplicate movies
	year := 2005
	_ = db.UpsertMediaFile(&database.MediaFile{
		Path:            "/storage1/Movies/Robots (2005)/Robots.mkv",
		NormalizedTitle: "robots",
		Year:            &year,
		MediaType:       "movie",
		Size:            4000000000,
		QualityScore:    284,
		Resolution:      "720p",
		SourceType:      "BluRay",
	})
	_ = db.UpsertMediaFile(&database.MediaFile{
		Path:            "/storage2/Movies/Robots (2005)/Robots.mkv",
		NormalizedTitle: "robots",
		Year:            &year,
		MediaType:       "movie",
		Size:            4400000000,
		QualityScore:    84,
		Resolution:      "unknown",
		SourceType:      "unknown",
	})

	svc := NewCleanupService(db)
	analysis, err := svc.AnalyzeDuplicates()
	if err != nil {
		t.Fatalf("AnalyzeDuplicates failed: %v", err)
	}

	if analysis.TotalGroups != 1 {
		t.Errorf("Expected 1 group, got %d", analysis.TotalGroups)
	}

	if len(analysis.Groups) > 0 && len(analysis.Groups[0].Files) != 2 {
		t.Errorf("Expected 2 files in group, got %d", len(analysis.Groups[0].Files))
	}
}

func TestAnalyzeScattered_FindsConflicts(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert conflicting series in different locations (series table, not media_files)
	// The DetectConflicts function queries series/movies tables for conflicts
	year := 2005
	series1 := &database.Series{
		Title:           "American Dad",
		TitleNormalized: "american dad",
		Year:            year,
		CanonicalPath:   "/storage1/TV/American Dad (2005)",
		LibraryRoot:     "/storage1/TV",
		Source:          "filesystem",
		SourcePriority:  50,
		EpisodeCount:    1,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	_, _ = db.UpsertSeries(series1)

	series2 := &database.Series{
		Title:           "American Dad!",
		TitleNormalized: "american dad",
		Year:            year,
		CanonicalPath:   "/storage2/TV/American Dad! (2005)",
		LibraryRoot:     "/storage2/TV",
		Source:          "filesystem",
		SourcePriority:  50,
		EpisodeCount:    1,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	_, _ = db.UpsertSeries(series2)

	svc := NewCleanupService(db)
	analysis, err := svc.AnalyzeScattered()
	if err != nil {
		t.Fatalf("AnalyzeScattered failed: %v", err)
	}

	if analysis.TotalItems != 1 {
		t.Errorf("Expected 1 scattered item, got %d", analysis.TotalItems)
	}
}
