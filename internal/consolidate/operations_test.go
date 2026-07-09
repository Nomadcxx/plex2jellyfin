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

	file := &database.MediaFile{
		Path:            sourcePath,
		Size:            5,
		ModifiedAt:      time.Now(),
		MediaType:       "episode",
		ParentSeriesID:  &series.ID,
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
	if got.LibraryRoot != targetRoot {
		t.Fatalf("LibraryRoot = %q, want %q", got.LibraryRoot, targetRoot)
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
