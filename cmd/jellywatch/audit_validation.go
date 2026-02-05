package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/ai"
	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/radarr"
	"github.com/Nomadcxx/jellywatch/internal/sonarr"
)

// inferTypeFromLibraryRoot analyzes library path to infer expected media type.
// Returns "episode" for TV paths, "movie" for movie paths, "unknown" otherwise.
func inferTypeFromLibraryRoot(libraryRoot string) string {
	if libraryRoot == "" {
		return "unknown"
	}

	lower := strings.ToLower(libraryRoot)

	// Check for TV indicators (check these first as they're more specific)
	tvKeywords := []string{"tvshow", "tv show", "tv_show", "tv-show", "tvseries",
		"tv series", "tv_series", "tv-series", "/tv/", "/tv", "shows", "series"}
	for _, keyword := range tvKeywords {
		if strings.Contains(lower, keyword) {
			return "episode"
		}
	}

	// Check for movie indicators
	movieKeywords := []string{"movie", "film", "/mv/", "/mv"}
	for _, keyword := range movieKeywords {
		if strings.Contains(lower, keyword) {
			return "movie"
		}
	}

	return "unknown"
}

// mapAITypeToMediaType converts AI Result.Type ("tv", "movie") to MediaFile.MediaType ("episode", "movie")
func mapAITypeToMediaType(aiType string) string {
	switch aiType {
	case "tv":
		return "episode"
	case "movie":
		return "movie"
	default:
		return "unknown"
	}
}

// validateMediaType checks if AI-suggested type matches library context.
// Returns (valid, reason) where reason explains why validation failed.
//
// Validation layers:
// 1. Primary: library_root path analysis (if path indicates TV or Movies)
// 2. Secondary: Sonarr/Radarr API lookup (if APIs are configured and enabled)
//
// If library_root and API disagree, returns invalid with explanation.
func validateMediaType(file *database.MediaFile, aiResult *ai.Result, cfg *config.Config) (valid bool, reason string) {
	// Map AI type to media type for comparison
	aiMediaType := mapAITypeToMediaType(aiResult.Type)

	// Primary validation: Check library_root path
	libraryType := inferTypeFromLibraryRoot(file.LibraryRoot)

	if libraryType != "unknown" {
		// Library path clearly indicates type - validate against it
		if libraryType != aiMediaType {
			return false, fmt.Sprintf("AI suggests %s but file is in %s library", aiMediaType, libraryType)
		}
		// Library type matches AI type - valid
		return true, ""
	}

	// Secondary validation: Check Sonarr/Radarr APIs if configured
	sonarrMatch := false
	radarrMatch := false

	// Check Sonarr for TV series match
	if cfg.Sonarr.Enabled && cfg.Sonarr.URL != "" && cfg.Sonarr.APIKey != "" {
		sonarrClient := sonarr.NewClient(sonarr.Config{
			URL:     cfg.Sonarr.URL,
			APIKey:  cfg.Sonarr.APIKey,
			Timeout: 10 * time.Second,
		})

		series, err := sonarrClient.FindSeriesByTitle(aiResult.Title)
		if err == nil && len(series) > 0 {
			sonarrMatch = true
		}
		// Ignore API errors - fall through to trust AI
	}

	// Check Radarr for movie match
	if cfg.Radarr.Enabled && cfg.Radarr.URL != "" && cfg.Radarr.APIKey != "" {
		radarrClient := radarr.NewClient(radarr.Config{
			URL:     cfg.Radarr.URL,
			APIKey:  cfg.Radarr.APIKey,
			Timeout: 10 * time.Second,
		})

		movies, err := radarrClient.LookupMovie(aiResult.Title)
		if err == nil && len(movies) > 0 {
			titleLower := strings.ToLower(aiResult.Title)
			for _, m := range movies {
				if strings.ToLower(m.Title) == titleLower {
					radarrMatch = true
					break
				}
			}
		}
		// Ignore API errors - fall through to trust AI
	}

	// Conflict resolution: both APIs found matches
	if sonarrMatch && radarrMatch {
		return false, fmt.Sprintf("Ambiguous: '%s' found in both Sonarr (TV) and Radarr (Movies)", aiResult.Title)
	}

	// API confirms type mismatch
	if sonarrMatch && aiMediaType != "episode" {
		return false, fmt.Sprintf("Sonarr confirms '%s' is a TV series, but AI suggests movie", aiResult.Title)
	}
	if radarrMatch && aiMediaType != "movie" {
		return false, fmt.Sprintf("Radarr confirms '%s' is a movie, but AI suggests episode", aiResult.Title)
	}

	// No contradiction found - trust AI
	return true, ""
}

// ProgressBar tracks and displays audit generation progress
type ProgressBar struct {
	total          int
	current        int
	actionsCreated int
	errorsCount    int
	updateInterval int
}

// NewProgressBar creates a new progress bar for audit generation
func NewProgressBar(total int) *ProgressBar {
	return &ProgressBar{
		total:          total,
		current:        0,
		actionsCreated: 0,
		errorsCount:    0,
		updateInterval: 10,
	}
}

// Update increments progress counter and optionally updates display
func (p *ProgressBar) Update(actionsCreated, errorsCount int) {
	p.current++
	p.actionsCreated = actionsCreated
	p.errorsCount = errorsCount

	if p.current%p.updateInterval == 0 || p.current == p.total {
		p.render()
	}
}

// render draws the progress bar to stdout
func (p *ProgressBar) render() {
	if p.total == 0 {
		return
	}

	percentage := float64(p.current) / float64(p.total) * 100
	barWidth := 20
	filled := int(float64(barWidth) * float64(p.current) / float64(p.total))

	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	fmt.Printf("\r%s %3.0f%% (%d/%d files) | %d actions | %d errors",
		bar, percentage, p.current, p.total, p.actionsCreated, p.errorsCount)
}

// Finish completes the progress bar and moves to next line
func (p *ProgressBar) Finish() {
	p.render()
	fmt.Println()
}

// AuditStats tracks statistics during audit generation
type AuditStats struct {
	AITotalCalls     int
	AISuccessCount   int
	AIErrorCount     int
	TypeMismatches   int
	ConfidenceTooLow int
	TitleUnchanged   int
	ActionsCreated   int
}

// NewAuditStats creates a new statistics tracker
func NewAuditStats() *AuditStats {
	return &AuditStats{}
}

// RecordAICall records an AI call result
func (s *AuditStats) RecordAICall(success bool) {
	s.AITotalCalls++
	if success {
		s.AISuccessCount++
	} else {
		s.AIErrorCount++
	}
}

// RecordSkip records a skipped file with reason
func (s *AuditStats) RecordSkip(reason string) {
	switch {
	case strings.Contains(reason, "type") || strings.Contains(reason, "library"):
		s.TypeMismatches++
	case strings.Contains(reason, "confidence"):
		s.ConfidenceTooLow++
	case strings.Contains(reason, "unchanged") || strings.Contains(reason, "same"):
		s.TitleUnchanged++
	}
}

// RecordAction records a created action
func (s *AuditStats) RecordAction() {
	s.ActionsCreated++
}

// ToSummary converts stats to AuditSummary fields
func (s *AuditStats) ToSummary(totalFiles int) (aiTotal, aiSuccess, aiError, typeMismatch, confLow, titleUnch int) {
	return s.AITotalCalls, s.AISuccessCount, s.AIErrorCount,
		s.TypeMismatches, s.ConfidenceTooLow, s.TitleUnchanged
}

// normalizeForComparison normalizes filenames for comparison
// to detect trivial differences (hyphen vs space, etc.)
func normalizeForComparison(filename string) string {
	// Remove extension
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)

	// Normalize separators
	base = strings.ReplaceAll(base, " - ", " ")
	base = strings.ReplaceAll(base, ".", " ")
	base = strings.ReplaceAll(base, "_", " ")

	// Collapse spaces
	base = strings.Join(strings.Fields(base), " ")

	// Lowercase for comparison
	return strings.ToLower(base) + strings.ToLower(ext)
}
