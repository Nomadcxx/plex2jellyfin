package naming

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

var (
	// Matches MD5/SHA1 hashes: 32+ hexadecimal chars
	hexStringRegex = regexp.MustCompile(`^[0-9a-fA-F]{32,}$`)

	// Matches UUID: 8-4-4-4-12 format with optional separators
	uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}[-_]?[0-9a-fA-F]{4}[-_]?[0-9a-fA-F]{4}[-_]?[0-9a-fA-F]{4}[-_]?[0-9a-fA-F]{12}$`)

	// Matches base64-like: 20+ alphanumeric with special chars
	base64LikeRegex = regexp.MustCompile(`^[A-Za-z0-9+/=]{20,}$`)
)

// IsObfuscatedFilename detects obfuscated filenames like:
//   - "30e2dc4173fc4798bbe5fd40137ed621.mkv" (hex/MD5)
//   - "675d75953e9b4602-9464-6424b664c6d7.mkv" (UUID)
//   - "RTVA3rFvM11jjtr6pdNPpUDg2.mkv" (random alphanumeric)
func IsObfuscatedFilename(filename string) bool {
	baseName := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))

	if len(baseName) < 8 {
		return false
	}

	if hexStringRegex.MatchString(baseName) {
		return true
	}

	if uuidRegex.MatchString(baseName) {
		return true
	}

	if episodeSERegex.MatchString(baseName) || episodeXRegex.MatchString(baseName) {
		return false
	}

	if yearRegex.MatchString(baseName) {
		return false
	}

	if IsGarbageTitle(baseName) && !strings.Contains(baseName, " ") && !strings.Contains(baseName, ".") && hasHighEntropy(baseName) {
		return true
	}

	if len(baseName) >= 20 && isRandomAlphanumeric(baseName) {
		return true
	}

	if base64LikeRegex.MatchString(baseName) && hasHighEntropy(baseName) {
		return true
	}

	return false
}

func isRandomAlphanumeric(s string) bool {
	if len(s) < 20 {
		return false
	}

	hasUpper := false
	hasLower := false
	hasDigit := false
	letterCount := 0
	digitCount := 0

	for _, r := range s {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
			letterCount++
		case unicode.IsLower(r):
			hasLower = true
			letterCount++
		case unicode.IsDigit(r):
			hasDigit = true
			digitCount++
		}
	}

	if !hasUpper || !hasLower || !hasDigit {
		return false
	}

	digitRatio := float64(digitCount) / float64(len(s))
	if digitRatio < 0.05 || digitRatio > 0.5 {
		return false
	}

	return hasHighEntropy(s)
}

// hasHighEntropy returns true if string has high character variance (randomness).
// Random strings have uniqueChars/totalChars > 0.4
func hasHighEntropy(s string) bool {
	if len(s) < 10 {
		return false
	}

	freq := make(map[rune]int)
	for _, r := range s {
		freq[r]++
	}

	uniqueRatio := float64(len(freq)) / float64(len(s))
	return uniqueRatio > 0.4
}

// ParseTVShowFromPath extracts TV info from path when filename is obfuscated.
// Example: /downloads/The.White.Lotus.S02E07.1080p/random.mkv -> TVShowInfo
func ParseTVShowFromPath(path string) (*TVShowInfo, error) {
	filename := filepath.Base(path)
	if !IsObfuscatedFilename(filename) {
		return ParseTVShowName(filename)
	}

	dir := filepath.Dir(path)
	for i := 0; i < 3; i++ {
		if dir == "/" || dir == "." || dir == "" {
			break
		}

		folderName := filepath.Base(dir)
		if IsTVEpisodeFilename(folderName) {
			return ParseTVShowName(folderName)
		}

		dir = filepath.Dir(dir)
	}

	return nil, &ParseError{
		Filename: path,
		Message:  "could not extract TV show info from path (obfuscated filename, no episode markers in parent folders)",
	}
}

// ParseMovieFromPath extracts movie info from path when filename is obfuscated.
// Example: /downloads/Inception.2010.1080p.BluRay/random.mkv -> MovieInfo
func ParseMovieFromPath(path string) (*MovieInfo, error) {
	filename := filepath.Base(path)
	if !IsObfuscatedFilename(filename) {
		return ParseMovieName(filename)
	}

	dir := filepath.Dir(path)
	for i := 0; i < 3; i++ {
		if dir == "/" || dir == "." || dir == "" {
			break
		}

		folderName := filepath.Base(dir)

		if IsObfuscatedFilename(folderName + ".mkv") {
			dir = filepath.Dir(dir)
			continue
		}

		if !IsTVEpisodeFilename(folderName) {
			info, err := ParseMovieName(folderName)
			if err == nil && !IsGarbageTitle(info.Title) {
				return info, nil
			}
		}

		dir = filepath.Dir(dir)
	}

	return nil, &ParseError{
		Filename: path,
		Message:  "could not extract movie info from path (obfuscated filename, no valid movie name in parent folders)",
	}
}

// IsTVEpisodeFromPath returns true if path represents a TV episode.
// The hint parameter indicates source watch folder (if known).
func IsTVEpisodeFromPath(path string, hint SourceHint) bool {
	// If hint says TV, trust it (user configured this folder for TV)
	if hint == SourceTV {
		return true
	}

	// If hint says Movie, trust it
	if hint == SourceMovie {
		return false
	}

	// SourceUnknown: use filename-based detection
	filename := filepath.Base(path)
	if IsTVEpisodeFilename(filename) {
		return true
	}

	// Check parent folders for obfuscated files
	if IsObfuscatedFilename(filename) {
		dir := filepath.Dir(path)
		for i := 0; i < 3; i++ {
			if dir == "/" || dir == "." || dir == "" {
				break
			}

			folderName := filepath.Base(dir)
			if IsTVEpisodeFilename(folderName) {
				return true
			}

			dir = filepath.Dir(dir)
		}
	}

	return false
}

// IsMovieFromPath returns true if path represents a movie (not a TV episode).
func IsMovieFromPath(path string, hint SourceHint) bool {
	return !IsTVEpisodeFromPath(path, hint)
}

type ParseError struct {
	Filename string
	Message  string
}

func (e *ParseError) Error() string {
	return e.Message + ": " + e.Filename
}
