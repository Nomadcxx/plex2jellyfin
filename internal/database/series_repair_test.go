package database

import "testing"

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
