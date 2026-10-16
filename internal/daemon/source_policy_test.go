package daemon

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/naming"
	"github.com/Nomadcxx/plex2jellyfin/internal/organizer"
)

func TestSourcePolicyResolvesSeedingAndDeleteSource(t *testing.T) {
	root := t.TempDir()
	ordinaryTV := filepath.Join(root, "ordinary", "tv")
	ordinaryMovies := filepath.Join(root, "ordinary", "movies")
	seedingTV := filepath.Join(root, "seeding", "tv")
	seedingMovies := filepath.Join(root, "seeding", "movies")
	watch := config.WatchConfig{
		TV:            []string{ordinaryTV},
		Movies:        []string{ordinaryMovies},
		SeedingTV:     []string{seedingTV},
		SeedingMovies: []string{seedingMovies},
	}

	for _, tt := range []struct {
		name         string
		deleteSource bool
		path         string
		want         organizer.SourceTransferMode
	}{
		{name: "ordinary move", deleteSource: true, path: filepath.Join(ordinaryTV, "show.mkv"), want: organizer.TransferMove},
		{name: "ordinary copy", deleteSource: false, path: filepath.Join(ordinaryMovies, "movie.mkv"), want: organizer.TransferCopy},
		{name: "seeding tv", deleteSource: true, path: filepath.Join(seedingTV, "show.mkv"), want: organizer.TransferSeed},
		{name: "seeding movie ignores delete source", deleteSource: false, path: filepath.Join(seedingMovies, "movie.mkv"), want: organizer.TransferSeed},
		{name: "adjacent prefix", deleteSource: true, path: filepath.Join(root, "ordinary", "tv2", "show.mkv"), want: organizer.TransferCopy},
		{name: "unknown is non-destructive", deleteSource: true, path: filepath.Join(root, "elsewhere", "movie.mkv"), want: organizer.TransferCopy},
	} {
		t.Run(tt.name, func(t *testing.T) {
			policy := newSourcePolicy(watch, tt.deleteSource)
			if got := policy.Resolve(tt.path); got != tt.want {
				t.Fatalf("Resolve(%q) = %d, want %d", tt.path, got, tt.want)
			}
		})
	}
}

func TestSourcePolicyPathsAreCleanCopiesAndUnique(t *testing.T) {
	root := t.TempDir()
	tv := filepath.Join(root, "tv")
	movies := filepath.Join(root, "movies")
	policy := newSourcePolicy(config.WatchConfig{
		TV:            []string{tv, filepath.Join(tv, ".")},
		Movies:        []string{movies},
		SeedingTV:     []string{filepath.Join(root, "seed-tv")},
		SeedingMovies: []string{filepath.Join(root, "seed-movies")},
	}, true)

	all := policy.AllPaths()
	if len(all) != 4 {
		t.Fatalf("AllPaths() = %v, want four unique roots", all)
	}
	all[0] = "/mutated"
	if policy.AllPaths()[0] == "/mutated" {
		t.Fatal("AllPaths returned aliased snapshot storage")
	}
	if got := policy.SourceHint(filepath.Join(root, "seed-tv", "Show.S01E01.mkv")); got.String() != "tv" {
		t.Fatalf("seeding TV hint = %q, want tv", got.String())
	}
	if got := policy.SourceHint(filepath.Join(root, "seed-movies", "Movie.2026.mkv")); got.String() != "movie" {
		t.Fatalf("seeding movie hint = %q, want movie", got.String())
	}
}

func TestSourcePolicyConcurrentResolveAndUpdate(t *testing.T) {
	root := t.TempDir()
	policy := newSourcePolicy(config.WatchConfig{TV: []string{filepath.Join(root, "old")}}, true)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				_ = policy.Resolve(filepath.Join(root, "old", "Show.S01E01.mkv"))
				_ = policy.AllPaths()
			}
		}()
	}
	for i := 0; i < 200; i++ {
		policy.Update(config.WatchConfig{
			TV:        []string{filepath.Join(root, "old")},
			SeedingTV: []string{filepath.Join(root, "seed")},
		}, i%2 == 0)
	}
	wg.Wait()
}

func TestMediaHandlerReplaceSourcePolicyUpdatesHintsAndTransferMode(t *testing.T) {
	root := t.TempDir()
	oldTV := filepath.Join(root, "old-tv")
	newSeedTV := filepath.Join(root, "new-seed-tv")
	handler, err := NewMediaHandler(MediaHandlerConfig{
		TVLibraries:  []string{filepath.Join(root, "tv-library")},
		MovieLibs:    []string{filepath.Join(root, "movie-library")},
		Watch:        config.WatchConfig{TV: []string{oldTV}},
		DeleteSource: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	oldFile := filepath.Join(oldTV, "Show.S01E01.mkv")
	if got := handler.sourcePolicy.Resolve(oldFile); got != organizer.TransferMove {
		t.Fatalf("old mode = %d, want move", got)
	}

	handler.ReplaceSourcePolicy(config.WatchConfig{SeedingTV: []string{newSeedTV}}, false)
	newFile := filepath.Join(newSeedTV, "Show.S01E01.mkv")
	if got := handler.sourcePolicy.Resolve(newFile); got != organizer.TransferSeed {
		t.Fatalf("new mode = %d, want seed", got)
	}
	if got := handler.getSourceHint(newFile); got != naming.SourceTV {
		t.Fatalf("new hint = %q, want tv", got.String())
	}
	if got := handler.sourcePolicy.Resolve(oldFile); got != organizer.TransferCopy {
		t.Fatalf("removed old root mode = %d, want safe unknown copy", got)
	}
}
