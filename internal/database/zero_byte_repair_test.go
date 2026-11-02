package database

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateEpisodeBestFileRejectsZeroByteMediaFile(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	series := &Series{Title: "Empty Best File", Year: 2024, CanonicalPath: "/tv/Empty Best File", LibraryRoot: "/tv", Source: "filesystem", SourcePriority: 50}
	if _, err := db.UpsertSeries(series); err != nil {
		t.Fatalf("UpsertSeries: %v", err)
	}
	episode := &Episode{SeriesID: series.ID, Season: 1, Episode: 1}
	if err := db.UpsertEpisode(episode); err != nil {
		t.Fatalf("UpsertEpisode: %v", err)
	}
	file := &MediaFile{Path: "/tv/empty-best.mkv", Size: 0, MediaType: "episode", ParentSeriesID: &series.ID, NormalizedTitle: "empty best file"}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("UpsertMediaFile: %v", err)
	}

	if err := db.UpdateEpisodeBestFile(episode.ID, &file.ID); err == nil {
		t.Fatal("UpdateEpisodeBestFile accepted a zero-byte file")
	}
}

func TestUpsertEpisodeRejectsZeroByteBestFile(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	series := &Series{Title: "Empty Upsert File", Year: 2024, CanonicalPath: "/tv/Empty Upsert File", LibraryRoot: "/tv", Source: "filesystem", SourcePriority: 50}
	if _, err := db.UpsertSeries(series); err != nil {
		t.Fatalf("UpsertSeries: %v", err)
	}
	file := &MediaFile{Path: "/tv/empty-upsert.mkv", Size: 0, MediaType: "episode", ParentSeriesID: &series.ID, NormalizedTitle: "empty upsert file"}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("UpsertMediaFile: %v", err)
	}
	episode := &Episode{SeriesID: series.ID, Season: 1, Episode: 1, BestFileID: &file.ID}
	if err := db.UpsertEpisode(episode); err == nil {
		t.Fatal("UpsertEpisode accepted a zero-byte best file")
	}
}

func TestUpdateMovieBestFileRejectsZeroByteMediaFile(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	movie := &Movie{Title: "Empty Movie", Year: 2024, CanonicalPath: "/movies/Empty Movie (2024)", LibraryRoot: "/movies", Source: "filesystem", SourcePriority: 50}
	if _, err := db.UpsertMovie(movie); err != nil {
		t.Fatalf("UpsertMovie: %v", err)
	}
	year := 2024
	file := &MediaFile{Path: "/movies/empty-movie.mkv", Size: 0, MediaType: "movie", ParentMovieID: &movie.ID, NormalizedTitle: "empty movie", Year: &year}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("UpsertMediaFile: %v", err)
	}
	if err := db.UpdateMovieBestFile(movie.ID, &file.ID); err == nil {
		t.Fatal("UpdateMovieBestFile accepted a zero-byte file")
	}
}

func TestFindDuplicateMoviesExcludesZeroByteRowsFromBestFileSelection(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	year := 2024
	empty := &MediaFile{Path: "/movies/empty.mkv", Size: 0, MediaType: "movie", NormalizedTitle: "sample film", Year: &year, QualityScore: 1000}
	good := &MediaFile{Path: "/movies/good.mkv", Size: 100, MediaType: "movie", NormalizedTitle: "sample film", Year: &year, QualityScore: 10}
	other := &MediaFile{Path: "/movies/other.mkv", Size: 90, MediaType: "movie", NormalizedTitle: "sample film", Year: &year, QualityScore: 5}
	for _, file := range []*MediaFile{empty, good, other} {
		if err := db.UpsertMediaFile(file); err != nil {
			t.Fatalf("insert media file %s: %v", file.Path, err)
		}
	}

	groups, err := db.FindDuplicateMovies()
	if err != nil {
		t.Fatalf("FindDuplicateMovies: %v", err)
	}
	if len(groups) != 1 || len(groups[0].Files) != 2 {
		t.Fatalf("duplicate groups = %#v, want one group with the two usable files", groups)
	}
	if groups[0].BestFile == nil || groups[0].BestFile.ID != good.ID {
		t.Fatalf("best file = %#v, want nonempty file id %d", groups[0].BestFile, good.ID)
	}
}

func TestFindDuplicateEpisodesDoesNotTreatZeroByteRowAsDuplicate(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	series := &Series{Title: "Empty Episode", Year: 2024, CanonicalPath: "/tv/Empty Episode", LibraryRoot: "/tv", Source: "filesystem", SourcePriority: 50}
	if _, err := db.UpsertSeries(series); err != nil {
		t.Fatalf("UpsertSeries: %v", err)
	}
	year, season, episode := 2024, 1, 1
	for _, file := range []*MediaFile{
		{Path: "/tv/empty.mkv", Size: 0, MediaType: "episode", ParentSeriesID: &series.ID, NormalizedTitle: "empty episode", Year: &year, Season: &season, Episode: &episode, QualityScore: 1000},
		{Path: "/tv/good.mkv", Size: 100, MediaType: "episode", ParentSeriesID: &series.ID, NormalizedTitle: "empty episode", Year: &year, Season: &season, Episode: &episode, QualityScore: 10},
	} {
		if err := db.UpsertMediaFile(file); err != nil {
			t.Fatalf("insert media file %s: %v", file.Path, err)
		}
	}

	groups, err := db.FindDuplicateEpisodes()
	if err != nil {
		t.Fatalf("FindDuplicateEpisodes: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("duplicate groups = %#v, want no duplicate group with only one nonempty file", groups)
	}
}

func TestRepairZeroByteMediaFilesDryRunAndExecute(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	root := t.TempDir()
	series := &Series{Title: "Zero Byte Repair", Year: 2024, CanonicalPath: filepath.Join(root, "Zero Byte Repair"), LibraryRoot: root, Source: "filesystem", SourcePriority: 50}
	if _, err := db.UpsertSeries(series); err != nil {
		t.Fatalf("UpsertSeries: %v", err)
	}
	withoutAlternative := &Episode{SeriesID: series.ID, Season: 1, Episode: 1}
	withAlternative := &Episode{SeriesID: series.ID, Season: 1, Episode: 2}
	for _, episode := range []*Episode{withoutAlternative, withAlternative} {
		if err := db.UpsertEpisode(episode); err != nil {
			t.Fatalf("UpsertEpisode: %v", err)
		}
	}

	emptyPath := filepath.Join(root, "empty.mkv")
	if err := os.WriteFile(emptyPath, nil, 0o644); err != nil {
		t.Fatalf("create empty file: %v", err)
	}
	empty := &MediaFile{Path: emptyPath, Size: 0, MediaType: "episode", ParentSeriesID: &series.ID, ParentEpisodeID: &withoutAlternative.ID, NormalizedTitle: "zero byte repair", Season: intPtr(1), Episode: intPtr(1), QualityScore: 50}
	if err := db.UpsertMediaFile(empty); err != nil {
		t.Fatalf("insert empty file row: %v", err)
	}

	secondEmptyPath := filepath.Join(root, "second-empty.mkv")
	if err := os.WriteFile(secondEmptyPath, nil, 0o644); err != nil {
		t.Fatalf("create second empty file: %v", err)
	}
	secondEmpty := &MediaFile{Path: secondEmptyPath, Size: 0, MediaType: "episode", ParentSeriesID: &series.ID, ParentEpisodeID: &withAlternative.ID, NormalizedTitle: "zero byte repair", Season: intPtr(1), Episode: intPtr(2), QualityScore: 100}
	if err := db.UpsertMediaFile(secondEmpty); err != nil {
		t.Fatalf("insert second empty file row: %v", err)
	}

	alternativePath := filepath.Join(root, "alternative.mkv")
	if err := os.WriteFile(alternativePath, make([]byte, 100), 0o644); err != nil {
		t.Fatalf("create alternative file: %v", err)
	}
	alternative := &MediaFile{Path: alternativePath, Size: 100, MediaType: "episode", ParentSeriesID: &series.ID, NormalizedTitle: "zero byte repair", Season: intPtr(1), Episode: intPtr(2), QualityScore: 10}
	if err := db.UpsertMediaFile(alternative); err != nil {
		t.Fatalf("insert alternative file row: %v", err)
	}

	staleNonEmptyPath := filepath.Join(root, "stale-nonempty.mkv")
	if err := os.WriteFile(staleNonEmptyPath, []byte("not empty"), 0o644); err != nil {
		t.Fatalf("create stale nonempty file: %v", err)
	}
	staleNonEmpty := &MediaFile{Path: staleNonEmptyPath, Size: 0, MediaType: "episode", NormalizedTitle: "stale nonempty", QualityScore: 20}
	if err := db.UpsertMediaFile(staleNonEmpty); err != nil {
		t.Fatalf("insert stale nonempty row: %v", err)
	}
	missing := &MediaFile{Path: filepath.Join(root, "missing.mkv"), Size: 0, MediaType: "episode", NormalizedTitle: "missing", QualityScore: 20}
	if err := db.UpsertMediaFile(missing); err != nil {
		t.Fatalf("insert missing row: %v", err)
	}

	if _, err := db.SQL().Exec(`UPDATE episodes SET best_file_id = ? WHERE id = ?`, empty.ID, withoutAlternative.ID); err != nil {
		t.Fatalf("seed old best file reference: %v", err)
	}
	if _, err := db.SQL().Exec(`UPDATE episodes SET best_file_id = ? WHERE id = ?`, secondEmpty.ID, withAlternative.ID); err != nil {
		t.Fatalf("seed second old best file reference: %v", err)
	}

	dryRun, err := db.RepairZeroByteMediaFiles(true)
	if err != nil {
		t.Fatalf("dry-run repair: %v", err)
	}
	if len(dryRun.VerifiedPaths) != 2 || dryRun.Unavailable != 1 || dryRun.NotEmpty != 1 || dryRun.RowsMarked != 0 {
		t.Fatalf("dry-run report = %+v, want two verified empty paths and no writes", dryRun)
	}
	if file, err := db.GetMediaFile(emptyPath); err != nil || file == nil || file.NeedsReview {
		t.Fatalf("dry run changed empty row: file=%+v err=%v", file, err)
	}
	if episode, err := db.GetEpisode(series.ID, 1, 1); err != nil || episode == nil || episode.BestFileID == nil || *episode.BestFileID != empty.ID {
		t.Fatalf("dry run changed empty best-file reference: episode=%+v err=%v", episode, err)
	}

	executed, err := db.RepairZeroByteMediaFiles(false)
	if err != nil {
		t.Fatalf("execute repair: %v", err)
	}
	if len(executed.VerifiedPaths) != 2 || executed.RowsMarked != 2 || executed.Unavailable != 1 || executed.NotEmpty != 1 {
		t.Fatalf("execute report = %+v, want two marked empty paths and skipped nonempty/missing paths", executed)
	}
	for _, path := range []string{emptyPath, secondEmptyPath} {
		file, err := db.GetMediaFile(path)
		if err != nil || file == nil || !file.NeedsReview {
			t.Errorf("verified zero-byte row %s was not retained and marked for review: file=%+v err=%v", path, file, err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("repair removed zero-byte source %s: %v", path, err)
		}
	}
	if episode, err := db.GetEpisode(series.ID, 1, 1); err != nil || episode == nil || episode.BestFileID != nil {
		t.Errorf("empty-only episode best file = %+v, %v; want nil", episode, err)
	}
	if episode, err := db.GetEpisode(series.ID, 1, 2); err != nil || episode == nil || episode.BestFileID == nil || *episode.BestFileID != alternative.ID {
		t.Errorf("episode alternative best file = %+v, %v; want %d", episode, err, alternative.ID)
	}
	for _, path := range []string{staleNonEmptyPath, missing.Path} {
		file, err := db.GetMediaFile(path)
		if err != nil || file == nil || file.NeedsReview {
			t.Errorf("unverified row %s changed: file=%+v err=%v", path, file, err)
		}
	}
}

func TestRepairZeroByteMediaFilesRepairsMovieBestFile(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	root := t.TempDir()
	year := 2024
	movie := &Movie{Title: "Zero Byte Movie", Year: year, CanonicalPath: filepath.Join(root, "Zero Byte Movie (2024)"), LibraryRoot: root, Source: "filesystem", SourcePriority: 50}
	if _, err := db.UpsertMovie(movie); err != nil {
		t.Fatalf("UpsertMovie: %v", err)
	}

	emptyPath := filepath.Join(root, "empty.mkv")
	if err := os.WriteFile(emptyPath, nil, 0o644); err != nil {
		t.Fatalf("create empty file: %v", err)
	}
	empty := &MediaFile{Path: emptyPath, Size: 0, MediaType: "movie", ParentMovieID: &movie.ID, NormalizedTitle: "zero byte movie", Year: &year, QualityScore: 100}
	if err := db.UpsertMediaFile(empty); err != nil {
		t.Fatalf("insert empty movie row: %v", err)
	}

	goodPath := filepath.Join(root, "good.mkv")
	if err := os.WriteFile(goodPath, make([]byte, 100), 0o644); err != nil {
		t.Fatalf("create nonempty file: %v", err)
	}
	good := &MediaFile{Path: goodPath, Size: 100, MediaType: "movie", ParentMovieID: &movie.ID, NormalizedTitle: "zero byte movie", Year: &year, QualityScore: 10}
	if err := db.UpsertMediaFile(good); err != nil {
		t.Fatalf("insert nonempty movie row: %v", err)
	}
	if _, err := db.SQL().Exec(`UPDATE movies SET best_file_id = ? WHERE id = ?`, empty.ID, movie.ID); err != nil {
		t.Fatalf("seed zero-byte best-file reference: %v", err)
	}

	if _, err := db.RepairZeroByteMediaFiles(false); err != nil {
		t.Fatalf("execute repair: %v", err)
	}

	var bestFileID int64
	if err := db.SQL().QueryRow(`SELECT best_file_id FROM movies WHERE id = ?`, movie.ID).Scan(&bestFileID); err != nil {
		t.Fatalf("read movie best-file reference: %v", err)
	}
	if bestFileID != good.ID {
		t.Fatalf("movie best_file_id = %d, want nonempty file id %d", bestFileID, good.ID)
	}
}

func TestRepairEpisodeFileLinksNeverSelectsZeroByteBestFile(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	series := &Series{Title: "Episode Link Empty", Year: 2024, CanonicalPath: "/tv/Episode Link Empty", LibraryRoot: "/tv", Source: "filesystem", SourcePriority: 50}
	if _, err := db.UpsertSeries(series); err != nil {
		t.Fatalf("UpsertSeries: %v", err)
	}
	episode := &Episode{SeriesID: series.ID, Season: 1, Episode: 1}
	if err := db.UpsertEpisode(episode); err != nil {
		t.Fatalf("UpsertEpisode: %v", err)
	}
	season, number := 1, 1
	empty := &MediaFile{Path: "/tv/episode-empty.mkv", Size: 0, MediaType: "episode", ParentSeriesID: &series.ID, NormalizedTitle: "episode link empty", Season: &season, Episode: &number, QualityScore: 1000}
	good := &MediaFile{Path: "/tv/episode-good.mkv", Size: 100, MediaType: "episode", ParentSeriesID: &series.ID, NormalizedTitle: "episode link empty", Season: &season, Episode: &number, QualityScore: 10}
	for _, file := range []*MediaFile{empty, good} {
		if err := db.UpsertMediaFile(file); err != nil {
			t.Fatalf("UpsertMediaFile: %v", err)
		}
	}
	if _, err := db.SQL().Exec(`UPDATE episodes SET best_file_id = ? WHERE id = ?`, empty.ID, episode.ID); err != nil {
		t.Fatalf("seed zero-byte best-file reference: %v", err)
	}

	if _, err := db.RepairEpisodeFileLinks(false); err != nil {
		t.Fatalf("RepairEpisodeFileLinks: %v", err)
	}
	got, err := db.GetEpisode(series.ID, 1, 1)
	if err != nil {
		t.Fatalf("GetEpisode: %v", err)
	}
	if got == nil || got.BestFileID == nil || *got.BestFileID != good.ID {
		t.Fatalf("best file = %+v, want nonempty file id %d", got, good.ID)
	}
}
