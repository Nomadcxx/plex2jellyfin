package database

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Nomadcxx/plex2jellyfin/internal/naming"
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

type NestedSeriesRepairReport struct {
	DryRun            bool
	Groups            []NestedSeriesRepairGroup
	Ambiguous         int
	EpisodesMoved     int
	EpisodesMerged    int
	MediaFilesUpdated int
	SeriesDeleted     int
}

type NestedSeriesRepairGroup struct {
	SourceSeriesID  int64
	SourcePath      string
	TargetSeriesID  int64
	TargetPath      string
	MediaFiles      int
	EpisodesMoved   int
	EpisodesMerged  int
	SeriesDeleted   bool
	AmbiguousReason string
}

type nestedSeriesRow struct {
	id              int64
	title           string
	titleNormalized string
	year            int
	canonicalPath   string
	libraryRoot     string
}

type nestedSeriesMediaFile struct {
	id              int64
	path            string
	size            int64
	season          sql.NullInt64
	episode         sql.NullInt64
	parentEpisodeID sql.NullInt64
}

// RepairNestedReleaseSeries relinks nested release files to an existing show
// row. Dry-run performs the same transaction and rolls it back. Files on disk
// remain untouched.
func (m *MediaDB) RepairNestedReleaseSeries(dryRun bool) (*NestedSeriesRepairReport, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	tx, err := m.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin nested series repair: %w", err)
	}
	defer tx.Rollback()

	seriesRows, err := loadNestedSeriesRows(tx)
	if err != nil {
		return nil, err
	}
	report := &NestedSeriesRepairReport{DryRun: dryRun}
	for _, source := range seriesRows {
		group := NestedSeriesRepairGroup{SourceSeriesID: source.id, SourcePath: source.canonicalPath}
		showPath, season, reason := nestedReleaseShowPath(source.canonicalPath, source.libraryRoot)
		if reason == "" {
			var target nestedSeriesRow
			target, reason, err = findNestedSeriesTarget(tx, source, showPath)
			if err != nil {
				return nil, err
			}
			if reason == "" {
				var files []nestedSeriesMediaFile
				files, reason, err = validateNestedSeriesFiles(tx, source, showPath, season, target)
				if err != nil {
					return nil, err
				}
				group.TargetSeriesID = target.id
				group.TargetPath = target.canonicalPath
				group.MediaFiles = len(files)
				if reason == "" {
					if _, err := tx.Exec(`
						UPDATE media_files
						   SET normalized_title = ?,
						       year = NULLIF(?, 0),
						       updated_at = CURRENT_TIMESTAMP
						 WHERE parent_series_id = ?`, target.titleNormalized, target.year, source.id); err != nil {
						return nil, fmt.Errorf("update nested media identity for series %d: %w", source.id, err)
					}
					moved, merged, _, err := mergeDuplicateSeriesIntoKeeper(tx, target.id, source.id)
					if err != nil {
						return nil, fmt.Errorf("relink nested series %d to %d: %w", source.id, target.id, err)
					}
					if err := refreshSeriesEpisodeCount(tx, target.id); err != nil {
						return nil, fmt.Errorf("refresh episode count for series %d: %w", target.id, err)
					}
					group.EpisodesMoved = moved
					group.EpisodesMerged = merged
					group.SeriesDeleted = true
					report.EpisodesMoved += moved
					report.EpisodesMerged += merged
					report.MediaFilesUpdated += len(files)
					report.SeriesDeleted++
				}
			}
		}
		if reason != "" {
			group.AmbiguousReason = reason
			report.Ambiguous++
		}
		report.Groups = append(report.Groups, group)
	}

	if dryRun {
		return report, nil
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit nested series repair: %w", err)
	}
	return report, nil
}

func loadNestedSeriesRows(tx *sql.Tx) ([]nestedSeriesRow, error) {
	// ponytail: scan the series table (O(n)); add a nesting index only if this repair becomes recurring.
	rows, err := tx.Query(`
		SELECT id, title, title_normalized, COALESCE(year, 0), canonical_path, library_root
		  FROM series
		 WHERE canonical_path IS NOT NULL AND canonical_path != ''
		 ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("load series for nested repair: %w", err)
	}
	defer rows.Close()

	var out []nestedSeriesRow
	for rows.Next() {
		var row nestedSeriesRow
		if err := rows.Scan(&row.id, &row.title, &row.titleNormalized, &row.year, &row.canonicalPath, &row.libraryRoot); err != nil {
			return nil, fmt.Errorf("scan series for nested repair: %w", err)
		}
		if looksLikeNestedReleasePath(row.canonicalPath) {
			out = append(out, row)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read series for nested repair: %w", err)
	}
	return out, nil
}

func looksLikeNestedReleasePath(path string) bool {
	return strings.HasPrefix(strings.ToLower(filepath.Base(filepath.Dir(filepath.Clean(path)))), "season")
}

func nestedReleaseShowPath(canonicalPath, libraryRoot string) (string, int, string) {
	path := filepath.Clean(canonicalPath)
	root := filepath.Clean(libraryRoot)
	seasonDir := filepath.Dir(path)
	seasonName := filepath.Base(seasonDir)
	if !strings.HasPrefix(strings.ToLower(seasonName), "season") {
		return "", 0, "canonical path is not below a season directory"
	}
	seasonText := strings.TrimSpace(seasonName[len("season"):])
	season, err := strconv.Atoi(seasonText)
	if err != nil || season <= 0 {
		return "", 0, fmt.Sprintf("season directory %q has no valid season number", seasonName)
	}
	if strings.TrimSpace(libraryRoot) == "" || filepath.IsAbs(path) != filepath.IsAbs(root) {
		return "", 0, "series path has no matching configured library root"
	}
	showPath := filepath.Dir(seasonDir)
	rel, err := filepath.Rel(root, showPath)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", 0, "derived show path is outside the configured library root"
	}
	return showPath, season, ""
}

func findNestedSeriesTarget(tx *sql.Tx, source nestedSeriesRow, showPath string) (nestedSeriesRow, string, error) {
	title, year := seriesIdentityFromCanonicalPath(showPath)
	if title == "" {
		return nestedSeriesRow{}, "show directory has no usable title", nil
	}
	querySeries := func(query string, args ...interface{}) ([]nestedSeriesRow, error) {
		rows, err := tx.Query(query, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []nestedSeriesRow
		for rows.Next() {
			var row nestedSeriesRow
			if err := rows.Scan(&row.id, &row.title, &row.titleNormalized, &row.year, &row.canonicalPath, &row.libraryRoot); err != nil {
				return nil, err
			}
			out = append(out, row)
		}
		return out, rows.Err()
	}

	rows, err := querySeries(`
		SELECT id, title, title_normalized, COALESCE(year, 0), canonical_path, library_root
		  FROM series WHERE canonical_path = ? AND id != ? ORDER BY id`, showPath, source.id)
	if err != nil {
		return nestedSeriesRow{}, "", fmt.Errorf("find canonical show at %q: %w", showPath, err)
	}
	if len(rows) > 1 {
		return nestedSeriesRow{}, "multiple series rows use the derived show path", nil
	}
	if len(rows) == 1 {
		if looksLikeNestedReleasePath(rows[0].canonicalPath) {
			return nestedSeriesRow{}, "matched show row is itself nested under a season directory", nil
		}
		if NormalizeTitle(rows[0].title) != NormalizeTitle(title) || (year > 0 && rows[0].year > 0 && rows[0].year != year) {
			return nestedSeriesRow{}, "series at the derived show path has a different identity", nil
		}
		return rows[0], "", nil
	}

	if year > 0 {
		rows, err = querySeries(`
			SELECT id, title, title_normalized, COALESCE(year, 0), canonical_path, library_root
			  FROM series WHERE title_normalized = ? AND COALESCE(year, 0) = ? AND library_root = ? AND id != ? ORDER BY id`,
			NormalizeTitle(title), year, source.libraryRoot, source.id)
	} else {
		rows, err = querySeries(`
			SELECT id, title, title_normalized, COALESCE(year, 0), canonical_path, library_root
			  FROM series WHERE title_normalized = ? AND library_root = ? AND id != ? ORDER BY id`,
			NormalizeTitle(title), source.libraryRoot, source.id)
	}
	if err != nil {
		return nestedSeriesRow{}, "", fmt.Errorf("find canonical show by identity %q (%d): %w", title, year, err)
	}
	if len(rows) == 0 {
		return nestedSeriesRow{}, "no existing series matches the derived show identity", nil
	}
	if len(rows) > 1 {
		return nestedSeriesRow{}, "multiple existing series match the derived show identity", nil
	}
	if looksLikeNestedReleasePath(rows[0].canonicalPath) {
		return nestedSeriesRow{}, "matched show row is itself nested under a season directory", nil
	}
	return rows[0], "", nil
}

func validateNestedSeriesFiles(tx *sql.Tx, source nestedSeriesRow, showPath string, season int, target nestedSeriesRow) ([]nestedSeriesMediaFile, string, error) {
	rows, err := tx.Query(`
		SELECT id, path, size, media_type, season, episode, parent_episode_id
		  FROM media_files WHERE parent_series_id = ? ORDER BY id`, source.id)
	if err != nil {
		return nil, "", fmt.Errorf("load nested media files for series %d: %w", source.id, err)
	}
	defer rows.Close()

	var files []nestedSeriesMediaFile
	expectedTitle, expectedYear := seriesIdentityFromCanonicalPath(showPath)
	var mediaType string
	for rows.Next() {
		var file nestedSeriesMediaFile
		if err := rows.Scan(&file.id, &file.path, &file.size, &mediaType, &file.season, &file.episode, &file.parentEpisodeID); err != nil {
			return nil, "", fmt.Errorf("scan nested media file for series %d: %w", source.id, err)
		}
		if mediaType != "episode" || !file.season.Valid || !file.episode.Valid || file.season.Int64 != int64(season) || file.episode.Int64 <= 0 {
			return nil, fmt.Sprintf("media file %q has no matching episode identity", file.path), nil
		}
		if !file.parentEpisodeID.Valid {
			return nil, fmt.Sprintf("media file %q has no episode row", file.path), nil
		}
		var linkedSeason, linkedEpisode int
		if err := tx.QueryRow(`SELECT season, episode FROM episodes WHERE id = ? AND series_id = ?`, file.parentEpisodeID.Int64, source.id).Scan(&linkedSeason, &linkedEpisode); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Sprintf("media file %q is linked to an unrelated episode row", file.path), nil
			}
			return nil, "", fmt.Errorf("read linked episode for media file %q: %w", file.path, err)
		}
		if linkedSeason != season || int64(linkedEpisode) != file.episode.Int64 {
			return nil, fmt.Sprintf("media file %q disagrees with its linked episode", file.path), nil
		}
		fileRel, relErr := filepath.Rel(source.canonicalPath, file.path)
		if relErr != nil || fileRel == "." || fileRel == ".." || strings.HasPrefix(fileRel, ".."+string(filepath.Separator)) {
			return nil, fmt.Sprintf("media file %q is outside the nested release directory", file.path), nil
		}
		info, statErr := os.Stat(file.path)
		if statErr != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() != file.size {
			return nil, fmt.Sprintf("media file %q is missing, empty, non-regular, or has changed size", file.path), nil
		}
		parsed, parseErr := naming.ParseTVShowFromPath(file.path)
		if parseErr != nil || parsed.Season != season || int64(parsed.Episode) != file.episode.Int64 || parsed.EpisodeEnd > parsed.Episode || NormalizeTitle(parsed.Title) != NormalizeTitle(expectedTitle) {
			return nil, fmt.Sprintf("media file %q does not unambiguously match the derived show and episode", file.path), nil
		}
		if parsed.Year != "" {
			parsedYear, err := strconv.Atoi(parsed.Year)
			if err != nil || (expectedYear > 0 && parsedYear != expectedYear) || (target.year > 0 && parsedYear != target.year) {
				return nil, fmt.Sprintf("media file %q has a different series year", file.path), nil
			}
		}
		files = append(files, file)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("read nested media files for series %d: %w", source.id, err)
	}
	if len(files) == 0 {
		return nil, "synthetic series has no media files", nil
	}
	validatedFileIDs := make(map[int64]map[int64]struct{}, len(files))
	for _, file := range files {
		episodeID := file.parentEpisodeID.Int64
		if validatedFileIDs[episodeID] == nil {
			validatedFileIDs[episodeID] = make(map[int64]struct{})
		}
		validatedFileIDs[episodeID][file.id] = struct{}{}
	}
	episodeRows, err := tx.Query(`SELECT id, best_file_id FROM episodes WHERE series_id = ?`, source.id)
	if err != nil {
		return nil, "", fmt.Errorf("load best-file references for nested series %d: %w", source.id, err)
	}
	for episodeRows.Next() {
		var episodeID int64
		var bestFileID sql.NullInt64
		if err := episodeRows.Scan(&episodeID, &bestFileID); err != nil {
			episodeRows.Close()
			return nil, "", fmt.Errorf("scan best-file reference for nested series %d: %w", source.id, err)
		}
		if !bestFileID.Valid {
			continue
		}
		if _, ok := validatedFileIDs[episodeID][bestFileID.Int64]; !ok {
			episodeRows.Close()
			return nil, fmt.Sprintf("episode %d has a best-file reference outside its validated media files", episodeID), nil
		}
	}
	if err := episodeRows.Err(); err != nil {
		episodeRows.Close()
		return nil, "", fmt.Errorf("read best-file references for nested series %d: %w", source.id, err)
	}
	if err := episodeRows.Close(); err != nil {
		return nil, "", fmt.Errorf("close best-file references for nested series %d: %w", source.id, err)
	}
	var orphanEpisodes int
	if err := tx.QueryRow(`
		SELECT COUNT(*) FROM episodes e
		 WHERE e.series_id = ?
		   AND NOT EXISTS (
		       SELECT 1 FROM media_files mf
		        WHERE mf.parent_series_id = ? AND mf.parent_episode_id = e.id
		   )`, source.id, source.id).Scan(&orphanEpisodes); err != nil {
		return nil, "", fmt.Errorf("check orphan episodes for nested series %d: %w", source.id, err)
	}
	if orphanEpisodes != 0 {
		return nil, "synthetic series has episode rows without matching media files", nil
	}
	return files, "", nil
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

	var remainingMediaFiles, remainingEpisodes int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM media_files WHERE parent_series_id = ?`, duplicateID).Scan(&remainingMediaFiles); err != nil {
		return 0, 0, 0, fmt.Errorf("check remaining media files for series %d: %w", duplicateID, err)
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM episodes WHERE series_id = ?`, duplicateID).Scan(&remainingEpisodes); err != nil {
		return 0, 0, 0, fmt.Errorf("check remaining episodes for series %d: %w", duplicateID, err)
	}
	if remainingMediaFiles != 0 || remainingEpisodes != 0 {
		return 0, 0, 0, fmt.Errorf("cannot delete series %d with %d media files and %d episodes still linked", duplicateID, remainingMediaFiles, remainingEpisodes)
	}

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
