package database

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
)

type SeriesDedupeReport struct {
	DryRun            bool
	Groups            []SeriesDedupeGroup
	EpisodesMoved     int
	EpisodesMerged    int
	MediaFilesUpdated int
	SeriesDeleted     int
}

type SeriesDedupeGroup struct {
	CanonicalPath     string
	Keeper            SeriesDedupeRow
	Duplicates        []SeriesDedupeRow
	EpisodesMoved     int
	EpisodesMerged    int
	MediaFilesUpdated int
	SeriesDeleted     int
}

type SeriesDedupeRow struct {
	ID             int64
	Title          string
	Year           int
	Source         string
	SourcePriority int
	EpisodeCount   int
}

// DedupeSeriesByCanonicalPath collapses historical duplicate series rows that
// point at the same canonical folder. It rewires episodes and media_files onto
// the selected keeper row, then deletes the redundant series rows.
func (m *MediaDB) DedupeSeriesByCanonicalPath(dryRun bool) (*SeriesDedupeReport, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	tx, err := m.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	report := &SeriesDedupeReport{DryRun: dryRun}

	paths, err := duplicateSeriesCanonicalPaths(tx)
	if err != nil {
		return nil, err
	}

	for _, canonicalPath := range paths {
		group, err := dedupeSeriesCanonicalPath(tx, canonicalPath)
		if err != nil {
			return nil, err
		}
		report.Groups = append(report.Groups, group)
		report.EpisodesMoved += group.EpisodesMoved
		report.EpisodesMerged += group.EpisodesMerged
		report.MediaFilesUpdated += group.MediaFilesUpdated
		report.SeriesDeleted += group.SeriesDeleted
	}

	if dryRun {
		return report, nil
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return report, nil
}

func duplicateSeriesCanonicalPaths(tx *sql.Tx) ([]string, error) {
	rows, err := tx.Query(`
		SELECT canonical_path
		  FROM series
		 WHERE canonical_path IS NOT NULL AND canonical_path != ''
		 GROUP BY canonical_path
		HAVING COUNT(*) > 1
		 ORDER BY canonical_path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, rows.Err()
}

func dedupeSeriesCanonicalPath(tx *sql.Tx, canonicalPath string) (SeriesDedupeGroup, error) {
	rows, err := tx.Query(`
		SELECT id, title, COALESCE(year, 0), COALESCE(source, ''),
		       COALESCE(source_priority, 0), COALESCE(episode_count, 0)
		  FROM series
		 WHERE canonical_path = ?
		 ORDER BY id ASC`,
		canonicalPath)
	if err != nil {
		return SeriesDedupeGroup{}, err
	}
	defer rows.Close()

	var series []SeriesDedupeRow
	for rows.Next() {
		var row SeriesDedupeRow
		if err := rows.Scan(&row.ID, &row.Title, &row.Year, &row.Source, &row.SourcePriority, &row.EpisodeCount); err != nil {
			return SeriesDedupeGroup{}, err
		}
		series = append(series, row)
	}
	if err := rows.Err(); err != nil {
		return SeriesDedupeGroup{}, err
	}
	if len(series) < 2 {
		return SeriesDedupeGroup{}, fmt.Errorf("canonical path %q has %d series rows, want at least 2", canonicalPath, len(series))
	}

	keeperIndex := selectSeriesDedupeKeeper(series, canonicalPath)
	keeper := series[keeperIndex]
	duplicates := make([]SeriesDedupeRow, 0, len(series)-1)
	for i, row := range series {
		if i != keeperIndex {
			duplicates = append(duplicates, row)
		}
	}

	group := SeriesDedupeGroup{
		CanonicalPath: canonicalPath,
		Keeper:        keeper,
		Duplicates:    duplicates,
	}

	for _, duplicate := range group.Duplicates {
		moved, merged, mediaFiles, err := mergeDuplicateSeriesIntoKeeper(tx, group.Keeper.ID, duplicate.ID)
		if err != nil {
			return SeriesDedupeGroup{}, err
		}
		group.EpisodesMoved += moved
		group.EpisodesMerged += merged
		group.MediaFilesUpdated += mediaFiles
		group.SeriesDeleted++
	}

	if err := refreshSeriesEpisodeCount(tx, group.Keeper.ID); err != nil {
		return SeriesDedupeGroup{}, err
	}

	return group, nil
}

func selectSeriesDedupeKeeper(rows []SeriesDedupeRow, canonicalPath string) int {
	pathTitle, pathYear := seriesIdentityFromCanonicalPath(canonicalPath)
	best := 0
	for i := 1; i < len(rows); i++ {
		if seriesDedupeScore(rows[i], pathTitle, pathYear) > seriesDedupeScore(rows[best], pathTitle, pathYear) {
			best = i
		}
	}
	return best
}

func seriesDedupeScore(row SeriesDedupeRow, pathTitle string, pathYear int) int {
	score := 0
	if pathTitle != "" && NormalizeTitle(row.Title) == NormalizeTitle(pathTitle) {
		score += 1_000_000
	}
	if pathYear > 0 && row.Year == pathYear {
		score += 500_000
	}
	if row.Year > 0 {
		score += 50_000
	}
	score += row.SourcePriority * 100
	score += row.EpisodeCount
	score -= int(row.ID / 1_000_000)
	return score
}

func seriesIdentityFromCanonicalPath(canonicalPath string) (string, int) {
	base := filepath.Base(filepath.Clean(canonicalPath))
	year := ExtractYear(base)
	if year > 0 {
		base = strings.TrimSpace(strings.Replace(base, fmt.Sprintf("(%d)", year), "", 1))
	}
	return base, year
}

func mergeDuplicateSeriesIntoKeeper(tx *sql.Tx, keeperID, duplicateID int64) (episodesMoved, episodesMerged, mediaFilesUpdated int, err error) {
	rows, err := tx.Query(`
		SELECT id, season, episode, title, best_file_id
		  FROM episodes
		 WHERE series_id = ?
		 ORDER BY season, episode, id`,
		duplicateID)
	if err != nil {
		return 0, 0, 0, err
	}
	defer rows.Close()

	type duplicateEpisode struct {
		id         int64
		season     int
		episode    int
		title      sql.NullString
		bestFileID sql.NullInt64
	}

	var episodes []duplicateEpisode
	for rows.Next() {
		var ep duplicateEpisode
		if err := rows.Scan(&ep.id, &ep.season, &ep.episode, &ep.title, &ep.bestFileID); err != nil {
			return 0, 0, 0, err
		}
		episodes = append(episodes, ep)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, 0, err
	}

	for _, ep := range episodes {
		var keeperEpisodeID int64
		err := tx.QueryRow(`
			SELECT id
			  FROM episodes
			 WHERE series_id = ? AND season = ? AND episode = ?`,
			keeperID, ep.season, ep.episode,
		).Scan(&keeperEpisodeID)
		if err == sql.ErrNoRows {
			if _, err := tx.Exec(`UPDATE episodes SET series_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, keeperID, ep.id); err != nil {
				return 0, 0, 0, err
			}
			episodesMoved++
			continue
		}
		if err != nil {
			return 0, 0, 0, err
		}

		result, err := tx.Exec(`UPDATE media_files SET parent_episode_id = ? WHERE parent_episode_id = ?`, keeperEpisodeID, ep.id)
		if err != nil {
			return 0, 0, 0, err
		}
		updated, _ := result.RowsAffected()
		mediaFilesUpdated += int(updated)

		if _, err := tx.Exec(`
			UPDATE episodes
			   SET best_file_id = COALESCE(best_file_id, ?),
			       title = CASE WHEN title IS NULL OR title = '' THEN ? ELSE title END,
			       updated_at = CURRENT_TIMESTAMP
			 WHERE id = ?`,
			nullInt64Interface(ep.bestFileID), nullStringInterface(ep.title), keeperEpisodeID); err != nil {
			return 0, 0, 0, err
		}
		if _, err := tx.Exec(`DELETE FROM episodes WHERE id = ?`, ep.id); err != nil {
			return 0, 0, 0, err
		}
		episodesMerged++
	}

	result, err := tx.Exec(`UPDATE media_files SET parent_series_id = ? WHERE parent_series_id = ?`, keeperID, duplicateID)
	if err != nil {
		return 0, 0, 0, err
	}
	updated, _ := result.RowsAffected()
	mediaFilesUpdated += int(updated)

	if _, err := tx.Exec(`DELETE FROM series WHERE id = ?`, duplicateID); err != nil {
		return 0, 0, 0, err
	}

	return episodesMoved, episodesMerged, mediaFilesUpdated, nil
}

func refreshSeriesEpisodeCount(tx *sql.Tx, seriesID int64) error {
	_, err := tx.Exec(`
		UPDATE series
		   SET episode_count = (SELECT COUNT(*) FROM episodes WHERE series_id = ?),
		       updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		seriesID, seriesID)
	return err
}

func nullInt64Interface(v sql.NullInt64) interface{} {
	if !v.Valid {
		return nil
	}
	return v.Int64
}

func nullStringInterface(v sql.NullString) interface{} {
	if !v.Valid {
		return nil
	}
	return v.String
}
