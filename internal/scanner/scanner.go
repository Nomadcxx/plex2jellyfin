package scanner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/naming"
	"github.com/Nomadcxx/jellywatch/internal/quality"
)

// FileScanner scans libraries and populates the media_files database
type FileScanner struct {
	db             *database.MediaDB
	minMovieSize   int64
	minEpisodeSize int64
	skipPatterns   []string
}

// ScanResult contains statistics from a scan operation
type ScanResult struct {
	FilesScanned int
	FilesAdded   int
	FilesUpdated int
	FilesSkipped int
	FilesRemoved int // Files in DB but not on disk
	Duration     time.Duration
	Errors       []error
}

// ScanProgress reports progress during scanning
type ScanProgress struct {
	FilesScanned   int
	CurrentPath    string
	LibrariesDone  int
	LibrariesTotal int
}

// ProgressCallback is called periodically during scanning
type ProgressCallback func(ScanProgress)

// ScanOptions configures the scanning behavior
type ScanOptions struct {
	TVLibraries    []string
	MovieLibraries []string
	OnProgress     ProgressCallback // Optional progress callback
}

// progressReportInterval controls how often progress is reported during scanning
const progressReportInterval = 10

// NewFileScanner creates a new file scanner with default settings
func NewFileScanner(db *database.MediaDB) *FileScanner {
	return &FileScanner{
		db:             db,
		minMovieSize:   quality.MinMovieSize,   // 500MB
		minEpisodeSize: quality.MinEpisodeSize, // 50MB
		skipPatterns: []string{
			"sample", "trailer", "extras", "extra",
			"featurette", "behind the scenes", "deleted scene",
			"interview", "bonus", "cover", "artwork",
			"proof", "rarbg",
		},
	}
}

// ScanLibraries scans multiple libraries (TV and Movie)
func (s *FileScanner) ScanLibraries(ctx context.Context, tvLibs, movieLibs []string) (*ScanResult, error) {
	start := time.Now()
	result := &ScanResult{}

	// Scan TV libraries
	for _, lib := range tvLibs {
		if err := s.scanPath(ctx, lib, "episode", result); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("TV library %s: %w", lib, err))
		}
	}

	// Scan movie libraries
	for _, lib := range movieLibs {
		if err := s.scanPath(ctx, lib, "movie", result); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("Movie library %s: %w", lib, err))
		}
	}

	result.Duration = time.Since(start)
	return result, nil
}

// ScanPath scans a single path (file or directory) and adds files to database
func (s *FileScanner) ScanPath(ctx context.Context, path string, libraryRoot string, mediaType string) (*ScanResult, error) {
	start := time.Now()
	result := &ScanResult{}

	if err := s.scanPath(ctx, libraryRoot, mediaType, result); err != nil {
		return nil, err
	}

	result.Duration = time.Since(start)
	return result, nil
}

// ScanWithOptions scans libraries with configurable options including progress callback
func (s *FileScanner) ScanWithOptions(ctx context.Context, opts ScanOptions) (*ScanResult, error) {
	start := time.Now()
	result := &ScanResult{}

	totalLibs := len(opts.TVLibraries) + len(opts.MovieLibraries)
	libsDone := 0

	progressFn := func(currentPath string, filesScanned int) {
		if opts.OnProgress != nil {
			opts.OnProgress(ScanProgress{
				FilesScanned:   filesScanned,
				CurrentPath:    currentPath,
				LibrariesDone:  libsDone,
				LibrariesTotal: totalLibs,
			})
		}
	}

	// Scan TV libraries
	for _, lib := range opts.TVLibraries {
		// Send initial progress for this library
		if opts.OnProgress != nil {
			opts.OnProgress(ScanProgress{
				FilesScanned:   result.FilesScanned,
				CurrentPath:    lib,
				LibrariesDone:  libsDone,
				LibrariesTotal: totalLibs,
			})
		}
		if err := s.scanPathWithProgress(ctx, lib, "episode", result, progressFn); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("TV library %s: %w", lib, err))
		}
		libsDone++
	}

	// Scan movie libraries
	for _, lib := range opts.MovieLibraries {
		// Send initial progress for this library
		if opts.OnProgress != nil {
			opts.OnProgress(ScanProgress{
				FilesScanned:   result.FilesScanned,
				CurrentPath:    lib,
				LibrariesDone:  libsDone,
				LibrariesTotal: totalLibs,
			})
		}
		if err := s.scanPathWithProgress(ctx, lib, "movie", result, progressFn); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("Movie library %s: %w", lib, err))
		}
		libsDone++
	}

	// Final progress update
	if opts.OnProgress != nil {
		opts.OnProgress(ScanProgress{
			FilesScanned:   result.FilesScanned,
			CurrentPath:    "",
			LibrariesDone:  totalLibs,
			LibrariesTotal: totalLibs,
		})
	}

	result.Duration = time.Since(start)
	return result, nil
}

// scanPath is the internal recursive scanner
func (s *FileScanner) scanPath(ctx context.Context, path string, mediaType string, result *ScanResult) error {
	return filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("walk error %s: %w", filePath, err))
			return nil // Continue walking
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Skip non-video files
		if !isVideoFile(filePath) {
			return nil
		}

		result.FilesScanned++

		// Apply filtering rules
		if !s.shouldIncludeFile(filePath, info.Size(), mediaType) {
			result.FilesSkipped++
			return nil
		}

		// Process the file
		if err := s.processFile(filePath, info, path, mediaType); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("process file %s: %w", filePath, err))
			return nil // Continue walking
		}

		result.FilesAdded++
		return nil
	})
}

// scanPathWithProgress is like scanPath but calls progress callback
func (s *FileScanner) scanPathWithProgress(ctx context.Context, path string, mediaType string, result *ScanResult, progressFn func(string, int)) error {
	return filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("walk error %s: %w", filePath, err))
			return nil
		}

		if info.IsDir() {
			return nil
		}

		if !isVideoFile(filePath) {
			return nil
		}

		result.FilesScanned++

		// Report progress periodically
		if result.FilesScanned%progressReportInterval == 0 {
			progressFn(filePath, result.FilesScanned)
		}

		if !s.shouldIncludeFile(filePath, info.Size(), mediaType) {
			result.FilesSkipped++
			return nil
		}

		if err := s.processFile(filePath, info, path, mediaType); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("process file %s: %w", filePath, err))
			return nil
		}

		result.FilesAdded++
		return nil
	})
}

// processFile extracts metadata and stores file in database
func (s *FileScanner) processFile(filePath string, info os.FileInfo, libraryRoot string, mediaType string) error {
	filename := filepath.Base(filePath)

	// Determine media type if not specified
	if mediaType == "" {
		if naming.IsTVEpisodeFilename(filename) {
			mediaType = "episode"
		} else if naming.IsMovieFilename(filename) {
			mediaType = "movie"
		} else {
			return fmt.Errorf("unable to determine media type")
		}
	}

	// Extract quality metadata
	isEpisode := mediaType == "episode"
	qualityMeta := quality.ExtractMetadata(filePath, info.Size(), isEpisode)

	// Check compliance
	isCompliant, issues := database.CheckCompliance(filePath, libraryRoot)

	// Parse title and episode/movie details
	var normalizedTitle string
	var year *int
	var season, episode *int

	if isEpisode {
		tv, err := naming.ParseTVShowName(filename)
		if err != nil {
			return fmt.Errorf("parse TV show: %w", err)
		}

		normalizedTitle = database.NormalizeTitle(tv.Title)
		if tv.Year != "" {
			if yearInt, err := parseInt(tv.Year); err == nil {
				year = &yearInt
			}
		}
		season = &tv.Season
		episode = &tv.Episode
	} else {
		movie, err := naming.ParseMovieName(filename)
		if err != nil {
			return fmt.Errorf("parse movie: %w", err)
		}

		normalizedTitle = database.NormalizeTitle(movie.Title)
		if movie.Year != "" {
			if yearInt, err := parseInt(movie.Year); err == nil {
				year = &yearInt
			}
		}
	}

	// Create MediaFile record
	file := &database.MediaFile{
		Path:                filePath,
		Size:                info.Size(),
		ModifiedAt:          info.ModTime(),
		MediaType:           mediaType,
		NormalizedTitle:     normalizedTitle,
		Year:                year,
		Season:              season,
		Episode:             episode,
		Resolution:          qualityMeta.Resolution,
		SourceType:          qualityMeta.SourceType,
		Codec:               qualityMeta.Codec,
		AudioFormat:         qualityMeta.AudioFormat,
		QualityScore:        qualityMeta.QualityScore,
		IsJellyfinCompliant: isCompliant,
		ComplianceIssues:    issues,
		Source:              "filesystem",
		SourcePriority:      50,
		LibraryRoot:         libraryRoot,
	}

	// Upsert to database
	return s.db.UpsertMediaFile(file)
}

// shouldIncludeFile determines if a file should be indexed
func (s *FileScanner) shouldIncludeFile(path string, size int64, mediaType string) bool {
	// Check if it's extra content (sample, trailer, etc.)
	if s.isExtraContent(path) {
		return false
	}

	// Apply size thresholds
	if mediaType == "movie" && size < s.minMovieSize {
		return false
	}
	if mediaType == "episode" && size < s.minEpisodeSize {
		return false
	}

	return true
}

// isExtraContent checks if file is a sample, trailer, or other extra
func (s *FileScanner) isExtraContent(path string) bool {
	lowerPath := strings.ToLower(path)

	// Check filename and parent folder
	for _, pattern := range s.skipPatterns {
		if strings.Contains(lowerPath, pattern) {
			return true
		}
	}

	return false
}

// isVideoFile checks if file is a video based on extension
func isVideoFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	videoExts := map[string]bool{
		".mkv":  true,
		".mp4":  true,
		".avi":  true,
		".m4v":  true,
		".mov":  true,
		".wmv":  true,
		".ts":   true,
		".m2ts": true,
		".webm": true,
		".flv":  true,
	}
	return videoExts[ext]
}

// parseInt safely converts string to int
func parseInt(s string) (int, error) {
	var i int
	_, err := fmt.Sscanf(s, "%d", &i)
	return i, err
}
