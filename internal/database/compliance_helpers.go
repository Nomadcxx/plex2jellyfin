package database

import (
	"github.com/Nomadcxx/plex2jellyfin/internal/compliance"
	"github.com/Nomadcxx/plex2jellyfin/internal/naming"
)

// CheckCompliance runs Jellyfin compliance checks on a file path
//
// This is the integration point between the compliance checker and the database.
// It determines media type, runs appropriate validation, and returns results
// in a format ready for database storage.
//
// Parameters:
//   - fullPath: Complete file path including directory structure
//   - libraryRoot: Root path of the library (e.g., "/media/Movies")
//
// Returns:
//   - isCompliant: true if file follows all Jellyfin naming conventions
//   - issues: List of specific compliance violations (empty if compliant)
func CheckCompliance(fullPath string, libraryRoot string) (isCompliant bool, issues []string) {
	checker := compliance.NewChecker(libraryRoot)
	result := checker.CheckFile(fullPath)
	return result.IsCompliant, result.Issues
}

// CheckMovieCompliance runs movie-specific compliance checks
func CheckMovieCompliance(fullPath string, libraryRoot string) (isCompliant bool, issues []string) {
	checker := compliance.NewChecker(libraryRoot)
	result := checker.CheckMovie(fullPath)
	return result.IsCompliant, result.Issues
}

// CheckEpisodeCompliance runs TV episode-specific compliance checks
func CheckEpisodeCompliance(fullPath string, libraryRoot string) (isCompliant bool, issues []string) {
	checker := compliance.NewChecker(libraryRoot)
	result := checker.CheckEpisode(fullPath)
	return result.IsCompliant, result.Issues
}

// NormalizeTitleFromFilename extracts and normalizes title from a filename
//
// This parses the filename to extract the title, then uses the existing
// NormalizeTitle function for consistent database grouping.
//
// For movies: "interstellar"
// For TV shows: "silo"
func NormalizeTitleFromFilename(filename string) string {
	// Try TV show first
	if naming.IsTVEpisodeFilename(filename) {
		tv, err := naming.ParseTVShowName(filename)
		if err == nil {
			// Use existing NormalizeTitle for consistent format
			return NormalizeTitle(tv.Title)
		}
	}

	// Try movie
	if naming.IsMovieFilename(filename) {
		movie, err := naming.ParseMovieName(filename)
		if err == nil {
			// Use existing NormalizeTitle for consistent format
			return NormalizeTitle(movie.Title)
		}
	}

	// Fallback to raw normalization
	return NormalizeTitle(filename)
}
