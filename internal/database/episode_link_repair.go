package database

import (
	"fmt"
)

type EpisodeLinkRepairReport struct {
	DryRun                     bool
	MediaFilesRepaired         int
	BestFileReferencesRepaired int
	UnmatchedMediaFiles        int
	EpisodesWithoutMediaFiles  int
	OrphanedEpisodeSeries      int
}

// RepairEpisodeFileLinks restores file-to-episode links from each file's
// parent series, season, and episode. Dry-run executes the same transaction
// and rolls it back, so its counts match an apply run.
func (m *MediaDB) RepairEpisodeFileLinks(dryRun bool) (*EpisodeLinkRepairReport, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	tx, err := m.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin episode link repair: %w", err)
	}
	defer tx.Rollback()

	report := &EpisodeLinkRepairReport{DryRun: dryRun}
	if err := tx.QueryRow(`
		SELECT COUNT(*)
		  FROM media_files mf
		 WHERE mf.media_type = 'episode'
		   AND mf.parent_series_id IS NOT NULL
		   AND mf.season IS NOT NULL
		   AND mf.episode IS NOT NULL
		   AND NOT EXISTS (
		       SELECT 1 FROM episodes e
		        WHERE e.series_id = mf.parent_series_id
		          AND e.season = mf.season
		          AND e.episode = mf.episode
		   )`).Scan(&report.UnmatchedMediaFiles); err != nil {
		return nil, fmt.Errorf("count unmatched episode files: %w", err)
	}
	if err := tx.QueryRow(`
		SELECT COUNT(*)
		  FROM episodes e
		 WHERE NOT EXISTS (
		       SELECT 1 FROM media_files mf
		        WHERE mf.media_type = 'episode'
		          AND mf.parent_series_id = e.series_id
		          AND mf.season = e.season
		          AND mf.episode = e.episode
		   )`).Scan(&report.EpisodesWithoutMediaFiles); err != nil {
		return nil, fmt.Errorf("count episodes without files: %w", err)
	}
	if err := tx.QueryRow(`
		SELECT COUNT(*)
		  FROM episodes e
		  LEFT JOIN series s ON s.id = e.series_id
		 WHERE s.id IS NULL`).Scan(&report.OrphanedEpisodeSeries); err != nil {
		return nil, fmt.Errorf("count orphaned episode series: %w", err)
	}

	result, err := tx.Exec(`
		UPDATE media_files
		   SET parent_episode_id = (
		           SELECT e.id FROM episodes e
		            WHERE e.series_id = media_files.parent_series_id
		              AND e.season = media_files.season
		              AND e.episode = media_files.episode
		       ),
		       updated_at = CURRENT_TIMESTAMP
		 WHERE media_type = 'episode'
		   AND parent_series_id IS NOT NULL
		   AND season IS NOT NULL
		   AND episode IS NOT NULL
		   AND EXISTS (
		       SELECT 1 FROM episodes e
		        WHERE e.series_id = media_files.parent_series_id
		          AND e.season = media_files.season
		          AND e.episode = media_files.episode
		   )
		   AND parent_episode_id IS NOT (
		       SELECT e.id FROM episodes e
		        WHERE e.series_id = media_files.parent_series_id
		          AND e.season = media_files.season
		          AND e.episode = media_files.episode
		   )`)
	if err != nil {
		return nil, fmt.Errorf("repair media file episode links: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("count repaired media file episode links: %w", err)
	}
	report.MediaFilesRepaired = int(changed)

	result, err = tx.Exec(`
		UPDATE episodes
		   SET best_file_id = (
		           SELECT mf.id FROM media_files mf
		            WHERE mf.media_type = 'episode'
		              AND mf.parent_series_id = episodes.series_id
		              AND mf.season = episodes.season
		              AND mf.episode = episodes.episode
		            ORDER BY mf.quality_score DESC, mf.size DESC, mf.id ASC
		            LIMIT 1
		       ),
		       updated_at = CURRENT_TIMESTAMP
		 WHERE EXISTS (
		       SELECT 1 FROM media_files mf
		        WHERE mf.media_type = 'episode'
		          AND mf.parent_series_id = episodes.series_id
		          AND mf.season = episodes.season
		          AND mf.episode = episodes.episode
		   )
		   AND best_file_id IS NOT (
		       SELECT mf.id FROM media_files mf
		        WHERE mf.media_type = 'episode'
		          AND mf.parent_series_id = episodes.series_id
		          AND mf.season = episodes.season
		          AND mf.episode = episodes.episode
		        ORDER BY mf.quality_score DESC, mf.size DESC, mf.id ASC
	        LIMIT 1
	   )`)
	if err != nil {
		return nil, fmt.Errorf("repair episode best-file references: %w", err)
	}
	changed, err = result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("count repaired episode best-file references: %w", err)
	}
	report.BestFileReferencesRepaired = int(changed)

	if dryRun {
		return report, nil
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit episode link repair: %w", err)
	}
	return report, nil
}
