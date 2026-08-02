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

	err := m.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(size), 0) FROM media_files`).Scan(&stats.TotalFiles, &stats.TotalSize)
	if err != nil {
		return nil, err
	}

	err = m.db.QueryRow(`SELECT COUNT(*) FROM movies`).Scan(&stats.MovieCount)
	if err != nil {
		return nil, err
	}

	err = m.db.QueryRow(`SELECT COUNT(*) FROM series`).Scan(&stats.SeriesCount)
	if err != nil {
		return nil, err
	}

	err = m.db.QueryRow(`SELECT COUNT(*) FROM media_files WHERE media_type = 'episode'`).Scan(&stats.EpisodeCount)
	if err != nil {
		return nil, err
	}

	// Canonical duplicate analysis from media_files (not dead movie_duplicates /
	// episode_duplicates tables). Use no-lock helpers to avoid nested RLock.
	movieGroups, err := m.findDuplicateMoviesLocked()
	if err != nil {
		return nil, err
	}
	episodeGroups, err := m.findDuplicateEpisodesLocked()
	if err != nil {
		return nil, err
	}

	stats.DuplicateGroups = len(movieGroups) + len(episodeGroups)
	for _, g := range movieGroups {
		stats.ReclaimableBytes += g.SpaceReclaimable
	}
	for _, g := range episodeGroups {
		stats.ReclaimableBytes += g.SpaceReclaimable
	}

	err = m.db.QueryRow(`
		SELECT COUNT(*) FROM conflicts WHERE resolved = 0 AND media_type = 'series'
	`).Scan(&stats.ScatteredSeries)
	if err != nil {
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
