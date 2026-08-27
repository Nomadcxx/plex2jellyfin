package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	configpkg "github.com/Nomadcxx/plex2jellyfin/internal/config"
)

func TestDefaultWatchFolders_ExposeFixedSeedingEntries(t *testing.T) {
	folders := defaultWatchFolders()

	if len(folders) != fixedWatchFolderCount {
		t.Fatalf("defaultWatchFolders() = %d entries, want %d", len(folders), fixedWatchFolderCount)
	}

	want := []struct {
		mediaType string
		seeding   bool
	}{
		{"tv", false},
		{"movies", false},
		{"tv", true},
		{"movies", true},
	}
	for i, w := range want {
		got := folders[i]
		if got.Type != w.mediaType || got.Seeding != w.seeding {
			t.Errorf("folder %d = {Type:%q Seeding:%t}, want {Type:%q Seeding:%t}",
				i, got.Type, got.Seeding, w.mediaType, w.seeding)
		}
		if strings.TrimSpace(got.Label) == "" {
			t.Errorf("folder %d has no label", i)
		}
		if got.Seeding && !strings.Contains(strings.ToLower(got.Label), "seed") {
			t.Errorf("seeding folder %d label %q does not name its policy", i, got.Label)
		}
	}
}

func TestWatchFolderMutation_ProtectsFixedEntries(t *testing.T) {
	m := model{watchFolders: defaultWatchFolders()}

	added, _ := m.addWatchFolder()
	m = added.(model)
	if len(m.watchFolders) != fixedWatchFolderCount+1 {
		t.Fatalf("addWatchFolder() left %d folders, want %d", len(m.watchFolders), fixedWatchFolderCount+1)
	}
	if custom := m.watchFolders[fixedWatchFolderCount]; custom.Seeding {
		t.Errorf("custom folder must stay an ordinary root, got Seeding=true")
	}

	removed, _ := m.removeWatchFolder()
	m = removed.(model)
	if len(m.watchFolders) != fixedWatchFolderCount {
		t.Fatalf("removeWatchFolder() left %d folders, want %d", len(m.watchFolders), fixedWatchFolderCount)
	}

	// The four fixed entries must survive any further removal attempts.
	for i := 0; i < 3; i++ {
		next, _ := m.removeWatchFolder()
		m = next.(model)
	}
	if len(m.watchFolders) != fixedWatchFolderCount {
		t.Fatalf("fixed folders were removable: %d remain, want %d", len(m.watchFolders), fixedWatchFolderCount)
	}
	if !m.watchFolders[2].Seeding || !m.watchFolders[3].Seeding {
		t.Fatalf("seeding folders lost their policy flag: %+v", m.watchFolders)
	}
}

func TestGenerateConfigString_RoutesSeedingRootsThroughConfigLoad(t *testing.T) {
	m := &model{
		watchFolders: []WatchFolder{
			{Label: "TV Shows", Type: "tv", Paths: "/watch/tv"},
			{Label: "Movies", Type: "movies", Paths: "/watch/movies"},
			{Label: "TV Seeding", Type: "tv", Seeding: true, Paths: "/seed/tv"},
			{Label: "Movie Seeding", Type: "movies", Seeding: true, Paths: "/seed/movies"},
		},
		tvLibraryPaths:    "/lib/tv",
		movieLibraryPaths: "/lib/movies",
		serviceEnabled:    true,
	}

	configStr, err := m.generateConfigString()
	if err != nil {
		t.Fatalf("generateConfigString() error = %v", err)
	}

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, ".config"))
	t.Setenv("SUDO_USER", "")
	dir := filepath.Join(tmp, ".config", "plex2jellyfin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(configStr), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := configpkg.Load()
	if err != nil {
		t.Fatalf("config.Load() on generated config: %v\n%s", err, configStr)
	}

	assertPaths := func(label string, got, want []string) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s = %v, want %v", label, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s = %v, want %v", label, got, want)
			}
		}
	}
	assertPaths("watch.tv", cfg.Watch.TV, []string{"/watch/tv"})
	assertPaths("watch.movies", cfg.Watch.Movies, []string{"/watch/movies"})
	assertPaths("watch.seeding_tv", cfg.Watch.SeedingTV, []string{"/seed/tv"})
	assertPaths("watch.seeding_movies", cfg.Watch.SeedingMovies, []string{"/seed/movies"})
}

func TestMediaPathsError_AcceptsSeedingOnlyIntake(t *testing.T) {
	m := model{
		watchFolders: []WatchFolder{
			{Label: "TV Shows", Type: "tv"},
			{Label: "Movies", Type: "movies"},
			{Label: "TV Seeding", Type: "tv", Seeding: true, Paths: "/seed/tv"},
			{Label: "Movie Seeding", Type: "movies", Seeding: true},
		},
		tvLibraryPaths: "/lib/tv",
	}

	if err := m.mediaPathsError(); err != "" {
		t.Fatalf("seeding-only TV intake with a TV library must pass, got %q", err)
	}
}

func TestMediaPathsError_RequiresLibraryForSeedingIntake(t *testing.T) {
	m := model{
		watchFolders: []WatchFolder{
			{Label: "TV Seeding", Type: "tv", Seeding: true, Paths: "/seed/tv"},
		},
	}

	if err := m.mediaPathsError(); err == "" {
		t.Fatal("seeding TV intake without a TV library must fail")
	}
}

func TestMediaPathsError_RejectsConflictingRootsThroughSharedValidation(t *testing.T) {
	cases := []struct {
		name    string
		folders []WatchFolder
		libTV   string
		libMov  string
	}{
		{
			name: "ordinary tv contains seeding movies",
			folders: []WatchFolder{
				{Type: "tv", Paths: "/downloads"},
				{Type: "movies", Seeding: true, Paths: "/downloads/movies"},
			},
			libTV:  "/lib/tv",
			libMov: "/lib/movies",
		},
		{
			name: "same media split across policies",
			folders: []WatchFolder{
				{Type: "tv", Paths: "/downloads/tv"},
				{Type: "tv", Seeding: true, Paths: "/downloads/tv"},
			},
			libTV: "/lib/tv",
		},
		{
			name: "seeding root inside a library",
			folders: []WatchFolder{
				{Type: "tv", Seeding: true, Paths: "/lib/tv/incoming"},
			},
			libTV: "/lib/tv",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := model{
				watchFolders:      tc.folders,
				tvLibraryPaths:    tc.libTV,
				movieLibraryPaths: tc.libMov,
			}
			if err := m.mediaPathsError(); err == "" {
				t.Fatal("expected shared layout validation to block conflicting roots")
			}
		})
	}
}

func TestRenderConfirm_MarksSeedingPathsRetained(t *testing.T) {
	m := model{
		watchFolders: []WatchFolder{
			{Label: "TV Shows", Type: "tv", Paths: "/watch/tv"},
			{Label: "TV Seeding Completions", Type: "tv", Seeding: true, Paths: "/seed/tv"},
		},
		tvLibraryPaths: "/lib/tv",
	}

	out := m.renderConfirm()

	if !strings.Contains(out, "/seed/tv") {
		t.Fatalf("confirmation omits the seeding path:\n%s", out)
	}
	if !strings.Contains(strings.ToLower(out), "retain") {
		t.Fatalf("confirmation does not mark seeding paths as retained:\n%s", out)
	}

	seedIdx := strings.Index(out, "/seed/tv")
	watchIdx := strings.Index(out, "/watch/tv")
	retainIdx := strings.Index(strings.ToLower(out), "retain")
	if !(watchIdx < retainIdx && retainIdx < seedIdx) {
		t.Fatalf("seeding paths are not separated from ordinary watch folders:\n%s", out)
	}
}
