package labeling_test

import (
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/labeling"
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
	}
	for _, tc := range cases {
		got := labeling.FuzzyTitleEqual(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("FuzzyTitleEqual(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
