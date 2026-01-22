package database

import (
	"os"
	"testing"
	"time"
)

func TestMediaFileMigrations(t *testing.T) {
	// Create temporary database
	tmpFile, err := os.CreateTemp("", "jellywatch-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Open database (should run migrations)
	db, err := OpenPath(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Verify schema version
	var version int
	err = db.db.QueryRow("SELECT version FROM schema_version ORDER BY version DESC LIMIT 1").Scan(&version)
	if err != nil {
		t.Fatalf("failed to get schema version: %v", err)
	}

	if version != currentSchemaVersion {
		t.Errorf("expected schema version %d, got %d", currentSchemaVersion, version)
	}

	// Verify tables exist
	tables := []string{"media_files", "episodes", "consolidation_plans"}
	for _, table := range tables {
		var count int
		err := db.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
		if err != nil {
			t.Errorf("table %s does not exist or is not accessible: %v", table, err)
		}
	}
}

func TestMediaFileCRUD(t *testing.T) {
	// Create temporary database
	tmpFile, err := os.CreateTemp("", "jellywatch-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	db, err := OpenPath(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Test insert
	year := 2014
	file := &MediaFile{
		Path:       "/test/Interstellar (2014)/Interstellar (2014).mkv",
		Size:       3500000000, // 3.5 GB
		ModifiedAt: time.Now(),

		MediaType:       "movie",
		NormalizedTitle: "interstellar",
		Year:            &year,

		Resolution:          "1080p",
		SourceType:          "BluRay",
		Codec:               "x264",
		AudioFormat:         "DTS",
		QualityScore:        305,
		IsJellyfinCompliant: true,
		ComplianceIssues:    []string{},

		Source:         "filesystem",
		SourcePriority: 50,
		LibraryRoot:    "/test",
	}

	err = db.UpsertMediaFile(file)
	if err != nil {
		t.Fatalf("failed to insert media file: %v", err)
	}

	if file.ID == 0 {
		t.Error("expected ID to be set after insert")
	}

	// Test retrieve
	retrieved, err := db.GetMediaFile(file.Path)
	if err != nil {
		t.Fatalf("failed to retrieve media file: %v", err)
	}

	if retrieved == nil {
		t.Fatal("expected to retrieve media file, got nil")
	}

	if retrieved.NormalizedTitle != file.NormalizedTitle {
		t.Errorf("expected normalized title %s, got %s", file.NormalizedTitle, retrieved.NormalizedTitle)
	}

	if retrieved.Resolution != file.Resolution {
		t.Errorf("expected resolution %s, got %s", file.Resolution, retrieved.Resolution)
	}

	// Test update
	file.QualityScore = 350
	file.Resolution = "2160p"

	err = db.UpsertMediaFile(file)
	if err != nil {
		t.Fatalf("failed to update media file: %v", err)
	}

	updated, err := db.GetMediaFile(file.Path)
	if err != nil {
		t.Fatalf("failed to retrieve updated media file: %v", err)
	}

	if updated.QualityScore != 350 {
		t.Errorf("expected quality score 350, got %d", updated.QualityScore)
	}

	if updated.Resolution != "2160p" {
		t.Errorf("expected resolution 2160p, got %s", updated.Resolution)
	}

	// Test delete
	err = db.DeleteMediaFile(file.Path)
	if err != nil {
		t.Fatalf("failed to delete media file: %v", err)
	}

	deleted, err := db.GetMediaFile(file.Path)
	if err != nil {
		t.Fatalf("failed to check deleted media file: %v", err)
	}

	if deleted != nil {
		t.Error("expected media file to be deleted, but it still exists")
	}
}

func TestDuplicateDetection(t *testing.T) {
	// Create temporary database
	tmpFile, err := os.CreateTemp("", "jellywatch-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	db, err := OpenPath(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Insert duplicate movies
	year := 2013
	files := []*MediaFile{
		{
			Path:            "/storage1/Her (2013)/Her (2013).mkv",
			Size:            5500000000,
			MediaType:       "movie",
			NormalizedTitle: "her",
			Year:            &year,
			Resolution:      "1080p",
			SourceType:      "WEB-DL",
			QualityScore:    305,
			LibraryRoot:     "/storage1",
		},
		{
			Path:            "/storage8/Her (2013)/Her.2013.MULTI.1080p.BluRay.x264-Goatlove.mkv",
			Size:            9800000000,
			MediaType:       "movie",
			NormalizedTitle: "her",
			Year:            &year,
			Resolution:      "1080p",
			SourceType:      "BluRay",
			QualityScore:    380,
			LibraryRoot:     "/storage8",
		},
	}

	for _, f := range files {
		if err := db.UpsertMediaFile(f); err != nil {
			t.Fatalf("failed to insert media file: %v", err)
		}
	}

	// Find duplicates
	duplicates, err := db.FindDuplicateMovies()
	if err != nil {
		t.Fatalf("failed to find duplicate movies: %v", err)
	}

	if len(duplicates) != 1 {
		t.Fatalf("expected 1 duplicate group, got %d", len(duplicates))
	}

	group := duplicates[0]
	if group.NormalizedTitle != "her" {
		t.Errorf("expected normalized title 'her', got %s", group.NormalizedTitle)
	}

	if len(group.Files) != 2 {
		t.Errorf("expected 2 files in group, got %d", len(group.Files))
	}

	// Verify best file (highest quality score)
	if group.BestFile.QualityScore != 380 {
		t.Errorf("expected best file quality score 380, got %d", group.BestFile.QualityScore)
	}

	// Verify space reclaimable
	expectedReclaimable := int64(5500000000)
	if group.SpaceReclaimable != expectedReclaimable {
		t.Errorf("expected space reclaimable %d, got %d", expectedReclaimable, group.SpaceReclaimable)
	}
}

func TestEpisodeCRUD(t *testing.T) {
	// Create temporary database
	tmpFile, err := os.CreateTemp("", "jellywatch-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	db, err := OpenPath(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// First create a series
	year := 2023
	series := &Series{
		Title:           "Silo",
		TitleNormalized: "silo",
		Year:            year,
		CanonicalPath:   "/test/Silo (2023)",
		LibraryRoot:     "/test",
		Source:          "filesystem",
		SourcePriority:  50,
	}

	_, err = db.UpsertSeries(series)
	if err != nil {
		t.Fatalf("failed to insert series: %v", err)
	}

	// Insert episode
	episode := &Episode{
		SeriesID: series.ID,
		Season:   1,
		Episode:  1,
		Title:    "Freedom Day",
	}

	err = db.UpsertEpisode(episode)
	if err != nil {
		t.Fatalf("failed to insert episode: %v", err)
	}

	if episode.ID == 0 {
		t.Error("expected episode ID to be set after insert")
	}

	// Retrieve episode
	retrieved, err := db.GetEpisode(series.ID, 1, 1)
	if err != nil {
		t.Fatalf("failed to retrieve episode: %v", err)
	}

	if retrieved == nil {
		t.Fatal("expected to retrieve episode, got nil")
	}

	if retrieved.Title != episode.Title {
		t.Errorf("expected title %s, got %s", episode.Title, retrieved.Title)
	}

	// Update best file
	fileID := int64(123)
	err = db.UpdateEpisodeBestFile(episode.ID, &fileID)
	if err != nil {
		t.Fatalf("failed to update episode best file: %v", err)
	}

	updated, err := db.GetEpisode(series.ID, 1, 1)
	if err != nil {
		t.Fatalf("failed to retrieve updated episode: %v", err)
	}

	if updated.BestFileID == nil || *updated.BestFileID != fileID {
		t.Errorf("expected best file ID %d, got %v", fileID, updated.BestFileID)
	}
}

func TestConsolidationStats(t *testing.T) {
	// Create temporary database
	tmpFile, err := os.CreateTemp("", "jellywatch-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	db, err := OpenPath(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Insert test data
	year := 2014
	files := []*MediaFile{
		{
			Path:                "/test1/movie1.mkv",
			Size:                1000000000,
			MediaType:           "movie",
			NormalizedTitle:     "movie1",
			Year:                &year,
			QualityScore:        300,
			IsJellyfinCompliant: true,
			LibraryRoot:         "/test1",
		},
		{
			Path:                "/test2/movie1.mkv",
			Size:                2000000000,
			MediaType:           "movie",
			NormalizedTitle:     "movie1",
			Year:                &year,
			QualityScore:        350,
			IsJellyfinCompliant: true,
			LibraryRoot:         "/test2",
		},
		{
			Path:                "/test3/movie2.mkv",
			Size:                3000000000,
			MediaType:           "movie",
			NormalizedTitle:     "movie2",
			Year:                &year,
			QualityScore:        300,
			IsJellyfinCompliant: false,
			LibraryRoot:         "/test3",
		},
	}

	for _, f := range files {
		if err := db.UpsertMediaFile(f); err != nil {
			t.Fatalf("failed to insert media file: %v", err)
		}
	}

	// Get stats
	stats, err := db.GetConsolidationStats()
	if err != nil {
		t.Fatalf("failed to get consolidation stats: %v", err)
	}

	if stats.TotalFiles != 3 {
		t.Errorf("expected 3 total files, got %d", stats.TotalFiles)
	}

	if stats.DuplicateGroups != 1 {
		t.Errorf("expected 1 duplicate group, got %d", stats.DuplicateGroups)
	}

	if stats.NonCompliantFiles != 1 {
		t.Errorf("expected 1 non-compliant file, got %d", stats.NonCompliantFiles)
	}

	if stats.SpaceReclaimable != 1000000000 {
		t.Errorf("expected space reclaimable 1000000000, got %d", stats.SpaceReclaimable)
	}
}

func TestMigration7NullableSourceFileID(t *testing.T) {
	// Create temporary database
	tmpFile, err := os.CreateTemp("", "jellywatch-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	db, err := OpenPath(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	query := `
		INSERT INTO consolidation_plans 
		(action, source_path, target_path, reason) 
		VALUES (?, ?, ?, ?)
	`
	_, err = db.db.Exec(query, "move", "/src/file.mkv", "/dst/file.mkv", "test")

	if err != nil {
		t.Errorf("Expected no error inserting plan without source_file_id, got: %v", err)
	}

	var count int
	err = db.db.QueryRow("SELECT COUNT(*) FROM consolidation_plans WHERE source_path = ?", "/src/file.mkv").Scan(&count)
	if err != nil {
		t.Errorf("Failed to query consolidation_plans: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 consolidation plan, got %d", count)
	}
}
