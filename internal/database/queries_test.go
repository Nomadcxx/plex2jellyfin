package database

import (
	"path/filepath"
	"testing"
)

func TestFindDuplicateEpisodesScopesSameTitleToParentSeries(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer db.Close()

	makeSeries := func(title string) *Series {
		t.Helper()
		s := &Series{
			Title: title, Year: 2025, CanonicalPath: "/tv/" + title,
			LibraryRoot: "/tv", Source: "filesystem", SourcePriority: 50,
		}
		if _, err := db.UpsertSeries(s); err != nil {
			t.Fatalf("UpsertSeries(%q): %v", title, err)
		}
		return s
	}
	mainShow := makeSeries("The Pitt")
	podcast := makeSeries("The Pitt Podcast")
	season, episode, year := 1, 5, 2025
	files := []*MediaFile{
		{Path: "/tv/main-a.mkv", Size: 100, MediaType: "episode", ParentSeriesID: &mainShow.ID, NormalizedTitle: "the pitt", Year: &year, Season: &season, Episode: &episode, QualityScore: 100, LibraryRoot: "/tv"},
		{Path: "/tv/main-b.mkv", Size: 200, MediaType: "episode", ParentSeriesID: &mainShow.ID, NormalizedTitle: "the pitt", Year: &year, Season: &season, Episode: &episode, QualityScore: 200, LibraryRoot: "/tv"},
		{Path: "/tv/podcast.mkv", Size: 150, MediaType: "episode", ParentSeriesID: &podcast.ID, NormalizedTitle: "the pitt", Year: &year, Season: &season, Episode: &episode, QualityScore: 150, LibraryRoot: "/tv"},
	}
	for _, file := range files {
		if err := db.UpsertMediaFile(file); err != nil {
			t.Fatalf("UpsertMediaFile(%s): %v", file.Path, err)
		}
	}

	groups, err := db.FindDuplicateEpisodes()
	if err != nil {
		t.Fatalf("FindDuplicateEpisodes: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1 same-series group", len(groups))
	}
	if len(groups[0].Files) != 2 {
		t.Fatalf("files = %d, want 2 files from The Pitt", len(groups[0].Files))
	}
	for _, file := range groups[0].Files {
		if file.ParentSeriesID == nil || *file.ParentSeriesID != mainShow.ID {
			t.Fatalf("duplicate group contains wrong series file: %+v", file)
		}
	}
}

func TestFindDuplicateEpisodes_PreservesNullYear(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer db.Close()

	show := &Series{
		Title: "Show", Year: 2023, CanonicalPath: "/tv/Show", LibraryRoot: "/tv",
		Source: "filesystem", SourcePriority: 50,
	}
	if _, err := db.UpsertSeries(show); err != nil {
		t.Fatalf("UpsertSeries: %v", err)
	}
	season, episode := 1, 1
	files := []*MediaFile{
		{
			Path:            "/lib1/Show/Season 01/Show S01E01.mkv",
			Size:            100,
			MediaType:       "episode",
			ParentSeriesID:  &show.ID,
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
			ParentSeriesID:  &show.ID,
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
