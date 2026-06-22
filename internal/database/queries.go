package database

import (
	"encoding/json"
	"fmt"
)

// DuplicateGroup represents a group of duplicate files
type DuplicateGroup struct {
	NormalizedTitle  string
	Year             *int
	Season           *int // nil for movies
	Episode          *int // nil for movies
	Files            []*MediaFile
	BestFile         *MediaFile
	SpaceReclaimable int64
}

// ConsolidationStats provides summary statistics
type ConsolidationStats struct {
	TotalFiles          int
	TotalSize           int64
	DuplicateGroups     int
	DuplicateFiles      int
	SpaceReclaimable    int64
	NonCompliantFiles   int
	NonCompliantFolders int
}

// FindDuplicateMovies returns movies with multiple files
func (m *MediaDB) FindDuplicateMovies() ([]DuplicateGroup, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `
		SELECT normalized_title, year
		FROM media_files
		WHERE media_type = 'movie'
		GROUP BY normalized_title, year
		HAVING COUNT(*) > 1
		ORDER BY normalized_title, year
	`

	rows, err := m.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to find duplicate movies: %w", err)
	}
	defer rows.Close()

	var groups []DuplicateGroup

	for rows.Next() {
		var title string
		var year *int

		if err := rows.Scan(&title, &year); err != nil {
			return nil, fmt.Errorf("failed to scan duplicate movie: %w", err)
		}

		// Get all files for this movie
		var yearVal int
		if year != nil {
			yearVal = *year
		}
		files, err := m.getMediaFilesForGroup(title, yearVal, nil, nil)
		if err != nil {
			return nil, err
		}

		group := DuplicateGroup{
			NormalizedTitle: title,
			Year:            year,
			Files:           files,
		}

		// Find best file (highest quality score)
		if len(files) > 0 {
			group.BestFile = files[0] // Already sorted by quality_score DESC

			// Calculate space reclaimable
			for _, f := range files[1:] {
				group.SpaceReclaimable += f.Size
			}
		}

		groups = append(groups, group)
	}

	return groups, rows.Err()
}

// FindDuplicateEpisodes returns episodes with multiple files
func (m *MediaDB) FindDuplicateEpisodes() ([]DuplicateGroup, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `
		SELECT normalized_title, year, season, episode
		FROM media_files
		WHERE media_type = 'episode' AND season IS NOT NULL AND episode IS NOT NULL
		GROUP BY normalized_title, year, season, episode
		HAVING COUNT(*) > 1
		ORDER BY normalized_title, year, season, episode
	`

	rows, err := m.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to find duplicate episodes: %w", err)
	}
	defer rows.Close()

	var groups []DuplicateGroup

	for rows.Next() {
		var title string
		var year, season, episode *int

		if err := rows.Scan(&title, &year, &season, &episode); err != nil {
			return nil, fmt.Errorf("failed to scan duplicate episode: %w", err)
		}

		// Get all files for this episode
		var yearVal int
		if year != nil {
			yearVal = *year
		}
		files, err := m.getMediaFilesForGroup(title, yearVal, season, episode)
		if err != nil {
			return nil, err
		}

		group := DuplicateGroup{
			NormalizedTitle: title,
			Year:            year,
			Season:          season,
			Episode:         episode,
			Files:           files,
		}

		// Find best file (highest quality score)
		if len(files) > 0 {
			group.BestFile = files[0] // Already sorted by quality_score DESC

			// Calculate space reclaimable
			for _, f := range files[1:] {
				group.SpaceReclaimable += f.Size
			}
		}

		groups = append(groups, group)
	}

	return groups, rows.Err()
}

// getMediaFilesForGroup retrieves all files for a duplicate group
func (m *MediaDB) getMediaFilesForGroup(title string, year int, season, episode *int) ([]*MediaFile, error) {
	var files []*MediaFile

	query := `
		SELECT
			id, path, size, modified_at,
			media_type, parent_movie_id, parent_series_id, parent_episode_id,
			normalized_title, year, season, episode,
			resolution, source_type, codec, audio_format, quality_score,
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

	query += " ORDER BY quality_score DESC, size DESC, id ASC"

	rows, err := m.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query media files: %w", err)
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
			&file.IsJellyfinCompliant, &complianceJSON,
			&file.Source, &file.SourcePriority, &file.LibraryRoot,
			&file.CreatedAt, &file.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan media file: %w", err)
		}

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

// FindNonCompliantFiles returns all files that don't follow Jellyfin naming
func (m *MediaDB) FindNonCompliantFiles() ([]*MediaFile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var files []*MediaFile

	query := `
		SELECT
			id, path, size, modified_at,
			media_type, parent_movie_id, parent_series_id, parent_episode_id,
			normalized_title, year, season, episode,
			resolution, source_type, codec, audio_format, quality_score,
			is_jellyfin_compliant, compliance_issues,
			source, source_priority, library_root,
			created_at, updated_at
		FROM media_files
		WHERE is_jellyfin_compliant = 0
		ORDER BY media_type, normalized_title, year, season, episode
	`

	rows, err := m.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query non-compliant files: %w", err)
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
			&file.IsJellyfinCompliant, &complianceJSON,
			&file.Source, &file.SourcePriority, &file.LibraryRoot,
			&file.CreatedAt, &file.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan media file: %w", err)
		}

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

// FindInferiorDuplicates returns files that should be deleted (lower quality than best)
func (m *MediaDB) FindInferiorDuplicates() ([]*MediaFile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Get all duplicate groups
	movieGroups, err := m.FindDuplicateMovies()
	if err != nil {
		return nil, err
	}

	episodeGroups, err := m.FindDuplicateEpisodes()
	if err != nil {
		return nil, err
	}

	var inferiorFiles []*MediaFile

	// Collect all non-best files from movie groups
	for _, group := range movieGroups {
		if len(group.Files) > 1 {
			inferiorFiles = append(inferiorFiles, group.Files[1:]...)
		}
	}

	// Collect all non-best files from episode groups
	for _, group := range episodeGroups {
		if len(group.Files) > 1 {
			inferiorFiles = append(inferiorFiles, group.Files[1:]...)
		}
	}

	return inferiorFiles, nil
}

// GetConsolidationStats returns summary statistics
func (m *MediaDB) GetConsolidationStats() (*ConsolidationStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := &ConsolidationStats{}

	// Total files and size
	err := m.db.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(size), 0)
		FROM media_files
	`).Scan(&stats.TotalFiles, &stats.TotalSize)
	if err != nil {
		return nil, fmt.Errorf("failed to get total stats: %w", err)
	}

	// Non-compliant files
	err = m.db.QueryRow(`
		SELECT COUNT(*)
		FROM media_files
		WHERE is_jellyfin_compliant = 0
	`).Scan(&stats.NonCompliantFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to get non-compliant count: %w", err)
	}

	// Get duplicate stats
	movieGroups, err := m.FindDuplicateMovies()
	if err != nil {
		return nil, err
	}

	episodeGroups, err := m.FindDuplicateEpisodes()
	if err != nil {
		return nil, err
	}

	stats.DuplicateGroups = len(movieGroups) + len(episodeGroups)

	// Count total duplicate files and space reclaimable
	for _, group := range movieGroups {
		stats.DuplicateFiles += len(group.Files)
		stats.SpaceReclaimable += group.SpaceReclaimable
	}

	for _, group := range episodeGroups {
		stats.DuplicateFiles += len(group.Files)
		stats.SpaceReclaimable += group.SpaceReclaimable
	}

	return stats, nil
}

// CountMediaFiles returns the count of media files
func (m *MediaDB) CountMediaFiles() (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var count int
	err := m.db.QueryRow("SELECT COUNT(*) FROM media_files").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count media files: %w", err)
	}

	return count, nil
}

// CountMediaFilesByType returns the count of media files by type
func (m *MediaDB) CountMediaFilesByType(mediaType string) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var count int
	err := m.db.QueryRow("SELECT COUNT(*) FROM media_files WHERE media_type = ?", mediaType).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count media files by type: %w", err)
	}

	return count, nil
}

// GetMediaFilesByLibrary returns all files in a specific library root
func (m *MediaDB) GetMediaFilesByLibrary(libraryRoot string) ([]*MediaFile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var files []*MediaFile

	query := `
		SELECT
			id, path, size, modified_at,
			media_type, parent_movie_id, parent_series_id, parent_episode_id,
			normalized_title, year, season, episode,
			resolution, source_type, codec, audio_format, quality_score,
			is_jellyfin_compliant, compliance_issues,
			source, source_priority, library_root,
			created_at, updated_at
		FROM media_files
		WHERE library_root = ?
		ORDER BY path
	`

	rows, err := m.db.Query(query, libraryRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to query media files by library: %w", err)
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
			&file.IsJellyfinCompliant, &complianceJSON,
			&file.Source, &file.SourcePriority, &file.LibraryRoot,
			&file.CreatedAt, &file.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan media file: %w", err)
		}

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
