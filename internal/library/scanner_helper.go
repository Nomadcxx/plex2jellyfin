package library

import "regexp"

// extractYearFromDir extracts year from directory name if present
// Looks for pattern: "Name (YYYY)" and returns "YYYY"
// Returns empty string if no year found
func extractYearFromDir(dirName string) string {
	yearPattern := regexp.MustCompile(`\((\d{4})\)`)
	matches := yearPattern.FindStringSubmatch(dirName)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}
