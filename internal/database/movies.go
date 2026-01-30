package database

import (
	"database/sql"
	"time"
)

// Movie represents a movie in the database
type Movie struct {
	ID              int64
	Title           string
	TitleNormalized string
	Year            int
	TmdbID          *int
	ImdbID          *string
	RadarrID        *int
	CanonicalPath   string
	LibraryRoot     string
	Source          string
	SourcePriority  int
	CreatedAt       time.Time
	UpdatedAt       time.Time
	LastSyncedAt    *time.Time
	RadarrSyncedAt  *time.Time // NEW: last sync to Radarr
	RadarrPathDirty bool       // NEW: needs sync to Radarr
}

// GetMovieByTitle looks up a movie by normalized title and optional year
func (m *MediaDB) GetMovieByTitle(title string, year int) (*Movie, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	normalized := NormalizeTitle(title)

	var query string
	var args []interface{}

	if year > 0 {
		query = `SELECT id, title, title_normalized, year, tmdb_id, imdb_id, 
		                radarr_id, canonical_path, library_root, source, 
		                source_priority, created_at, updated_at, last_synced_at,
		                radarr_synced_at, radarr_path_dirty
		         FROM movies 
		         WHERE title_normalized = ? AND year = ?`
		args = []interface{}{normalized, year}
	} else {
		query = `SELECT id, title, title_normalized, year, tmdb_id, imdb_id,
		                radarr_id, canonical_path, library_root, source,
		                source_priority, created_at, updated_at, last_synced_at,
		                radarr_synced_at, radarr_path_dirty
		         FROM movies 
		         WHERE title_normalized = ?
		         ORDER BY source_priority DESC
		         LIMIT 1`
		args = []interface{}{normalized}
	}

	var mov Movie
	err := m.db.QueryRow(query, args...).Scan(
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

// UpsertMovie inserts or updates a movie record
// Returns (shouldUpdateExternal, error) where shouldUpdateExternal=true means
// JellyWatch path differs from existing and Radarr should be updated
func (m *MediaDB) UpsertMovie(mov *Movie) (shouldUpdateExternal bool, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	mov.TitleNormalized = NormalizeTitle(mov.Title)
	mov.UpdatedAt = time.Now()

	// Check existing record
	var existingID int64
	var existingPath string
	var existingPriority int
	var existingRadarrID *int

	err = m.db.QueryRow(
		`SELECT id, canonical_path, source_priority, radarr_id FROM movies 
		 WHERE title_normalized = ? AND year = ?`,
		mov.TitleNormalized, mov.Year,
	).Scan(&existingID, &existingPath, &existingPriority, &existingRadarrID)

	if err == sql.ErrNoRows {
		// Insert new record
		if mov.CreatedAt.IsZero() {
			mov.CreatedAt = time.Now()
		}

		result, err := m.db.Exec(`
			INSERT INTO movies (
				title, title_normalized, year, tmdb_id, imdb_id, radarr_id,
				canonical_path, library_root, source, source_priority,
				created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			mov.Title, mov.TitleNormalized, mov.Year, mov.TmdbID, mov.ImdbID, mov.RadarrID,
			mov.CanonicalPath, mov.LibraryRoot, mov.Source, mov.SourcePriority,
			mov.CreatedAt, mov.UpdatedAt,
		)
		if err != nil {
			return false, err
		}

		mov.ID, _ = result.LastInsertId()
		return false, nil
	}

	if err != nil {
		return false, err
	}

	// Update existing - only if we have higher or equal priority
	if mov.SourcePriority >= existingPriority {
		// CONFLICT DETECTION: If paths differ at same priority level, record conflict
		if mov.CanonicalPath != existingPath {
			// Record conflict before updating - we want to track both locations
			if err := m.recordConflictLocked("movie", mov.Title, mov.Year, existingPath, mov.CanonicalPath); err != nil {
				// Log but don't fail - conflict recording is informational
				// The update should still proceed
			}
		}

		_, err = m.db.Exec(`
			UPDATE movies SET
				title = ?, canonical_path = ?, library_root = ?,
				source = ?, source_priority = ?, updated_at = ?,
				tmdb_id = COALESCE(?, tmdb_id),
				imdb_id = COALESCE(?, imdb_id),
				radarr_id = COALESCE(?, radarr_id)
			WHERE id = ?`,
			mov.Title, mov.CanonicalPath, mov.LibraryRoot,
			mov.Source, mov.SourcePriority, mov.UpdatedAt,
			mov.TmdbID, mov.ImdbID, mov.RadarrID,
			existingID,
		)
		if err != nil {
			return false, err
		}

		mov.ID = existingID

		// If JellyWatch is updating and path differs, signal external update needed
		if mov.Source == "jellywatch" && mov.CanonicalPath != existingPath && existingRadarrID != nil {
			return true, nil
		}
	} else {
		// Lower priority - don't update but set ID
		mov.ID = existingID
	}

	return false, nil
}

// GetAllMoviesInLibrary returns all movies in a specific library root
func (m *MediaDB) GetAllMoviesInLibrary(libraryRoot string) ([]*Movie, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows, err := m.db.Query(`
		SELECT id, title, title_normalized, year, tmdb_id, imdb_id,
		       radarr_id, canonical_path, library_root, source,
		       source_priority, created_at, updated_at, last_synced_at,
		       radarr_synced_at, radarr_path_dirty
		FROM movies
		WHERE library_root = ?
		ORDER BY title`,
		libraryRoot,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var movies []*Movie
	for rows.Next() {
		var mov Movie
		err := rows.Scan(
			&mov.ID, &mov.Title, &mov.TitleNormalized, &mov.Year,
			&mov.TmdbID, &mov.ImdbID, &mov.RadarrID,
			&mov.CanonicalPath, &mov.LibraryRoot, &mov.Source,
			&mov.SourcePriority, &mov.CreatedAt, &mov.UpdatedAt, &mov.LastSyncedAt,
			&mov.RadarrSyncedAt, &mov.RadarrPathDirty,
		)
		if err != nil {
			return nil, err
		}
		movies = append(movies, &mov)
	}

	return movies, rows.Err()
}

// CountMoviesInLibrary returns the number of movies in a library
func (m *MediaDB) CountMoviesInLibrary(libraryRoot string) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var count int
	err := m.db.QueryRow(`SELECT COUNT(*) FROM movies WHERE library_root = ?`, libraryRoot).Scan(&count)
	return count, err
}
