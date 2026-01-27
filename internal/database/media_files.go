package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// MediaFile represents a single media file (movie or episode)
type MediaFile struct {
	ID         int64
	Path       string
	Size       int64
	ModifiedAt time.Time

	// Classification
	MediaType       string // "movie" or "episode"
	ParentMovieID   *int64 // NULL for episodes
	ParentSeriesID  *int64 // NULL for movies
	ParentEpisodeID *int64 // NULL for movies

	// Normalized identity (for duplicate detection)
	NormalizedTitle string
	Year            *int
	Season          *int // NULL for movies
	Episode         *int // NULL for movies

	// Quality metadata
	Resolution   string
	SourceType   string
	Codec        string
	AudioFormat  string
	QualityScore int

	// Confidence tracking
	Confidence  float64
	NeedsReview bool
	ParseMethod string // "regex", "folder", "ai"

	// Jellyfin compliance
	IsJellyfinCompliant bool
	ComplianceIssues    []string // Stored as JSON

	// Provenance
	Source         string
	SourcePriority int
	LibraryRoot    string

	// Timestamps
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Episode represents a TV episode entry (links series to files)
type Episode struct {
	ID         int64
	SeriesID   int64
	Season     int
	Episode    int
	Title      string
	BestFileID *int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// UpsertMediaFile inserts or updates a media file record
func (m *MediaDB) UpsertMediaFile(file *MediaFile) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Serialize compliance issues
	complianceJSON, err := json.Marshal(file.ComplianceIssues)
	if err != nil {
		return fmt.Errorf("failed to marshal compliance issues: %w", err)
	}

	query := `
		INSERT INTO media_files (
			path, size, modified_at,
			media_type, parent_movie_id, parent_series_id, parent_episode_id,
			normalized_title, year, season, episode,
			resolution, source_type, codec, audio_format, quality_score,
			is_jellyfin_compliant, compliance_issues,
			source, source_priority, library_root,
			confidence, parse_method, needs_review,
			updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(path) DO UPDATE SET
			size = excluded.size,
			modified_at = excluded.modified_at,
			media_type = excluded.media_type,
			parent_movie_id = excluded.parent_movie_id,
			parent_series_id = excluded.parent_series_id,
			parent_episode_id = excluded.parent_episode_id,
			normalized_title = excluded.normalized_title,
			year = excluded.year,
			season = excluded.season,
			episode = excluded.episode,
			resolution = excluded.resolution,
			source_type = excluded.source_type,
			codec = excluded.codec,
			audio_format = excluded.audio_format,
			quality_score = excluded.quality_score,
			is_jellyfin_compliant = excluded.is_jellyfin_compliant,
			compliance_issues = excluded.compliance_issues,
			source = excluded.source,
			source_priority = excluded.source_priority,
			library_root = excluded.library_root,
			confidence = excluded.confidence,
			parse_method = excluded.parse_method,
			needs_review = excluded.needs_review,
			updated_at = CURRENT_TIMESTAMP
	`

	// Convert bool to int for SQLite
	needsReviewInt := 0
	if file.NeedsReview {
		needsReviewInt = 1
	}

	result, err := m.db.Exec(
		query,
		file.Path, file.Size, file.ModifiedAt,
		file.MediaType, file.ParentMovieID, file.ParentSeriesID, file.ParentEpisodeID,
		file.NormalizedTitle, file.Year, file.Season, file.Episode,
		file.Resolution, file.SourceType, file.Codec, file.AudioFormat, file.QualityScore,
		file.IsJellyfinCompliant, string(complianceJSON),
		file.Source, file.SourcePriority, file.LibraryRoot,
		file.Confidence, file.ParseMethod, needsReviewInt,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert media file: %w", err)
	}

	// Get ID if this was an insert
	if file.ID == 0 {
		id, err := result.LastInsertId()
		if err == nil {
			file.ID = id
		}
	}

	return nil
}

// GetMediaFile retrieves a media file by path
func (m *MediaDB) GetMediaFile(path string) (*MediaFile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var file MediaFile
	var complianceJSON string

	query := `
		SELECT
			id, path, size, modified_at,
			media_type, parent_movie_id, parent_series_id, parent_episode_id,
			normalized_title, year, season, episode,
			resolution, source_type, codec, audio_format, quality_score,
			confidence, parse_method, needs_review,
			is_jellyfin_compliant, compliance_issues,
			source, source_priority, library_root,
			created_at, updated_at
		FROM media_files
		WHERE path = ?
	`

	var needsReviewInt int
	err := m.db.QueryRow(query, path).Scan(
		&file.ID, &file.Path, &file.Size, &file.ModifiedAt,
		&file.MediaType, &file.ParentMovieID, &file.ParentSeriesID, &file.ParentEpisodeID,
		&file.NormalizedTitle, &file.Year, &file.Season, &file.Episode,
		&file.Resolution, &file.SourceType, &file.Codec, &file.AudioFormat, &file.QualityScore,
		&file.Confidence, &file.ParseMethod, &needsReviewInt,
		&file.IsJellyfinCompliant, &complianceJSON,
		&file.Source, &file.SourcePriority, &file.LibraryRoot,
		&file.CreatedAt, &file.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get media file: %w", err)
	}

	file.NeedsReview = needsReviewInt != 0

	// Deserialize compliance issues
	if complianceJSON != "" {
		if err := json.Unmarshal([]byte(complianceJSON), &file.ComplianceIssues); err != nil {
			return nil, fmt.Errorf("failed to unmarshal compliance issues: %w", err)
		}
	}

	return &file, nil
}

// GetMediaFileByID retrieves a media file by ID
func (m *MediaDB) GetMediaFileByID(id int64) (*MediaFile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var file MediaFile
	var complianceJSON string

	query := `
		SELECT
			id, path, size, modified_at,
			media_type, parent_movie_id, parent_series_id, parent_episode_id,
			normalized_title, year, season, episode,
			resolution, source_type, codec, audio_format, quality_score,
			confidence, parse_method, needs_review,
			is_jellyfin_compliant, compliance_issues,
			source, source_priority, library_root,
			created_at, updated_at
		FROM media_files
		WHERE id = ?
	`

	var needsReviewInt int
	err := m.db.QueryRow(query, id).Scan(
		&file.ID, &file.Path, &file.Size, &file.ModifiedAt,
		&file.MediaType, &file.ParentMovieID, &file.ParentSeriesID, &file.ParentEpisodeID,
		&file.NormalizedTitle, &file.Year, &file.Season, &file.Episode,
		&file.Resolution, &file.SourceType, &file.Codec, &file.AudioFormat, &file.QualityScore,
		&file.Confidence, &file.ParseMethod, &needsReviewInt,
		&file.IsJellyfinCompliant, &complianceJSON,
		&file.Source, &file.SourcePriority, &file.LibraryRoot,
		&file.CreatedAt, &file.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get media file: %w", err)
	}

	file.NeedsReview = needsReviewInt != 0

	// Deserialize compliance issues
	if complianceJSON != "" {
		if err := json.Unmarshal([]byte(complianceJSON), &file.ComplianceIssues); err != nil {
			return nil, fmt.Errorf("failed to unmarshal compliance issues: %w", err)
		}
	}

	return &file, nil
}

// UpdateMediaFile updates a media file's metadata in the database
func (m *MediaDB) UpdateMediaFile(file *MediaFile) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Serialize compliance issues
	complianceJSON, err := json.Marshal(file.ComplianceIssues)
	if err != nil {
		return fmt.Errorf("failed to marshal compliance issues: %w", err)
	}

	query := `
		UPDATE media_files SET
			path = ?,
			size = ?,
			modified_at = ?,
			media_type = ?,
			parent_movie_id = ?,
			parent_series_id = ?,
			parent_episode_id = ?,
			normalized_title = ?,
			year = ?,
			season = ?,
			episode = ?,
			resolution = ?,
			source_type = ?,
			codec = ?,
			audio_format = ?,
			quality_score = ?,
			is_jellyfin_compliant = ?,
			compliance_issues = ?,
			source = ?,
			source_priority = ?,
			library_root = ?,
			confidence = ?,
			parse_method = ?,
			needs_review = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	// Convert bool to int for SQLite
	needsReviewInt := 0
	if file.NeedsReview {
		needsReviewInt = 1
	}

	_, err = m.db.Exec(
		query,
		file.Path, file.Size, file.ModifiedAt,
		file.MediaType, file.ParentMovieID, file.ParentSeriesID, file.ParentEpisodeID,
		file.NormalizedTitle, file.Year, file.Season, file.Episode,
		file.Resolution, file.SourceType, file.Codec, file.AudioFormat, file.QualityScore,
		file.IsJellyfinCompliant, string(complianceJSON),
		file.Source, file.SourcePriority, file.LibraryRoot,
		file.Confidence, file.ParseMethod, needsReviewInt,
		file.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update media file: %w", err)
	}

	return nil
}

// DeleteMediaFile removes a media file from the database
func (m *MediaDB) DeleteMediaFile(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec("DELETE FROM media_files WHERE path = ?", path)
	if err != nil {
		return fmt.Errorf("failed to delete media file: %w", err)
	}

	return nil
}

// DeleteMediaFileByID removes a media file by ID
func (m *MediaDB) DeleteMediaFileByID(id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec("DELETE FROM media_files WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete media file: %w", err)
	}

	return nil
}

// GetMediaFilesByNormalizedKey retrieves all files matching a normalized key
func (m *MediaDB) GetMediaFilesByNormalizedKey(title string, year int, season, episode *int) ([]*MediaFile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var files []*MediaFile

	query := `
		SELECT
			id, path, size, modified_at,
			media_type, parent_movie_id, parent_series_id, parent_episode_id,
			normalized_title, year, season, episode,
			resolution, source_type, codec, audio_format, quality_score,
			confidence, parse_method, needs_review,
			is_jellyfin_compliant, compliance_issues,
			source, source_priority, library_root,
			created_at, updated_at
		FROM media_files
		WHERE normalized_title = ? AND year = ?
	`

	args := []interface{}{title, year}

	if season != nil {
		query += " AND season = ?"
		args = append(args, *season)
	} else {
		query += " AND season IS NULL"
	}

	if episode != nil {
		query += " AND episode = ?"
		args = append(args, *episode)
	} else {
		query += " AND episode IS NULL"
	}

	query += " ORDER BY quality_score DESC"

	rows, err := m.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query media files: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var file MediaFile
		var complianceJSON string
		var needsReviewInt int

		err := rows.Scan(
			&file.ID, &file.Path, &file.Size, &file.ModifiedAt,
			&file.MediaType, &file.ParentMovieID, &file.ParentSeriesID, &file.ParentEpisodeID,
			&file.NormalizedTitle, &file.Year, &file.Season, &file.Episode,
			&file.Resolution, &file.SourceType, &file.Codec, &file.AudioFormat, &file.QualityScore,
			&file.Confidence, &file.ParseMethod, &needsReviewInt,
			&file.IsJellyfinCompliant, &complianceJSON,
			&file.Source, &file.SourcePriority, &file.LibraryRoot,
			&file.CreatedAt, &file.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan media file: %w", err)
		}

		file.NeedsReview = needsReviewInt != 0

		// Deserialize compliance issues
		if complianceJSON != "" {
			if err := json.Unmarshal([]byte(complianceJSON), &file.ComplianceIssues); err != nil {
				return nil, fmt.Errorf("failed to unmarshal compliance issues: %w", err)
			}
		}

		files = append(files, &file)
	}

	return files, rows.Err()
}

// UpsertEpisode inserts or updates an episode record
func (m *MediaDB) UpsertEpisode(episode *Episode) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	query := `
		INSERT INTO episodes (
			series_id, season, episode, title, best_file_id, updated_at
		) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(series_id, season, episode) DO UPDATE SET
			title = excluded.title,
			best_file_id = excluded.best_file_id,
			updated_at = CURRENT_TIMESTAMP
	`

	result, err := m.db.Exec(
		query,
		episode.SeriesID, episode.Season, episode.Episode, episode.Title, episode.BestFileID,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert episode: %w", err)
	}

	// Get ID if this was an insert
	if episode.ID == 0 {
		id, err := result.LastInsertId()
		if err == nil {
			episode.ID = id
		}
	}

	return nil
}

// GetEpisode retrieves an episode by series, season, and episode number
func (m *MediaDB) GetEpisode(seriesID int64, season, episode int) (*Episode, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var ep Episode

	query := `
		SELECT id, series_id, season, episode, title, best_file_id, created_at, updated_at
		FROM episodes
		WHERE series_id = ? AND season = ? AND episode = ?
	`

	err := m.db.QueryRow(query, seriesID, season, episode).Scan(
		&ep.ID, &ep.SeriesID, &ep.Season, &ep.Episode, &ep.Title, &ep.BestFileID,
		&ep.CreatedAt, &ep.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get episode: %w", err)
	}

	return &ep, nil
}

// UpdateMovieBestFile updates the best_file_id for a movie
func (m *MediaDB) UpdateMovieBestFile(movieID int64, fileID *int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec("UPDATE movies SET best_file_id = ? WHERE id = ?", fileID, movieID)
	if err != nil {
		return fmt.Errorf("failed to update movie best file: %w", err)
	}

	return nil
}

// UpdateEpisodeBestFile updates the best_file_id for an episode
func (m *MediaDB) UpdateEpisodeBestFile(episodeID int64, fileID *int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec("UPDATE episodes SET best_file_id = ? WHERE id = ?", fileID, episodeID)
	if err != nil {
		return fmt.Errorf("failed to update episode best file: %w", err)
	}

	return nil
}

// GetLowConfidenceFiles retrieves files with confidence below threshold
func (m *MediaDB) GetLowConfidenceFiles(threshold float64, limit int) ([]*MediaFile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var files []*MediaFile

	query := `
		SELECT
			id, path, size, modified_at,
			media_type, parent_movie_id, parent_series_id, parent_episode_id,
			normalized_title, year, season, episode,
			resolution, source_type, codec, audio_format, quality_score,
			confidence, needs_review,
			is_jellyfin_compliant, compliance_issues,
			source, source_priority, library_root,
			created_at, updated_at
		FROM media_files
		WHERE confidence < ?
		ORDER BY confidence ASC
	`

	args := []interface{}{threshold}
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := m.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query low confidence files: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var file MediaFile
		var complianceJSON string

		err := rows.Scan(
			&file.ID, &file.Path, &file.Size, &file.ModifiedAt,
			&file.MediaType, &file.ParentMovieID, &file.ParentSeriesID, &file.ParentEpisodeID,
			&file.NormalizedTitle, &file.Year, &file.Season, &file.Episode,
			&file.Resolution, &file.SourceType, &file.Codec, &file.AudioFormat, &file.QualityScore,
			&file.Confidence, &file.NeedsReview,
			&file.IsJellyfinCompliant, &complianceJSON,
			&file.Source, &file.SourcePriority, &file.LibraryRoot,
			&file.CreatedAt, &file.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan low confidence file: %w", err)
		}

		if complianceJSON != "" {
			if err := json.Unmarshal([]byte(complianceJSON), &file.ComplianceIssues); err != nil {
				return nil, fmt.Errorf("failed to unmarshal compliance issues: %w", err)
			}
		}

		files = append(files, &file)
	}

	return files, rows.Err()
}
