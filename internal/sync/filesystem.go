package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/naming"
	"github.com/Nomadcxx/plex2jellyfin/internal/scanner"
	"github.com/Nomadcxx/plex2jellyfin/internal/video"
)

const filesystemSourcePriority = 50

// SyncFromFilesystem scans library directories and updates the database
// Returns the scan result including AI statistics
func (s *SyncService) SyncFromFilesystem(ctx context.Context) (result *scanner.ScanResult, retErr error) {
	s.logger.Info("syncing from filesystem")

	logID, err := s.db.StartSyncLog("filesystem")
	if err != nil {
		return nil, err
	}

	// Recover from panics to avoid leaving sync_log stuck in "running"
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("panic in SyncFromFilesystem: %v", r)
			if err := s.db.CompleteSyncLog(logID, "failed", 0, 0, 0, retErr.Error()); err != nil {
				s.logger.Error("sync", "Failed to complete sync log after panic", err)
			}
		}
	}()

	// Create file scanner for file-level scanning (Phase 4: CONDOR system)
	var fileScanner *scanner.FileScanner
	if s.aiHelper != nil {
		fileScanner = scanner.NewFileScannerWithAI(s.db, s.aiHelper)
	} else {
		fileScanner = scanner.NewFileScanner(s.db)
	}

	// Scan files into media_files table
	s.logger.Info("scanning files into media_files table")
	result, err = fileScanner.ScanLibraries(ctx, s.tvLibraries, s.movieLibraries)
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
			if logErr := s.db.CompleteSyncLog(logID, "failed", processed, added, updated, "context cancelled"); logErr != nil {
				s.logger.Error("sync", "Failed to complete sync log", logErr)
			}
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
			if logErr := s.db.CompleteSyncLog(logID, "failed", processed, added, updated, "context cancelled"); logErr != nil {
				s.logger.Error("sync", "Failed to complete sync log", logErr)
			}
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

	if logErr := s.db.CompleteSyncLog(logID, "success", processed, added, updated, ""); logErr != nil {
		s.logger.Error("sync", "Failed to complete sync log", logErr)
	}
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
	year = database.ExtractYearFlexible(dirName)
	if year > 0 {
		title = database.StripYear(dirName)
		title = strings.TrimSpace(title)
		if title == dirName || containsDirectoryReleaseMarker(title) || containsDirectoryReleaseMarker(dirName) {
			if info, err := naming.ParseMovieName(dirName); err == nil {
				parsedYear, _ := strconv.Atoi(info.Year)
				if parsedYear == year && strings.TrimSpace(info.Title) != "" {
					title = strings.TrimSpace(info.Title)
				}
			}
		}
	} else {
		title = dirName
	}
	return
}

func containsDirectoryReleaseMarker(title string) bool {
	upper := strings.ToUpper(title)
	for _, marker := range []string{"HMAX", "REMASTERED", "REMASTER"} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}

// countVideoFiles counts video files recursively in a directory
func countVideoFiles(dir string) int {
	count := 0
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if video.IsVideo(path) {
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
