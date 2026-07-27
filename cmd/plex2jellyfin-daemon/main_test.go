package main

import (
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin"
)

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
