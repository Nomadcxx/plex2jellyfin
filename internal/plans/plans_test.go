package plans

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestExecuteAuditAction(t *testing.T) {
	action := &AuditAction{
		Action:   "rename",
		NewTitle: "Correct Title",
		NewYear:  func() *int { y := 2020; return &y }(),
	}

	err := ExecuteAuditAction(action, "test/path.mkv")
	if err == nil {
		t.Fatal("ExecuteAuditAction should return not implemented error")
	}

	if err.Error() != "not implemented" {
		t.Fatalf("Expected 'not implemented' error, got: %v", err)
	}
}
