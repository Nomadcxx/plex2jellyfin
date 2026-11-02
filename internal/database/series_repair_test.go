package database

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDedupeSeriesByCanonicalPathDryRunDoesNotMutate(t *testing.T) {
	db := setupSeriesDedupeFixture(t)
	defer db.Close()

	report, err := db.DedupeSeriesByCanonicalPath(true)
	if err != nil {
		t.Fatalf("DedupeSeriesByCanonicalPath dry-run: %v", err)
	}
	if len(report.Groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(report.Groups))
	}
	if !report.DryRun {
		t.Fatal("report should be marked dry-run")
	}
	if report.SeriesDeleted != 1 || report.EpisodesMoved != 1 || report.EpisodesMerged != 1 {
		t.Fatalf("report counts = deleted %d moved %d merged %d, want 1/1/1",
			report.SeriesDeleted, report.EpisodesMoved, report.EpisodesMerged)
	}

	var rows int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM series WHERE canonical_path = ?`, "/tv/Project Runway (2004)").Scan(&rows); err != nil {
		t.Fatalf("count series: %v", err)
	}
	if rows != 2 {
		t.Fatalf("dry-run mutated series rows: got %d, want 2", rows)
	}
}

func TestDedupeSeriesByCanonicalPathMergesRelations(t *testing.T) {
	db := setupSeriesDedupeFixture(t)
	defer db.Close()

	report, err := db.DedupeSeriesByCanonicalPath(false)
	if err != nil {
		t.Fatalf("DedupeSeriesByCanonicalPath execute: %v", err)
	}
	if len(report.Groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(report.Groups))
	}
	keeperID := report.Groups[0].Keeper.ID

	var rows int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM series WHERE canonical_path = ?`, "/tv/Project Runway (2004)").Scan(&rows); err != nil {
		t.Fatalf("count series: %v", err)
	}
	if rows != 1 {
		t.Fatalf("series rows after repair = %d, want 1", rows)
	}

	var parentSeriesID int64
	if err := db.db.QueryRow(`SELECT parent_series_id FROM media_files WHERE path = ?`, "/tv/Project Runway (2004)/duplicate-s01e01.mkv").Scan(&parentSeriesID); err != nil {
		t.Fatalf("read repaired media file: %v", err)
	}
	if parentSeriesID != keeperID {
		t.Fatalf("parent_series_id = %d, want keeper %d", parentSeriesID, keeperID)
	}

	var episodeRows int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM episodes WHERE series_id = ?`, keeperID).Scan(&episodeRows); err != nil {
		t.Fatalf("count keeper episodes: %v", err)
	}
	if episodeRows != 2 {
		t.Fatalf("keeper episode rows = %d, want 2", episodeRows)
	}

	var episodeCount int
	if err := db.db.QueryRow(`SELECT episode_count FROM series WHERE id = ?`, keeperID).Scan(&episodeCount); err != nil {
		t.Fatalf("read episode_count: %v", err)
	}
	if episodeCount != 2 {
		t.Fatalf("episode_count = %d, want 2", episodeCount)
	}
}

func TestRepairNestedReleaseSeriesPreservesMediaAndBestFile(t *testing.T) {
	db, synthetic, canonical, episode, file, filePath := setupNestedReleaseSeriesFixture(t, true)
	defer db.Close()

	dryRun, err := db.RepairNestedReleaseSeries(true)
	if err != nil {
		t.Fatalf("RepairNestedReleaseSeries dry-run: %v", err)
	}
	if dryRun.Ambiguous != 0 || dryRun.SeriesDeleted != 1 || dryRun.EpisodesMoved != 1 {
		t.Fatalf("dry-run counts = ambiguous %d, deleted %d, moved %d, want 0/1/1", dryRun.Ambiguous, dryRun.SeriesDeleted, dryRun.EpisodesMoved)
	}
	assertNestedSeriesRelations(t, db, synthetic.ID, 0, episode.ID, file.ID)

	contentBefore, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read media before repair: %v", err)
	}

	report, err := db.RepairNestedReleaseSeries(false)
	if err != nil {
		t.Fatalf("RepairNestedReleaseSeries execute: %v", err)
	}
	if report.DryRun || report.Ambiguous != 0 || report.SeriesDeleted != 1 || report.EpisodesMoved != 1 {
		t.Fatalf("execute report = %+v, want one series deletion and episode move", report)
	}

	assertNestedSeriesRelations(t, db, synthetic.ID, canonical.ID, episode.ID, file.ID)
	var targetEpisodeCount int
	if err := db.db.QueryRow(`SELECT episode_count FROM series WHERE id = ?`, canonical.ID).Scan(&targetEpisodeCount); err != nil {
		t.Fatalf("read target episode count: %v", err)
	}
	if targetEpisodeCount != 1 {
		t.Fatalf("target episode_count = %d, want 1 after relinking", targetEpisodeCount)
	}
	var syntheticRows int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM series WHERE id = ?`, synthetic.ID).Scan(&syntheticRows); err != nil {
		t.Fatalf("count synthetic series: %v", err)
	}
	if syntheticRows != 0 {
		t.Fatalf("synthetic series rows = %d, want 0", syntheticRows)
	}
	contentAfter, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read media after repair: %v", err)
	}
	if string(contentAfter) != string(contentBefore) {
		t.Fatal("repair changed the media file")
	}
}

func TestRepairNestedReleaseSeriesReportsAmbiguousMapping(t *testing.T) {
	db, synthetic, _, episode, file, _ := setupNestedReleaseSeriesFixture(t, false)
	defer db.Close()

	report, err := db.RepairNestedReleaseSeries(true)
	if err != nil {
		t.Fatalf("RepairNestedReleaseSeries dry-run: %v", err)
	}
	if report.Ambiguous != 1 || len(report.Groups) != 1 || report.Groups[0].AmbiguousReason == "" {
		t.Fatalf("report = %+v, want one reported ambiguous mapping", report)
	}
	assertNestedSeriesRelations(t, db, synthetic.ID, 0, episode.ID, file.ID)
}

func TestRepairNestedReleaseSeriesDoesNotMatchShowInDifferentLibraryRoot(t *testing.T) {
	db, synthetic, _, episode, file, _ := setupNestedReleaseSeriesFixture(t, false)
	defer db.Close()

	otherRoot := t.TempDir()
	otherShow := &Series{
		Title:         "The Show",
		Year:          2024,
		CanonicalPath: filepath.Join(otherRoot, "The Show (2024)"),
		LibraryRoot:   otherRoot,
	}
	if _, err := db.UpsertSeries(otherShow); err != nil {
		t.Fatalf("insert show in different library root: %v", err)
	}

	report, err := db.RepairNestedReleaseSeries(true)
	if err != nil {
		t.Fatalf("RepairNestedReleaseSeries dry-run: %v", err)
	}
	if report.Ambiguous != 1 || report.SeriesDeleted != 0 || len(report.Groups) != 1 || report.Groups[0].AmbiguousReason == "" {
		t.Fatalf("report = %+v, want cross-root mapping marked ambiguous", report)
	}
	assertNestedSeriesRelations(t, db, synthetic.ID, 0, episode.ID, file.ID)
}

func TestRepairNestedReleaseSeriesRejectsStaleBestFileReference(t *testing.T) {
	db, synthetic, canonical, episode, file, _ := setupNestedReleaseSeriesFixture(t, true)
	defer db.Close()

	staleBestFileID := file.ID + 1000
	if _, err := db.db.Exec(`UPDATE episodes SET best_file_id = ? WHERE id = ?`, staleBestFileID, episode.ID); err != nil {
		t.Fatalf("set stale best-file reference: %v", err)
	}

	report, err := db.RepairNestedReleaseSeries(true)
	if err != nil {
		t.Fatalf("RepairNestedReleaseSeries dry-run: %v", err)
	}
	if report.Ambiguous != 1 || report.SeriesDeleted != 0 || len(report.Groups) != 1 || report.Groups[0].AmbiguousReason == "" {
		t.Fatalf("report = %+v, want stale best-file reference marked ambiguous", report)
	}
	repaired, err := db.GetMediaFileByID(file.ID)
	if err != nil {
		t.Fatalf("read nested media file: %v", err)
	}
	if repaired.ParentSeriesID == nil || *repaired.ParentSeriesID != synthetic.ID {
		t.Fatalf("dry-run changed source media parent series: %v", repaired.ParentSeriesID)
	}
	var gotBestFileID int64
	if err := db.db.QueryRow(`SELECT best_file_id FROM episodes WHERE id = ?`, episode.ID).Scan(&gotBestFileID); err != nil {
		t.Fatalf("read source best-file reference: %v", err)
	}
	if gotBestFileID != staleBestFileID {
		t.Fatalf("dry-run changed best_file_id to %d, want %d", gotBestFileID, staleBestFileID)
	}
	if _, err := db.GetSeriesByID(canonical.ID); err != nil {
		t.Fatalf("read canonical series after dry-run: %v", err)
	}
}

func TestRepairNestedReleaseSeriesMergesIntoExistingEpisode(t *testing.T) {
	db, synthetic, canonical, _, nestedFile, nestedPath := setupNestedReleaseSeriesFixture(t, true)
	defer db.Close()

	targetEpisode := &Episode{SeriesID: canonical.ID, Season: 2, Episode: 7, Title: "The Show S02E07"}
	if err := db.UpsertEpisode(targetEpisode); err != nil {
		t.Fatalf("insert canonical episode: %v", err)
	}
	showPath := filepath.Dir(filepath.Dir(filepath.Dir(nestedPath)))
	targetPath := filepath.Join(showPath, "Season 02", "The Show S02E07.mkv")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("create canonical season directory: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("canonical episode"), 0o644); err != nil {
		t.Fatalf("create canonical episode file: %v", err)
	}
	season, number := 2, 7
	canonicalID := canonical.ID
	canonicalFile := &MediaFile{
		Path: targetPath, Size: int64(len("canonical episode")), MediaType: "episode",
		ParentSeriesID: &canonicalID, ParentEpisodeID: &targetEpisode.ID,
		NormalizedTitle: NormalizeTitle(canonical.Title), Season: &season, Episode: &number,
		QualityScore: 200, Source: "filesystem", SourcePriority: 50,
	}
	if err := db.UpsertMediaFile(canonicalFile); err != nil {
		t.Fatalf("insert canonical media file: %v", err)
	}
	persistedCanonicalFile, err := db.GetMediaFile(targetPath)
	if err != nil {
		t.Fatalf("read canonical media file: %v", err)
	}
	canonicalFile.ID = persistedCanonicalFile.ID
	if err := db.UpdateEpisodeBestFile(targetEpisode.ID, &canonicalFile.ID); err != nil {
		t.Fatalf("set canonical episode best file: %v", err)
	}

	report, err := db.RepairNestedReleaseSeries(false)
	if err != nil {
		t.Fatalf("RepairNestedReleaseSeries execute: %v", err)
	}
	if report.Ambiguous != 0 || report.EpisodesMerged != 1 || report.SeriesDeleted != 1 {
		t.Fatalf("report = %+v, want one merged episode and deleted series", report)
	}
	repaired, err := db.GetMediaFileByID(nestedFile.ID)
	if err != nil {
		t.Fatalf("read repaired media file: %v", err)
	}
	if repaired.ParentSeriesID == nil || *repaired.ParentSeriesID != canonical.ID || repaired.ParentEpisodeID == nil || *repaired.ParentEpisodeID != targetEpisode.ID {
		t.Fatalf("repaired links = series %v episode %v, want %d/%d", repaired.ParentSeriesID, repaired.ParentEpisodeID, canonical.ID, targetEpisode.ID)
	}
	var bestFileID int64
	if err := db.db.QueryRow(`SELECT best_file_id FROM episodes WHERE id = ?`, targetEpisode.ID).Scan(&bestFileID); err != nil {
		t.Fatalf("read canonical best file: %v", err)
	}
	if bestFileID != canonicalFile.ID {
		t.Fatalf("canonical best file id = %d, want preserved id %d", bestFileID, canonicalFile.ID)
	}
	var syntheticRows int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM series WHERE id = ?`, synthetic.ID).Scan(&syntheticRows); err != nil {
		t.Fatalf("count synthetic series: %v", err)
	}
	if syntheticRows != 0 {
		t.Fatalf("synthetic series rows = %d, want 0", syntheticRows)
	}
}

func setupNestedReleaseSeriesFixture(t *testing.T, withCanonical bool) (*MediaDB, *Series, *Series, *Episode, *MediaFile, string) {
	t.Helper()
	db := setupTestDB(t)

	tvRoot := t.TempDir()
	showPath := filepath.Join(tvRoot, "The Show (2024)")
	releaseDir := filepath.Join(showPath, "Season 02", "The.Show.S02E07.1080p-GROUP")
	filePath := filepath.Join(releaseDir, "0123456789abcdef0123456789abcdef.mkv")
	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		t.Fatalf("create release directory: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("episode payload"), 0o644); err != nil {
		t.Fatalf("create episode file: %v", err)
	}

	var canonical *Series
	if withCanonical {
		canonical = &Series{Title: "The Show", Year: 2024, CanonicalPath: showPath, LibraryRoot: tvRoot, Source: "filesystem", SourcePriority: 50}
		if _, err := db.UpsertSeries(canonical); err != nil {
			t.Fatalf("insert canonical series: %v", err)
		}
	}
	synthetic := &Series{Title: filepath.Base(releaseDir), CanonicalPath: releaseDir, LibraryRoot: tvRoot, Source: "filesystem", SourcePriority: 50}
	if _, err := db.UpsertSeries(synthetic); err != nil {
		t.Fatalf("insert synthetic series: %v", err)
	}
	episode := &Episode{SeriesID: synthetic.ID, Season: 2, Episode: 7, Title: "The Show S02E07"}
	if err := db.UpsertEpisode(episode); err != nil {
		t.Fatalf("insert synthetic episode: %v", err)
	}
	season, number := 2, 7
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("stat episode: %v", err)
	}
	syntheticID := synthetic.ID
	file := &MediaFile{
		Path: filePath, Size: info.Size(), MediaType: "episode", ParentSeriesID: &syntheticID, ParentEpisodeID: &episode.ID,
		NormalizedTitle: NormalizeTitle(synthetic.Title), Season: &season, Episode: &number,
		QualityScore: 100, Source: "filesystem", SourcePriority: 50, LibraryRoot: tvRoot,
	}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("insert nested media file: %v", err)
	}
	retrieved, err := db.GetMediaFile(filePath)
	if err != nil {
		t.Fatalf("read nested media file: %v", err)
	}
	file.ID = retrieved.ID
	if err := db.UpdateEpisodeBestFile(episode.ID, &file.ID); err != nil {
		t.Fatalf("set synthetic episode best file: %v", err)
	}
	return db, synthetic, canonical, episode, file, filePath
}

func assertNestedSeriesRelations(t *testing.T, db *MediaDB, oldSeriesID, targetSeriesID, episodeID, fileID int64) {
	t.Helper()
	file, err := db.GetMediaFileByID(fileID)
	if err != nil {
		t.Fatalf("read media file %d: %v", fileID, err)
	}
	if file.ID != fileID {
		t.Fatalf("media file id = %d, want preserved id %d", file.ID, fileID)
	}
	if targetSeriesID == 0 {
		targetSeriesID = oldSeriesID
	}
	if file.ParentSeriesID == nil || *file.ParentSeriesID != targetSeriesID {
		t.Fatalf("media parent series = %v, want %d", file.ParentSeriesID, targetSeriesID)
	}
	if file.ParentEpisodeID == nil || *file.ParentEpisodeID != episodeID {
		t.Fatalf("media parent episode = %v, want preserved id %d", file.ParentEpisodeID, episodeID)
	}
	var bestFileID int64
	if err := db.db.QueryRow(`SELECT best_file_id FROM episodes WHERE id = ?`, episodeID).Scan(&bestFileID); err != nil {
		t.Fatalf("read best file id: %v", err)
	}
	if bestFileID != fileID {
		t.Fatalf("episode best file id = %d, want preserved id %d", bestFileID, fileID)
	}
}

func setupSeriesDedupeFixture(t *testing.T) *MediaDB {
	t.Helper()

	db := setupTestDB(t)
	canonical := "/tv/Project Runway (2004)"

	keeper := &Series{
		Title:          "Project Runway",
		Year:           2004,
		CanonicalPath:  canonical,
		LibraryRoot:    "/tv",
		Source:         "filesystem",
		SourcePriority: 50,
		EpisodeCount:   1,
	}
	if _, err := db.UpsertSeries(keeper); err != nil {
		t.Fatalf("insert keeper: %v", err)
	}

	result, err := db.db.Exec(`
		INSERT INTO series (
			title, title_normalized, year, canonical_path, library_root,
			source, source_priority, episode_count
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"Welcome to the Urban Jungle", NormalizeTitle("Welcome to the Urban Jungle"), 2023,
		canonical, "/tv", "filesystem", 50, 2)
	if err != nil {
		t.Fatalf("insert duplicate series: %v", err)
	}
	duplicateID, _ := result.LastInsertId()

	keeperEpisode := &Episode{SeriesID: keeper.ID, Season: 1, Episode: 1, Title: "Episode 1"}
	if err := db.UpsertEpisode(keeperEpisode); err != nil {
		t.Fatalf("insert keeper episode: %v", err)
	}
	duplicateEpisode1 := &Episode{SeriesID: duplicateID, Season: 1, Episode: 1, Title: "Fragment Episode 1"}
	if err := db.UpsertEpisode(duplicateEpisode1); err != nil {
		t.Fatalf("insert duplicate episode 1: %v", err)
	}
	duplicateEpisode2 := &Episode{SeriesID: duplicateID, Season: 1, Episode: 2, Title: "Fragment Episode 2"}
	if err := db.UpsertEpisode(duplicateEpisode2); err != nil {
		t.Fatalf("insert duplicate episode 2: %v", err)
	}

	if _, err := db.db.Exec(`
		INSERT INTO media_files (
			path, size, media_type, parent_series_id, parent_episode_id,
			normalized_title, season, episode
		) VALUES (?, ?, 'episode', ?, ?, ?, 1, 1)`,
		"/tv/Project Runway (2004)/duplicate-s01e01.mkv", 100, duplicateID, duplicateEpisode1.ID, NormalizeTitle("Project Runway")); err != nil {
		t.Fatalf("insert duplicate media file 1: %v", err)
	}
	if _, err := db.db.Exec(`
		INSERT INTO media_files (
			path, size, media_type, parent_series_id, parent_episode_id,
			normalized_title, season, episode
		) VALUES (?, ?, 'episode', ?, ?, ?, 1, 2)`,
		"/tv/Project Runway (2004)/duplicate-s01e02.mkv", 100, duplicateID, duplicateEpisode2.ID, NormalizeTitle("Project Runway")); err != nil {
		t.Fatalf("insert duplicate media file 2: %v", err)
	}

	return db
}
