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
