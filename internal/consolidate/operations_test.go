package consolidate

import (
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
