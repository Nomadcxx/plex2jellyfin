// Package correction decides whether a persistently-unidentified movie import
// has a wrong folder name and, if so, what the correct title is. It performs
// no filesystem or Jellyfin mutation — it only decides.
package correction

import "strings"

// editionWords are trailing tokens that are release metadata rather than part
// of a real title when they leak into a parsed name. Bare "cut" is included
// here only for candidate generation (not for the parser strip, where "Final
// Cut" would be damaged) because generated candidates are always verified
// against TMDB before use.
var editionWords = map[string]bool{
	"cut": true, "uncut": true, "unrated": true, "extended": true,
	"theatrical": true, "remastered": true, "imax": true, "edition": true,
	"redux": true, "recut": true,
}

// GenerateCandidates returns ordered alternative titles to verify: first the
// title with trailing edition words removed, then progressive trailing-token
// trims. Every candidate has >= 2 tokens (single-token titles collide too
// hard to auto-apply), is de-duplicated, and never equals the input.
func GenerateCandidates(currentTitle string) []string {
	tokens := strings.Fields(currentTitle)
	if len(tokens) < 2 {
		return nil
	}
	seen := map[string]bool{strings.ToLower(currentTitle): true}
	var out []string
	add := func(cand []string) {
		if len(cand) < 2 {
			return
		}
		s := strings.Join(cand, " ")
		key := strings.ToLower(s)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, s)
	}

	stripped := append([]string(nil), tokens...)
	for len(stripped) >= 2 && editionWords[strings.ToLower(stripped[len(stripped)-1])] {
		stripped = stripped[:len(stripped)-1]
	}
	if len(stripped) != len(tokens) {
		add(stripped)
	}

	for n := len(tokens) - 1; n >= 2; n-- {
		add(tokens[:n])
	}
	return out
}