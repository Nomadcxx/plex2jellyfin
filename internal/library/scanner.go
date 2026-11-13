package library

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/Nomadcxx/plex2jellyfin/internal/mounts"
	"github.com/Nomadcxx/plex2jellyfin/internal/video"
)

// ShowLocation represents a show found in a library
type ShowLocation struct {
	Library      string // Library path (e.g., "/mnt/STORAGE1/TVSHOWS")
	ShowDir      string // Full show directory path
	EpisodeCount int    // Number of video files found
	Available    int64  // Available space in bytes
}

// findAllShowLocations scans all libraries for directories matching the show name.
// An unreadable root may hold the show, so placement proceeds only when the show
// already exists on a readable root and its canonical record does not point at
// an unreadable one. Otherwise a new episode could start a second show folder.
func (s *Selector) findAllShowLocations(showName, year string) ([]ShowLocation, error) {
	var locations []ShowLocation
	var unreadable []string
	var readErrs []error
	normalizedTitle := normalizeTitle(showName)

	for _, lib := range s.libraries {
		showDir, err := s.findShowDirInLibrary(lib, normalizedTitle, year)
		if err != nil {
			unreadable = append(unreadable, lib)
			readErrs = append(readErrs, fmt.Errorf("read TV library root %s: %w", lib, err))
			continue
		}
		if showDir == "" {
			continue
		}

		episodeCount := countEpisodesInShow(showDir)
		available, _ := AvailableSpace(lib)

		locations = append(locations, ShowLocation{
			Library:      lib,
			ShowDir:      showDir,
			EpisodeCount: episodeCount,
			Available:    available,
		})
	}

	// The canonical series folder can be named differently from the release
	// title, e.g. "Lingo (UK) (2021)" for "Lingo UK", so folder matching alone
	// would treat a known show as new.
	if s.db != nil {
		yearInt := 0
		fmt.Sscanf(year, "%d", &yearInt)
		series, err := s.db.GetSeriesByTitle(showName, yearInt)
		if err != nil && len(readErrs) > 0 {
			return nil, errors.Join(append(readErrs, fmt.Errorf("lookup canonical series: %w", err))...)
		}
		if err == nil && series != nil && series.CanonicalPath != "" {
			lib := libraryContainingPath(series.CanonicalPath, s.libraries)
			if slices.Contains(unreadable, lib) {
				return nil, errors.Join(append(readErrs, fmt.Errorf("canonical series path %s is on an unreadable root", series.CanonicalPath))...)
			}
			if lib != "" && !slices.ContainsFunc(locations, func(l ShowLocation) bool { return l.Library == lib }) {
				if info, err := os.Stat(series.CanonicalPath); err == nil && info.IsDir() {
					available, _ := AvailableSpace(lib)
					locations = append(locations, ShowLocation{
						Library:      lib,
						ShowDir:      series.CanonicalPath,
						EpisodeCount: countEpisodesInShow(series.CanonicalPath),
						Available:    available,
					})
				}
			}
		}
	}

	if len(readErrs) > 0 && len(locations) == 0 {
		return nil, errors.Join(readErrs...)
	}
	return locations, nil
}

var yearParenRegex = regexp.MustCompile(`\((\d{4})\)`)

// findShowDirInLibrary finds a show directory in a single library
func (s *Selector) findShowDirInLibrary(library, normalizedTitle, year string) (string, error) {
	if err := mounts.Check(library); err != nil {
		return "", err
	}
	entries, err := os.ReadDir(library)
	if err != nil {
		return "", err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirName := entry.Name()
		normalizedDir := normalizeTitle(dirName)

		// Extract year from directory if present
		dirYear := ""
		if matches := yearParenRegex.FindStringSubmatch(dirName); len(matches) >= 2 {
			dirYear = matches[1]
		}

		// If both have years, they MUST match
		if year != "" && dirYear != "" && year != dirYear {
			continue // Different years - not a match
		}

		// Match patterns (now year-safe):
		if normalizedDir == normalizedTitle || normalizedDir == normalizedTitle+year || strings.HasPrefix(normalizedDir, normalizedTitle+"(") {
			return filepath.Join(library, dirName), nil
		}
	}

	return "", nil
}

// countEpisodesInShow counts video files in a show directory (recursively)
func countEpisodesInShow(showDir string) int {
	count := 0
	filepath.Walk(showDir, func(path string, info os.FileInfo, err error) error {
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
