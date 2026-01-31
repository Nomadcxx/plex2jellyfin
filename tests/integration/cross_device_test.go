package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/plans"
	"github.com/Nomadcxx/jellywatch/internal/transfer"
)

func TestAuditMove_CrossDevice(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db, err := database.OpenPath(":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer db.Close()

	// Create source and destination directories
	sourceDir := t.TempDir()
	destDir := t.TempDir()

	// Create test file
	srcPath := filepath.Join(sourceDir, "test.movie.2024.1080p.mkv")
	if err := os.WriteFile(srcPath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Insert into database
	file := &database.MediaFile{
		Path:            srcPath,
		NormalizedTitle: "test.movie",
		Year:            intPtr(2024),
		MediaType:       "movie",
	}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("Failed to insert test file: %v", err)
	}

	// Create audit action
	item := plans.AuditItem{ID: file.ID, Path: srcPath}
	action := plans.AuditAction{
		Action:   "rename",
		NewTitle: "Test Movie",
		NewPath:  filepath.Join(destDir, "Test Movie (2024).mkv"),
	}

	// Execute rename
	err = plans.ExecuteAuditAction(db, item, action, false)

	// Verify success
	if err != nil {
		t.Errorf("Audit action failed: %v", err)
	}

	// Verify file moved
	if _, err := os.Stat(action.NewPath); os.IsNotExist(err) {
		t.Error("Destination file does not exist after move")
	}
	if _, err := os.Stat(srcPath); err == nil {
		t.Error("Source file still exists after move")
	}
}

func TestTransfer_BackendAvailability(t *testing.T) {
	// Test that at least one transfer backend is available
	backends := []transfer.Backend{
		transfer.BackendAuto,
		transfer.BackendRsync,
		transfer.BackendNative,
	}

	for _, backend := range backends {
		_, err := transfer.New(backend)
		if err == nil {
			t.Logf("Backend %s is available", backend)
			return
		}
		t.Logf("Backend %s not available: %v", backend, err)
	}

	t.Fatal("No transfer backend available (rsync or native required)")
}

func intPtr(i int) *int {
	return &i
}
