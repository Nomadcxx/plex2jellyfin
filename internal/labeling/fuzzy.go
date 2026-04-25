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

// FuzzyTitleEqual returns true when a and b represent the same title under a
// conservative comparison:
//   - Exact token equality after normalisation.
//   - One title may extend the other only when the extra tokens begin with
//     "with" (e.g. "The Daily Show with Trevor Noah" vs "The Daily Show").
func FuzzyTitleEqual(a, b string) bool {
	ta := titleTokens(a)
	tb := titleTokens(b)

	if tokensEqual(ta, tb) {
		return true
	}

	// Determine which is longer; the longer must extend the shorter via "with …"
	short, long := ta, tb
	if len(ta) > len(tb) {
		short, long = tb, ta
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

	// The first extra token must be "with".
	return long[len(short)] == "with"
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
