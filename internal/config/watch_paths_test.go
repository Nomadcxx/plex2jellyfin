package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestWatchConfigSeedingPathsRoundTrip(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Watch.TV = []string{"/downloads/usenet/tv"}
	cfg.Watch.Movies = []string{"/downloads/usenet/movies"}
	cfg.Watch.SeedingTV = []string{"/downloads/torrents/tv"}
	cfg.Watch.SeedingMovies = []string{"/downloads/torrents/movies"}

	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(cfg.ToTOML()), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Watch, cfg.Watch) {
		t.Fatalf("watch config round trip = %+v, want %+v", loaded.Watch, cfg.Watch)
	}
}

func TestWatchConfigLegacyTOMLDefaultsSeedingPathsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	data := []byte("[watch]\ntv = [\"/watch/tv\"]\nmovies = [\"/watch/movies\"]\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Watch.SeedingTV) != 0 || len(loaded.Watch.SeedingMovies) != 0 {
		t.Fatalf("legacy config gained seeding paths: %+v", loaded.Watch)
	}
}

func TestWatchConfigPathHelpersReturnCopies(t *testing.T) {
	w := WatchConfig{
		TV:            []string{"/ordinary/tv"},
		Movies:        []string{"/ordinary/movies"},
		SeedingTV:     []string{"/seeding/tv"},
		SeedingMovies: []string{"/seeding/movies"},
	}

	tv := w.TVPaths()
	movies := w.MoviePaths()
	all := w.AllPaths()
	if !reflect.DeepEqual(tv, []string{"/ordinary/tv", "/seeding/tv"}) {
		t.Fatalf("TVPaths = %v", tv)
	}
	if !reflect.DeepEqual(movies, []string{"/ordinary/movies", "/seeding/movies"}) {
		t.Fatalf("MoviePaths = %v", movies)
	}
	if !reflect.DeepEqual(all, []string{"/ordinary/movies", "/seeding/movies", "/ordinary/tv", "/seeding/tv"}) {
		t.Fatalf("AllPaths = %v", all)
	}
	tv[0] = "/mutated"
	if w.TV[0] != "/ordinary/tv" {
		t.Fatal("TVPaths returned aliased storage")
	}
}

func TestPathContainsUsesFilesystemBoundaries(t *testing.T) {
	tests := []struct {
		name      string
		root      string
		candidate string
		want      bool
	}{
		{name: "same", root: "/downloads/tv", candidate: "/downloads/tv", want: true},
		{name: "child", root: "/downloads/tv", candidate: "/downloads/tv/release/episode.mkv", want: true},
		{name: "adjacent prefix", root: "/downloads/tv", candidate: "/downloads/tv2/episode.mkv", want: false},
		{name: "parent", root: "/downloads/tv", candidate: "/downloads", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PathContains(tt.root, tt.candidate); got != tt.want {
				t.Fatalf("PathContains(%q, %q) = %v, want %v", tt.root, tt.candidate, got, tt.want)
			}
		})
	}
}

func TestValidateWatchLayoutRejectsConflictingRoots(t *testing.T) {
	tests := []struct {
		name      string
		watch     WatchConfig
		libraries LibrariesConfig
	}{
		{
			name:  "ordinary and seeding overlap",
			watch: WatchConfig{TV: []string{"/downloads/tv"}, SeedingTV: []string{"/downloads/tv/seeding"}},
		},
		{
			name:  "media types share root",
			watch: WatchConfig{TV: []string{"/downloads/shared"}, Movies: []string{"/downloads/shared"}},
		},
		{
			name:      "watch inside library",
			watch:     WatchConfig{TV: []string{"/media/tv/incoming"}},
			libraries: LibrariesConfig{TV: []string{"/media/tv"}},
		},
		{
			name:      "library inside watch",
			watch:     WatchConfig{Movies: []string{"/media"}},
			libraries: LibrariesConfig{Movies: []string{"/media/movies"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateWatchLayout(tt.watch, tt.libraries); err == nil {
				t.Fatal("expected conflicting layout error")
			}
		})
	}
}

func TestValidateWatchLayoutAllowsAdjacentPrefixes(t *testing.T) {
	watch := WatchConfig{
		TV:            []string{"/downloads/tv"},
		SeedingTV:     []string{"/downloads/tv2"},
		Movies:        []string{"/downloads/movies"},
		SeedingMovies: []string{"/downloads/movies2"},
	}
	libraries := LibrariesConfig{TV: []string{"/media/tv"}, Movies: []string{"/media/movies"}}
	if err := ValidateWatchLayout(watch, libraries); err != nil {
		t.Fatalf("adjacent prefixes rejected: %v", err)
	}
}
