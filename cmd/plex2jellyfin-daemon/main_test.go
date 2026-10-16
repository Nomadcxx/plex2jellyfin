package main

import (
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin"
)

func TestPositiveTickerInterval(t *testing.T) {
	for _, tc := range []struct {
		name     string
		interval time.Duration
		fallback time.Duration
		want     time.Duration
	}{
		{name: "positive", interval: time.Minute, fallback: 5 * time.Minute, want: time.Minute},
		{name: "zero", fallback: 5 * time.Minute, want: 5 * time.Minute},
		{name: "negative", interval: -time.Second, fallback: 5 * time.Minute, want: 5 * time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := positiveTickerInterval(tc.interval, tc.fallback); got != tc.want {
				t.Fatalf("positiveTickerInterval() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPositiveTickerIntervalSeconds(t *testing.T) {
	for _, tc := range []struct {
		name     string
		seconds  int
		fallback time.Duration
		want     time.Duration
	}{
		{name: "positive", seconds: 30, fallback: time.Minute, want: 30 * time.Second},
		{name: "zero", fallback: time.Minute, want: time.Minute},
		{name: "negative", seconds: -1, fallback: time.Minute, want: time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := positiveTickerIntervalSeconds(tc.seconds, tc.fallback); got != tc.want {
				t.Fatalf("positiveTickerIntervalSeconds() = %v, want %v", got, tc.want)
			}
		})
	}

	maxInt := int(^uint(0) >> 1)
	if uint64(maxInt) > uint64((1<<63-1)/int64(time.Second)) {
		fallback := time.Minute
		if got := positiveTickerIntervalSeconds(maxInt, fallback); got != fallback {
			t.Fatalf("positiveTickerIntervalSeconds(max int) = %v, want %v", got, fallback)
		}
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
