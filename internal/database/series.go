package database

import (
	"database/sql"
	"time"
)

// Series represents a TV series in the database
type Series struct {
	ID               int64
	Title            string
	TitleNormalized  string
	Year             int
	TvdbID           *int
	ImdbID           *string
	SonarrID         *int
	CanonicalPath    string
	LibraryRoot      string
	Source           string
	SourcePriority   int
	EpisodeCount     int
	LastEpisodeAdded *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
	LastSyncedAt     *time.Time
	SonarrSyncedAt   *time.Time // NEW: last sync to Sonarr
	SonarrPathDirty  bool       // NEW: needs sync to Sonarr
	RadarrSyncedAt   *time.Time // NEW: last sync to Radarr (future)
	RadarrPathDirty  bool       // NEW: needs sync to Radarr (future)
}

// GetSeriesByTitle looks up a series by normalized title and optional year
func (m *MediaDB) GetSeriesByTitle(title string, year int) (*Series, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	normalized := NormalizeTitle(title)

	var query string
	var args []interface{}

	if year > 0 {
		query = `SELECT id, title, title_normalized, year, tvdb_id, imdb_id, 
		                sonarr_id, canonical_path, library_root, source, 
		                source_priority, episode_count, last_episode_added,
		                created_at, updated_at, last_synced_at,
		                sonarr_synced_at, sonarr_path_dirty,
		                radarr_synced_at, radarr_path_dirty
		         FROM series 
		         WHERE title_normalized = ? AND year = ?`
		args = []interface{}{normalized, year}
	} else {
		query = `SELECT id, title, title_normalized, year, tvdb_id, imdb_id,
		                sonarr_id, canonical_path, library_root, source,
		                source_priority, episode_count, last_episode_added,
		                created_at, updated_at, last_synced_at,
		                sonarr_synced_at, sonarr_path_dirty,
		                radarr_synced_at, radarr_path_dirty
		         FROM series 
		         WHERE title_normalized = ?
		         ORDER BY source_priority DESC, episode_count DESC
		         LIMIT 1`
		args = []interface{}{normalized}
	}

	var s Series
	err := m.db.QueryRow(query, args...).Scan(
		&s.ID, &s.Title, &s.TitleNormalized, &s.Year,
		&s.TvdbID, &s.ImdbID, &s.SonarrID,
		&s.CanonicalPath, &s.LibraryRoot, &s.Source,
		&s.SourcePriority, &s.EpisodeCount, &s.LastEpisodeAdded,
		&s.CreatedAt, &s.UpdatedAt, &s.LastSyncedAt,
		&s.SonarrSyncedAt, &s.SonarrPathDirty,
		&s.RadarrSyncedAt, &s.RadarrPathDirty,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &s, nil
}

// UpsertSeries inserts or updates a series record
// Returns (shouldUpdateExternal, error) where shouldUpdateExternal=true means
// JellyWatch path differs from existing and Sonarr/Radarr should be updated
func (m *MediaDB) UpsertSeries(s *Series) (shouldUpdateExternal bool, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s.TitleNormalized = NormalizeTitle(s.Title)
	s.UpdatedAt = time.Now()

	// Check existing record
	var existingID int64
	var existingPath string
	var existingPriority int
	var existingSonarrID *int

	err = m.db.QueryRow(
		`SELECT id, canonical_path, source_priority, sonarr_id FROM series 
		 WHERE title_normalized = ? AND year = ?`,
		s.TitleNormalized, s.Year,
	).Scan(&existingID, &existingPath, &existingPriority, &existingSonarrID)

	if err == sql.ErrNoRows {
		// Insert new record
		if s.CreatedAt.IsZero() {
			s.CreatedAt = time.Now()
		}

		result, err := m.db.Exec(`
			INSERT INTO series (
				title, title_normalized, year, tvdb_id, imdb_id, sonarr_id,
				canonical_path, library_root, source, source_priority,
				episode_count, last_episode_added, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			s.Title, s.TitleNormalized, s.Year, s.TvdbID, s.ImdbID, s.SonarrID,
			s.CanonicalPath, s.LibraryRoot, s.Source, s.SourcePriority,
			s.EpisodeCount, s.LastEpisodeAdded, s.CreatedAt, s.UpdatedAt,
		)
		if err != nil {
			return false, err
		}

		s.ID, _ = result.LastInsertId()
		return false, nil
	}

	if err != nil {
		return false, err
	}

	// Update existing - only if we have higher or equal priority
	if s.SourcePriority >= existingPriority {
		// CONFLICT DETECTION: If paths differ at same priority level, record conflict
		if s.CanonicalPath != existingPath {
			// Record conflict before updating - we want to track both locations
			if err := m.recordConflictLocked("series", s.Title, s.Year, existingPath, s.CanonicalPath); err != nil {
				// Log but don't fail - conflict recording is informational
				// The update should still proceed
			}
		}

		_, err = m.db.Exec(`
			UPDATE series SET
				title = ?, canonical_path = ?, library_root = ?,
				source = ?, source_priority = ?, episode_count = ?,
				last_episode_added = ?, updated_at = ?,
				tvdb_id = COALESCE(?, tvdb_id),
				imdb_id = COALESCE(?, imdb_id),
				sonarr_id = COALESCE(?, sonarr_id)
			WHERE id = ?`,
			s.Title, s.CanonicalPath, s.LibraryRoot,
			s.Source, s.SourcePriority, s.EpisodeCount,
			s.LastEpisodeAdded, s.UpdatedAt,
			s.TvdbID, s.ImdbID, s.SonarrID,
			existingID,
		)
		if err != nil {
			return false, err
		}

		s.ID = existingID

		// If JellyWatch is updating and path differs, signal external update needed
		if s.Source == "jellywatch" && s.CanonicalPath != existingPath && existingSonarrID != nil {
			return true, nil
		}
	} else {
		// Lower priority - don't update but set ID
		s.ID = existingID
	}

	return false, nil
}

// GetAllSeriesInLibrary returns all series in a specific library root
func (m *MediaDB) GetAllSeriesInLibrary(libraryRoot string) ([]*Series, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows, err := m.db.Query(`
		SELECT id, title, title_normalized, year, tvdb_id, imdb_id,
		       sonarr_id, canonical_path, library_root, source,
		       source_priority, episode_count, last_episode_added,
		       created_at, updated_at, last_synced_at,
		       sonarr_synced_at, sonarr_path_dirty,
		       radarr_synced_at, radarr_path_dirty
		FROM series
		WHERE library_root = ?
		ORDER BY title`,
		libraryRoot,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var series []*Series
	for rows.Next() {
		var s Series
		err := rows.Scan(
			&s.ID, &s.Title, &s.TitleNormalized, &s.Year,
			&s.TvdbID, &s.ImdbID, &s.SonarrID,
			&s.CanonicalPath, &s.LibraryRoot, &s.Source,
			&s.SourcePriority, &s.EpisodeCount, &s.LastEpisodeAdded,
			&s.CreatedAt, &s.UpdatedAt, &s.LastSyncedAt,
			&s.SonarrSyncedAt, &s.SonarrPathDirty,
			&s.RadarrSyncedAt, &s.RadarrPathDirty,
		)
		if err != nil {
			return nil, err
		}
		series = append(series, &s)
	}

	return series, rows.Err()
}

// IncrementEpisodeCount adds to the episode count and updates last_episode_added
func (m *MediaDB) IncrementEpisodeCount(seriesID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	_, err := m.db.Exec(`
		UPDATE series SET 
			episode_count = episode_count + 1,
			last_episode_added = ?,
			updated_at = ?
		WHERE id = ?`,
		now, now, seriesID,
	)
	return err
}

// CountSeriesInLibrary returns the number of series in a library
func (m *MediaDB) CountSeriesInLibrary(libraryRoot string) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var count int
	err := m.db.QueryRow(`SELECT COUNT(*) FROM series WHERE library_root = ?`, libraryRoot).Scan(&count)
	return count, err
}
