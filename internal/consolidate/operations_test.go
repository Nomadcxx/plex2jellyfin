package consolidate

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/database"
)

func TestConsolidator_GenerateAllPlans_ExcludesMovies(t *testing.T) {
	tempDir := filepath.Join(t.TempDir(), "test.db")
	db, err := database.OpenPath(tempDir)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cfg := &config.Config{
		Options: config.OptionsConfig{
			DryRun:          false,
			VerifyChecksums: false,
			DeleteSource:    true,
		},
	}

	c := NewConsolidator(db, cfg)

	conflicts := []struct {
		mediaType string
		title     string
		year      int
		locations []string
	}{
		{"series", "TV Show 1", 2020, []string{"/path1/show1", "/path2/show1"}},
		{"series", "TV Show 2", 2021, []string{"/path1/show2", "/path2/show2"}},
		{"series", "TV Show 3", 2022, []string{"/path1/show3", "/path2/show3"}},
		{"movie", "Movie 1", 2018, []string{"/path1/movie1", "/path2/movie1"}},
		{"movie", "Movie 2", 2019, []string{"/path1/movie2", "/path2/movie2"}},
	}

	for _, conflict := range conflicts {
		_, err := db.DB().Exec(`
			INSERT INTO conflicts (media_type, title, title_normalized, year, locations, created_at)
			VALUES (?, ?, ?, ?, ?, datetime('now'))
		`, conflict.mediaType, conflict.title, database.NormalizeTitle(conflict.title), conflict.year,
			`["`+conflict.locations[0]+`","`+conflict.locations[1]+`"]`)
		if err != nil {
			t.Fatalf("Failed to insert conflict: %v", err)
		}
	}

	plans, err := c.GenerateAllPlans()
	if err != nil {
		t.Fatalf("GenerateAllPlans failed: %v", err)
	}

	if len(plans) != 3 {
		t.Errorf("Expected 3 plans (TV shows only), got %d", len(plans))
	}

	for _, plan := range plans {
		if plan.MediaType != "series" {
			t.Errorf("Expected all plans to be for series, got %s", plan.MediaType)
		}
	}

	if c.stats.SkippedConflicts != 2 {
		t.Errorf("Expected 2 skipped conflicts (movies), got %d", c.stats.SkippedConflicts)
	}

	if c.stats.ConflictsFound != 5 {
		t.Errorf("Expected 5 conflicts found, got %d", c.stats.ConflictsFound)
	}
}

func TestConsolidator_GenerateAllPlans_AllSeries(t *testing.T) {
	tempDir := filepath.Join(t.TempDir(), "test.db")
	db, err := database.OpenPath(tempDir)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cfg := &config.Config{}
	c := NewConsolidator(db, cfg)

	conflicts := []struct {
		mediaType string
		title     string
		year      int
		locations []string
	}{
		{"series", "TV Show 1", 2020, []string{"/path1/show1", "/path2/show1"}},
		{"series", "TV Show 2", 2021, []string{"/path1/show2", "/path2/show2"}},
	}

	for _, conflict := range conflicts {
		_, err := db.DB().Exec(`
			INSERT INTO conflicts (media_type, title, title_normalized, year, locations, created_at)
			VALUES (?, ?, ?, ?, ?, datetime('now'))
		`, conflict.mediaType, conflict.title, database.NormalizeTitle(conflict.title), conflict.year,
			`["`+conflict.locations[0]+`","`+conflict.locations[1]+`"]`)
		if err != nil {
			t.Fatalf("Failed to insert conflict: %v", err)
		}
	}

	plans, err := c.GenerateAllPlans()
	if err != nil {
		t.Fatalf("GenerateAllPlans failed: %v", err)
	}

	if len(plans) != 2 {
		t.Errorf("Expected 2 plans, got %d", len(plans))
	}

	if c.stats.SkippedConflicts != 0 {
		t.Errorf("Expected 0 skipped conflicts, got %d", c.stats.SkippedConflicts)
	}
}

func TestConsolidator_GenerateAllPlans_AllMovies(t *testing.T) {
	tempDir := filepath.Join(t.TempDir(), "test.db")
	db, err := database.OpenPath(tempDir)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cfg := &config.Config{}
	c := NewConsolidator(db, cfg)

	conflicts := []struct {
		mediaType string
		title     string
		year      int
		locations []string
	}{
		{"movie", "Movie 1", 2018, []string{"/path1/movie1", "/path2/movie1"}},
		{"movie", "Movie 2", 2019, []string{"/path1/movie2", "/path2/movie2"}},
		{"movie", "Movie 3", 2020, []string{"/path1/movie3", "/path2/movie3"}},
		{"movie", "Movie 4", 2021, []string{"/path1/movie4", "/path2/movie4"}},
		{"movie", "Movie 5", 2022, []string{"/path1/movie5", "/path2/movie5"}},
	}

	for _, conflict := range conflicts {
		_, err := db.DB().Exec(`
			INSERT INTO conflicts (media_type, title, title_normalized, year, locations, created_at)
			VALUES (?, ?, ?, ?, ?, datetime('now'))
		`, conflict.mediaType, conflict.title, database.NormalizeTitle(conflict.title), conflict.year,
			`["`+conflict.locations[0]+`","`+conflict.locations[1]+`"]`)
		if err != nil {
			t.Fatalf("Failed to insert conflict: %v", err)
		}
	}

	plans, err := c.GenerateAllPlans()
	if err != nil {
		t.Fatalf("GenerateAllPlans failed: %v", err)
	}

	if len(plans) != 0 {
		t.Errorf("Expected 0 plans (all movies), got %d", len(plans))
	}

	if c.stats.SkippedConflicts != 5 {
		t.Errorf("Expected 5 skipped conflicts (all movies), got %d", c.stats.SkippedConflicts)
	}

	if c.stats.ConflictsFound != 5 {
		t.Errorf("Expected 5 conflicts found, got %d", c.stats.ConflictsFound)
	}
}

func TestExecutorOutput(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		args     []interface{}
		expected string
	}{
		{
			name:     "simple message",
			format:   "Test message\n",
			args:     nil,
			expected: "Test message\n",
		},
		{
			name:     "message with string argument",
			format:   "Hello %s\n",
			args:     []interface{}{"World"},
			expected: "Hello World\n",
		},
		{
			name:     "message with multiple arguments",
			format:   "Files: %d, Size: %d bytes\n",
			args:     []interface{}{5, 1024},
			expected: "Files: 5, Size: 1024 bytes\n",
		},
		{
			name:     "warning message format",
			format:   "Warning: Failed to cleanup empty directory %s: %v\n",
			args:     []interface{}{"/path/to/dir", "permission denied"},
			expected: "Warning: Failed to cleanup empty directory /path/to/dir: permission denied\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a buffer to capture output
			buf := &bytes.Buffer{}

			// Create executor with our buffer as writer
			// We can pass nil for db since we're only testing Printf
			executor := &Executor{
				writer: buf,
			}

			// Call Printf with the test format and args
			executor.Printf(tt.format, tt.args...)

			// Verify the output
			got := buf.String()
			if got != tt.expected {
				t.Errorf("Printf() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestExecutorPrintfWithNilWriter(t *testing.T) {
	// This test documents that Printf will panic with nil writer
	// NewExecutor ensures writer is never nil by defaulting to os.Stdout
	executor := &Executor{
		writer: nil,
	}

	// Expect panic with nil writer - this is standard fmt.Fprintf behavior
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Printf() with nil writer should have panicked, but didn't")
		}
	}()

	executor.Printf("Test message\n")
}

func TestExecutePlanFailsEarlyWhenTargetNotWritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write through directory mode restrictions")
	}

	tempDir := t.TempDir()
	target := filepath.Join(tempDir, "target")
	source := filepath.Join(tempDir, "source.mkv")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatalf("failed to create target: %v", err)
	}
	if err := os.WriteFile(source, []byte("video"), 0644); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}
	if err := os.Chmod(target, 0555); err != nil {
		t.Fatalf("failed to make target read-only: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(target, 0755)
	})

	plan := &Plan{
		CanProceed: true,
		TargetPath: target,
		Operations: []*Operation{
			{
				SourcePath:      source,
				DestinationPath: filepath.Join(target, "Season 01", "source.mkv"),
				Size:            5,
			},
		},
	}
	consolidator := &Consolidator{cfg: &config.Config{}}

	err := consolidator.ExecutePlan(plan, false)
	if err == nil {
		t.Fatal("ExecutePlan returned nil, want target write error")
	}
	if !strings.Contains(err.Error(), "target path is not writable") {
		t.Fatalf("ExecutePlan error = %q, want target write error", err)
	}
	if strings.Contains(err.Error(), "operations failed") {
		t.Fatalf("ExecutePlan error = %q, should fail before per-operation execution", err)
	}
}

func TestExecutePlanChecksCapacityBeforeMovingFiles(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source", "movie.mkv")
	target := filepath.Join(root, "target")
	destination := filepath.Join(target, "movie.mkv")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("create source directory: %v", err)
	}
	if err := os.WriteFile(source, []byte("media"), 0o644); err != nil {
		t.Fatalf("create source: %v", err)
	}

	consolidator := NewConsolidator(nil, &config.Config{})
	consolidator.spaceAvailable = func(string) (int64, error) { return int64(len("media") - 1), nil }
	plan := &Plan{
		TargetPath: target,
		CanProceed: true,
		TotalBytes: int64(len("media")),
		Operations: []*Operation{{SourcePath: source, DestinationPath: destination, Size: int64(len("media")), Type: "movie"}},
		Collisions: []*Collision{{SourcePath: "existing duplicate"}},
	}

	if err := consolidator.ExecutePlan(plan, false); err == nil || !strings.Contains(err.Error(), "insufficient target space") {
		t.Fatalf("ExecutePlan error = %v, want insufficient target space", err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source changed before capacity check: %v", err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("destination exists before capacity check, stat error = %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target directory was created before capacity check, stat error = %v", err)
	}
}

func TestExecutePlanUpdatesDatabaseAfterMove(t *testing.T) {
	tempDir := t.TempDir()
	sourceRoot := filepath.Join(tempDir, "storage2", "TV")
	targetRoot := filepath.Join(tempDir, "storage1", "TV")
	sourceDir := filepath.Join(sourceRoot, "Silo (2023)", "Season 01")
	targetSeries := filepath.Join(targetRoot, "Silo (2023)")
	sourcePath := filepath.Join(sourceDir, "Silo S01E01.mkv")
	targetPath := filepath.Join(targetSeries, "Season 01", "Silo S01E01.mkv")

	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatalf("failed to create source dir: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("video"), 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	db, err := database.OpenPath(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	year := 2023
	series := &database.Series{
		Title:          "Silo",
		Year:           year,
		CanonicalPath:  filepath.Join(sourceRoot, "Silo (2023)"),
		LibraryRoot:    sourceRoot,
		Source:         "plex2jellyfin",
		SourcePriority: 100,
	}
	if _, err := db.UpsertSeries(series); err != nil {
		t.Fatalf("failed to insert series: %v", err)
	}
	episode := &database.Episode{SeriesID: series.ID, Season: 1, Episode: 1, Title: "First Episode"}
	if err := db.UpsertEpisode(episode); err != nil {
		t.Fatalf("failed to insert episode: %v", err)
	}

	file := &database.MediaFile{
		Path:            sourcePath,
		Size:            5,
		ModifiedAt:      time.Now(),
		MediaType:       "episode",
		ParentSeriesID:  &series.ID,
		ParentEpisodeID: &episode.ID,
		NormalizedTitle: "silo",
		Year:            &year,
		Season:          intPtr(1),
		Episode:         intPtr(1),
		Source:          "plex2jellyfin",
		SourcePriority:  100,
		LibraryRoot:     sourceRoot,
	}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("failed to insert media file: %v", err)
	}
	mediaFileID := file.ID
	if err := db.UpdateEpisodeBestFile(episode.ID, &mediaFileID); err != nil {
		t.Fatalf("failed to link episode best file: %v", err)
	}

	consolidator := NewConsolidator(db, &config.Config{
		Libraries: config.LibrariesConfig{TV: []string{targetRoot}},
		Options: config.OptionsConfig{
			VerifyChecksums: false,
		},
	})
	err = consolidator.ExecutePlan(&Plan{
		CanProceed: true,
		TargetPath: targetSeries,
		Operations: []*Operation{{
			SourcePath:      sourcePath,
			DestinationPath: targetPath,
			Size:            5,
			Type:            "series",
		}},
	}, false)
	if err != nil {
		t.Fatalf("ExecutePlan failed: %v", err)
	}

	if got, err := db.GetMediaFile(sourcePath); err != nil {
		t.Fatalf("GetMediaFile(source) failed: %v", err)
	} else if got != nil {
		t.Fatalf("source DB row still exists after move")
	}
	got, err := db.GetMediaFile(targetPath)
	if err != nil {
		t.Fatalf("GetMediaFile(target) failed: %v", err)
	}
	if got == nil {
		t.Fatalf("target DB row was not inserted after move")
	}
	if got.ID != mediaFileID {
		t.Fatalf("media file ID = %d after move, want preserved ID %d", got.ID, mediaFileID)
	}
	if got.LibraryRoot != targetRoot {
		t.Fatalf("LibraryRoot = %q, want %q", got.LibraryRoot, targetRoot)
	}
	updatedEpisode, err := db.GetEpisode(series.ID, 1, 1)
	if err != nil {
		t.Fatalf("GetEpisode failed: %v", err)
	}
	if updatedEpisode.BestFileID == nil || *updatedEpisode.BestFileID != mediaFileID {
		t.Fatalf("episode best file ID = %v, want %d", updatedEpisode.BestFileID, mediaFileID)
	}
	updatedSeries, err := db.GetSeriesByID(series.ID)
	if err != nil {
		t.Fatalf("GetSeriesByID failed: %v", err)
	}
	if updatedSeries.CanonicalPath != targetSeries {
		t.Fatalf("CanonicalPath = %q, want %q", updatedSeries.CanonicalPath, targetSeries)
	}
	if !updatedSeries.SonarrPathDirty {
		t.Fatalf("series should be marked dirty after consolidation path change")
	}
}

func TestExecutePlanPreservesMovieIdentityAfterMove(t *testing.T) {
	tempDir := t.TempDir()
	sourceRoot := filepath.Join(tempDir, "storage2", "Movies")
	targetRoot := filepath.Join(tempDir, "storage1", "Movies")
	sourceMovie := filepath.Join(sourceRoot, "Arrival (2016)")
	targetMovie := filepath.Join(targetRoot, "Arrival (2016)")
	sourcePath := filepath.Join(sourceMovie, "Arrival.mkv")
	targetPath := filepath.Join(targetMovie, "Arrival.mkv")
	if err := os.MkdirAll(sourceMovie, 0o755); err != nil {
		t.Fatalf("create source directory: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("video"), 0o644); err != nil {
		t.Fatalf("create source file: %v", err)
	}

	db, err := database.OpenPath(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	year := 2016
	movie := &database.Movie{
		Title: "Arrival", Year: year, CanonicalPath: sourceMovie,
		LibraryRoot: sourceRoot, Source: "plex2jellyfin", SourcePriority: 100,
	}
	if _, err := db.UpsertMovie(movie); err != nil {
		t.Fatalf("insert movie: %v", err)
	}
	file := &database.MediaFile{
		Path: sourcePath, Size: 5, ModifiedAt: time.Now(), MediaType: "movie",
		ParentMovieID: &movie.ID, NormalizedTitle: "arrival", Year: &year,
		Source: "plex2jellyfin", SourcePriority: 100, LibraryRoot: sourceRoot,
	}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("insert media file: %v", err)
	}
	mediaFileID := file.ID
	if err := db.UpdateMovieBestFile(movie.ID, &mediaFileID); err != nil {
		t.Fatalf("link movie best file: %v", err)
	}

	consolidator := NewConsolidator(db, &config.Config{
		Libraries: config.LibrariesConfig{Movies: []string{targetRoot}},
	})
	err = consolidator.ExecutePlan(&Plan{
		CanProceed: true,
		TargetPath: targetMovie,
		Operations: []*Operation{{
			SourcePath: sourcePath, DestinationPath: targetPath,
			Size: 5, Type: "movie",
		}},
	}, false)
	if err != nil {
		t.Fatalf("ExecutePlan: %v", err)
	}

	movedFile, err := db.GetMediaFile(targetPath)
	if err != nil {
		t.Fatalf("GetMediaFile(target): %v", err)
	}
	if movedFile == nil || movedFile.ID != mediaFileID {
		t.Fatalf("moved media row = %#v, want ID %d", movedFile, mediaFileID)
	}
	updatedMovie, err := db.GetMovieByID(movie.ID)
	if err != nil {
		t.Fatalf("GetMovieByID: %v", err)
	}
	if updatedMovie.CanonicalPath != targetMovie || !updatedMovie.RadarrPathDirty {
		t.Fatalf("movie canonical path/dirty = %q/%v, want %q/true", updatedMovie.CanonicalPath, updatedMovie.RadarrPathDirty, targetMovie)
	}
	var bestFileID int64
	if err := db.DB().QueryRow("SELECT best_file_id FROM movies WHERE id = ?", movie.ID).Scan(&bestFileID); err != nil {
		t.Fatalf("read movie best file ID: %v", err)
	}
	if bestFileID != mediaFileID {
		t.Fatalf("movie best file ID = %d, want %d", bestFileID, mediaFileID)
	}
}

func TestExecutePlanLeavesDatabaseUnchangedOnDestinationPathConflict(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "source")
	targetRoot := filepath.Join(root, "target")
	sourceSeriesPath := filepath.Join(sourceRoot, "Show (2023)")
	targetSeriesPath := filepath.Join(targetRoot, "Show (2023)")
	sourcePath := filepath.Join(sourceSeriesPath, "Season 01", "Show.S01E01.mkv")
	targetPath := filepath.Join(targetSeriesPath, "Season 01", "Show.S01E01.mkv")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("create source directory: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("source media"), 0o644); err != nil {
		t.Fatalf("create source file: %v", err)
	}

	db, err := database.OpenPath(filepath.Join(root, "test.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	series := &database.Series{Title: "Show", Year: 2023, CanonicalPath: sourceSeriesPath, LibraryRoot: sourceRoot, Source: "plex2jellyfin", SourcePriority: 100}
	if _, err := db.UpsertSeries(series); err != nil {
		t.Fatalf("insert source series: %v", err)
	}
	otherSeries := &database.Series{Title: "Other Show", Year: 2022, CanonicalPath: filepath.Join(targetRoot, "Other Show"), LibraryRoot: targetRoot, Source: "plex2jellyfin", SourcePriority: 100}
	if _, err := db.UpsertSeries(otherSeries); err != nil {
		t.Fatalf("insert target series: %v", err)
	}
	sourceFile := &database.MediaFile{Path: sourcePath, Size: 12, MediaType: "episode", ParentSeriesID: &series.ID, NormalizedTitle: "show", LibraryRoot: sourceRoot}
	if err := db.UpsertMediaFile(sourceFile); err != nil {
		t.Fatalf("insert source media row: %v", err)
	}
	conflictingFile := &database.MediaFile{Path: targetPath, Size: 99, MediaType: "episode", ParentSeriesID: &otherSeries.ID, NormalizedTitle: "other show", LibraryRoot: targetRoot}
	if err := db.UpsertMediaFile(conflictingFile); err != nil {
		t.Fatalf("insert conflicting destination row: %v", err)
	}
	sourceID, conflictingID := sourceFile.ID, conflictingFile.ID

	consolidator := NewConsolidator(db, &config.Config{Libraries: config.LibrariesConfig{TV: []string{targetRoot}}})
	err = consolidator.ExecutePlan(&Plan{
		CanProceed: true,
		TargetPath: targetSeriesPath,
		Operations: []*Operation{{SourcePath: sourcePath, DestinationPath: targetPath, Size: 12, Type: "episode"}},
	}, false)
	if err == nil || !strings.Contains(err.Error(), "already tracked") {
		t.Fatalf("ExecutePlan error = %v, want database path conflict", err)
	}
	if _, err := os.Stat(sourcePath); err != nil {
		t.Fatalf("source file changed before database conflict was rejected: %v", err)
	}
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("destination file exists after database conflict, stat error = %v", err)
	}

	gotSource, err := db.GetMediaFile(sourcePath)
	if err != nil {
		t.Fatalf("read source media row: %v", err)
	}
	if gotSource == nil || gotSource.ID != sourceID {
		t.Fatalf("source media row = %#v, want preserved ID %d", gotSource, sourceID)
	}
	gotTarget, err := db.GetMediaFile(targetPath)
	if err != nil {
		t.Fatalf("read conflicting destination row: %v", err)
	}
	if gotTarget == nil || gotTarget.ID != conflictingID || gotTarget.NormalizedTitle != "other show" {
		t.Fatalf("destination row = %#v, want unchanged conflicting row ID %d", gotTarget, conflictingID)
	}
	updatedSeries, err := db.GetSeriesByID(series.ID)
	if err != nil {
		t.Fatalf("read source series: %v", err)
	}
	if updatedSeries.CanonicalPath != sourceSeriesPath || updatedSeries.SonarrPathDirty {
		t.Fatalf("source series = %q dirty=%v, want unchanged path and clean flag", updatedSeries.CanonicalPath, updatedSeries.SonarrPathDirty)
	}
}

func TestExecutePlanRestoresFileWhenParentDatabaseUpdateFails(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "source", "TV")
	targetRoot := filepath.Join(root, "target", "TV")
	sourceSeriesPath := filepath.Join(sourceRoot, "Show (2024)")
	targetSeriesPath := filepath.Join(targetRoot, "Show (2024)")
	sourcePath := filepath.Join(sourceSeriesPath, "Season 01", "Episode.mkv")
	targetPath := filepath.Join(targetSeriesPath, "Season 01", "Episode.mkv")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("create source directory: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("episode media"), 0o644); err != nil {
		t.Fatalf("create source file: %v", err)
	}

	db, err := database.OpenPath(filepath.Join(root, "test.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	series := &database.Series{
		Title: "Show", Year: 2024, CanonicalPath: sourceSeriesPath,
		LibraryRoot: sourceRoot, Source: "plex2jellyfin", SourcePriority: 100,
	}
	if _, err := db.UpsertSeries(series); err != nil {
		t.Fatalf("insert series: %v", err)
	}
	episode := &database.Episode{SeriesID: series.ID, Season: 1, Episode: 1, Title: "Episode 1"}
	if err := db.UpsertEpisode(episode); err != nil {
		t.Fatalf("insert episode: %v", err)
	}
	file := &database.MediaFile{
		Path: sourcePath, Size: int64(len("episode media")), MediaType: "episode",
		ParentSeriesID: &series.ID, ParentEpisodeID: &episode.ID,
		NormalizedTitle: "show", LibraryRoot: sourceRoot,
	}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("insert media file: %v", err)
	}
	mediaFileID := file.ID
	if err := db.UpdateEpisodeBestFile(episode.ID, &mediaFileID); err != nil {
		t.Fatalf("set episode best file: %v", err)
	}
	if _, err := db.DB().Exec(`CREATE TRIGGER fail_series_path_update BEFORE UPDATE OF canonical_path ON series BEGIN SELECT RAISE(ABORT, 'forced path update failure'); END`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	consolidator := NewConsolidator(db, &config.Config{Libraries: config.LibrariesConfig{TV: []string{targetRoot}}})
	err = consolidator.ExecutePlan(&Plan{
		CanProceed: true,
		TargetPath: targetSeriesPath,
		Operations: []*Operation{{SourcePath: sourcePath, DestinationPath: targetPath, Type: "episode"}},
	}, false)
	if err == nil || !strings.Contains(err.Error(), "forced path update failure") {
		t.Fatalf("ExecutePlan error = %v, want injected parent update failure", err)
	}
	if got, err := os.ReadFile(sourcePath); err != nil || string(got) != "episode media" {
		t.Fatalf("source file = %q, error = %v; want original file restored", got, err)
	}
	if _, err := os.Lstat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("target file exists after rollback, stat error = %v", err)
	}
	gotFile, err := db.GetMediaFile(sourcePath)
	if err != nil || gotFile == nil || gotFile.ID != mediaFileID {
		t.Fatalf("source media row = %#v, error = %v; want preserved ID %d", gotFile, err, mediaFileID)
	}
	if got, err := db.GetMediaFile(targetPath); err != nil || got != nil {
		t.Fatalf("target media row = %#v, error = %v; want no target row", got, err)
	}
	updatedSeries, err := db.GetSeriesByID(series.ID)
	if err != nil {
		t.Fatalf("read series after rollback: %v", err)
	}
	if updatedSeries.CanonicalPath != sourceSeriesPath || updatedSeries.SonarrPathDirty {
		t.Fatalf("series path/dirty = %q/%v, want original path and clean flag", updatedSeries.CanonicalPath, updatedSeries.SonarrPathDirty)
	}
	updatedEpisode, err := db.GetEpisode(series.ID, 1, 1)
	if err != nil {
		t.Fatalf("read episode after rollback: %v", err)
	}
	if updatedEpisode.BestFileID == nil || *updatedEpisode.BestFileID != mediaFileID {
		t.Fatalf("episode best file ID = %v, want preserved ID %d", updatedEpisode.BestFileID, mediaFileID)
	}
	var reservations int
	if err := db.SQL().QueryRow(`SELECT COUNT(*) FROM media_file_move_reservations`).Scan(&reservations); err != nil {
		t.Fatal(err)
	}
	if reservations != 0 {
		t.Fatalf("remaining move reservations after rollback = %d, want 0", reservations)
	}
}
