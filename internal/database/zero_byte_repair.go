package database

import (
	"fmt"
	"os"
)

type ZeroByteMediaRepairReport struct {
	DryRun        bool
	VerifiedPaths []string
	Unavailable   int
	NotEmpty      int
	RowsMarked    int
}

// RepairZeroByteMediaFiles marks indexed rows unusable only when their paths
// still resolve to regular, empty files. Dry-run rolls no database changes.
func (m *MediaDB) RepairZeroByteMediaFiles(dryRun bool) (*ZeroByteMediaRepairReport, error) {
	rows, err := m.zeroByteMediaRows()
	if err != nil {
		return nil, err
	}

	report := &ZeroByteMediaRepairReport{DryRun: dryRun}
	verified := make([]zeroByteMediaRow, 0, len(rows))
	for _, row := range rows {
		info, err := os.Stat(row.path)
		if err != nil {
			report.Unavailable++
			continue
		}
		if !info.Mode().IsRegular() || info.Size() != 0 {
			report.NotEmpty++
			continue
		}
		verified = append(verified, row)
		report.VerifiedPaths = append(report.VerifiedPaths, row.path)
	}
	if dryRun || len(verified) == 0 {
		return report, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	tx, err := m.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin zero-byte media repair: %w", err)
	}
	defer tx.Rollback()

	for _, row := range verified {
		result, err := tx.Exec(`
			UPDATE media_files
			   SET needs_review = 1, updated_at = CURRENT_TIMESTAMP
			 WHERE id = ? AND path = ? AND size = 0`, row.id, row.path)
		if err != nil {
			return nil, fmt.Errorf("mark zero-byte media file %s: %w", row.path, err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("count marked zero-byte media file %s: %w", row.path, err)
		}
		if changed == 0 {
			continue
		}
		report.RowsMarked += int(changed)

		if _, err := tx.Exec(`
			UPDATE movies
			   SET best_file_id = (
			       SELECT id FROM media_files
			        WHERE media_type = 'movie' AND parent_movie_id = movies.id AND size > 0
			        ORDER BY quality_score DESC, size DESC, id ASC
			        LIMIT 1
			   )
			 WHERE best_file_id = ?`, row.id); err != nil {
			return nil, fmt.Errorf("repair movie best-file reference for %s: %w", row.path, err)
		}
		if _, err := tx.Exec(`
			UPDATE episodes
			   SET best_file_id = (
			       SELECT id FROM media_files
			        WHERE media_type = 'episode' AND size > 0
			          AND (parent_episode_id = episodes.id OR
			               (parent_episode_id IS NULL AND parent_series_id = episodes.series_id
			                AND season = episodes.season AND episode = episodes.episode))
			        ORDER BY quality_score DESC, size DESC, id ASC
			        LIMIT 1
			   ),
			       updated_at = CURRENT_TIMESTAMP
			 WHERE best_file_id = ?`, row.id); err != nil {
			return nil, fmt.Errorf("repair episode best-file reference for %s: %w", row.path, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit zero-byte media repair: %w", err)
	}
	return report, nil
}

type zeroByteMediaRow struct {
	id   int64
	path string
}

func (m *MediaDB) zeroByteMediaRows() ([]zeroByteMediaRow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows, err := m.db.Query(`SELECT id, path FROM media_files WHERE size = 0 ORDER BY path`)
	if err != nil {
		return nil, fmt.Errorf("query zero-byte media files: %w", err)
	}
	defer rows.Close()

	var files []zeroByteMediaRow
	for rows.Next() {
		var file zeroByteMediaRow
		if err := rows.Scan(&file.id, &file.path); err != nil {
			return nil, fmt.Errorf("read zero-byte media file: %w", err)
		}
		files = append(files, file)
	}
	return files, rows.Err()
}
