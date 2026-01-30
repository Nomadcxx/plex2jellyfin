package library

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ShowLocation represents a show found in a library
type ShowLocation struct {
	Library      string // Library path (e.g., "/mnt/STORAGE1/TVSHOWS")
	ShowDir      string // Full show directory path
	EpisodeCount int    // Number of video files found
	Available    int64  // Available space in bytes
}

// findAllShowLocations scans all libraries for directories matching the show name
func (s *Selector) findAllShowLocations(showName, year string) []ShowLocation {
	var locations []ShowLocation
	normalizedTitle := normalizeTitle(showName)

	for _, lib := range s.libraries {
		showDir := s.findShowDirInLibrary(lib, normalizedTitle, year)
		if showDir == "" {
			continue
		}

		episodeCount := countEpisodesInShow(showDir)
		available, _ := getAvailableSpace(lib)

		locations = append(locations, ShowLocation{
			Library:      lib,
			ShowDir:      showDir,
			EpisodeCount: episodeCount,
			Available:    available,
		})
	}

	return locations
}

// findShowDirInLibrary finds a show directory in a single library
func (s *Selector) findShowDirInLibrary(library, normalizedTitle, year string) string {
	entries, err := os.ReadDir(library)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirName := entry.Name()
		normalizedDir := normalizeTitle(dirName)

		// Extract year from directory if present
		dirYear := extractYearFromDir(dirName)

		// If both have years, they MUST match
		if year != "" && dirYear != "" && year != dirYear {
			continue // Different years - not a match
		}

		// Match patterns (now year-safe):
		if normalizedDir == normalizedTitle || normalizedDir == normalizedTitle+year || strings.HasPrefix(normalizedDir, normalizedTitle+"(") {
			return filepath.Join(library, dirName)
		}
	}

	return ""
}

// countEpisodesInShow counts video files in a show directory (recursively)
func countEpisodesInShow(showDir string) int {
	count := 0
	videoExts := map[string]bool{
		".mkv":  true,
		".mp4":  true,
		".avi":  true,
		".m4v":  true,
		".mov":  true,
		".wmv":  true,
		".ts":   true,
		".m2ts": true,
	}

	filepath.Walk(showDir, func(path string, info os.FileInfo, err error) error {
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

// normalizeTitle removes spaces, punctuation, and lowercases for comparison
func normalizeTitle(title string) string {
	// Remove year suffix like "(2024)"
	yearPattern := regexp.MustCompile(`\s*\(\d{4}\)\s*$`)
	title = yearPattern.ReplaceAllString(title, "")

	// Lowercase
	title = strings.ToLower(title)

	// Remove common separators and punctuation
	title = strings.ReplaceAll(title, " ", "")
	title = strings.ReplaceAll(title, ".", "")
	title = strings.ReplaceAll(title, "-", "")
	title = strings.ReplaceAll(title, "_", "")
	title = strings.ReplaceAll(title, "'", "")
	title = strings.ReplaceAll(title, ":", "")
	title = strings.ReplaceAll(title, "&", "")
	title = strings.ReplaceAll(title, "*", "")

	return title
}
