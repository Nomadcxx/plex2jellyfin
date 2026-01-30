package database

import (
	"database/sql"
	"time"
)

// GetDirtySeries returns all series with sonarr_path_dirty = 1 or radarr_path_dirty = 1
func (m *MediaDB) GetDirtySeries() ([]Series, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `SELECT id, title, title_normalized, year, tvdb_id, imdb_id,
	                 sonarr_id, canonical_path, library_root, source,
	                 source_priority, episode_count, last_episode_added,
	                 created_at, updated_at, last_synced_at,
	                 sonarr_synced_at, sonarr_path_dirty,
	                 radarr_synced_at, radarr_path_dirty
	          FROM series
	          WHERE sonarr_path_dirty = 1 OR radarr_path_dirty = 1
	          ORDER BY source_priority DESC`

	rows, err := m.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var series []Series
	for rows.Next() {
		var s Series
		err := rows.Scan(
			&s.ID, &s.Title, &s.TitleNormalized, &s.Year, &s.TvdbID, &s.ImdbID,
			&s.SonarrID, &s.CanonicalPath, &s.LibraryRoot, &s.Source,
			&s.SourcePriority, &s.EpisodeCount, &s.LastEpisodeAdded,
			&s.CreatedAt, &s.UpdatedAt, &s.LastSyncedAt,
			&s.SonarrSyncedAt, &s.SonarrPathDirty,
			&s.RadarrSyncedAt, &s.RadarrPathDirty,
		)
		if err != nil {
			return nil, err
		}
		series = append(series, s)
	}

	return series, rows.Err()
}

// GetDirtyMovies returns all movies with radarr_path_dirty = 1
func (m *MediaDB) GetDirtyMovies() ([]Movie, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `SELECT id, title, title_normalized, year, tmdb_id, imdb_id,
	                 radarr_id, canonical_path, library_root, source,
	                 source_priority, created_at, updated_at, last_synced_at,
	                 radarr_synced_at, radarr_path_dirty
	          FROM movies
	          WHERE radarr_path_dirty = 1
	          ORDER BY source_priority DESC`

	rows, err := m.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var movies []Movie
	for rows.Next() {
		var m Movie
		err := rows.Scan(
			&m.ID, &m.Title, &m.TitleNormalized, &m.Year, &m.TmdbID, &m.ImdbID,
			&m.RadarrID, &m.CanonicalPath, &m.LibraryRoot, &m.Source,
			&m.SourcePriority, &m.CreatedAt, &m.UpdatedAt, &m.LastSyncedAt,
			&m.RadarrSyncedAt, &m.RadarrPathDirty,
		)
		if err != nil {
			return nil, err
		}
		movies = append(movies, m)
	}

	return movies, rows.Err()
}

// MarkSeriesSynced marks a series as synced to Sonarr/Radarr
func (m *MediaDB) MarkSeriesSynced(id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	_, err := m.db.Exec(`
		UPDATE series
		SET sonarr_path_dirty = 0, sonarr_synced_at = ?,
		    radarr_path_dirty = 0, radarr_synced_at = ?
		WHERE id = ?
	`, now, now, id)

	return err
}

// MarkMovieSynced marks a movie as synced to Radarr
func (m *MediaDB) MarkMovieSynced(id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	_, err := m.db.Exec(`
		UPDATE movies
		SET radarr_path_dirty = 0, radarr_synced_at = ?
		WHERE id = ?
	`, now, id)

	return err
}

// SetSeriesDirty marks a series as needing sync to Sonarr
func (m *MediaDB) SetSeriesDirty(id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(`UPDATE series SET sonarr_path_dirty = 1 WHERE id = ?`, id)
	return err
}

// SetMovieDirty marks a movie as needing sync to Radarr
func (m *MediaDB) SetMovieDirty(id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(`UPDATE movies SET radarr_path_dirty = 1 WHERE id = ?`, id)
	return err
}

// GetSeriesByID retrieves a series by its database ID
func (m *MediaDB) GetSeriesByID(id int64) (*Series, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `SELECT id, title, title_normalized, year, tvdb_id, imdb_id,
	                 sonarr_id, canonical_path, library_root, source,
	                 source_priority, episode_count, last_episode_added,
	                 created_at, updated_at, last_synced_at,
	                 sonarr_synced_at, sonarr_path_dirty,
	                 radarr_synced_at, radarr_path_dirty
	          FROM series WHERE id = ?`

	var s Series
	err := m.db.QueryRow(query, id).Scan(
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

// GetMovieByID retrieves a movie by its database ID
func (m *MediaDB) GetMovieByID(id int64) (*Movie, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `SELECT id, title, title_normalized, year, tmdb_id, imdb_id,
	                 radarr_id, canonical_path, library_root, source,
	                 source_priority, created_at, updated_at, last_synced_at,
	                 radarr_synced_at, radarr_path_dirty
	          FROM movies WHERE id = ?`

	var mov Movie
	err := m.db.QueryRow(query, id).Scan(
		&mov.ID, &mov.Title, &mov.TitleNormalized, &mov.Year,
		&mov.TmdbID, &mov.ImdbID, &mov.RadarrID,
		&mov.CanonicalPath, &mov.LibraryRoot, &mov.Source,
		&mov.SourcePriority, &mov.CreatedAt, &mov.UpdatedAt, &mov.LastSyncedAt,
		&mov.RadarrSyncedAt, &mov.RadarrPathDirty,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &mov, nil
}
