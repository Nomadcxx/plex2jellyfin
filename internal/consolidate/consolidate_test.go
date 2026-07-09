package consolidate

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/database"
)

func createLargeMediaFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create media dir: %v", err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create media file: %v", err)
	}
	defer f.Close()
	if err := f.Truncate(MinConsolidationFileSize + 1); err != nil {
		t.Fatalf("failed to size media file: %v", err)
	}
}

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

func TestGeneratePlanSkipsMissingSourceLocation(t *testing.T) {
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

	target := filepath.Join(tempDir, "storage1", "Silo (2023)")
	source := filepath.Join(tempDir, "storage2", "Silo (2023)")
	missing := filepath.Join(tempDir, "storage3", "Silo (2023)")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatalf("failed to create target: %v", err)
	}
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}
	for _, name := range []string{"Silo S01E01.mkv", "Silo S01E02.mkv"} {
		f, err := os.Create(filepath.Join(target, name))
		if err != nil {
			t.Fatalf("failed to create target file: %v", err)
		}
		if err := f.Truncate(MinConsolidationFileSize + 1); err != nil {
			f.Close()
			t.Fatalf("failed to size target file: %v", err)
		}
		f.Close()
	}
	sourceFile := filepath.Join(source, "Silo S01E03.mkv")
	f, err := os.Create(sourceFile)
	if err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}
	if err := f.Truncate(MinConsolidationFileSize + 1); err != nil {
		f.Close()
		t.Fatalf("failed to size source file: %v", err)
	}
	f.Close()

	year := 2023
	conflict := &database.Conflict{
		ID:              42,
		MediaType:       "series",
		Title:           "Silo",
		TitleNormalized: "silo",
		Year:            &year,
		Locations:       []string{missing, target, source},
	}
	cfg := &config.Config{
		Options: config.OptionsConfig{
			DryRun:          false,
			VerifyChecksums: false,
			DeleteSource:    true,
		},
	}

	cons := NewConsolidator(db, cfg)
	plan, err := cons.GeneratePlan(conflict)
	if err != nil {
		t.Fatalf("GeneratePlan failed: %v", err)
	}
	if !plan.CanProceed {
		t.Fatalf("plan should proceed despite missing source dir: %v", plan.Reasons)
	}
	if plan.TargetPath != target {
		t.Fatalf("TargetPath = %q, want %q", plan.TargetPath, target)
	}
	if plan.TotalFiles != 1 {
		t.Fatalf("TotalFiles = %d, want 1", plan.TotalFiles)
	}
	if len(plan.Operations) != 1 || plan.Operations[0].SourcePath != sourceFile {
		t.Fatalf("unexpected operations: %#v", plan.Operations)
	}
}

func TestGeneratePlanBlocksDestinationCollisions(t *testing.T) {
	tempDir := t.TempDir()

	db, err := database.OpenPath(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	target := filepath.Join(tempDir, "storage1", "Silo (2023)")
	source := filepath.Join(tempDir, "storage2", "Silo (2023)")
	if err := os.MkdirAll(filepath.Join(target, "Season 01"), 0755); err != nil {
		t.Fatalf("failed to create target: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(source, "Season 01"), 0755); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	for _, path := range []string{
		filepath.Join(target, "Season 01", "Silo S01E01.mkv"),
		filepath.Join(source, "Season 01", "Silo S01E01.mkv"),
	} {
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
		if err := f.Truncate(MinConsolidationFileSize + 1); err != nil {
			f.Close()
			t.Fatalf("failed to size file: %v", err)
		}
		f.Close()
	}

	year := 2023
	plan, err := NewConsolidator(db, &config.Config{}).GeneratePlan(&database.Conflict{
		ID:              42,
		MediaType:       "series",
		Title:           "Silo",
		TitleNormalized: "silo",
		Year:            &year,
		Locations:       []string{target, source},
	})
	if err != nil {
		t.Fatalf("GeneratePlan failed: %v", err)
	}

	if plan.CanProceed {
		t.Fatalf("plan should not proceed when source files collide with existing target paths")
	}
	if len(plan.Collisions) != 1 {
		t.Fatalf("Collisions = %d, want 1", len(plan.Collisions))
	}
	if !strings.Contains(strings.Join(plan.Reasons, " "), "already exist at target") {
		t.Fatalf("Reasons = %v, want destination collision reason", plan.Reasons)
	}
}

func TestGeneratePlanNormalizesNestedSeriesLocations(t *testing.T) {
	tempDir := t.TempDir()

	db, err := database.OpenPath(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	tvRoot1 := filepath.Join(tempDir, "storage1", "TV")
	tvRoot2 := filepath.Join(tempDir, "storage2", "TV")
	target := filepath.Join(tvRoot1, "Show (2020)")
	source := filepath.Join(tvRoot2, "Show (2020)")
	targetRelease := filepath.Join(target, "Season 01", "Show.S01E01.Release")
	sourceRelease := filepath.Join(source, "Season 01", "Show.S01E02.Release")
	if err := os.MkdirAll(targetRelease, 0755); err != nil {
		t.Fatalf("failed to create target release: %v", err)
	}
	if err := os.MkdirAll(sourceRelease, 0755); err != nil {
		t.Fatalf("failed to create source release: %v", err)
	}
	for _, path := range []string{
		filepath.Join(targetRelease, "hash1.mkv"),
		filepath.Join(sourceRelease, "hash2.mkv"),
	} {
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
		if err := f.Truncate(MinConsolidationFileSize + 1); err != nil {
			f.Close()
			t.Fatalf("failed to size file: %v", err)
		}
		f.Close()
	}

	year := 2020
	plan, err := NewConsolidator(db, &config.Config{
		Libraries: config.LibrariesConfig{TV: []string{tvRoot1, tvRoot2}},
	}).GeneratePlan(&database.Conflict{
		ID:              42,
		MediaType:       "series",
		Title:           "Show",
		TitleNormalized: "show",
		Year:            &year,
		Locations:       []string{targetRelease, sourceRelease},
	})
	if err != nil {
		t.Fatalf("GeneratePlan failed: %v", err)
	}
	if !plan.CanProceed {
		t.Fatalf("plan should proceed after normalizing nested locations: %v", plan.Reasons)
	}
	if plan.TargetPath != target {
		t.Fatalf("TargetPath = %q, want series root %q", plan.TargetPath, target)
	}
	if len(plan.Operations) != 1 {
		t.Fatalf("operations = %d, want 1", len(plan.Operations))
	}
	wantDestination := filepath.Join(target, "Season 01", "Show.S01E02.Release", "hash2.mkv")
	if plan.Operations[0].DestinationPath != wantDestination {
		t.Fatalf("DestinationPath = %q, want %q", plan.Operations[0].DestinationPath, wantDestination)
	}
	if issues := SafetyIssues(plan); len(issues) != 0 {
		t.Fatalf("plan should be safe, got %#v", issues)
	}
}

func TestGeneratePlanSkipsNestedLocationsAlreadyUnderTarget(t *testing.T) {
	tempDir := t.TempDir()

	db, err := database.OpenPath(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	tvRoot := filepath.Join(tempDir, "storage1", "TV")
	target := filepath.Join(tvRoot, "Show (2020)")
	nested := filepath.Join(target, "Season 01", "Show.S01E01.Release")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatalf("failed to create nested release: %v", err)
	}
	file := filepath.Join(nested, "hash1.mkv")
	f, err := os.Create(file)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	if err := f.Truncate(MinConsolidationFileSize + 1); err != nil {
		f.Close()
		t.Fatalf("failed to size file: %v", err)
	}
	f.Close()

	year := 2020
	plan, err := NewConsolidator(db, &config.Config{
		Libraries: config.LibrariesConfig{TV: []string{tvRoot}},
	}).GeneratePlan(&database.Conflict{
		ID:              42,
		MediaType:       "series",
		Title:           "Show",
		TitleNormalized: "show",
		Year:            &year,
		Locations:       []string{target, nested},
	})
	if err != nil {
		t.Fatalf("GeneratePlan failed: %v", err)
	}
	if plan.CanProceed {
		t.Fatalf("plan should not proceed when all locations normalize to one root")
	}
	if len(plan.Operations) != 0 {
		t.Fatalf("operations = %d, want 0", len(plan.Operations))
	}
	if !strings.Contains(strings.Join(plan.Reasons, " "), "No files to move") {
		t.Fatalf("Reasons = %v, want no files to move", plan.Reasons)
	}
}

func TestGeneratePlanSkipsJellywatchQuarantineLocations(t *testing.T) {
	tempDir := t.TempDir()

	db, err := database.OpenPath(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	tvRoot1 := filepath.Join(tempDir, "storage1", "TV")
	tvRoot2 := filepath.Join(tempDir, "storage2", "TV")
	target := filepath.Join(tvRoot1, "Farscape (1999)")
	quarantine := filepath.Join(tvRoot2, "_jellywatch_quarantine_20260607", "teneighty farscape duplicate S04E21")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatalf("failed to create target: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(quarantine, "Season 04"), 0755); err != nil {
		t.Fatalf("failed to create quarantine: %v", err)
	}
	quarantineFile := filepath.Join(quarantine, "Season 04", "teneighty farscape S04E21.mkv")
	f, err := os.Create(quarantineFile)
	if err != nil {
		t.Fatalf("failed to create quarantine file: %v", err)
	}
	if err := f.Truncate(MinConsolidationFileSize + 1); err != nil {
		f.Close()
		t.Fatalf("failed to size quarantine file: %v", err)
	}
	f.Close()

	year := 1999
	plan, err := NewConsolidator(db, &config.Config{
		Libraries: config.LibrariesConfig{TV: []string{tvRoot1, tvRoot2}},
	}).GeneratePlan(&database.Conflict{
		ID:              42,
		MediaType:       "series",
		Title:           "Farscape",
		TitleNormalized: "farscape",
		Year:            &year,
		Locations:       []string{target, quarantine},
	})
	if err != nil {
		t.Fatalf("GeneratePlan failed: %v", err)
	}
	if plan.CanProceed {
		t.Fatalf("plan should not proceed when only source candidate is quarantined")
	}
	if len(plan.SourcePaths) != 1 || plan.SourcePaths[0] != target {
		t.Fatalf("SourcePaths = %#v, want only non-quarantine target", plan.SourcePaths)
	}
	if len(plan.Operations) != 0 {
		t.Fatalf("operations = %d, want 0", len(plan.Operations))
	}
}

func TestGeneratePlanBlocksIdentitySafetyMismatches(t *testing.T) {
	tests := []struct {
		name      string
		locationA string
		fileA     string
		locationB string
		fileB     string
	}{
		{
			name:      "survivor australian survivor",
			locationA: "Australian Survivor (2002)",
			fileA:     "Australian Survivor S08E01.mkv",
			locationB: "Survivor (2000)",
			fileB:     "Survivor S41E01.mkv",
		},
		{
			name:      "survivor au survivor",
			locationA: "Survivor AU (2016)",
			fileA:     "Survivor AU S14E23.mkv",
			locationB: "Survivor (2000)",
			fileB:     "Survivor S41E01.mkv",
		},
		{
			name:      "utopia au utopia",
			locationA: "Utopia (AU) (2014)",
			fileA:     "Utopia (AU) S02E07.mkv",
			locationB: "Utopia (2013)",
			fileB:     "Utopia S01E01.mkv",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			db, err := database.OpenPath(filepath.Join(tempDir, "test.db"))
			if err != nil {
				t.Fatalf("Failed to open database: %v", err)
			}
			defer db.Close()

			tvRoot1 := filepath.Join(tempDir, "storage1", "TV")
			tvRoot2 := filepath.Join(tempDir, "storage2", "TV")
			locationA := filepath.Join(tvRoot1, tt.locationA)
			locationB := filepath.Join(tvRoot2, tt.locationB)
			createLargeMediaFile(t, filepath.Join(locationA, "Season 01", tt.fileA))
			createLargeMediaFile(t, filepath.Join(locationB, "Season 01", tt.fileB))

			plan, err := NewConsolidator(db, &config.Config{
				Libraries: config.LibrariesConfig{TV: []string{tvRoot1, tvRoot2}},
			}).GeneratePlan(&database.Conflict{
				ID:        42,
				MediaType: "series",
				Title:     "Identity Check",
				Locations: []string{locationA, locationB},
			})
			if err != nil {
				t.Fatalf("GeneratePlan failed: %v", err)
			}
			if plan.CanProceed {
				t.Fatalf("plan should not proceed for identity mismatch")
			}
			if !strings.Contains(strings.Join(plan.Reasons, " "), "identity safety") {
				t.Fatalf("Reasons = %v, want identity safety reason", plan.Reasons)
			}
		})
	}
}

func TestGeneratePlanAllowsIdentitySafeSameSeries(t *testing.T) {
	tempDir := t.TempDir()
	db, err := database.OpenPath(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	tvRoot1 := filepath.Join(tempDir, "storage1", "TV")
	tvRoot2 := filepath.Join(tempDir, "storage2", "TV")
	target := filepath.Join(tvRoot1, "Silo (2023)")
	source := filepath.Join(tvRoot2, "Silo (2023)")
	createLargeMediaFile(t, filepath.Join(target, "Season 01", "Silo S01E01.mkv"))
	sourceFile := filepath.Join(source, "Season 01", "Silo S01E02.mkv")
	createLargeMediaFile(t, sourceFile)

	year := 2023
	plan, err := NewConsolidator(db, &config.Config{
		Libraries: config.LibrariesConfig{TV: []string{tvRoot1, tvRoot2}},
	}).GeneratePlan(&database.Conflict{
		ID:              42,
		MediaType:       "series",
		Title:           "Silo",
		TitleNormalized: "silo",
		Year:            &year,
		Locations:       []string{target, source},
	})
	if err != nil {
		t.Fatalf("GeneratePlan failed: %v", err)
	}
	if !plan.CanProceed {
		t.Fatalf("plan should proceed for same series: %v", plan.Reasons)
	}
	if len(plan.Operations) != 1 || plan.Operations[0].SourcePath != sourceFile {
		t.Fatalf("unexpected operations: %#v", plan.Operations)
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
		result, _, err := consolidator.getFilesToMove(tmpDir, "/target", conflict)
		if err != nil {
			t.Fatal(err)
		}

		if len(result) > 0 {
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
		result, _, err := consolidator.getFilesToMove(tmpDir, "/target", conflict)
		if err != nil {
			t.Fatal(err)
		}

		if len(result) == 0 {
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
		result, _, err := consolidator.getFilesToMove(tmpDir, "/target", conflict)
		if err != nil {
			t.Fatal(err)
		}

		if len(result) > 0 {
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
