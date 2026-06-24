package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Conflict represents a media item existing in multiple locations
type Conflict struct {
	ID              int64  `json:"id"`
	MediaType       string `json:"media_type"` // "movie" or "series"
	Title           string `json:"title"`
	TitleNormalized string `json:"title_normalized"`
	Year            *int   `json:"year"`

	// JSON array of paths
	Locations    []string   `json:"locations"`
	Resolved     bool       `json:"resolved"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
	ResolvedPath *string    `json:"resolved_path,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// GetUnresolvedConflicts returns all unresolved conflicts
func (m *MediaDB) GetUnresolvedConflicts() ([]Conflict, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.getUnresolvedConflictsLocked()
}

// getUnresolvedConflictsLocked is the internal version that assumes lock is already held
func (m *MediaDB) getUnresolvedConflictsLocked() ([]Conflict, error) {
	rows, err := m.db.Query(`
		SELECT id, media_type, title, title_normalized, year, locations, created_at
		FROM conflicts
		WHERE resolved = FALSE
		ORDER BY media_type, title_normalized
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query conflicts: %w", err)
	}
	defer rows.Close()

	var conflicts []Conflict
	for rows.Next() {
		var c Conflict
		var locationsJSON string

		err := rows.Scan(
			&c.ID, &c.MediaType, &c.Title, &c.TitleNormalized, &c.Year,
			&locationsJSON, &c.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan conflict: %w", err)
		}

		// Parse locations JSON
		if err := json.Unmarshal([]byte(locationsJSON), &c.Locations); err != nil {
			return nil, fmt.Errorf("failed to parse locations JSON: %w", err)
		}

		conflicts = append(conflicts, c)
	}

	return conflicts, rows.Err()
}

// DetectConflicts identifies shows/movies that exist in multiple locations
func (m *MediaDB) DetectConflicts() ([]Conflict, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	tvConflicts, err := m.detectConflicts("series", "series")
	if err != nil {
		return nil, fmt.Errorf("failed to detect series conflicts: %w", err)
	}

	movieConflicts, err := m.detectConflicts("movies", "movie")
	if err != nil {
		return nil, fmt.Errorf("failed to detect movie conflicts: %w", err)
	}

	conflicts := append(tvConflicts, movieConflicts...)

	for _, conflict := range conflicts {
		if err := m.upsertConflict(conflict); err != nil {
			return nil, fmt.Errorf("failed to insert conflict: %w", err)
		}
	}

	return m.getUnresolvedConflictsLocked()
}

// detectConflicts finds items in the given table with multiple canonical_path values.
// tableName is "series" or "movies"; mediaType is "series" or "movie" (stored on Conflict).
func (m *MediaDB) detectConflicts(tableName, mediaType string) ([]Conflict, error) {
	rows, err := m.db.Query(`
		SELECT title, title_normalized, year,
		       json_group_array(DISTINCT canonical_path) as locations,
		       COUNT(DISTINCT canonical_path) as location_count
		FROM ` + tableName + `
		GROUP BY title_normalized, year
		HAVING location_count > 1
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query %s conflicts: %w", mediaType, err)
	}
	defer rows.Close()

	var conflicts []Conflict
	for rows.Next() {
		var c Conflict
		var locationsJSON string
		var locationCount int

		err := rows.Scan(
			&c.Title, &c.TitleNormalized, &c.Year, &locationsJSON, &locationCount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan %s conflict: %w", mediaType, err)
		}

		if err := json.Unmarshal([]byte(locationsJSON), &c.Locations); err != nil {
			return nil, fmt.Errorf("failed to parse locations JSON: %w", err)
		}

		c.MediaType = mediaType
		c.CreatedAt = time.Now()
		conflicts = append(conflicts, c)
	}

	return conflicts, rows.Err()
}

// upsertConflict inserts or updates a conflict
func (m *MediaDB) upsertConflict(c Conflict) error {
	// Convert locations to JSON
	locationsJSON, err := json.Marshal(c.Locations)
	if err != nil {
		return fmt.Errorf("failed to marshal locations: %w", err)
	}

	var existingID int64
	err = m.db.QueryRow(`
		SELECT id FROM conflicts
		WHERE media_type = ? AND title_normalized = ? AND year IS ? AND resolved = FALSE`,
		c.MediaType, c.TitleNormalized, c.Year,
	).Scan(&existingID)
	if err == nil {
		_, err = m.db.Exec(`
			UPDATE conflicts
			   SET title = ?,
			       locations = ?,
			       resolved = FALSE,
			       resolved_at = NULL,
			       resolved_path = NULL
			 WHERE id = ?`,
			c.Title, string(locationsJSON), existingID,
		)
		return err
	}
	if err != sql.ErrNoRows {
		return err
	}

	_, err = m.db.Exec(`
		INSERT INTO conflicts (
			media_type, title, title_normalized, year, locations, created_at
		) VALUES (?, ?, ?, ?, ?, ?)`,
		c.MediaType, c.Title, c.TitleNormalized, c.Year,
		string(locationsJSON), time.Now(),
	)
	return err
}

// RecordConflict records a conflict when the same media is found in multiple locations.
// This is called from UpsertSeries/UpsertMovie when a duplicate is detected.
// IMPORTANT: Caller must already hold the mutex lock.
func (m *MediaDB) recordConflictLocked(mediaType, title string, year int, existingPath, newPath string) error {
	titleNormalized := NormalizeTitle(title)

	// Check if we already have an unresolved conflict for this media
	var existingID int64
	var existingLocationsJSON string
	err := m.db.QueryRow(`
		SELECT id, locations FROM conflicts
		WHERE media_type = ? AND title_normalized = ? AND year = ? AND resolved = FALSE`,
		mediaType, titleNormalized, year,
	).Scan(&existingID, &existingLocationsJSON)

	if err == sql.ErrNoRows {
		// No existing conflict - create new one with both locations
		locations := []string{existingPath, newPath}
		locationsJSON, err := json.Marshal(locations)
		if err != nil {
			return fmt.Errorf("failed to marshal locations: %w", err)
		}

		_, err = m.db.Exec(`
			INSERT INTO conflicts (media_type, title, title_normalized, year, locations, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			mediaType, title, titleNormalized, year, string(locationsJSON), time.Now(),
		)
		return err
	}

	if err != nil {
		return fmt.Errorf("failed to check existing conflict: %w", err)
	}

	// Existing conflict found - add new location if not already present
	var locations []string
	if err := json.Unmarshal([]byte(existingLocationsJSON), &locations); err != nil {
		return fmt.Errorf("failed to parse existing locations: %w", err)
	}

	// Check if newPath is already in locations
	for _, loc := range locations {
		if loc == newPath {
			return nil // Already recorded
		}
	}

	// Add new location
	locations = append(locations, newPath)
	locationsJSON, err := json.Marshal(locations)
	if err != nil {
		return fmt.Errorf("failed to marshal updated locations: %w", err)
	}

	_, err = m.db.Exec(`UPDATE conflicts SET locations = ? WHERE id = ?`,
		string(locationsJSON), existingID)
	return err
}

// ResolveConflict marks a conflict as resolved
func (m *MediaDB) ResolveConflict(conflictID int64, resolvedPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	_, err := m.db.Exec(`
		UPDATE conflicts SET
			resolved = TRUE,
			resolved_at = ?,
			resolved_path = ?
		WHERE id = ? AND resolved = FALSE
	`, now, resolvedPath, conflictID)

	return err
}

// GetConflict returns a specific conflict by ID
func (m *MediaDB) GetConflict(conflictID int64) (*Conflict, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var c Conflict
	var locationsJSON string
	var resolvedAt sql.NullTime
	var resolvedPath sql.NullString

	query := `
		SELECT id, media_type, title, title_normalized, year,
		       locations, resolved, resolved_at, resolved_path, created_at
		FROM conflicts
		WHERE id = ?
	`

	err := m.db.QueryRow(query, conflictID).Scan(
		&c.ID, &c.MediaType, &c.Title, &c.TitleNormalized, &c.Year,
		&locationsJSON, &c.Resolved, &resolvedAt, &resolvedPath, &c.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil // Conflict not found
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan conflict: %w", err)
	}

	// Parse locations JSON
	if err := json.Unmarshal([]byte(locationsJSON), &c.Locations); err != nil {
		return nil, fmt.Errorf("failed to parse locations JSON: %w", err)
	}

	// Set nullable fields
	if resolvedAt.Valid {
		c.ResolvedAt = &resolvedAt.Time
	}
	if resolvedPath.Valid {
		c.ResolvedPath = &resolvedPath.String
	}

	return &c, nil
}
