package naming

import (
	"regexp"
	"strings"
)

// extrasMarkerRegex matches release-name tokens that denote bonus/extras
// content (bonus discs, featurettes, behind-the-scenes reels).
var extrasMarkerRegex = regexp.MustCompile(`(?i)\b(bonus|extras?|behind[._ ]the[._ ]scenes|featurettes?)\b`)

// IsExtrasRelease reports whether a release name denotes bonus/extras content
// rather than episodes. Names containing an SxxExx episode marker are never
// extras releases (e.g. the shows "Bonus Family" or "The Extras").
func IsExtrasRelease(name string) bool {
	if episodeSERegex.MatchString(name) {
		return false
	}
	return extrasMarkerRegex.MatchString(name)
}

// CleanExtrasName converts a release-style name (dot separators, quality and
// codec tags, release group suffixes) into a human-readable extras title:
// "The.Last.Ship.S02.BONUS.2015.BluRay.1080p.AC3.x264-MTeam" ->
// "The Last Ship S02 BONUS". Any extension must be stripped by the caller.
// Returns "" when the name consists only of release metadata.
func CleanExtrasName(name string) string {
	var kept []string
	for _, tok := range strings.FieldsFunc(name, func(r rune) bool {
		return r == '.' || r == '_' || r == ' '
	}) {
		if episodeTitleStopToken.MatchString(tok) ||
			episodeTitleYearToken.MatchString(tok) ||
			qualityMarkerDetect.MatchString(tok) {
			break
		}
		kept = append(kept, tok)
	}
	return normalizeSpaces(strings.TrimSpace(strings.Join(kept, " ")))
}
