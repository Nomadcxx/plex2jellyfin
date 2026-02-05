package naming

import (
	"regexp"
	"strings"
)

var (
	// Jellyfin-compliant TV episode patterns:
	// "Show S01E01.ext" or "Show - S01E01.ext" or "Show (Year) S01E01.ext"
	jellyfinEpisodePattern = regexp.MustCompile(`^(.+?)(?:\s*\(\d{4}\))?\s*[-.]?\s*S\d{2}E\d{2,3}(?:\s*[-.]?\s*E\d{2,3})?\.[\w]+$`)

	// Jellyfin-compliant movie patterns:
	// "Movie (Year).ext"
	jellyfinMoviePattern = regexp.MustCompile(`^(.+?)\s*\(\d{4}\)\.[\w]+$`)

	// Release markers that indicate non-compliance
	releaseMarkerPattern = regexp.MustCompile(`(?i)(1080p|720p|2160p|4k|uhd|bluray|blu-ray|web-dl|webdl|webrip|hdtv|dvdrip|remux|x264|x265|h\.?264|h\.?265|hevc|avc|aac|dts|atmos|truehd|ddp?5\.?1|dd5\.?1|10bit|hdr|dolby|vision|dv|proper|repack|internal)`)

	// Release group suffix pattern for Jellyfin compliance check
	jellyfinReleaseGroupSuffix = regexp.MustCompile(`-[A-Za-z0-9]+(?:\[[\w]+\])?\.[\w]+$`)
)

// IsJellyfinCompliantFilename checks if a filename follows Jellyfin naming conventions
// without extraneous release markers or group names.
func IsJellyfinCompliantFilename(filename, mediaType string) bool {
	if filename == "" {
		return false
	}

	// Check for release markers anywhere in filename
	if releaseMarkerPattern.MatchString(filename) {
		return false
	}

	// Check for release group suffix (e.g., "-RARBG.mkv")
	// But allow simple hyphen-separated titles like "Spider-Man"
	base := strings.TrimSuffix(filename, "."+getExtension(filename))
	if hasReleaseGroupSuffix(base) {
		return false
	}

	switch mediaType {
	case "episode":
		return jellyfinEpisodePattern.MatchString(filename)
	case "movie":
		return jellyfinMoviePattern.MatchString(filename)
	default:
		return false
	}
}

// getExtension returns the file extension without the dot
func getExtension(filename string) string {
	for i := len(filename) - 1; i >= 0; i-- {
		if filename[i] == '.' {
			return filename[i+1:]
		}
	}
	return ""
}

// hasReleaseGroupSuffix checks for release group pattern like "-RARBG"
// but avoids false positives for hyphenated titles
func hasReleaseGroupSuffix(base string) bool {
	// Look for pattern: capital letters/numbers after final hyphen
	lastHyphen := strings.LastIndex(base, "-")
	if lastHyphen == -1 || lastHyphen == len(base)-1 {
		return false
	}

	suffix := base[lastHyphen+1:]

	// If suffix contains spaces, it's not a release group
	if strings.Contains(suffix, " ") {
		return false
	}

	// If suffix is all uppercase or mixed case with digits, likely release group
	hasUpper := false
	for _, ch := range suffix {
		if ch >= 'A' && ch <= 'Z' {
			hasUpper = true
		}
	}

	// Release groups are typically ALL CAPS or mixed with digits
	if len(suffix) >= 2 && len(suffix) <= 10 {
		if IsKnownReleaseGroup(strings.ToLower(suffix)) {
			return true
		}
		// All caps suffix is suspicious
		if hasUpper && suffix == strings.ToUpper(suffix) {
			return true
		}
	}

	return false
}
