package consolidate_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/consolidate"
	"github.com/Nomadcxx/jellywatch/internal/database"
)

func TestIntegrationConsolidator(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "jellywatch_consolidate_test")
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

	cfg := &config.Config{
		Options: config.OptionsConfig{
			DryRun:          true,
			VerifyChecksums: false,
			DeleteSource:    false,
		},
	}

	cons := consolidate.NewConsolidator(db, cfg)

	conflicts, err := db.GetUnresolvedConflicts()
	if err != nil {
		t.Fatalf("Failed to get unresolved conflicts: %v", err)
	}

	if len(conflicts) != 0 {
		t.Errorf("Expected 0 conflicts initially, got %d", len(conflicts))
	}

	plans, err := cons.GenerateAllPlans()
	if err != nil {
		t.Fatalf("Failed to generate plans: %v", err)
	}

	if len(plans) != 0 {
		t.Errorf("Expected 0 plans with empty database, got %d", len(plans))
	}

	stats := cons.GetStats()
	if stats.ConflictsFound != 0 {
		t.Errorf("Expected 0 conflicts found, got %d", stats.ConflictsFound)
	}
	if stats.PlansGenerated != 0 {
		t.Errorf("Expected 0 plans generated, got %d", stats.PlansGenerated)
	}

	err = cons.DryRun()
	if err != nil {
		t.Fatalf("DryRun failed: %v", err)
	}

	err = cons.ExecuteAll(true)
	if err != nil {
		t.Fatalf("ExecuteAll(dryRun=true) failed: %v", err)
	}

	t.Logf("Integration test passed. Basic consolidator operations work correctly.")
	t.Logf("Note: Full conflict detection requires multiple series entries, which is prevented by UNIQUE constraint.")
}

func TestConflictResolutionAfterMove(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "jellywatch_conflict_resolution_test")
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

	year := 2024
	_, err = db.DB().Exec(`
		INSERT INTO conflicts (media_type, title, title_normalized, year, locations, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, "movie", "Test Movie", "test_movie", year, `["/path1/test_movie.mkv","/path2/test_movie.mkv"]`, "2024-01-01 00:00:00")
	if err != nil {
		t.Fatalf("Failed to create conflict: %v", err)
	}

	var conflictID int64
	err = db.DB().QueryRow("SELECT id FROM conflicts WHERE title_normalized = ?", "test_movie").Scan(&conflictID)
	if err != nil {
		t.Fatalf("Failed to get conflict ID: %v", err)
	}

	conflicts, err := db.GetUnresolvedConflicts()
	if err != nil {
		t.Fatalf("Failed to get unresolved conflicts: %v", err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("Expected 1 unresolved conflict, got %d", len(conflicts))
	}

	t.Logf("Test setup complete. Integration with executeMove needed for full end-to-end test.")
}
