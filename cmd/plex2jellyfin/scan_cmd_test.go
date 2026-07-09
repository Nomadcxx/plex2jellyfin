package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/database"
)

func TestConfiguredLibraryForPathDetectsMovieRoot(t *testing.T) {
	root := t.TempDir()
	movieRoot := filepath.Join(root, "MOVIES")
	tvRoot := filepath.Join(root, "TVSHOWS")
	path := filepath.Join(movieRoot, "Example (2026)", "Example (2026).mkv")

	cfg := &config.Config{}
	cfg.Libraries.Movies = []string{movieRoot}
	cfg.Libraries.TV = []string{tvRoot}

	gotRoot, gotType, err := configuredLibraryForPath(path, cfg)
	if err != nil {
		t.Fatalf("configuredLibraryForPath failed: %v", err)
	}
	if gotRoot != movieRoot {
		t.Fatalf("root = %q, want %q", gotRoot, movieRoot)
	}
	if gotType != "movie" {
		t.Fatalf("media type = %q, want movie", gotType)
	}
}

func TestConfiguredLibraryForPathRejectsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{}
	cfg.Libraries.Movies = []string{filepath.Join(root, "MOVIES")}

	_, _, err := configuredLibraryForPath(filepath.Join(root, "Downloads", "Example.mkv"), cfg)
	if err == nil {
		t.Fatalf("expected path outside configured roots to be rejected")
	}
}

func TestNewScanCmdMassProcessingFlags(t *testing.T) {
	cmd := newScanCmd()

	analyze := cmd.Flags().Lookup("analyze")
	if analyze == nil {
		t.Fatal("expected scan --analyze flag")
	}
	if analyze.DefValue != "false" {
		t.Fatalf("expected --analyze default false, got %q", analyze.DefValue)
	}

	jsonFlag := cmd.Flags().Lookup("json")
	if jsonFlag == nil {
		t.Fatal("expected scan --json flag")
	}
	if jsonFlag.DefValue != "false" {
		t.Fatalf("expected --json default false, got %q", jsonFlag.DefValue)
	}
}

func TestShouldRunPostScanAnalysisRequiresStatsAndAnalyze(t *testing.T) {
	if shouldRunPostScanAnalysis(true, false) {
		t.Fatal("scan should not run duplicate/scattered analysis unless --analyze is set")
	}
	if shouldRunPostScanAnalysis(false, true) {
		t.Fatal("scan should not run analysis when stats output is disabled")
	}
	if !shouldRunPostScanAnalysis(true, true) {
		t.Fatal("scan should run analysis when stats and analyze are both enabled")
	}
}

func TestPruneStaleFilesystemMovieRowsFullScan(t *testing.T) {
	db, err := database.OpenPath(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatalf("OpenPath failed: %v", err)
	}
	defer db.Close()

	stale := &database.Movie{
		Title:          "Bad Parse REMASTERED",
		Year:           2025,
		CanonicalPath:  "/movies/Bad Parse (2025)",
		LibraryRoot:    "/movies",
		Source:         "filesystem",
		SourcePriority: 50,
	}
	if _, err := db.UpsertMovie(stale); err != nil {
		t.Fatalf("UpsertMovie stale failed: %v", err)
	}

	kept := &database.Movie{
		Title:          "Kept Movie",
		Year:           2025,
		CanonicalPath:  "/movies/Kept Movie (2025)",
		LibraryRoot:    "/movies",
		Source:         "filesystem",
		SourcePriority: 50,
	}
	if _, err := db.UpsertMovie(kept); err != nil {
		t.Fatalf("UpsertMovie kept failed: %v", err)
	}
	year := 2025
	if err := db.UpsertMediaFile(&database.MediaFile{
		Path:            "/movies/Kept Movie (2025)/Kept Movie (2025).mkv",
		Size:            100,
		MediaType:       "movie",
		ParentMovieID:   &kept.ID,
		NormalizedTitle: "keptmovie",
		Year:            &year,
	}); err != nil {
		t.Fatalf("UpsertMediaFile failed: %v", err)
	}

	pruned, err := pruneStaleFilesystemMovieRows(db, "")
	if err != nil {
		t.Fatalf("pruneStaleFilesystemMovieRows failed: %v", err)
	}
	if pruned != 1 {
		t.Fatalf("pruned = %d, want 1", pruned)
	}
	if got, err := db.GetMovieByID(stale.ID); err != nil {
		t.Fatalf("GetMovieByID stale failed: %v", err)
	} else if got != nil {
		t.Fatal("stale full-scan movie row should be pruned")
	}
	if got, err := db.GetMovieByID(kept.ID); err != nil {
		t.Fatalf("GetMovieByID kept failed: %v", err)
	} else if got == nil {
		t.Fatal("movie with media files should remain")
	}
}

func TestPruneMissingMediaRows(t *testing.T) {
	db, err := database.OpenPath(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatalf("OpenPath failed: %v", err)
	}
	defer db.Close()

	root := t.TempDir()
	existingPath := filepath.Join(root, "Existing (2025).mkv")
	if err := os.WriteFile(existingPath, []byte("media"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	year := 2025
	for _, file := range []*database.MediaFile{
		{
			Path:            existingPath,
			Size:            5,
			MediaType:       "movie",
			NormalizedTitle: "existing",
			Year:            &year,
		},
		{
			Path:            filepath.Join(root, "Missing (2025).mkv"),
			Size:            5,
			MediaType:       "movie",
			NormalizedTitle: "missing",
			Year:            &year,
		},
	} {
		if err := db.UpsertMediaFile(file); err != nil {
			t.Fatalf("UpsertMediaFile(%s) failed: %v", file.Path, err)
		}
	}

	pruned, err := pruneMissingMediaRows(db)
	if err != nil {
		t.Fatalf("pruneMissingMediaRows failed: %v", err)
	}
	if pruned != 1 {
		t.Fatalf("pruned = %d, want 1", pruned)
	}
	if got, err := db.GetMediaFile(existingPath); err != nil {
		t.Fatalf("GetMediaFile existing failed: %v", err)
	} else if got == nil {
		t.Fatal("existing media file row should remain")
	}
}
