package database

import (
	"fmt"
	"regexp"
	"strings"
)

var yearSuffixPattern = regexp.MustCompile(`\s*\((\d{4})\)\s*$`)

// NormalizeTitle converts a title to a normalized form for matching
// "For All Mankind (2019)" -> "forallmankind"
// "M*A*S*H" -> "mash"
func NormalizeTitle(title string) string {
	// Remove year suffix like "(2024)"
	title = yearSuffixPattern.ReplaceAllString(title, "")

	// Lowercase
	title = strings.ToLower(title)

	// Remove common separators and punctuation
	replacements := []string{
		" ", "", ".", "", "-", "", "_", "",
		"'", "", ":", "", "&", "", "*", "",
		",", "", "!", "", "?", "",
		"(", "", ")", "",
		"[", "", "]", "",
	}

	replacer := strings.NewReplacer(replacements...)
	title = replacer.Replace(title)

	return title
}

// NormalizeForMatch normalizes a title for directory matching.
// Lowercases, removes ALL punctuation and whitespace so that
// punctuation-variant titles compare equal.
// Use this when comparing a parsed title against existing directory names.
// "Chip 'n Dale Rescue Rangers (2022)" -> "chipndalerescuerangers2022"
// "Chip n Dale Rescue Rangers" -> "chipndalerescuerangers"
func NormalizeForMatch(title string) string {
	// Lowercase
	title = strings.ToLower(title)

	// Remove common separators and punctuation — same set as NormalizeTitle
	replacer := strings.NewReplacer(
		" ", "", ".", "", "-", "", "_", "",
		"'", "", "\u2019", "", ":", "", "&", "", "*", "",
		",", "", "!", "", "?", "",
		"(", "", ")", "",
		"[", "", "]", "",
	)
	return replacer.Replace(title)
}

// ExtractYear attempts to extract a year from a title string
// "For All Mankind (2019)" -> 2019
// "Fallout" -> 0
func ExtractYear(title string) int {
	matches := yearSuffixPattern.FindStringSubmatch(title)
	if len(matches) >= 2 {
		var year int
		fmt.Sscanf(matches[1], "%d", &year)
		if year >= 1900 && year <= 2100 {
			return year
		}
	}
	return 0
}

// StripYear removes the year suffix from a title
// "For All Mankind (2019)" -> "For All Mankind"
func StripYear(title string) string {
	return yearSuffixPattern.ReplaceAllString(title, "")
}

var bareYearPattern = regexp.MustCompile(`\b((?:19|20)\d{2})\b`)
var resolutionPattern = regexp.MustCompile(`(?i)\b\d{3,4}[pi]\b`)

// ExtractYearFlexible extracts a year from a title string, trying parenthesized
// format first "(YYYY)", then falling back to bare years "YYYY".
// Skips resolution numbers like 1080p, 720p.
// "Pandora (2019)" -> 2019
// "pandora 2019" -> 2019
// "Fallout" -> 0
func ExtractYearFlexible(title string) int {
	// Try parenthesized first (most reliable)
	if year := ExtractYear(title); year != 0 {
		return year
	}

	// Strip resolution markers to avoid matching 1080, 720, etc.
	clean := resolutionPattern.ReplaceAllString(title, "")

	// Find all bare years, return the last valid one
	matches := bareYearPattern.FindAllStringSubmatch(clean, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		var year int
		fmt.Sscanf(matches[i][1], "%d", &year)
		if year >= 1900 && year <= 2100 {
			return year
		}
	}

	return 0
}
