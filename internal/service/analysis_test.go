package service

import (
	"encoding/json"
	"os"
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

	root := t.TempDir()
	path1 := filepath.Join(root, "storage1", "American Dad (2005)")
	path2 := filepath.Join(root, "storage2", "American Dad! (2005)")
	if err := os.MkdirAll(path1, 0755); err != nil {
		t.Fatalf("failed to create path1: %v", err)
	}
	if err := os.MkdirAll(path2, 0755); err != nil {
		t.Fatalf("failed to create path2: %v", err)
	}
	for _, path := range []string{
		filepath.Join(path1, "American Dad S01E01.mkv"),
		filepath.Join(path1, "American Dad S01E02.mkv"),
		filepath.Join(path2, "American Dad S01E03.mkv"),
	} {
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("failed to create media file: %v", err)
		}
		if err := f.Truncate(minConsolidationFileSize + 1); err != nil {
			f.Close()
			t.Fatalf("failed to size media file: %v", err)
		}
		f.Close()
	}

	// Insert conflicting series in different locations (series table, not media_files)
	// The DetectConflicts function queries series/movies tables for conflicts
	year := 2005
	series1 := &database.Series{
		Title:           "American Dad",
		TitleNormalized: "american dad",
		Year:            year,
		CanonicalPath:   path1,
		LibraryRoot:     filepath.Dir(path1),
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
		CanonicalPath:   path2,
		LibraryRoot:     filepath.Dir(path2),
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

func TestAnalyzeScattered_FiltersMissingConflictLocations(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	root := t.TempDir()
	target := filepath.Join(root, "storage1", "American Dad! (2005)")
	source := filepath.Join(root, "storage2", "American Dad! (2005)")
	missing := filepath.Join(root, "storage3", "American Dad!")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatalf("failed to create target: %v", err)
	}
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}
	for _, path := range []string{
		filepath.Join(target, "American Dad S01E01.mkv"),
		filepath.Join(target, "American Dad S01E02.mkv"),
		filepath.Join(source, "American Dad S01E03.mkv"),
	} {
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("failed to create media file: %v", err)
		}
		if err := f.Truncate(minConsolidationFileSize + 1); err != nil {
			f.Close()
			t.Fatalf("failed to size media file: %v", err)
		}
		f.Close()
	}

	locations, err := json.Marshal([]string{missing, target, source})
	if err != nil {
		t.Fatalf("failed to marshal locations: %v", err)
	}
	_, err = db.DB().Exec(`
		INSERT INTO conflicts (media_type, title, title_normalized, year, locations, resolved, created_at)
		VALUES ('series', 'American Dad', 'americandad', 2005, ?, FALSE, CURRENT_TIMESTAMP)
	`, string(locations))
	if err != nil {
		t.Fatalf("failed to insert conflict: %v", err)
	}

	svc := NewCleanupService(db)
	analysis, err := svc.AnalyzeScattered()
	if err != nil {
		t.Fatalf("AnalyzeScattered failed: %v", err)
	}

	if analysis.TotalItems != 1 {
		t.Fatalf("Expected 1 scattered item, got %d", analysis.TotalItems)
	}
	item := analysis.Items[0]
	for _, loc := range item.Locations {
		if loc == missing {
			t.Fatalf("missing location leaked into API response: %v", item.Locations)
		}
	}
	if item.TargetLocation == missing {
		t.Fatalf("target location must not be missing")
	}
	if item.FilesToMove != 1 {
		t.Fatalf("FilesToMove = %d, want 1", item.FilesToMove)
	}
}
