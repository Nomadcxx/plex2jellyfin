package consolidate

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/database"
)

func TestConsolidatorGeneratePlan(t *testing.T) {
	// Create temporary database
	tempDir, err := ioutil.TempDir("", "jellywatch_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	db, err := database.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create test config
	cfg := &config.Config{
		Options: config.OptionsConfig{
			DryRun:          false,
			VerifyChecksums: false,
			DeleteSource:    true,
		},
	}

	// Create consolidator
	cons := NewConsolidator(db, cfg)

	// Create test conflict
	conflict := &database.Conflict{
		MediaType:       "series",
		Title:           "Test Show",
		TitleNormalized: "testshow",
		Year:            nil,
		Locations:       []string{"/tmp/location1", "/tmp/location2"},
		CreatedAt:       time.Now(),
	}

	// Generate plan
	plan, err := cons.GeneratePlan(conflict)
	if err != nil {
		t.Fatalf("Failed to generate plan: %v", err)
	}

	// Basic validations
	if plan.ConflictID != conflict.ID {
		t.Errorf("ConflictID = %d, want %d", plan.ConflictID, conflict.ID)
	}
	if plan.MediaType != "series" {
		t.Errorf("MediaType = %s, want series", plan.MediaType)
	}
	if plan.Title != "Test Show" {
		t.Errorf("Title = %s, want Test Show", plan.Title)
	}
	if len(plan.SourcePaths) != 2 {
		t.Errorf("SourcePaths length = %d, want 2", len(plan.SourcePaths))
	}
}

func TestConsolidatorChooseTargetPath(t *testing.T) {
	// Create temporary database
	tempDir, err := ioutil.TempDir("", "jellywatch_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	db, err := database.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create test config
	cfg := &config.Config{}

	// Create consolidator
	cons := NewConsolidator(db, cfg)

	conflict := &database.Conflict{
		MediaType: "series",
		Locations: []string{"/tmp/location1", "/tmp/location2"},
	}

	targetPath, err := cons.chooseTargetPath(conflict)
	if err != nil {
		t.Fatalf("Failed to choose target path: %v", err)
	}

	// Should select first location as fallback since we can't count files
	if targetPath != "/tmp/location1" {
		t.Errorf("TargetPath = %s, want /tmp/location1", targetPath)
	}
}

func TestSizeFilter(t *testing.T) {
	t.Run("skip files under 100MB", func(t *testing.T) {
		// Create temp directory
		tmpDir := t.TempDir()

		// Create temp file under 100MB
		smallFile := filepath.Join(tmpDir, "small.mkv")
		os.WriteFile(smallFile, make([]byte, 50*1024*1024), 0644) // 50MB

		// Create conflict with small file
		conflict := &database.Conflict{
			Locations: []string{tmpDir},
		}

		consolidator := &Consolidator{}

		// getFilesToMove should skip small files
		ops, err := consolidator.getFilesToMove(tmpDir, "/target", conflict)
		if err != nil {
			t.Fatal(err)
		}

		if len(ops) > 0 {
			t.Error("Expected no operations for files under 100MB")
		}
	})

	t.Run("process files over 100MB", func(t *testing.T) {
		// Create temp directory
		tmpDir := t.TempDir()

		// Create temp file over 100MB
		largeFile := filepath.Join(tmpDir, "large.mkv")
		os.WriteFile(largeFile, make([]byte, 150*1024*1024), 0644) // 150MB

		// Create conflict with large file
		conflict := &database.Conflict{
			Locations: []string{tmpDir},
		}

		consolidator := &Consolidator{}

		// getFilesToMove should process large files
		ops, err := consolidator.getFilesToMove(tmpDir, "/target", conflict)
		if err != nil {
			t.Fatal(err)
		}

		if len(ops) == 0 {
			t.Error("Expected operations for files over 100MB")
		}
	})

	t.Run("skip files exactly 100MB", func(t *testing.T) {
		// Create temp directory
		tmpDir := t.TempDir()

		// Create temp file exactly 100MB
		exactFile := filepath.Join(tmpDir, "exact.mkv")
		os.WriteFile(exactFile, make([]byte, 100*1024*1024), 0644) // 100MB

		// Create conflict with exact size file
		conflict := &database.Conflict{
			Locations: []string{tmpDir},
		}

		consolidator := &Consolidator{}

		// getFilesToMove should skip files exactly at 100MB (inclusive filter)
		ops, err := consolidator.getFilesToMove(tmpDir, "/target", conflict)
		if err != nil {
			t.Fatal(err)
		}

		if len(ops) > 0 {
			t.Error("Expected no operations for files exactly 100MB")
		}
	})
}

func TestIsMediaFile(t *testing.T) {
	tests := []struct {
		ext      string
		expected bool
	}{
		{".mkv", true},
		{".mp4", true},
		{".avi", true},
		{".mov", true},
		{".m4v", true},
		{".webm", true},
		{".txt", false},
		{".jpg", false},
		{".nfo", false},
		{".srt", false},
	}

	for _, tt := range tests {
		result := isMediaFile(tt.ext)
		if result != tt.expected {
			t.Errorf("isMediaFile(%s) = %v, want %v", tt.ext, result, tt.expected)
		}
	}
}
