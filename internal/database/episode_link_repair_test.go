package database

import (
	"database/sql"
	"testing"
)

func TestRepairEpisodeFileLinksDryRunAndExecute(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	series := &Series{
		Title: "Link Repair", Year: 2025, CanonicalPath: "/tv/Link Repair (2025)",
		LibraryRoot: "/tv", Source: "filesystem", SourcePriority: 50,
	}
	if _, err := db.UpsertSeries(series); err != nil {
		t.Fatalf("UpsertSeries: %v", err)
	}
	one := &Episode{SeriesID: series.ID, Season: 1, Episode: 1}
	two := &Episode{SeriesID: series.ID, Season: 1, Episode: 2}
	empty := &Episode{SeriesID: series.ID, Season: 1, Episode: 3}
	for _, episode := range []*Episode{one, two, empty} {
		if err := db.UpsertEpisode(episode); err != nil {
			t.Fatalf("UpsertEpisode: %v", err)
		}
	}
	insertFile := func(path string, parentEpisodeID *int64, episode, quality int) int64 {
		t.Helper()
		var parent any
		if parentEpisodeID != nil {
			parent = *parentEpisodeID
		}
		result, err := db.SQL().Exec(`
			INSERT INTO media_files (
				path, size, media_type, parent_series_id, parent_episode_id,
				normalized_title, season, episode, quality_score
			) VALUES (?, 100, 'episode', ?, ?, 'link repair', 1, ?, ?)`,
			path, series.ID, parent, episode, quality)
		if err != nil {
			t.Fatalf("insert media file %s: %v", path, err)
		}
		id, err := result.LastInsertId()
		if err != nil {
			t.Fatalf("LastInsertId %s: %v", path, err)
		}
		return id
	}
	fileOneLow := insertFile("/tv/one-low.mkv", &two.ID, 1, 10)
	fileOneBest := insertFile("/tv/one-best.mkv", nil, 1, 20)
	fileTwo := insertFile("/tv/two.mkv", &one.ID, 2, 5)
	insertFile("/tv/unmatched.mkv", nil, 99, 1)
	if err := db.UpdateEpisodeBestFile(one.ID, &fileOneLow); err != nil {
		t.Fatalf("set old best file for episode one: %v", err)
	}
	if err := db.UpdateEpisodeBestFile(two.ID, &fileOneBest); err != nil {
		t.Fatalf("set old best file for episode two: %v", err)
	}
	_, err := db.SQL().Exec(`INSERT INTO episodes (series_id, season, episode) VALUES (999999, 1, 1)`)
	if err != nil {
		t.Fatalf("insert orphan episode: %v", err)
	}

	assertReport := func(report *EpisodeLinkRepairReport) {
		t.Helper()
		if report.MediaFilesRepaired != 3 || report.BestFileReferencesRepaired != 2 {
			t.Fatalf("repair counts = files %d, best files %d; want 3 and 2", report.MediaFilesRepaired, report.BestFileReferencesRepaired)
		}
		if report.UnmatchedMediaFiles != 1 || report.EpisodesWithoutMediaFiles != 2 || report.OrphanedEpisodeSeries != 1 {
			t.Fatalf("review counts = unmatched files %d, episodes without files %d, orphaned series %d; want 1, 2, 1",
				report.UnmatchedMediaFiles, report.EpisodesWithoutMediaFiles, report.OrphanedEpisodeSeries)
		}
	}

	dryRun, err := db.RepairEpisodeFileLinks(true)
	if err != nil {
		t.Fatalf("dry-run repair: %v", err)
	}
	assertReport(dryRun)
	assertEpisodeFileLink(t, db.SQL(), "/tv/one-low.mkv", two.ID)
	assertEpisodeFileLink(t, db.SQL(), "/tv/one-best.mkv", 0)
	assertBestFile(t, db.SQL(), one.ID, fileOneLow)

	apply, err := db.RepairEpisodeFileLinks(false)
	if err != nil {
		t.Fatalf("execute repair: %v", err)
	}
	assertReport(apply)
	assertEpisodeFileLink(t, db.SQL(), "/tv/one-low.mkv", one.ID)
	assertEpisodeFileLink(t, db.SQL(), "/tv/one-best.mkv", one.ID)
	assertEpisodeFileLink(t, db.SQL(), "/tv/two.mkv", two.ID)
	assertEpisodeFileLink(t, db.SQL(), "/tv/unmatched.mkv", 0)
	assertBestFile(t, db.SQL(), one.ID, fileOneBest)
	assertBestFile(t, db.SQL(), two.ID, fileTwo)
}

func assertEpisodeFileLink(t *testing.T, db *sql.DB, path string, want int64) {
	t.Helper()
	var got sql.NullInt64
	if err := db.QueryRow(`SELECT parent_episode_id FROM media_files WHERE path = ?`, path).Scan(&got); err != nil {
		t.Fatalf("read parent episode for %s: %v", path, err)
	}
	if want == 0 {
		if got.Valid {
			t.Fatalf("parent_episode_id for %s = %d, want NULL", path, got.Int64)
		}
		return
	}
	if !got.Valid || got.Int64 != want {
		t.Fatalf("parent_episode_id for %s = %v, want %d", path, got, want)
	}
}

func assertBestFile(t *testing.T, db *sql.DB, episodeID, want int64) {
	t.Helper()
	var got sql.NullInt64
	if err := db.QueryRow(`SELECT best_file_id FROM episodes WHERE id = ?`, episodeID).Scan(&got); err != nil {
		t.Fatalf("read best file for episode %d: %v", episodeID, err)
	}
	if !got.Valid || got.Int64 != want {
		t.Fatalf("best_file_id for episode %d = %v, want %d", episodeID, got, want)
	}
}
