package labeling

import (
	"strings"
	"unicode"
)

// titleTokens normalises s to lowercase, replaces punctuation with spaces, and
// returns the resulting non-empty tokens.
func titleTokens(s string) []string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		} else if r == '\'' || r == '\u2019' {
			// Drop apostrophes so "Marvel's" → "marvels"
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Fields(b.String())
}

// originalTokens returns the same tokens as titleTokens but preserves the
// original case so the matcher can distinguish a lowercase function-word
// "with" (a bridge such as "The Daily Show with Trevor Noah") from a
// capitalised "With" inside a title proper (e.g. "Hunting With Dogs").
func originalTokens(s string) []string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else if r == '\'' || r == '\u2019' {
			// Drop apostrophes so "Marvel's" → "Marvels"
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Fields(b.String())
}

// FuzzyTitleEqual reports whether the parsed title and the Jellyfin item name
// represent the same title. The comparison is deliberately conservative:
//   - Exact token equality after normalisation.
//   - One title may extend the other only when the extra tokens begin with a
//     lowercase function-word "with" (e.g. "The Daily Show with Trevor Noah"
//     vs "The Daily Show").  A capitalised "With" is treated as part of the
//     title proper so that "Hunting With Dogs" does NOT match "Hunting".
//   - Long-release/short-canonical alias: a verbose release title may carry a
//     connector-led subtitle the provider drops (e.g. parsed "Life Larry And
//     The Pursuit Of Unhappiness…" vs Jellyfin "Life Larry").
//
// The alias rule is directional, and that direction is the whole safeguard.
// It applies only when the *parsed* title is the longer one, because the
// Jellyfin name is provider-canonical: a messy filename carrying extra
// subtitle text is ordinary, whereas the provider naming a work *more*
// specifically than we parsed ("Harry Potter" vs "Harry Potter and the
// Philosopher's Stone") is the signal that we matched the wrong or an
// over-generic title — exactly the DRIFT this labeller exists to catch.
func FuzzyTitleEqual(parsed, jellyfinName string) bool {
	tp := titleTokens(parsed)
	tj := titleTokens(jellyfinName)

	if tokensEqual(tp, tj) {
		return true
	}

	// Determine which is longer; the longer must extend the shorter via "with …"
	short, long := tp, tj
	longOrig := originalTokens(jellyfinName)
	parsedIsLong := false
	if len(tp) > len(tj) {
		short, long = tj, tp
		longOrig = originalTokens(parsed)
		parsedIsLong = true
	}

	if len(long) <= len(short) {
		return false
	}

	// The short must be a prefix of the long.
	for i, tok := range short {
		if tok != long[i] {
			return false
		}
	}

	// The first extra token must be the lowercase function-word "with".
	// "With" with a capital W is treated as part of the title and disqualifies
	// the match.
	idx := len(short)
	if long[idx] == "with" {
		if idx < len(longOrig) && longOrig[idx] == "with" {
			return true
		}
	}

	if !parsedIsLong {
		return false
	}
	return longReleaseShortCanonical(short, long)
}

// longReleaseShortCanonical accepts a provider-canonical short title as a
// proper prefix of a verbose parsed release title when the continuation is a
// connector-led subtitle of real substance.
//
// Requires ≥2 shared tokens so single-word prefixes ("Hunting", "Maximum")
// never alias, and ≥3 trailing tokens so a connector plus one word ("Blue
// Planet and Beyond") stays a distinct title rather than a dropped subtitle.
func longReleaseShortCanonical(short, long []string) bool {
	const minSubtitleTokens = 3 // connector + at least two subtitle words
	if len(short) < 2 || len(long) < len(short)+minSubtitleTokens {
		return false
	}
	for i, tok := range short {
		if tok != long[i] {
			return false
		}
	}
	switch long[len(short)] {
	case "and", "or", "a", "an", "the":
		return true
	default:
		return false
	}
}

func tokensEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
