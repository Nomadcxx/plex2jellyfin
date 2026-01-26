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
		{".wmv", true},
		{".flv", true},
		{".webm", true},
		{".m4v", true},
		{".mpg", true},
		{".mpeg", true},
		{".m2ts", true},
		{".ts", true},
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

func TestStorePlanUntrackedFile(t *testing.T) {
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
	consolidator := NewConsolidator(db, cfg)

	// Create plan for file NOT in database
	plan := &Plan{
		Title:       "Test",
		SourcePaths: []string{"/tmp/untracked.mkv"},
		TargetPath:  "/dst/test.mkv",
		Operations: []*Operation{{
			SourcePath:      "/tmp/untracked.mkv",
			DestinationPath: "/dst/test.mkv",
			Size:            200 * 1024 * 1024, // 200MB
		}},
	}

	err = consolidator.StorePlan(plan)
	if err != nil {
		t.Errorf("Expected no error storing untracked file plan: %v", err)
	}

	// Verify plan was created with NULL source_file_id
	var sourceFileID *int64
	query := `SELECT source_file_id FROM consolidation_plans WHERE source_path = ?`
	err = db.DB().QueryRow(query, "/tmp/untracked.mkv").Scan(&sourceFileID)

	if err != nil {
		t.Fatalf("Failed to query consolidation plan: %v", err)
	}

	if sourceFileID != nil {
		t.Errorf("Expected source_file_id to be NULL, got: %v", *sourceFileID)
	}
}

func TestCleanupEmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	emptyDir := filepath.Join(tmpDir, "empty")
	os.MkdirAll(emptyDir, 0755)

	// Should delete empty directory
	err := CleanupEmptyDir(emptyDir)
	if err != nil {
		t.Errorf("Expected no error cleaning empty dir: %v", err)
	}

	// Verify deleted
	if _, err := os.Stat(emptyDir); !os.IsNotExist(err) {
		t.Error("Expected empty dir to be deleted")
	}
}

func TestCleanupNotEmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	notEmptyDir := filepath.Join(tmpDir, "files")
	os.MkdirAll(notEmptyDir, 0755)
	os.WriteFile(filepath.Join(notEmptyDir, "file.mkv"), []byte("data"), 0644)

	// Should NOT delete non-empty directory
	err := CleanupEmptyDir(notEmptyDir)
	if err != nil {
		t.Errorf("Expected no error for non-empty dir: %v", err)
	}

	// Verify still exists
	if _, err := os.Stat(notEmptyDir); os.IsNotExist(err) {
		t.Error("Expected non-empty dir to remain")
	}
}

func TestStats_SkippedConflicts_Field(t *testing.T) {
	stats := Stats{}
	if stats.SkippedConflicts != 0 {
		t.Errorf("Expected SkippedConflicts to default to 0, got %d", stats.SkippedConflicts)
	}
}
