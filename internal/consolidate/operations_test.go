package consolidate

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/database"
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
