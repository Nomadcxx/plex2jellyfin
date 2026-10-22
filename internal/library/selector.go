package library

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/sonarr"
	"github.com/Nomadcxx/plex2jellyfin/internal/video"
)

type Selector struct {
	libraries    []string
	sonarrClient *sonarr.Client
	cache        *SeriesCache
	db           *database.MediaDB // HOLDEN: Database for fast lookups
	mu           sync.RWMutex
}

// SelectorConfig contains configuration for creating a Selector
type SelectorConfig struct {
	Libraries     []string
	SonarrClient  *sonarr.Client
	CacheDuration time.Duration     // Default: 5 minutes (deprecated - use DB)
	DB            *database.MediaDB // HOLDEN: Database for fast lookups
}

// NewSelector creates a new library selector with optional Sonarr integration
func NewSelector(libraries []string) *Selector {
	return NewSelectorWithConfig(SelectorConfig{
		Libraries: libraries,
	})
}

// NewSelectorWithConfig creates a selector with full configuration options
func NewSelectorWithConfig(cfg SelectorConfig) *Selector {
	s := &Selector{
		libraries:    cfg.Libraries,
		sonarrClient: cfg.SonarrClient,
		db:           cfg.DB,
	}

	if cfg.CacheDuration == 0 {
		cfg.CacheDuration = 5 * time.Minute
	}

	// Keep cache for backwards compatibility, but DB takes precedence
	if cfg.SonarrClient != nil {
		s.cache = NewSeriesCache(cfg.SonarrClient, cfg.CacheDuration)
	}

	return s
}

type SelectionResult struct {
	Library   string
	Reason    string
	Available int64
}

func (s *Selector) SelectMovieLibrary(movieTitle string, year string, fileSize int64) (*SelectionResult, error) {
	if len(s.libraries) == 0 {
		return nil, fmt.Errorf("no libraries configured")
	}

	// A known movie stays on its canonical library. Sending a replacement to
	// another root when that disk is full would create a second library copy.
	if s.db != nil && year != "" {
		yearInt := 0
		fmt.Sscanf(year, "%d", &yearInt)

		movie, err := s.db.GetMovieByTitle(movieTitle, yearInt)
		if err != nil {
			return nil, fmt.Errorf("lookup canonical movie: %w", err)
		}
		if movie != nil {
			placementPath, placementKind := movie.CanonicalPath, "canonical path"
			if placementPath == "" {
				placementPath, placementKind = movie.LibraryRoot, "library root"
			}
			if placementPath == "" {
				return nil, fmt.Errorf("known movie %q (%d) has no canonical path or library root", movie.Title, movie.Year)
			}

			canonicalLibrary := libraryContainingPath(placementPath, s.libraries)
			if canonicalLibrary != "" {
				available, err := getAvailableSpace(canonicalLibrary)
				if err != nil {
					return nil, fmt.Errorf("movie exists in canonical library %s but free space is unavailable: %w", canonicalLibrary, err)
				}
				if available < fileSize {
					return nil, fmt.Errorf("insufficient space in canonical movie library %s: need %d, have %d", canonicalLibrary, fileSize, available)
				}
				sourceDesc := fmt.Sprintf("Database %s (%s): %s", placementKind, movie.Source, placementPath)
				return &SelectionResult{
					Library:   canonicalLibrary,
					Reason:    sourceDesc,
					Available: available,
				}, nil
			}
			return nil, fmt.Errorf("movie %s %s is outside configured movie libraries", placementKind, placementPath)
		}
	}

	if len(s.libraries) == 1 {
		lib := s.libraries[0]
		available, err := getAvailableSpace(lib)
		if err != nil {
			return nil, err
		}
		if available < fileSize {
			return nil, fmt.Errorf("insufficient space in %s: need %d, have %d", lib, fileSize, available)
		}
		return &SelectionResult{
			Library:   lib,
			Reason:    "Only library available",
			Available: available,
		}, nil
	}

	var candidates []SelectionResult
	for _, lib := range s.libraries {
		available, err := getAvailableSpace(lib)
		if err != nil || available < fileSize {
			continue
		}

		hasExisting, franchiseCount := s.findRelatedContent(lib, movieTitle, year, true)
		var reasons []string
		if hasExisting {
			reasons = append(reasons, fmt.Sprintf("Contains %d related items", franchiseCount))
		}

		spaceInGB := available / (1024 * 1024 * 1024)
		reasons = append(reasons, fmt.Sprintf("%d GB available", spaceInGB))

		candidates = append(candidates, SelectionResult{
			Library:   lib,
			Available: available,
			Reason:    strings.Join(reasons, ", "),
		})
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("insufficient space: no suitable movie library has room for %d bytes", fileSize)
	}

	var best *SelectionResult
	for i := range candidates {
		if best == nil || s.scoreLibrary(candidates[i].Library, movieTitle, year, fileSize, true) >
			s.scoreLibrary(best.Library, movieTitle, year, fileSize, true) {
			best = &candidates[i]
		}
	}

	return best, nil
}

func libraryContainingPath(path string, libraries []string) string {
	if path == "" {
		return ""
	}

	var containingLibrary string
	for _, library := range libraries {
		rel, err := filepath.Rel(filepath.Clean(library), filepath.Clean(path))
		inside := err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
		if inside && (containingLibrary == "" || len(filepath.Clean(library)) > len(filepath.Clean(containingLibrary))) {
			containingLibrary = library
		}
	}
	return containingLibrary
}

func (s *Selector) SelectTVShowLibrary(showName string, year string, fileSize int64) (*SelectionResult, error) {
	if len(s.libraries) == 0 {
		return nil, fmt.Errorf("no libraries configured")
	}

	if len(s.libraries) == 1 {
		return s.selectSingleLibrary(s.libraries[0], fileSize)
	}

	// STEP 1: Find ALL libraries that have this show
	matches, err := s.findAllShowLocations(showName, year)
	if err != nil {
		return nil, err
	}

	// STEP 2: If no matches, this is a new show
	if len(matches) == 0 {
		return s.selectForNewShow(showName, year, fileSize)
	}

	// STEP 3: If single match, use it
	if len(matches) == 1 {
		if !s.hasSpace(matches[0].Library, fileSize) {
			return nil, fmt.Errorf("show exists in %s but insufficient space", matches[0].Library)
		}
		return &SelectionResult{
			Library:   matches[0].Library,
			Reason:    fmt.Sprintf("Show exists in %s (%d episodes)", matches[0].Library, matches[0].EpisodeCount),
			Available: matches[0].Available,
		}, nil
	}

	// STEP 4: Multiple matches - need to pick canonical location
	return s.resolveMultipleLocations(showName, year, fileSize, matches)
}

// selectSingleLibrary handles the case where only one library is available
func (s *Selector) selectSingleLibrary(lib string, fileSize int64) (*SelectionResult, error) {
	available, err := getAvailableSpace(lib)
	if err != nil {
		return nil, err
	}
	if available < fileSize {
		return nil, fmt.Errorf("insufficient space in %s: need %d, have %d", lib, fileSize, available)
	}
	return &SelectionResult{
		Library:   lib,
		Reason:    "Only library available",
		Available: available,
	}, nil
}

// hasSpace checks if a library has sufficient space for a file
func (s *Selector) hasSpace(lib string, fileSize int64) bool {
	available, err := getAvailableSpace(lib)
	if err != nil {
		return false
	}
	return available >= fileSize
}

// resolveMultipleLocations chooses the best library when a show exists in multiple locations
func (s *Selector) resolveMultipleLocations(showName, year string, fileSize int64, matches []ShowLocation) (*SelectionResult, error) {
	// HOLDEN Phase 3: Check database first for authoritative path
	if s.db != nil {
		yearInt := 0
		if year != "" {
			fmt.Sscanf(year, "%d", &yearInt)
		}

		series, err := s.db.GetSeriesByTitle(showName, yearInt)
		if err == nil && series != nil && series.CanonicalPath != "" {
			canonicalLibrary := libraryContainingPath(series.CanonicalPath, s.libraries)
			for _, m := range matches {
				if canonicalLibrary == m.Library {
					if s.hasSpace(m.Library, fileSize) {
						sourceDesc := fmt.Sprintf("Database canonical path (%s): %s", series.Source, series.CanonicalPath)
						return &SelectionResult{
							Library:   m.Library,
							Reason:    sourceDesc,
							Available: m.Available,
						}, nil
					}
				}
			}
		}
	}

	// 4a: Fallback to Sonarr cache if database didn't have answer
	if s.cache != nil {
		series := s.cache.FindSeries(showName, year)
		if series != nil && series.Path != "" {
			sonarrLibrary := libraryContainingPath(series.Path, s.libraries)
			for _, m := range matches {
				if sonarrLibrary == m.Library {
					if s.hasSpace(m.Library, fileSize) {
						return &SelectionResult{
							Library:   m.Library,
							Reason:    fmt.Sprintf("Sonarr authoritative path: %s", series.Path),
							Available: m.Available,
						}, nil
					}
				}
			}
		}
	}

	// 4b: Fall back to episode count
	var best *ShowLocation
	for i := range matches {
		if !s.hasSpace(matches[i].Library, fileSize) {
			continue
		}
		if best == nil || matches[i].EpisodeCount > best.EpisodeCount {
			best = &matches[i]
		}
	}

	if best == nil {
		return nil, fmt.Errorf("show exists in multiple libraries but none have sufficient space")
	}

	return &SelectionResult{
		Library:   best.Library,
		Reason:    fmt.Sprintf("Most episodes (%d) in this library", best.EpisodeCount),
		Available: best.Available,
	}, nil
}

// selectForNewShow chooses a library for a show that doesn't exist anywhere yet.
// Uses weighted scoring: free space (40%) + show count balance (60%) to prevent
// concentration on a single volume (e.g., STORAGE5 holding 3x more than others).
func (s *Selector) selectForNewShow(showName, year string, fileSize int64) (*SelectionResult, error) {
	type candidate struct {
		library   string
		available int64
		showCount int
	}

	var candidates []candidate
	for _, lib := range s.libraries {
		available, err := getAvailableSpace(lib)
		if err != nil || available < fileSize {
			continue
		}
		count := s.countMediaItems(lib, false)
		candidates = append(candidates, candidate{
			library:   lib,
			available: available,
			showCount: count,
		})
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no suitable library with sufficient space for new show")
	}

	// Find min show count for scoring
	minShows := candidates[0].showCount
	maxSpace := candidates[0].available
	for _, c := range candidates[1:] {
		if c.showCount < minShows {
			minShows = c.showCount
		}
		if c.available > maxSpace {
			maxSpace = c.available
		}
	}

	// Score each candidate: prefer volumes with fewer shows AND adequate space
	var best *candidate
	bestScore := -1.0
	for i := range candidates {
		c := &candidates[i]
		// Space score: 0.0 to 1.0 (fraction of max available)
		spaceScore := float64(c.available) / float64(maxSpace)
		// Balance score: 0.0 to 1.0 (inverse of show count relative to minimum)
		balanceScore := 1.0
		if c.showCount > 0 {
			balanceScore = float64(minShows+1) / float64(c.showCount+1)
		}
		// Weighted: balance matters more to prevent lopsided distribution
		score := spaceScore*0.4 + balanceScore*0.6
		if score > bestScore {
			bestScore = score
			best = c
		}
	}

	return &SelectionResult{
		Library:   best.library,
		Reason:    fmt.Sprintf("New show, balanced selection (%d GB free, %d shows)", best.available/(1024*1024*1024), best.showCount),
		Available: best.available,
	}, nil
}

func (s *Selector) scoreLibrary(library, title, year string, fileSize int64, isMovie bool) int {
	score := 0

	hasExisting, itemCount := s.findRelatedContent(library, title, year, isMovie)
	if hasExisting {
		score += 1000 + itemCount*100
	}

	available, _ := getAvailableSpace(library)
	spaceScore := int(available / (1024 * 1024 * 1024))
	if spaceScore > 100 {
		spaceScore = 100
	}
	score += spaceScore

	itemCount = s.countMediaItems(library, isMovie)
	score += min(itemCount/10, 50)

	return score
}

func (s *Selector) findRelatedContent(library, title, year string, isMovie bool) (hasContent bool, itemCount int) {
	normalizedTitle := strings.ToLower(strings.ReplaceAll(title, " ", ""))

	_ = filepath.Walk(library, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			dirName := strings.ToLower(info.Name())
			dirName = strings.ReplaceAll(dirName, " ", "")
			dirName = strings.ReplaceAll(dirName, "(", "")
			dirName = strings.ReplaceAll(dirName, ")", "")

			if strings.Contains(dirName, normalizedTitle) {
				itemCount++
				hasContent = true
			}
		}

		return nil
	})

	return hasContent, itemCount
}

func (s *Selector) countMediaItems(library string, isMovie bool) int {
	count := 0

	if isMovie {
		_ = filepath.Walk(library, func(path string, info fs.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				if video.IsVideo(path) {
					if strings.Contains(info.Name(), "(") && strings.Contains(info.Name(), ")") {
						count++
					}
				}
			}
			return nil
		})
	} else {
		_ = filepath.Walk(library, func(path string, info fs.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				if video.IsVideo(path) {
					if strings.Contains(info.Name(), "S") && strings.Contains(info.Name(), "E") {
						count++
					}
				}
			}
			return nil
		})
	}

	return count
}

func getAvailableSpace(path string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, fmt.Errorf("unable to get disk space for %s: %w", path, err)
	}

	return int64(stat.Bavail) * int64(stat.Bsize), nil
}

// findExistingShowDir searches for an existing show directory in the library
// Returns the full path to the show directory if found, empty string otherwise
func (s *Selector) findExistingShowDir(library, showName, year string) string {
	normalizedTitle := database.NormalizeForMatch(showName)
	yearPattern := regexp.MustCompile(`\s*\(\d{4}\)\s*$`)

	entries, err := filepath.Glob(filepath.Join(library, "*"))
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		dirName := filepath.Base(entry)
		// Strip year from directory name, then normalize and compare
		baseName := yearPattern.ReplaceAllString(dirName, "")
		normalizedDir := database.NormalizeForMatch(baseName)

		if normalizedDir == normalizedTitle {
			return entry
		}
	}

	return ""
}
