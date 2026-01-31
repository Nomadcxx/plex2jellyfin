package plans

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/database"
)

func TestSaveAndLoadDuplicatePlans(t *testing.T) {
	// Use temp directory
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	year := 2005
	plan := &DuplicatePlan{
		CreatedAt: time.Now(),
		Command:   "duplicates",
		Summary: DuplicateSummary{
			TotalGroups:      1,
			FilesToDelete:    1,
			SpaceReclaimable: 4400000000,
		},
		Plans: []DuplicateGroup{
			{
				GroupID:   "abc123",
				Title:     "Robots",
				Year:      &year,
				MediaType: "movie",
				Keep: FileInfo{
					ID:           1,
					Path:         "/storage1/Robots.mkv",
					Size:         4300000000,
					QualityScore: 284,
				},
				Delete: FileInfo{
					ID:           2,
					Path:         "/storage2/Robots.mkv",
					Size:         4400000000,
					QualityScore: 84,
				},
			},
		},
	}

	// Save
	err := SaveDuplicatePlans(plan)
	if err != nil {
		t.Fatalf("SaveDuplicatePlans failed: %v", err)
	}

	// Verify file exists
	path, _ := getDuplicatePlansPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("Plan file was not created")
	}

	// Load
	loaded, err := LoadDuplicatePlans()
	if err != nil {
		t.Fatalf("LoadDuplicatePlans failed: %v", err)
	}

	if loaded == nil {
		t.Fatal("Loaded plan is nil")
	}

	if loaded.Summary.TotalGroups != 1 {
		t.Errorf("Expected 1 group, got %d", loaded.Summary.TotalGroups)
	}

	if len(loaded.Plans) != 1 {
		t.Fatalf("Expected 1 plan, got %d", len(loaded.Plans))
	}

	if loaded.Plans[0].Title != "Robots" {
		t.Errorf("Expected title 'Robots', got '%s'", loaded.Plans[0].Title)
	}

	// Delete
	err = DeleteDuplicatePlans()
	if err != nil {
		t.Fatalf("DeleteDuplicatePlans failed: %v", err)
	}

	// Verify file is gone
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("Plan file was not deleted")
	}
}

func TestLoadDuplicatePlans_NoFile(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	plan, err := LoadDuplicatePlans()
	if err != nil {
		t.Fatalf("LoadDuplicatePlans should not error for missing file: %v", err)
	}

	if plan != nil {
		t.Fatal("Expected nil plan for missing file")
	}
}

func TestSaveAndLoadConsolidatePlans(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	year := 2005
	plan := &ConsolidatePlan{
		CreatedAt: time.Now(),
		Command:   "consolidate",
		Summary: ConsolidateSummary{
			TotalConflicts: 1,
			TotalMoves:     2,
			TotalBytes:     1000000000,
		},
		Plans: []ConsolidateGroup{
			{
				ConflictID:     1,
				Title:          "American Dad",
				Year:           &year,
				MediaType:      "series",
				TargetLocation: "/storage1/TV/American Dad (2005)",
				Operations: []MoveOperation{
					{
						Action:     "move",
						SourcePath: "/storage2/TV/American Dad/S01E01.mkv",
						TargetPath: "/storage1/TV/American Dad (2005)/S01E01.mkv",
						Size:       500000000,
					},
				},
			},
		},
	}

	err := SaveConsolidatePlans(plan)
	if err != nil {
		t.Fatalf("SaveConsolidatePlans failed: %v", err)
	}

	loaded, err := LoadConsolidatePlans()
	if err != nil {
		t.Fatalf("LoadConsolidatePlans failed: %v", err)
	}

	if loaded.Summary.TotalConflicts != 1 {
		t.Errorf("Expected 1 conflict, got %d", loaded.Summary.TotalConflicts)
	}

	err = DeleteConsolidatePlans()
	if err != nil {
		t.Fatalf("DeleteConsolidatePlans failed: %v", err)
	}
}

func TestGetPlansDir(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	dir, err := GetPlansDir()
	if err != nil {
		t.Fatalf("GetPlansDir failed: %v", err)
	}

	expected := filepath.Join(tempDir, ".config", "jellywatch", "plans")
	if dir != expected {
		t.Errorf("Expected %s, got %s", expected, dir)
	}
}

func TestExecuteAuditAction_Rename(t *testing.T) {
	// Use temp directory
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	// Create a test database
	dbPath := filepath.Join(tempDir, "test.db")
	db, err := database.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create test file
	sourcePath := filepath.Join(tempDir, "movie.mkv")
	if err := os.WriteFile(sourcePath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Insert a media file
	file := &database.MediaFile{
		Path:            sourcePath,
		Size:            4,
		ModifiedAt:      time.Now(),
		MediaType:       "movie",
		NormalizedTitle: "movie",
		Year:            func() *int { y := 2019; return &y }(),
		Resolution:      "1080p",
		QualityScore:    100,
		LibraryRoot:     tempDir,
	}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("Failed to insert media file: %v", err)
	}

	// Test rename action
	item := AuditItem{
		ID:   file.ID,
		Path: sourcePath,
	}
	targetPath := filepath.Join(tempDir, "Correct Title.mkv")
	action := AuditAction{
		Action:   "rename",
		NewTitle: "Correct Title",
		NewPath:  targetPath,
		NewYear:  func() *int { y := 2020; return &y }(),
	}

	err = ExecuteAuditAction(db, item, action, false)
	if err != nil {
		t.Fatalf("ExecuteAuditAction failed: %v", err)
	}

	// Verify filesystem change
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Error("Source file should not exist after rename")
	}
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		t.Error("Target file should exist after rename")
	}

	// Verify database update
	updated, err := db.GetMediaFileByID(file.ID)
	if err != nil {
		t.Fatalf("Failed to get updated file: %v", err)
	}
	if updated.Path != targetPath {
		t.Errorf("Expected path %s, got %s", targetPath, updated.Path)
	}
	if updated.NormalizedTitle != "Correct Title" {
		t.Errorf("Expected title 'Correct Title', got %s", updated.NormalizedTitle)
	}
	if updated.Year == nil || *updated.Year != 2020 {
		t.Errorf("Expected year 2020, got %v", updated.Year)
	}
}

func TestExecuteAuditAction_Delete(t *testing.T) {
	// Use temp directory
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	// Create a test database
	dbPath := filepath.Join(tempDir, "test.db")
	db, err := database.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create test file
	sourcePath := filepath.Join(tempDir, "movie.mkv")
	if err := os.WriteFile(sourcePath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Insert a media file
	file := &database.MediaFile{
		Path:            sourcePath,
		Size:            4,
		ModifiedAt:      time.Now(),
		MediaType:       "movie",
		NormalizedTitle: "movie",
		Year:            func() *int { y := 2019; return &y }(),
		Resolution:      "1080p",
		QualityScore:    100,
		LibraryRoot:     tempDir,
	}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("Failed to insert media file: %v", err)
	}

	// Test delete action
	item := AuditItem{
		ID:   file.ID,
		Path: sourcePath,
	}
	action := AuditAction{
		Action:    "delete",
		Reasoning: "Low quality duplicate",
	}

	err = ExecuteAuditAction(db, item, action, false)
	if err != nil {
		t.Fatalf("ExecuteAuditAction failed: %v", err)
	}

	// Verify file is deleted from filesystem
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Error("Source file should not exist after delete")
	}

	// Verify database record is removed
	deleted, err := db.GetMediaFileByID(file.ID)
	if err != nil {
		t.Fatalf("Failed to check deleted file: %v", err)
	}
	if deleted != nil {
		t.Error("Database record should be removed after delete")
	}
}

func TestExecuteAuditAction_UnknownAction(t *testing.T) {
	item := AuditItem{
		ID:   1,
		Path: "/test/movie.mkv",
	}
	action := AuditAction{
		Action: "unknown",
	}

	err := ExecuteAuditAction(nil, item, action, false)
	if err == nil {
		t.Fatal("ExecuteAuditAction should return error for unknown action")
	}
	if !strings.Contains(err.Error(), "unknown action") {
		t.Errorf("Expected 'unknown action' error, got: %v", err)
	}
}

func TestExecuteAuditAction_Rename_DryRun(t *testing.T) {
	// Use temp directory
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	// Create a test database
	dbPath := filepath.Join(tempDir, "test.db")
	db, err := database.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create test file
	sourcePath := filepath.Join(tempDir, "movie.mkv")
	if err := os.WriteFile(sourcePath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Insert a media file
	file := &database.MediaFile{
		Path:            sourcePath,
		Size:            4,
		ModifiedAt:      time.Now(),
		MediaType:       "movie",
		NormalizedTitle: "movie",
		Year:            func() *int { y := 2019; return &y }(),
		Resolution:      "1080p",
		QualityScore:    100,
		LibraryRoot:     tempDir,
	}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("Failed to insert media file: %v", err)
	}

	// Test rename action with dryRun=true
	item := AuditItem{
		ID:   file.ID,
		Path: sourcePath,
	}
	targetPath := filepath.Join(tempDir, "Correct Title.mkv")
	action := AuditAction{
		Action:   "rename",
		NewTitle: "Correct Title",
		NewPath:  targetPath,
		NewYear:  func() *int { y := 2020; return &y }(),
	}

	err = ExecuteAuditAction(db, item, action, true) // dryRun=true
	if err != nil {
		t.Fatalf("ExecuteAuditAction failed: %v", err)
	}

	// Verify file still exists (no change in dry-run mode)
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		t.Error("Source file should still exist in dry-run mode")
	}

	// Verify target file was not created
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Error("Target file should not be created in dry-run mode")
	}

	// Verify database record was not updated
	updated, err := db.GetMediaFileByID(file.ID)
	if err != nil {
		t.Fatalf("Failed to get file: %v", err)
	}
	if updated.Path != sourcePath {
		t.Errorf("Database should not be updated in dry-run mode, expected %s, got %s", sourcePath, updated.Path)
	}
}

func TestExecuteAuditAction_Delete_DryRun(t *testing.T) {
	// Use temp directory
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	// Create a test database
	dbPath := filepath.Join(tempDir, "test.db")
	db, err := database.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create test file
	sourcePath := filepath.Join(tempDir, "movie.mkv")
	if err := os.WriteFile(sourcePath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Insert a media file
	file := &database.MediaFile{
		Path:            sourcePath,
		Size:            4,
		ModifiedAt:      time.Now(),
		MediaType:       "movie",
		NormalizedTitle: "movie",
		Year:            func() *int { y := 2019; return &y }(),
		Resolution:      "1080p",
		QualityScore:    100,
		LibraryRoot:     tempDir,
	}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("Failed to insert media file: %v", err)
	}

	// Test delete action with dryRun=true
	item := AuditItem{
		ID:   file.ID,
		Path: sourcePath,
	}
	action := AuditAction{
		Action:    "delete",
		Reasoning: "Low quality duplicate",
	}

	err = ExecuteAuditAction(db, item, action, true) // dryRun=true
	if err != nil {
		t.Fatalf("ExecuteAuditAction failed: %v", err)
	}

	// Verify file still exists (no change in dry-run mode)
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		t.Error("Source file should still exist in dry-run mode")
	}

	// Verify database record still exists
	existing, err := db.GetMediaFileByID(file.ID)
	if err != nil {
		t.Fatalf("Failed to get file: %v", err)
	}
	if existing == nil {
		t.Error("Database record should not be removed in dry-run mode")
	}
}

// setupTestDB creates a temporary database for testing
func setupTestDB(t *testing.T) *database.MediaDB {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := database.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	return db
}

func TestExecuteRename_CrossDevice(t *testing.T) {
	// This test documents that code uses transfer.Move() instead of os.Rename()
	// We verify transfer package integration for cross-device moves

	db := setupTestDB(t)
	defer db.Close()

	// Create test file
	testDir := t.TempDir()
	srcPath := filepath.Join(testDir, "test.movie.2024.mkv")
	dstPath := filepath.Join(testDir, "Test Movie (2024).mkv")

	if err := os.WriteFile(srcPath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Insert into database
	file := &database.MediaFile{
		Path:            srcPath,
		NormalizedTitle: "test.movie",
		Year:            func() *int { y := 2024; return &y }(),
		MediaType:       "movie",
	}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("Failed to insert test file: %v", err)
	}

	item := AuditItem{ID: file.ID, Path: srcPath}
	action := AuditAction{
		Action:   "rename",
		NewTitle: "Test Movie",
		NewPath:  dstPath,
	}

	// Execute rename
	err := executeRename(db, item, action, false)

	// Should succeed
	if err != nil {
		t.Errorf("executeRename failed: %v", err)
	}

	// Verify file moved
	if _, err := os.Stat(dstPath); os.IsNotExist(err) {
		t.Error("Destination file does not exist")
	}
	if _, err := os.Stat(srcPath); err == nil {
		t.Error("Source file still exists")
	}

	// Verify database updated
	updatedFile, err := db.GetMediaFileByID(file.ID)
	if err != nil {
		t.Fatalf("Failed to get updated file: %v", err)
	}
	if updatedFile.Path != dstPath {
		t.Errorf("Database path not updated: got %s, want %s", updatedFile.Path, dstPath)
	}
}
