package compliance

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Nomadcxx/plex2jellyfin/internal/naming"
)

// ComplianceResult represents the compliance check result for database storage
type ComplianceResult struct {
	IsCompliant bool
	Issues      []string
}

// IssueType categorizes compliance issues
const (
	IssueInvalidFilename        = "invalid_filename"
	IssueReleaseMarkers         = "release_markers"
	IssueMissingYear            = "missing_year"
	IssueInvalidYearFormat      = "invalid_year_format"
	IssueInvalidFolderStructure = "invalid_folder_structure"
	IssueWrongSeasonFolder      = "wrong_season_folder"
	IssueSpecialCharacters      = "special_characters"
	IssueInvalidPadding         = "invalid_padding"
)

// Checker validates media files against Jellyfin naming conventions
type Checker struct {
	libraryRoot string
}

// NewChecker creates a new compliance checker
func NewChecker(libraryRoot string) *Checker {
	return &Checker{
		libraryRoot: libraryRoot,
	}
}

// CheckMovie validates a movie file and its path structure
//
// Expected structure:
//   - Movies/Movie Name (YYYY)/Movie Name (YYYY).ext
//
// Checks:
//  1. File is in a movie folder with year
//  2. Filename matches folder name
//  3. Year is in parentheses format (YYYY)
//  4. No release markers (1080p, BluRay, x264, etc)
//  5. No special characters that break Jellyfin
func (c *Checker) CheckMovie(fullPath string) ComplianceResult {
	result := ComplianceResult{
		IsCompliant: true,
		Issues:      []string{},
	}

	filename := filepath.Base(fullPath)
	parentDir := filepath.Base(filepath.Dir(fullPath))
	ext := filepath.Ext(filename)

	// Parse movie from filename
	movie, err := naming.ParseMovieName(filename)
	if err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("%s: %v", IssueInvalidFilename, err))
		result.IsCompliant = false
		return result
	}

	// Check year format (must be in parentheses)
	if movie.Year == "" {
		result.Issues = append(result.Issues, fmt.Sprintf("%s: missing year", IssueMissingYear))
	} else if !naming.HasYearInParentheses(filename) {
		result.Issues = append(result.Issues, fmt.Sprintf("%s: year must be in format (YYYY)", IssueInvalidYearFormat))
	}

	// Check for release markers
	if hasReleaseMarkers(filename) {
		result.Issues = append(result.Issues, fmt.Sprintf("%s: contains quality/codec markers", IssueReleaseMarkers))
	}

	// Check special characters
	if invalidChars := findInvalidCharacters(filename); len(invalidChars) > 0 {
		result.Issues = append(result.Issues, fmt.Sprintf("%s: contains invalid characters: %s", IssueSpecialCharacters, strings.Join(invalidChars, ", ")))
	}

	// Validate expected filename
	expectedFilename := naming.FormatMovieFilename(movie.Title, movie.Year, ext[1:])
	if filename != expectedFilename {
		result.Issues = append(result.Issues, fmt.Sprintf("%s: expected '%s'", IssueInvalidFilename, expectedFilename))
	}

	// Validate expected folder name
	expectedFolder := naming.NormalizeMediaName(movie.Title, movie.Year)
	if parentDir != expectedFolder {
		result.Issues = append(result.Issues, fmt.Sprintf("%s: folder should be '%s'", IssueInvalidFolderStructure, expectedFolder))
	}

	// Also check if folder name differs from title (catches missing year cases)
	if movie.Year == "" && parentDir != movie.Title {
		result.Issues = append(result.Issues, fmt.Sprintf("%s: folder name doesn't match title", IssueInvalidFolderStructure))
	}

	result.IsCompliant = len(result.Issues) == 0
	return result
}

// CheckEpisode validates a TV episode file and its path structure
//
// Expected structure:
//   - TV Shows/Show Name (Year)/Season XX/Show Name (Year) SXXEXX.ext
//
// Checks:
//  1. File is in proper Season folder
//  2. Season number is zero-padded (Season 01, not Season 1)
//  3. Filename contains SXXEXX format with zero-padding
//  4. No release markers
//  5. Year in parentheses
//  6. No special characters
func (c *Checker) CheckEpisode(fullPath string) ComplianceResult {
	result := ComplianceResult{
		IsCompliant: true,
		Issues:      []string{},
	}

	filename := filepath.Base(fullPath)
	seasonFolder := filepath.Base(filepath.Dir(fullPath))
	showFolder := filepath.Base(filepath.Dir(filepath.Dir(fullPath)))
	ext := filepath.Ext(filename)

	// Parse TV show from filename
	tv, err := naming.ParseTVShowName(filename)
	if err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("%s: %v", IssueInvalidFilename, err))
		result.IsCompliant = false
		return result
	}

	// Check season folder format
	expectedSeasonFolder := naming.FormatSeasonFolder(tv.Season)
	if !strings.EqualFold(seasonFolder, expectedSeasonFolder) {
		result.Issues = append(result.Issues, fmt.Sprintf("%s: expected '%s', found '%s'", IssueWrongSeasonFolder, expectedSeasonFolder, seasonFolder))
	}

	// Check season padding in folder name
	if !isValidSeasonFolder(seasonFolder) {
		result.Issues = append(result.Issues, fmt.Sprintf("%s: season folder must be zero-padded (Season 01, not Season 1)", IssueInvalidPadding))
	}

	// Check year format — for TV shows, missing year is informational, not a compliance failure.
	// Jellyfin recommends but does not require year in TV show folders.
	if tv.Year == "" {
		// Don't add as a compliance issue — year is optional for TV
	} else if !naming.HasYearInParentheses(filename) {
		result.Issues = append(result.Issues, fmt.Sprintf("%s: year must be in format (YYYY)", IssueInvalidYearFormat))
	}

	// Check for release markers
	if hasReleaseMarkers(filename) {
		result.Issues = append(result.Issues, fmt.Sprintf("%s: contains quality/codec markers", IssueReleaseMarkers))
	}

	// Check special characters
	if invalidChars := findInvalidCharacters(filename); len(invalidChars) > 0 {
		result.Issues = append(result.Issues, fmt.Sprintf("%s: contains invalid characters: %s", IssueSpecialCharacters, strings.Join(invalidChars, ", ")))
	}

	// Validate expected filename
	expectedFilename := naming.FormatTVEpisodeFilename(tv.Title, tv.Year, tv.Season, tv.Episode, ext[1:])
	if filename != expectedFilename {
		result.Issues = append(result.Issues, fmt.Sprintf("%s: expected '%s'", IssueInvalidFilename, expectedFilename))
	}

	// Validate expected show folder name
	expectedShowFolder := naming.NormalizeMediaName(tv.Title, tv.Year)
	if showFolder != expectedShowFolder {
		result.Issues = append(result.Issues, fmt.Sprintf("%s: show folder should be '%s'", IssueInvalidFolderStructure, expectedShowFolder))
	}

	// Also check if folder name differs from title (catches missing year cases)
	if tv.Year == "" && showFolder != tv.Title {
		result.Issues = append(result.Issues, fmt.Sprintf("%s: folder name doesn't match title", IssueInvalidFolderStructure))
	}

	result.IsCompliant = len(result.Issues) == 0
	return result
}

// CheckFile determines media type and runs appropriate validation
func (c *Checker) CheckFile(fullPath string) ComplianceResult {
	filename := filepath.Base(fullPath)

	if naming.IsTVEpisodeFilename(filename) {
		return c.CheckEpisode(fullPath)
	}

	if naming.IsMovieFilename(filename) {
		return c.CheckMovie(fullPath)
	}

	// Unknown media type
	return ComplianceResult{
		IsCompliant: false,
		Issues:      []string{fmt.Sprintf("%s: unable to determine media type", IssueInvalidFilename)},
	}
}

// hasReleaseMarkers checks if filename contains quality/release markers
func hasReleaseMarkers(filename string) bool {
	upper := strings.ToUpper(filename)

	markers := []string{
		"2160P", "1080P", "720P", "480P", "4K", "UHD", "8K",
		"BLURAY", "BLU-RAY", "BDRIP", "BRRIP", "BD-RIP",
		"WEB-DL", "WEBDL", "WEBRIP", "WEB-RIP",
		"HDTV", "DVDRIP", "DVD-RIP", "DVDSCR",
		"X264", "X265", "H264", "H265", "H.264", "H.265",
		"HEVC", "AVC", "AV1", "XVID",
		"AAC", "AC3", "DTS", "DD5.1", "ATMOS", "TRUEHD",
		"HDR", "HDR10", "DOLBY", "REMUX",
		"-GROUP", ".GROUP", "[GROUP]",
	}

	for _, marker := range markers {
		if strings.Contains(upper, marker) {
			return true
		}
	}

	return false
}

// findInvalidCharacters returns characters that are problematic for filesystems
// Jellyfin doesn't support: < > : " / \ | ? *
func findInvalidCharacters(filename string) []string {
	invalidChars := []string{"<", ">", ":", "\"", "/", "\\", "|", "?", "*"}
	found := []string{}

	for _, char := range invalidChars {
		if strings.Contains(filename, char) {
			found = append(found, char)
		}
	}

	return found
}

// isValidSeasonFolder checks if season folder uses proper zero-padding
func isValidSeasonFolder(folder string) bool {
	// Valid formats: "Season 01", "Season 02", ..., "Season 99"
	// Invalid: "Season 1", "season 01", "S01"

	if !strings.HasPrefix(folder, "Season ") {
		return false
	}

	seasonNum := strings.TrimPrefix(folder, "Season ")

	// Must be exactly 2 digits
	if len(seasonNum) != 2 {
		return false
	}

	// Must be numeric
	for _, c := range seasonNum {
		if c < '0' || c > '9' {
			return false
		}
	}

	return true
}
