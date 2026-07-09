package validator

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Nomadcxx/plex2jellyfin/internal/naming"
)

// MediaType represents the type of media file
type MediaType string

const (
	MediaTypeMovie MediaType = "movie"
	MediaTypeTV    MediaType = "tv"
)

// ValidationResult represents the result of validation
type ValidationResult struct {
	Valid        bool
	Type         MediaType
	Path         string
	CurrentName  string
	ExpectedName string
	Issues       []string
}

// Validator validates media files against Jellyfin naming conventions
type Validator struct {
	allowMissingYear bool
}

// NewValidator creates a new Validator instance
func NewValidator(options ...func(*Validator)) *Validator {
	v := &Validator{
		allowMissingYear: false,
	}

	for _, opt := range options {
		opt(v)
	}

	return v
}

// WithAllowMissingYear sets whether to allow files without year
func WithAllowMissingYear(allow bool) func(*Validator) {
	return func(v *Validator) {
		v.allowMissingYear = allow
	}
}

// ValidateFile validates a single media file
func (v *Validator) ValidateFile(path string) (*ValidationResult, error) {
	filename := filepath.Base(path)

	// Determine media type
	mediaType, err := v.detectMediaType(filename)
	if err != nil {
		return nil, err
	}

	result := &ValidationResult{
		Valid:       true,
		Type:        mediaType,
		Path:        path,
		CurrentName: filename,
		Issues:      []string{},
	}

	// Validate based on media type
	if mediaType == MediaTypeMovie {
		v.validateMovieFile(path, result)
	} else {
		v.validateTVFile(path, result)
	}

	result.Valid = len(result.Issues) == 0
	return result, nil
}

// detectMediaType determines if a file is a movie or TV show
func (v *Validator) detectMediaType(filename string) (MediaType, error) {
	if naming.IsMovieFilename(filename) {
		return MediaTypeMovie, nil
	}
	if naming.IsTVEpisodeFilename(filename) {
		return MediaTypeTV, nil
	}
	return "", fmt.Errorf("unable to determine media type for: %s", filename)
}

// validateMovieFile validates movie file naming
func (v *Validator) validateMovieFile(path string, result *ValidationResult) {
	filename := filepath.Base(path)

	// Parse movie name
	movie, err := naming.ParseMovieName(filename)
	if err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("Unable to parse movie name: %v", err))
		return
	}

	// Check if year is in parentheses
	if !v.allowMissingYear && !naming.HasYearInParentheses(filename) {
		result.Issues = append(result.Issues, "Year not in parentheses format (YYYY)")
	}

	// Check for release group markers
	if v.hasReleaseMarkers(filename) {
		result.Issues = append(result.Issues, "Contains release group markers")
	}

	// Calculate expected name
	ext := filepath.Ext(filename)
	expectedName := naming.FormatMovieFilename(movie.Title, movie.Year, ext[1:])

	if filename != expectedName {
		result.Issues = append(result.Issues, fmt.Sprintf("Filename doesn't match expected format"))
		result.ExpectedName = expectedName
	}
}

// validateTVFile validates TV episode file naming
func (v *Validator) validateTVFile(path string, result *ValidationResult) {
	filename := filepath.Base(path)
	dirName := filepath.Base(filepath.Dir(path))

	// Parse TV show name
	tv, err := naming.ParseTVShowName(filename)
	if err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("Unable to parse TV show name: %v", err))
		return
	}

	// Check if in proper season folder
	expectedSeason := naming.FormatSeasonFolder(tv.Season)
	if !strings.EqualFold(dirName, expectedSeason) {
		result.Issues = append(result.Issues,
			fmt.Sprintf("Not in proper season folder (expected: %s, found: %s)", expectedSeason, dirName))
	}

	// Check for release group markers
	if v.hasReleaseMarkers(filename) {
		result.Issues = append(result.Issues, "Contains release group markers")
	}

	// Calculate expected name
	ext := filepath.Ext(filename)
	expectedName := naming.FormatTVEpisodeFilenameFromInfo(tv, ext[1:])

	if filename != expectedName {
		result.Issues = append(result.Issues, "Filename doesn't match expected format")
		result.ExpectedName = expectedName
	}
}

// hasReleaseMarkers checks if filename contains release markers
func (v *Validator) hasReleaseMarkers(filename string) bool {
	upperName := strings.ToUpper(filename)

	markers := []string{
		"1080P", "720P", "2160P", "4K", "UHD",
		"BLURAY", "BDRIP", "BRRIP", "WEB-DL", "WEBDL", "WEBRIP",
		"HDTV", "X264", "X265", "HEVC", "H264", "H265",
	}

	for _, marker := range markers {
		if strings.Contains(upperName, marker) {
			return true
		}
	}

	return false
}
