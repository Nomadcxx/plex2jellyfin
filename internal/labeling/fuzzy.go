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

// FuzzyTitleEqual returns true when a and b represent the same title under a
// conservative comparison:
//   - Exact token equality after normalisation.
//   - One title may extend the other only when the extra tokens begin with a
//     lowercase function-word "with" (e.g. "The Daily Show with Trevor Noah"
//     vs "The Daily Show").  A capitalised "With" is treated as part of the
//     title proper so that "Hunting With Dogs" does NOT match "Hunting".
func FuzzyTitleEqual(a, b string) bool {
	ta := titleTokens(a)
	tb := titleTokens(b)

	if tokensEqual(ta, tb) {
		return true
	}

	// Determine which is longer; the longer must extend the shorter via "with …"
	short, long := ta, tb
	longOrig := originalTokens(b)
	if len(ta) > len(tb) {
		short, long = tb, ta
		longOrig = originalTokens(a)
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
	if long[idx] != "with" {
		return false
	}
	if idx >= len(longOrig) || longOrig[idx] != "with" {
		return false
	}
	return true
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
