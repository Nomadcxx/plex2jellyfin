package database

// LibraryStats represents comprehensive library statistics
type LibraryStats struct {
	TotalFiles       int
	TotalSize        int64
	MovieCount       int
	SeriesCount      int
	EpisodeCount     int
	DuplicateGroups  int
	ReclaimableBytes int64
	ScatteredSeries  int
}

// GetLibraryStats returns comprehensive database statistics
func (m *MediaDB) GetLibraryStats() (*LibraryStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := &LibraryStats{}

	// Get total files and size from media_files
	err := m.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(size), 0) FROM media_files`).Scan(&stats.TotalFiles, &stats.TotalSize)
	if err != nil {
		return nil, err
	}

	// Get movie count
	err = m.db.QueryRow(`SELECT COUNT(*) FROM movies`).Scan(&stats.MovieCount)
	if err != nil {
		return nil, err
	}

	// Get series count
	err = m.db.QueryRow(`SELECT COUNT(*) FROM series`).Scan(&stats.SeriesCount)
	if err != nil {
		return nil, err
	}

	// Get episode count (files with media_type = 'episode')
	err = m.db.QueryRow(`SELECT COUNT(*) FROM media_files WHERE media_type = 'episode'`).Scan(&stats.EpisodeCount)
	if err != nil {
		return nil, err
	}

	// Get duplicate groups count and reclaimable bytes
	// Movie duplicates
	var movieGroups, movieReclaimable int
	err = m.db.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(space_reclaimable), 0)
		FROM (
			SELECT space_reclaimable
			FROM movie_duplicates
			WHERE id IN (
				SELECT MIN(id) FROM movie_duplicates GROUP BY normalized_title, year
			)
		)
	`).Scan(&movieGroups, &movieReclaimable)
	if err != nil {
		// Table might not exist or be empty, continue with 0
		movieGroups = 0
		movieReclaimable = 0
	}

	// Episode duplicates
	var episodeGroups, episodeReclaimable int
	err = m.db.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(space_reclaimable), 0)
		FROM (
			SELECT space_reclaimable
			FROM episode_duplicates
			WHERE id IN (
				SELECT MIN(id) FROM episode_duplicates GROUP BY normalized_title, year, season, episode
			)
		)
	`).Scan(&episodeGroups, &episodeReclaimable)
	if err != nil {
		// Table might not exist or be empty, continue with 0
		episodeGroups = 0
		episodeReclaimable = 0
	}

	stats.DuplicateGroups = movieGroups + episodeGroups
	stats.ReclaimableBytes = int64(movieReclaimable + episodeReclaimable)

	// Get scattered series count from conflicts
	err = m.db.QueryRow(`
		SELECT COUNT(*) FROM conflicts WHERE resolved = 0 AND media_type = 'series'
	`).Scan(&stats.ScatteredSeries)
	if err != nil {
		// Table might not exist or be empty, continue with 0
		stats.ScatteredSeries = 0
	}

	return stats, nil
}

// Stats represents basic database statistics (legacy, kept for compatibility)
type Stats struct {
	SeriesCount int
	MoviesCount int
}

// GetStats returns basic database statistics (legacy method)
func (m *MediaDB) GetStats() (*Stats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var stats Stats

	err := m.db.QueryRow(`SELECT COUNT(*) FROM series`).Scan(&stats.SeriesCount)
	if err != nil {
		return nil, err
	}

	err = m.db.QueryRow(`SELECT COUNT(*) FROM movies`).Scan(&stats.MoviesCount)
	if err != nil {
		return nil, err
	}

	return &stats, nil
}