package sync

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/scanner"
)

const filesystemSourcePriority = 50

// SyncFromFilesystem scans library directories and updates the database
// Returns the scan result including AI statistics
func (s *SyncService) SyncFromFilesystem(ctx context.Context) (*scanner.ScanResult, error) {
	s.logger.Info("syncing from filesystem")

	logID, err := s.db.StartSyncLog("filesystem")
	if err != nil {
		return nil, err
	}

	// Create file scanner for file-level scanning (Phase 4: CONDOR system)
	var fileScanner *scanner.FileScanner
	if s.aiHelper != nil {
		fileScanner = scanner.NewFileScannerWithAI(s.db, s.aiHelper)
	} else {
		fileScanner = scanner.NewFileScanner(s.db)
	}

	// Scan files into media_files table
	s.logger.Info("scanning files into media_files table")
	result, err := fileScanner.ScanLibraries(ctx, s.tvLibraries, s.movieLibraries)
	if err != nil {
		s.logger.Warn("file scanner completed with errors", "errors", len(result.Errors))
		for _, scanErr := range result.Errors {
			s.logger.Debug("scan error", "error", scanErr)
		}
	}

	s.logger.Info("file scan completed",
		"scanned", result.FilesScanned,
		"added", result.FilesAdded,
		"skipped", result.FilesSkipped,
		"duration", result.Duration)

	// Also maintain backward compatibility: populate series/movies tables
	// These tables track show/movie folders (not individual files)
	var processed, added, updated int

	// Scan TV libraries (folder-level, for backward compat)
	for _, lib := range s.tvLibraries {
		select {
		case <-ctx.Done():
			s.db.CompleteSyncLog(logID, "failed", processed, added, updated, "context cancelled")
			return result, ctx.Err()
		default:
		}

		p, a, u, err := s.scanTVLibrary(ctx, lib)
		if err != nil {
			s.logger.Warn("failed to scan TV library", "path", lib, "error", err)
			continue
		}
		processed += p
		added += a
		updated += u
	}

	// Scan movie libraries (folder-level, for backward compat)
	for _, lib := range s.movieLibraries {
		select {
		case <-ctx.Done():
			s.db.CompleteSyncLog(logID, "failed", processed, added, updated, "context cancelled")
			return result, ctx.Err()
		default:
		}

		p, a, u, err := s.scanMovieLibrary(ctx, lib)
		if err != nil {
			s.logger.Warn("failed to scan movie library", "path", lib, "error", err)
			continue
		}
		processed += p
		added += a
		updated += u
	}

	s.db.CompleteSyncLog(logID, "success", processed, added, updated, "")
	s.logger.Info("filesystem sync completed", "processed", processed, "added", added, "updated", updated)

	return result, nil
}

func (s *SyncService) scanTVLibrary(ctx context.Context, libraryPath string) (processed, added, updated int, err error) {
	entries, err := os.ReadDir(libraryPath)
	if err != nil {
		return 0, 0, 0, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		select {
		case <-ctx.Done():
			return processed, added, updated, ctx.Err()
		default:
		}

		processed++

		dirName := entry.Name()
		showPath := filepath.Join(libraryPath, dirName)

		// Extract title and year from directory name
		title, year := parseTVShowDir(dirName)

		// Count episodes
		episodeCount := countVideoFiles(showPath)

		// Skip empty directories
		if episodeCount == 0 {
			continue
		}

		series := &database.Series{
			Title:          title,
			Year:           year,
			CanonicalPath:  showPath,
			LibraryRoot:    libraryPath,
			Source:         "filesystem",
			SourcePriority: filesystemSourcePriority,
			EpisodeCount:   episodeCount,
		}

		// Check if this is new or updated
		existing, _ := s.db.GetSeriesByTitle(title, year)
		isNew := (existing == nil)

		_, err := s.db.UpsertSeries(series)
		if err != nil {
			s.logger.Warn("failed to upsert series from filesystem",
				"path", showPath, "error", err)
			continue
		}

		if isNew {
			added++
			s.logger.Debug("added series from filesystem", "title", title, "year", year, "episodes", episodeCount)
		} else {
			updated++
		}
	}

	return processed, added, updated, nil
}

func (s *SyncService) scanMovieLibrary(ctx context.Context, libraryPath string) (processed, added, updated int, err error) {
	entries, err := os.ReadDir(libraryPath)
	if err != nil {
		return 0, 0, 0, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		select {
		case <-ctx.Done():
			return processed, added, updated, ctx.Err()
		default:
		}

		processed++

		dirName := entry.Name()
		moviePath := filepath.Join(libraryPath, dirName)

		// Extract title and year
		title, year := parseMovieDir(dirName)

		// Check if directory has video files
		hasVideo := hasVideoFiles(moviePath)
		if !hasVideo {
			continue
		}

		movie := &database.Movie{
			Title:          title,
			Year:           year,
			CanonicalPath:  moviePath,
			LibraryRoot:    libraryPath,
			Source:         "filesystem",
			SourcePriority: filesystemSourcePriority,
		}

		// Check if this is new or updated
		existing, _ := s.db.GetMovieByTitle(title, year)
		isNew := (existing == nil)

		_, err := s.db.UpsertMovie(movie)
		if err != nil {
			s.logger.Warn("failed to upsert movie from filesystem",
				"path", moviePath, "error", err)
			continue
		}

		if isNew {
			added++
			s.logger.Debug("added movie from filesystem", "title", title, "year", year)
		} else {
			updated++
		}
	}

	return processed, added, updated, nil
}

// parseTVShowDir extracts title and year from a directory name
// "For All Mankind (2019)" -> "For All Mankind", 2019
// "Fallout" -> "Fallout", 0
func parseTVShowDir(dirName string) (title string, year int) {
	title, year = parseMediaDir(dirName)
	return
}

// parseMovieDir extracts title and year from a movie directory name
func parseMovieDir(dirName string) (title string, year int) {
	title, year = parseMediaDir(dirName)
	return
}

// parseMediaDir is a helper that extracts title and year from any media directory
func parseMediaDir(dirName string) (title string, year int) {
	year = database.ExtractYear(dirName)
	if year > 0 {
		title = database.StripYear(dirName)
		title = strings.TrimSpace(title)
	} else {
		title = dirName
	}
	return
}

// countVideoFiles counts video files recursively in a directory
func countVideoFiles(dir string) int {
	count := 0
	videoExts := map[string]bool{
		".mkv": true, ".mp4": true, ".avi": true,
		".m4v": true, ".mov": true, ".wmv": true,
		".ts": true, ".m2ts": true,
	}

	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if videoExts[ext] {
			count++
		}
		return nil
	})

	return count
}

// hasVideoFiles checks if a directory has at least one video file
func hasVideoFiles(dir string) bool {
	return countVideoFiles(dir) > 0
}
