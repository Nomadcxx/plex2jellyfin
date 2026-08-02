package labeling_test

import (
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/labeling"
)

func TestFuzzyTitleEqual(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"Tracker", "tracker", true},
		{"The Daily Show with Trevor Noah", "The Daily Show", true},
		{"Outcome AAC5 1", "Outcome", false},
		{"the dreadful aac5 1 bz", "The Dreadful", false},
		{"Marvel's Daredevil", "Marvels Daredevil", true},
		{"X-Men", "x men", true},
		// l5: a capitalised "With" inside the title proper must NOT be treated
		// as a bridge token, so a longer title containing "With" is not
		// considered a fuzzy match for its prefix.
		{"Hunting With Dogs", "Hunting", false},
		{"Hunting", "Hunting With Dogs", false},
		// Provider-backed long title vs short series alias (Life Larry).
		{
			"Life Larry And The Pursuit Of Unhappiness An Almost History Of America",
			"Life Larry",
			true,
		},
		{
			"Life, Larry And The Pursuit Of Unhappiness An Almost History Of America",
			"Life Larry",
			true,
		},
		// Genuine DRIFT: unrelated titles stay unequal.
		{"Tracker", "Breaking Bad", false},
		// Short single-token prefix must not alias into a longer distinct title.
		{"Maximum Pleasure Guaranteed", "Maximum", false},
		// The alias is directional. When the provider names the work MORE
		// specifically than we parsed, that is the drift signal, not an alias:
		// these are distinct works sharing a franchise prefix.
		{"Harry Potter", "Harry Potter and the Philosopher's Stone", false},
		{"Doctor Who", "Doctor Who and the Daleks", false},
		{"Percy Jackson", "Percy Jackson and the Olympians", false},
		{"The Hobbit", "The Hobbit and the Desolation of Smaug", false},
		{"Blue Planet", "Blue Planet the Deep Ocean Special", false},
		// A connector plus a single word is a distinct title, not a dropped
		// subtitle, even in the permitted direction.
		{"Blue Planet And Beyond", "Blue Planet", false},
	}
	for _, tc := range cases {
		got := labeling.FuzzyTitleEqual(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("FuzzyTitleEqual(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
