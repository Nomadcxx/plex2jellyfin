package database

import (
	"path/filepath"
	"testing"
)

func TestFindDuplicateEpisodes_PreservesNullYear(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer db.Close()

	season, episode := 1, 1
	files := []*MediaFile{
		{
			Path:            "/lib1/Show/Season 01/Show S01E01.mkv",
			Size:            100,
			MediaType:       "episode",
			NormalizedTitle: "show",
			Year:            nil,
			Season:          &season,
			Episode:         &episode,
			QualityScore:    100,
			LibraryRoot:     "/lib1",
		},
		{
			Path:            "/lib2/Show/Season 01/Show S01E01.mkv",
			Size:            200,
			MediaType:       "episode",
			NormalizedTitle: "show",
			Year:            nil,
			Season:          &season,
			Episode:         &episode,
			QualityScore:    200,
			LibraryRoot:     "/lib2",
		},
	}
	for _, f := range files {
		if err := db.UpsertMediaFile(f); err != nil {
			t.Fatalf("UpsertMediaFile: %v", err)
		}
	}

	groups, err := db.FindDuplicateEpisodes()
	if err != nil {
		t.Fatalf("FindDuplicateEpisodes: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}
	if groups[0].Year != nil {
		t.Fatalf("Year = %v, want nil", groups[0].Year)
	}
	if len(groups[0].Files) != 2 {
		t.Fatalf("files = %d, want 2 (NULL year must use year IS NULL)", len(groups[0].Files))
	}
	if groups[0].SpaceReclaimable != 100 {
		t.Fatalf("SpaceReclaimable = %d, want 100", groups[0].SpaceReclaimable)
	}
}

func TestFindDuplicateMovies_ConstrainsByMediaType(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer db.Close()

	year := 2010
	season, episode := 1, 1
	files := []*MediaFile{
		{
			Path:            "/movies/Inception (2010)/Inception (2010).mkv",
			Size:            100,
			MediaType:       "movie",
			NormalizedTitle: "inception",
			Year:            &year,
			QualityScore:    100,
			LibraryRoot:     "/movies",
		},
		{
			Path:            "/movies2/Inception (2010)/Inception (2010).mkv",
			Size:            200,
			MediaType:       "movie",
			NormalizedTitle: "inception",
			Year:            &year,
			QualityScore:    200,
			LibraryRoot:     "/movies2",
		},
		{
			Path:            "/tv/Inception/Season 01/Inception S01E01.mkv",
			Size:            50,
			MediaType:       "episode",
			NormalizedTitle: "inception",
			Year:            &year,
			Season:          &season,
			Episode:         &episode,
			QualityScore:    50,
			LibraryRoot:     "/tv",
		},
	}
	for _, f := range files {
		if err := db.UpsertMediaFile(f); err != nil {
			t.Fatalf("UpsertMediaFile: %v", err)
		}
	}

	groups, err := db.FindDuplicateMovies()
	if err != nil {
		t.Fatalf("FindDuplicateMovies: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}
	if len(groups[0].Files) != 2 {
		t.Fatalf("files = %d, want 2 movie files only", len(groups[0].Files))
	}
	for _, f := range groups[0].Files {
		if f.MediaType != "movie" {
			t.Fatalf("unexpected media_type %q in movie duplicate group", f.MediaType)
		}
	}
}
