package scanner

import (
	"context"
	"fmt"
	"log"
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
	aiHelper       *AIHelper // Optional AI helper
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

	// AI stats
	AITriggered int // Files where AI was called
	AICacheHits int // Files served from cache
	AISucceeded int // AI calls that improved confidence
	AIFailed    int // AI calls that failed/timed out
	NeedsReview int // Files flagged for audit
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

// NewFileScannerWithAI creates a file scanner with AI support
func NewFileScannerWithAI(db *database.MediaDB, aiHelper *AIHelper) *FileScanner {
	scanner := NewFileScanner(db)
	scanner.aiHelper = aiHelper
	return scanner
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
		if err := s.processFile(filePath, info, path, mediaType, result); err != nil {
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

		if err := s.processFile(filePath, info, path, mediaType, result); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("process file %s: %w", filePath, err))
			return nil
		}

		result.FilesAdded++
		return nil
	})
}

// processFile extracts metadata and stores file in database
func (s *FileScanner) processFile(filePath string, info os.FileInfo, libraryRoot string, mediaType string, result *ScanResult) error {
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

	isEpisode := mediaType == "episode"

	// Extract quality metadata
	qualityMeta := quality.ExtractMetadata(filePath, info.Size(), isEpisode)

	// Check compliance
	isCompliant, issues := database.CheckCompliance(filePath, libraryRoot)

	// === STEP 1: REGEX PARSE ===
	var normalizedTitle string
	var year *int
	var season, episode *int
	parseMethod := "regex"

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

	bestConfidence := naming.CalculateTitleConfidence(normalizedTitle, filename)

	// === STEP 2: DE-OBFUSCATION (if obfuscated filename) ===
	if naming.IsObfuscatedFilename(filename) {
		if isEpisode {
			if tvInfo, err := naming.ParseTVShowFromPath(filePath); err == nil {
				folderTitle := database.NormalizeTitle(tvInfo.Title)
				folderConfidence := naming.CalculateTitleConfidence(folderTitle, filepath.Base(filepath.Dir(filePath)))
				if folderConfidence > bestConfidence {
					normalizedTitle = folderTitle
					if tvInfo.Year != "" {
						if yearInt, err := parseInt(tvInfo.Year); err == nil {
							year = &yearInt
						}
					}
					season = &tvInfo.Season
					episode = &tvInfo.Episode
					bestConfidence = folderConfidence
					parseMethod = "folder"
				}
			}
		} else {
			if movieInfo, err := naming.ParseMovieFromPath(filePath); err == nil {
				folderTitle := database.NormalizeTitle(movieInfo.Title)
				folderConfidence := naming.CalculateTitleConfidence(folderTitle, filepath.Base(filepath.Dir(filePath)))
				if folderConfidence > bestConfidence {
					normalizedTitle = folderTitle
					if movieInfo.Year != "" {
						if yearInt, err := parseInt(movieInfo.Year); err == nil {
							year = &yearInt
						}
					}
					bestConfidence = folderConfidence
					parseMethod = "folder"
				}
			}
		}
	}

	// === STEP 3: AI TRIGGER (if low confidence + AI enabled) ===
	if s.aiHelper != nil && s.aiHelper.IsEnabled() {
		if bestConfidence < s.aiHelper.GetAutoTriggerThreshold() {
			ctx := context.Background()
			aiResult, fromCache, err := s.aiHelper.TryParse(ctx, filename, mediaType)

			if fromCache {
				result.AICacheHits++
				log.Printf("[AI] Cache hit: %s", filename)
			}

			if err == nil && aiResult != nil {
				result.AITriggered++
				log.Printf("[AI] Parsing: %s", filename)

				// Check if AI result is better
				if aiResult.Confidence >= s.aiHelper.GetConfidenceThreshold() {
					// AI is confident - use it
					normalizedTitle = database.NormalizeTitle(aiResult.Title)
					year = aiResult.Year
					if isEpisode && aiResult.Season != nil {
						season = aiResult.Season
						if len(aiResult.Episodes) > 0 {
							ep := aiResult.Episodes[0]
							episode = &ep
						}
					}
					bestConfidence = aiResult.Confidence
					parseMethod = "ai"
					result.AISucceeded++
				} else if aiResult.Confidence > bestConfidence {
					// AI not confident but still better than regex
					normalizedTitle = database.NormalizeTitle(aiResult.Title)
					year = aiResult.Year
					if isEpisode && aiResult.Season != nil {
						season = aiResult.Season
						if len(aiResult.Episodes) > 0 {
							ep := aiResult.Episodes[0]
							episode = &ep
						}
					}
					bestConfidence = aiResult.Confidence
					parseMethod = "ai"
					result.AISucceeded++
				}
			} else if err != nil {
				result.AIFailed++
			}
		}
	}

	// === STEP 4: SAVE TO DATABASE ===
	needsReview := bestConfidence < 0.8
	if needsReview {
		result.NeedsReview++
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
		Confidence:          bestConfidence,
		ParseMethod:         parseMethod,
		NeedsReview:         needsReview,
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
