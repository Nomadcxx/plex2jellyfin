package naming

import (
	"math"
	"regexp"
	"strings"
)

var (
	// Matches codec patterns at end of title
	codecSuffixRegex = regexp.MustCompile(`(?i)\b(x264|x265|h264|h265|hevc|avc|av1|xvid|divx)$`)
	// Matches resolution patterns at end of title
	resolutionSuffixRegex = regexp.MustCompile(`(?i)\b(2160p|1080p|720p|480p|4k|uhd)$`)
	ordinalTokenRegex     = regexp.MustCompile(`(?i)^\d+(st|nd|rd|th)$`)
)

// CalculateTitleConfidence calculates a confidence score (0.0-1.0) for a parsed title.
// Higher scores indicate cleaner, more reliable parses.
// Uses the existing blacklist.go for release group detection, but only in
// contexts where the parsed title itself is likely to be a release artifact.
func CalculateTitleConfidence(title, originalFilename string) float64 {
	confidence := 1.0

	// Major penalties
	if shouldApplyGarbageTitlePenalty(title, originalFilename) && IsGarbageTitle(title) {
		confidence -= 0.8
	}
	if IsObfuscatedFilename(originalFilename) {
		confidence -= 0.9
	}
	if hasDuplicateYear(originalFilename) {
		confidence -= 0.5
	}

	// Moderate penalties
	if len(title) < 3 {
		confidence -= 0.5
	}
	if endsWithCodecOrSource(title) {
		confidence -= 0.4
	}
	if hasResolutionInTitle(title) {
		confidence -= 0.4
	}
	// Single-word penalty: reduced from -0.3 to -0.1, and skip for known media titles
	if !strings.Contains(title, " ") && len(title) > 3 && !IsKnownMediaTitle(title) {
		confidence -= 0.1
	}
	if hasReleaseMarkers(originalFilename) {
		confidence -= 0.1
	}

	// Bonuses
	if HasYearInParentheses(originalFilename) {
		confidence += 0.1
	}

	// Floor: well-formed SxxExx filenames should never score below 0.5
	if isTVPattern(originalFilename) && confidence < 0.5 {
		confidence = 0.5
	}

	return math.Max(math.Min(confidence, 1.0), 0.0)
}

func shouldApplyGarbageTitlePenalty(title, originalFilename string) bool {
	// The srrDB release-group list contains many normal title words. Applying
	// IsGarbageTitle to complete titles causes false low-confidence parses for
	// clean names like "Green Book", "Look Away", and "The 2nd". Keep the
	// broad release-group penalty for standalone artifacts, and for multi-word
	// titles only when a clearly technical token remains in the parsed title.
	if isTVPattern(originalFilename) {
		return isStandaloneReleaseArtifact(title)
	}
	words := strings.Fields(title)
	if len(words) <= 1 {
		return true
	}
	for _, word := range words {
		if isTechnicalTitleToken(word) {
			return true
		}
	}
	return false
}

func isStandaloneReleaseArtifact(title string) bool {
	words := strings.Fields(title)
	if len(words) != 1 {
		return false
	}
	word := strings.ToLower(strings.Trim(words[0], " ._-[]()"))
	if word == "" {
		return true
	}
	if IsKnownMediaTitle(word) || IsPreservedAcronym(word) {
		return false
	}
	return IsCodecMarker(word) || IsKnownReleaseGroup(word)
}

func isTechnicalTitleToken(word string) bool {
	token := strings.ToLower(strings.Trim(word, " ._-[]()"))
	if token == "" {
		return true
	}
	if ordinalTokenRegex.MatchString(token) {
		return false
	}
	if IsCodecMarker(token) || codecSuffixRegex.MatchString(token) || resolutionSuffixRegex.MatchString(token) {
		return true
	}
	switch token {
	case "bdrip", "bluray", "br-rip", "brrip", "cam", "dvdrip", "hdtv", "remux", "web", "web-dl", "webdl", "webrip":
		return true
	}

	hasDigit := false
	hasLetter := false
	for _, ch := range token {
		if ch >= '0' && ch <= '9' {
			hasDigit = true
		}
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
			hasLetter = true
		}
	}
	return hasDigit && hasLetter
}

// isTVPattern checks if filename has a valid SxxExx pattern
func isTVPattern(filename string) bool {
	return tvPatternForConfidence.MatchString(filename)
}

var tvPatternForConfidence = regexp.MustCompile(`(?i)S\d{1,2}E\d{1,2}`)

// hasDuplicateYear detects patterns like "Matrix (2001) (2001)" or "Movie 2020 2020"
// Does NOT flag cases like "2001 A Space Odyssey (2001)" where year is part of title
func hasDuplicateYear(s string) bool {
	// Get all year matches with their positions
	type yearMatch struct {
		year    string
		pos     int
		inParen bool
	}

	parenYearRegex := regexp.MustCompile(`\((19\d{2}|20\d{2})\)`) // Captures full year
	plainYearRegex := regexp.MustCompile(`\b(19|20)\d{2}\b`)

	// Find all parenthesized years with their positions
	parenMatches := parenYearRegex.FindAllStringSubmatchIndex(s, -1)
	plainMatches := plainYearRegex.FindAllStringSubmatchIndex(s, -1)

	// Helper: check if position is within any parenthesized year range
	isWithinParen := func(pos int) bool {
		for _, m := range parenMatches {
			if pos >= m[0] && pos < m[1] {
				return true
			}
		}
		return false
	}

	// Collect all years with metadata
	var allYears []yearMatch
	for _, m := range parenMatches {
		year := s[m[2]:m[3]] // Extract year number from inner capture group
		allYears = append(allYears, yearMatch{year: year, pos: m[0], inParen: true})
	}
	for _, m := range plainMatches {
		year := s[m[0]:m[1]] // Full match
		// Skip if this is part of a parenthesized year OR appears at the very start (likely title year)
		if !isWithinParen(m[0]) && m[0] > 0 {
			allYears = append(allYears, yearMatch{year: year, pos: m[0], inParen: false})
		}
	}

	// Check for duplicates
	yearCount := make(map[string]int)
	for _, y := range allYears {
		yearCount[y.year]++
		if yearCount[y.year] > 1 {
			return true
		}
	}

	return false
}

// endsWithCodecOrSource checks if title ends with codec/source markers
func endsWithCodecOrSource(title string) bool {
	return codecSuffixRegex.MatchString(title)
}

// hasResolutionInTitle checks if title contains resolution markers
func hasResolutionInTitle(title string) bool {
	return resolutionSuffixRegex.MatchString(title)
}

// hasReleaseMarkers checks if original filename has release group markers
func hasReleaseMarkers(filename string) bool {
	// Check for common release markers in filename
	markers := []string{
		"RARBG", "YTS", "YIFY", "SPARKS", "FGT", "NTb", "FLUX",
		"BluRay", "WEB-DL", "WEBRip", "HDTV", "REMUX",
	}
	upper := strings.ToUpper(filename)
	for _, m := range markers {
		if strings.Contains(upper, m) {
			return true
		}
	}
	return false
}
