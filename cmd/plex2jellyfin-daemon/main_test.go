package main

import (
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin"
)

func TestPositiveDurationFromUnits(t *testing.T) {
	for _, tc := range []struct {
		name     string
		units    int
		unit     time.Duration
		fallback time.Duration
		want     time.Duration
		valid    bool
	}{
		{name: "positive seconds", units: 30, unit: time.Second, fallback: time.Minute, want: 30 * time.Second, valid: true},
		{name: "zero", unit: time.Second, fallback: time.Minute, want: time.Minute},
		{name: "negative", units: -1, unit: time.Second, fallback: time.Minute, want: time.Minute},
		{name: "zero unit", units: 1, fallback: time.Minute, want: time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, valid := positiveDurationFromUnits(tc.units, tc.unit, tc.fallback)
			if got != tc.want || valid != tc.valid {
				t.Fatalf("positiveDurationFromUnits() = (%v, %v), want (%v, %v)", got, valid, tc.want, tc.valid)
			}
		})
	}

	maxInt := int(^uint(0) >> 1)
	for _, tc := range []struct {
		name string
		unit time.Duration
	}{
		{name: "seconds", unit: time.Second},
		{name: "minutes", unit: time.Minute},
		{name: "hours", unit: time.Hour},
	} {
		t.Run(tc.name+" maximum", func(t *testing.T) {
			maxUnits := uint64(1<<63-1) / uint64(tc.unit)
			if maxUnits > uint64(maxInt) {
				t.Skip("maximum representable duration exceeds int range")
			}
			want := time.Duration(maxUnits) * tc.unit
			got, valid := positiveDurationFromUnits(int(maxUnits), tc.unit, time.Minute)
			if got != want || !valid {
				t.Fatalf("positiveDurationFromUnits(max) = (%v, %v), want (%v, true)", got, valid, want)
			}
		})

		t.Run(tc.name+" overflow", func(t *testing.T) {
			maxUnits := uint64(1<<63-1) / uint64(tc.unit)
			if maxUnits >= uint64(maxInt) {
				t.Skip("overflowing duration exceeds int range")
			}
			fallback := time.Minute
			got, valid := positiveDurationFromUnits(int(maxUnits+1), tc.unit, fallback)
			if got != fallback || valid {
				t.Fatalf("positiveDurationFromUnits(overflow) = (%v, %v), want (%v, false)", got, valid, fallback)
			}
		})
	}
}

func TestResolveHealthAddr(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		flag       string
		changed    bool
		want       string
	}{
		{name: "config overrides flag default", configured: ":18686", flag: ":8686", want: ":18686"},
		{name: "explicit flag overrides config", configured: ":18686", flag: ":28686", changed: true, want: ":28686"},
		{name: "default without config", flag: ":8686", want: ":8686"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveHealthAddr(tt.configured, tt.flag, tt.changed); got != tt.want {
				t.Fatalf("resolveHealthAddr() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLabelComparisonName(t *testing.T) {
	tests := []struct {
		name string
		item *jellyfin.Item
		want string
	}{
		{
			name: "episode uses series identity",
			item: &jellyfin.Item{Type: "Episode", Name: "Pilot", SeriesName: "President Curtis"},
			want: "President Curtis",
		},
		{
			name: "episode without series name falls back to item",
			item: &jellyfin.Item{Type: "Episode", Name: "Show S01E01"},
			want: "Show S01E01",
		},
		{
			name: "movie uses item name",
			item: &jellyfin.Item{Type: "Movie", Name: "The Odyssey"},
			want: "The Odyssey",
		},
		{
			name: "nil item",
			item: nil,
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := labelComparisonName(tc.item); got != tc.want {
				t.Fatalf("labelComparisonName() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestShouldWarnMissingTMDBCorrectionKey(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		key     string
		want    bool
	}{
		{name: "enabled without key", enabled: true, want: true},
		{name: "enabled with whitespace key", enabled: true, key: "  ", want: true},
		{name: "enabled with key", enabled: true, key: "tmdb-key", want: false},
		{name: "disabled without key", enabled: false, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldWarnMissingTMDBCorrectionKey(tc.enabled, tc.key); got != tc.want {
				t.Fatalf("shouldWarnMissingTMDBCorrectionKey() = %v, want %v", got, tc.want)
			}
		})
	}
}
