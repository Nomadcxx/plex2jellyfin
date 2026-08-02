package database

import (
	"path/filepath"
	"testing"
)

func TestGetLibraryStats_UsesCanonicalDuplicateAnalysis(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer db.Close()

	year := 1999
	files := []*MediaFile{
		{
			Path:            "/a/The Matrix (1999)/The Matrix (1999).mkv",
			Size:            1000,
			MediaType:       "movie",
			NormalizedTitle: "the matrix",
			Year:            &year,
			QualityScore:    100,
			LibraryRoot:     "/a",
		},
		{
			Path:            "/b/The Matrix (1999)/The Matrix (1999).mkv",
			Size:            2000,
			MediaType:       "movie",
			NormalizedTitle: "the matrix",
			Year:            &year,
			QualityScore:    200,
			LibraryRoot:     "/b",
		},
	}
	for _, f := range files {
		if err := db.UpsertMediaFile(f); err != nil {
			t.Fatalf("UpsertMediaFile: %v", err)
		}
	}
	if _, err := db.UpsertMovie(&Movie{
		Title: "The Matrix", Year: year, CanonicalPath: "/a/The Matrix (1999)",
		LibraryRoot: "/a", Source: "filesystem", SourcePriority: 50,
	}); err != nil {
		t.Fatalf("UpsertMovie: %v", err)
	}

	stats, err := db.GetLibraryStats()
	if err != nil {
		t.Fatalf("GetLibraryStats: %v", err)
	}
	if stats.DuplicateGroups != 1 {
		t.Fatalf("DuplicateGroups = %d, want 1 (canonical media_files analysis, not dead duplicate tables)", stats.DuplicateGroups)
	}
	if stats.ReclaimableBytes != 1000 {
		t.Fatalf("ReclaimableBytes = %d, want 1000", stats.ReclaimableBytes)
	}
	if stats.TotalFiles != 2 {
		t.Fatalf("TotalFiles = %d, want 2", stats.TotalFiles)
	}
	if stats.MovieCount != 1 {
		t.Fatalf("MovieCount = %d, want 1", stats.MovieCount)
	}
}

func TestGetLibraryStats_NullYearDuplicatesCounted(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer db.Close()

	season, episode := 2, 5
	for i, root := range []string{"/lib1", "/lib2"} {
		f := &MediaFile{
			Path:            root + "/Show/Season 02/Show S02E05.mkv",
			Size:            int64(100 * (i + 1)),
			MediaType:       "episode",
			NormalizedTitle: "show",
			Year:            nil,
			Season:          &season,
			Episode:         &episode,
			QualityScore:    100 * (i + 1),
			LibraryRoot:     root,
		}
		if err := db.UpsertMediaFile(f); err != nil {
			t.Fatalf("UpsertMediaFile: %v", err)
		}
	}

	stats, err := db.GetLibraryStats()
	if err != nil {
		t.Fatalf("GetLibraryStats: %v", err)
	}
	if stats.DuplicateGroups != 1 {
		t.Fatalf("DuplicateGroups = %d, want 1 for NULL-year episode duplicates", stats.DuplicateGroups)
	}
	if stats.ReclaimableBytes != 100 {
		t.Fatalf("ReclaimableBytes = %d, want 100", stats.ReclaimableBytes)
	}
}
