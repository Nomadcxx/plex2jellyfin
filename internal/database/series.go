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
// Plex2Jellyfin path differs from existing and Sonarr/Radarr should be updated
func (m *MediaDB) UpsertSeries(s *Series) (shouldUpdateExternal bool, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s.TitleNormalized = NormalizeTitle(s.Title)
	s.UpdatedAt = time.Now()

	if s.CanonicalPath != "" {
		var existingID int64
		var existingTitle string
		var existingYear int
		var existingPriority int
		var existingSonarrID sql.NullInt64

		err = m.db.QueryRow(
			`SELECT id, title, year, source_priority, sonarr_id
			   FROM series
			  WHERE canonical_path = ?
			  ORDER BY CASE WHEN year > 0 THEN 0 ELSE 1 END, source_priority DESC, id ASC
			  LIMIT 1`,
			s.CanonicalPath,
		).Scan(&existingID, &existingTitle, &existingYear, &existingPriority, &existingSonarrID)
		if err != nil && err != sql.ErrNoRows {
			return false, err
		}
		if err == nil {
			title := existingTitle
			year := existingYear
			if shouldReplaceSeriesIdentity(existingTitle, existingYear, s.Title, s.Year) {
				title = s.Title
				year = s.Year
			}
			normalized := NormalizeTitle(title)
			if normalized != NormalizeTitle(existingTitle) || year != existingYear {
				merged, shouldUpdate, err := m.mergeSeriesIdentityCollisionLocked(existingID, title, normalized, year, s)
				if err != nil {
					return false, err
				}
				if merged {
					return shouldUpdate, nil
				}
			}

			if s.SourcePriority >= existingPriority {
				_, err = m.db.Exec(`
					UPDATE series SET
						title = ?, title_normalized = ?, year = ?,
						library_root = ?, source = ?, source_priority = ?,
						episode_count = MAX(episode_count, ?),
						last_episode_added = COALESCE(?, last_episode_added),
						updated_at = ?,
						tvdb_id = COALESCE(?, tvdb_id),
						imdb_id = COALESCE(?, imdb_id),
						sonarr_id = COALESCE(?, sonarr_id)
					WHERE id = ?`,
					title, normalized, year,
					s.LibraryRoot, s.Source, s.SourcePriority,
					s.EpisodeCount,
					s.LastEpisodeAdded, s.UpdatedAt,
					s.TvdbID, s.ImdbID, s.SonarrID,
					existingID,
				)
				if err != nil {
					return false, err
				}
			}

			s.ID = existingID
			s.Title = title
			s.TitleNormalized = normalized
			s.Year = year
			return false, nil
		}
	}

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

		// If Plex2Jellyfin is updating and path differs, signal external update needed
		if s.Source == "plex2jellyfin" && s.CanonicalPath != existingPath && existingSonarrID != nil {
			return true, nil
		}
	} else {
		// Lower priority - don't update but set ID
		s.ID = existingID
	}

	return false, nil
}

func (m *MediaDB) mergeSeriesIdentityCollisionLocked(duplicateID int64, title, normalized string, year int, incoming *Series) (merged bool, shouldUpdateExternal bool, err error) {
	var keeperID int64
	var keeperPath string
	var keeperPriority int
	var keeperSonarrID sql.NullInt64

	err = m.db.QueryRow(
		`SELECT id, canonical_path, source_priority, sonarr_id
		   FROM series
		  WHERE title_normalized = ? AND year = ? AND id != ?
		  ORDER BY source_priority DESC, episode_count DESC, id ASC
		  LIMIT 1`,
		normalized, year, duplicateID,
	).Scan(&keeperID, &keeperPath, &keeperPriority, &keeperSonarrID)
	if err == sql.ErrNoRows {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}

	tx, err := m.db.Begin()
	if err != nil {
		return false, false, err
	}
	defer tx.Rollback()

	if _, _, _, err := mergeDuplicateSeriesIntoKeeper(tx, keeperID, duplicateID); err != nil {
		return false, false, err
	}

	if incoming.SourcePriority >= keeperPriority {
		_, err = tx.Exec(`
			UPDATE series SET
				title = ?,
				title_normalized = ?,
				year = ?,
				source = ?,
				source_priority = MAX(source_priority, ?),
				episode_count = MAX(COALESCE(episode_count, 0), ?),
				last_episode_added = COALESCE(?, last_episode_added),
				updated_at = ?,
				tvdb_id = COALESCE(?, tvdb_id),
				imdb_id = COALESCE(?, imdb_id),
				sonarr_id = COALESCE(?, sonarr_id)
			WHERE id = ?`,
			title, normalized, year,
			incoming.Source,
			incoming.SourcePriority,
			incoming.EpisodeCount,
			incoming.LastEpisodeAdded,
			incoming.UpdatedAt,
			incoming.TvdbID, incoming.ImdbID, incoming.SonarrID,
			keeperID,
		)
		if err != nil {
			return false, false, err
		}
	}

	if err := tx.Commit(); err != nil {
		return false, false, err
	}

	if incoming.CanonicalPath != "" && incoming.CanonicalPath != keeperPath {
		if err := m.recordConflictLocked("series", title, year, keeperPath, incoming.CanonicalPath); err != nil {
			// Conflict recording is informational; the upsert should still succeed.
		}
	}

	incoming.ID = keeperID
	incoming.Title = title
	incoming.TitleNormalized = normalized
	incoming.Year = year

	if incoming.Source == "plex2jellyfin" && incoming.CanonicalPath != keeperPath && keeperSonarrID.Valid {
		return true, true, nil
	}
	return true, false, nil
}

func shouldReplaceSeriesIdentity(existingTitle string, existingYear int, incomingTitle string, incomingYear int) bool {
	if NormalizeTitle(existingTitle) == "" {
		return NormalizeTitle(incomingTitle) != ""
	}
	return existingYear == 0 && incomingYear > 0 && NormalizeTitle(incomingTitle) != ""
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

// UpdateSeriesCanonicalPath records a filesystem move for a series and marks
// the row dirty so external managers can be reconciled.
func (m *MediaDB) UpdateSeriesCanonicalPath(id int64, canonicalPath, libraryRoot string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	_, err := m.db.Exec(`
		UPDATE series SET
			canonical_path = ?,
			library_root = ?,
			updated_at = ?,
			sonarr_path_dirty = 1
		WHERE id = ?`,
		canonicalPath, libraryRoot, now, id,
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

func (m *MediaDB) GetAllSeries() ([]*Series, error) {
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
		ORDER BY title`)
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

// PruneFilesystemSeriesWithoutMediaFiles removes stale series rows created by
// filesystem scans when no tracked media file still belongs to the row.
func (m *MediaDB) PruneFilesystemSeriesWithoutMediaFiles() (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	result, err := m.db.Exec(`
		DELETE FROM series
		WHERE source = 'filesystem'
		  AND sonarr_id IS NULL
		  AND tvdb_id IS NULL
		  AND imdb_id IS NULL
		  AND NOT EXISTS (
			SELECT 1
			FROM media_files
			WHERE media_files.parent_series_id = series.id
		  )
	`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// PruneFilesystemSeriesWithoutMediaFilesUnder removes stale filesystem series
// rows only below a specific canonical path.
func (m *MediaDB) PruneFilesystemSeriesWithoutMediaFilesUnder(path string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	result, err := m.db.Exec(`
		DELETE FROM series
		WHERE source = 'filesystem'
		  AND sonarr_id IS NULL
		  AND tvdb_id IS NULL
		  AND imdb_id IS NULL
		  AND (canonical_path = ? OR canonical_path LIKE ?)
		  AND NOT EXISTS (
			SELECT 1
			FROM media_files
			WHERE media_files.parent_series_id = series.id
		  )
	`, path, path+"/%")
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
