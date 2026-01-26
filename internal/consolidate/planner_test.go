package consolidate

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/database"
)

func TestNewPlanner(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	planner := NewPlanner(db)
	if planner == nil {
		t.Fatal("Expected planner to be created")
	}

	if planner.db == nil {
		t.Error("Expected db to be set")
	}
}

func TestGeneratePlans_Duplicates(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	planner := NewPlanner(db)

	// Create duplicate movie files with different quality
	year2013 := 2013
	files := []*database.MediaFile{
		{
			Path:                "/media/STORAGE1/Movies/Her (2013)/Her.2013.MULTI.1080p.BluRay.x264-Goatlove.mkv",
			Size:                5500 * 1024 * 1024,
			MediaType:           "movie",
			NormalizedTitle:     "her",
			Year:                &year2013,
			Resolution:          "unknown",
			SourceType:          "unknown",
			QualityScore:        105, // Lower score
			IsJellyfinCompliant: false,
		},
		{
			Path:                "/media/STORAGE8/Movies/Her (2013)/Her (2013).mkv",
			Size:                9800 * 1024 * 1024,
			MediaType:           "movie",
			NormalizedTitle:     "her",
			Year:                &year2013,
			Resolution:          "1080p",
			SourceType:          "BluRay",
			QualityScore:        389, // Higher score - should be kept
			IsJellyfinCompliant: true,
		},
	}

	for _, file := range files {
		if err := db.UpsertMediaFile(file); err != nil {
			t.Fatalf("Failed to insert test file: %v", err)
		}
	}

	// Generate plans
	ctx := context.Background()
	summary, err := planner.GeneratePlans(ctx)
	if err != nil {
		t.Fatalf("GeneratePlans failed: %v", err)
	}

	// Should have 1 delete plan (remove the inferior duplicate)
	if summary.DeletePlans != 1 {
		t.Errorf("Expected 1 delete plan, got %d", summary.DeletePlans)
	}

	if summary.TotalPlans != 1 {
		t.Errorf("Expected 1 total plan, got %d", summary.TotalPlans)
	}

	// Verify the correct file is marked for deletion
	plans, err := planner.GetPendingPlans()
	if err != nil {
		t.Fatalf("GetPendingPlans failed: %v", err)
	}

	if len(plans) != 1 {
		t.Fatalf("Expected 1 pending plan, got %d", len(plans))
	}

	plan := plans[0]
	if plan.Action != "delete" {
		t.Errorf("Expected action 'delete', got '%s'", plan.Action)
	}

	// The inferior file should be marked for deletion
	if plan.SourcePath != files[0].Path {
		t.Errorf("Wrong file marked for deletion. Got: %s, Want: %s", plan.SourcePath, files[0].Path)
	}
}

func TestGeneratePlans_Episodes(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	planner := NewPlanner(db)

	// Create duplicate episode files
	year2023 := 2023
	season1 := 1
	episode1 := 1

	files := []*database.MediaFile{
		{
			Path:                "/media/STORAGE1/TV/Silo (2023)/Season 01/Silo.S01E01.720p.mkv",
			Size:                800 * 1024 * 1024,
			MediaType:           "episode",
			NormalizedTitle:     "silo",
			Year:                &year2023,
			Season:              &season1,
			Episode:             &episode1,
			Resolution:          "720p",
			SourceType:          "WEBRip",
			QualityScore:        250, // Lower score
			IsJellyfinCompliant: false,
		},
		{
			Path:                "/media/STORAGE2/TV/Silo (2023)/Season 01/Silo (2023) S01E01.mkv",
			Size:                1500 * 1024 * 1024,
			MediaType:           "episode",
			NormalizedTitle:     "silo",
			Year:                &year2023,
			Season:              &season1,
			Episode:             &episode1,
			Resolution:          "1080p",
			SourceType:          "WEB-DL",
			QualityScore:        361, // Higher score - should be kept
			IsJellyfinCompliant: true,
		},
	}

	for _, file := range files {
		if err := db.UpsertMediaFile(file); err != nil {
			t.Fatalf("Failed to insert test file: %v", err)
		}
	}

	// Generate plans
	ctx := context.Background()
	summary, err := planner.GeneratePlans(ctx)
	if err != nil {
		t.Fatalf("GeneratePlans failed: %v", err)
	}

	// Should have 1 delete plan
	if summary.DeletePlans != 1 {
		t.Errorf("Expected 1 delete plan, got %d", summary.DeletePlans)
	}

	// Verify correct file marked for deletion
	plans, err := planner.GetPendingPlans()
	if err != nil {
		t.Fatalf("GetPendingPlans failed: %v", err)
	}

	if len(plans) != 1 {
		t.Fatalf("Expected 1 pending plan, got %d", len(plans))
	}

	plan := plans[0]
	if plan.SourcePath != files[0].Path {
		t.Errorf("Wrong file marked for deletion. Got: %s, Want: %s", plan.SourcePath, files[0].Path)
	}
}

func TestGetPlanSummary(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	planner := NewPlanner(db)

	// Create some test files and generate plans
	year2013 := 2013
	files := []*database.MediaFile{
		{
			Path:            "/media/STORAGE1/Movies/Her (2013)/Her.mkv",
			Size:            5500 * 1024 * 1024,
			MediaType:       "movie",
			NormalizedTitle: "her",
			Year:            &year2013,
			QualityScore:    105,
		},
		{
			Path:            "/media/STORAGE8/Movies/Her (2013)/Her (2013).mkv",
			Size:            9800 * 1024 * 1024,
			MediaType:       "movie",
			NormalizedTitle: "her",
			Year:            &year2013,
			QualityScore:    389,
		},
	}

	for _, file := range files {
		if err := db.UpsertMediaFile(file); err != nil {
			t.Fatalf("Failed to insert test file: %v", err)
		}
	}

	ctx := context.Background()
	_, err := planner.GeneratePlans(ctx)
	if err != nil {
		t.Fatalf("GeneratePlans failed: %v", err)
	}

	// Get summary
	summary, err := planner.GetPlanSummary()
	if err != nil {
		t.Fatalf("GetPlanSummary failed: %v", err)
	}

	if summary.TotalPlans != 1 {
		t.Errorf("Expected 1 total plan, got %d", summary.TotalPlans)
	}

	if summary.DeletePlans != 1 {
		t.Errorf("Expected 1 delete plan, got %d", summary.DeletePlans)
	}

	// Space to reclaim should be the size of the inferior file
	expectedSpace := int64(5500 * 1024 * 1024)
	if summary.SpaceToReclaim != expectedSpace {
		t.Errorf("Expected space to reclaim %d, got %d", expectedSpace, summary.SpaceToReclaim)
	}
}

func TestClearPendingPlans(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	planner := NewPlanner(db)

	// Create some test data and generate plans
	year2013 := 2013
	files := []*database.MediaFile{
		{
			Path:            "/media/STORAGE1/Movies/Her (2013)/Her.mkv",
			Size:            5500 * 1024 * 1024,
			MediaType:       "movie",
			NormalizedTitle: "her",
			Year:            &year2013,
			QualityScore:    105,
		},
		{
			Path:            "/media/STORAGE8/Movies/Her (2013)/Her (2013).mkv",
			Size:            9800 * 1024 * 1024,
			MediaType:       "movie",
			NormalizedTitle: "her",
			Year:            &year2013,
			QualityScore:    389,
		},
	}

	for _, file := range files {
		if err := db.UpsertMediaFile(file); err != nil {
			t.Fatalf("Failed to insert test file: %v", err)
		}
	}

	ctx := context.Background()
	_, err := planner.GeneratePlans(ctx)
	if err != nil {
		t.Fatalf("GeneratePlans failed: %v", err)
	}

	// Verify plans exist
	plans, err := planner.GetPendingPlans()
	if err != nil {
		t.Fatalf("GetPendingPlans failed: %v", err)
	}
	if len(plans) == 0 {
		t.Fatal("Expected pending plans before clear")
	}

	// Clear plans
	if err := planner.clearPendingPlans(); err != nil {
		t.Fatalf("clearPendingPlans failed: %v", err)
	}

	// Verify plans are gone
	plans, err = planner.GetPendingPlans()
	if err != nil {
		t.Fatalf("GetPendingPlans failed: %v", err)
	}
	if len(plans) != 0 {
		t.Errorf("Expected 0 plans after clear, got %d", len(plans))
	}
}

func TestGeneratePlans_NoDuplicates(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	planner := NewPlanner(db)

	// Create files with different titles (no duplicates)
	year2013 := 2013
	year2014 := 2014
	files := []*database.MediaFile{
		{
			Path:            "/media/Movies/Her (2013)/Her (2013).mkv",
			Size:            9800 * 1024 * 1024,
			MediaType:       "movie",
			NormalizedTitle: "her",
			Year:            &year2013,
			QualityScore:    389,
		},
		{
			Path:            "/media/Movies/Interstellar (2014)/Interstellar (2014).mkv",
			Size:            15000 * 1024 * 1024,
			MediaType:       "movie",
			NormalizedTitle: "interstellar",
			Year:            &year2014,
			QualityScore:    405,
		},
	}

	for _, file := range files {
		if err := db.UpsertMediaFile(file); err != nil {
			t.Fatalf("Failed to insert test file: %v", err)
		}
	}

	// Generate plans
	ctx := context.Background()
	summary, err := planner.GeneratePlans(ctx)
	if err != nil {
		t.Fatalf("GeneratePlans failed: %v", err)
	}

	// Should have no plans (no duplicates)
	if summary.TotalPlans != 0 {
		t.Errorf("Expected 0 plans, got %d", summary.TotalPlans)
	}
}

func TestGetPendingPlans_NullSourceFileID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert a plan with NULL source_file_id (move operation)
	_, err := db.DB().Exec(`
		INSERT INTO consolidation_plans
		(status, action, source_file_id, source_path, target_path, reason, reason_details)
		VALUES ('pending', 'move', NULL, '/source/path', '/target/path', 'consolidation', 'moving to target')
	`)
	if err != nil {
		t.Fatalf("Failed to insert test plan: %v", err)
	}

	planner := NewPlanner(db)
	plans, err := planner.GetPendingPlans()
	if err != nil {
		t.Fatalf("GetPendingPlans failed with NULL source_file_id: %v", err)
	}

	if len(plans) != 1 {
		t.Fatalf("Expected 1 plan, got %d", len(plans))
	}

	if plans[0].SourceFileID.Valid {
		t.Error("Expected SourceFileID.Valid to be false for NULL value")
	}

}

// setupTestDB creates a temporary database for testing
func setupTestDB(t *testing.T) *database.MediaDB {
	tempFile := filepath.Join(t.TempDir(), "test.db")
	db, err := database.OpenPath(tempFile)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	return db
}
