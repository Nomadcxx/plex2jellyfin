package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/database"
)

func TestDeleteDuplicateWithPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := database.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	testFile := filepath.Join(t.TempDir(), "duplicate.mkv")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	file := &database.MediaFile{
		Path:      testFile,
		MediaType: "movie",
		Size:      4,
	}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}

	err = deleteDuplicateFile(db, testFile, -1, -1)

	if err != nil {
		t.Errorf("Delete failed even after permission fix: %v", err)
	}

	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("File still exists after delete")
	}

	dbFile, err := db.GetMediaFile(testFile)
	if err == nil && dbFile != nil {
		t.Error("Database entry still exists after successful delete")
	}
}

func TestDeleteDuplicateDatabaseCleanupOnFailure(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := database.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	testFile := "/nonexistent/duplicate.mkv"

	file := &database.MediaFile{
		Path:      testFile,
		MediaType: "movie",
		Size:      100,
	}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}

	err = deleteDuplicateFile(db, testFile, -1, -1)

	if err != nil {
		t.Logf("Expected error for non-existent file: %v", err)
	}

	dbFile, err := db.GetMediaFile(testFile)
	if err == nil && dbFile != nil {
		t.Error("Database entry should be removed even when file doesn't exist")
	}
}
