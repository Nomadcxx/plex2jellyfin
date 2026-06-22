package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/database"
)

func setupTestDB(t *testing.T) *database.MediaDB {
	tempFile := filepath.Join(t.TempDir(), "test.db")
	db, err := database.OpenPath(tempFile)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	return db
}

func intPtr(v int) *int {
	return &v
}

func TestAnalyzeDuplicates_Empty(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	svc := NewCleanupService(db)
	analysis, err := svc.AnalyzeDuplicates()
	if err != nil {
		t.Fatalf("AnalyzeDuplicates failed: %v", err)
	}

	if analysis.TotalGroups != 0 {
		t.Errorf("Expected 0 groups, got %d", analysis.TotalGroups)
	}
}

func TestPruneMissingMediaFilesRemovesOnlyMissingRows(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	root := t.TempDir()
	year := 2026
	existingPath := filepath.Join(root, "existing.mkv")
	if err := os.WriteFile(existingPath, []byte("video"), 0644); err != nil {
		t.Fatalf("failed to create existing file: %v", err)
	}

	existing := &database.MediaFile{
		Path:            existingPath,
		NormalizedTitle: "example",
		Year:            &year,
		MediaType:       "movie",
		Size:            5,
	}
	missing := &database.MediaFile{
		Path:            filepath.Join(root, "missing.mkv"),
		NormalizedTitle: "missing",
		Year:            &year,
		MediaType:       "movie",
		Size:            5,
	}
	if err := db.UpsertMediaFile(existing); err != nil {
		t.Fatalf("failed to insert existing row: %v", err)
	}
	if err := db.UpsertMediaFile(missing); err != nil {
		t.Fatalf("failed to insert missing row: %v", err)
	}

	svc := NewCleanupService(db)
	result, err := svc.PruneMissingMediaFiles()
	if err != nil {
		t.Fatalf("PruneMissingMediaFiles failed: %v", err)
	}
	if result.Checked != 2 {
		t.Fatalf("Checked = %d, want 2", result.Checked)
	}
	if result.Pruned != 1 {
		t.Fatalf("Pruned = %d, want 1", result.Pruned)
	}
	if got, err := db.GetMediaFileByID(missing.ID); err != nil {
		t.Fatalf("GetMediaFileByID failed: %v", err)
	} else if got != nil {
		t.Fatalf("missing row should be pruned")
	}
	if got, err := db.GetMediaFileByID(existing.ID); err != nil {
		t.Fatalf("GetMediaFileByID failed: %v", err)
	} else if got == nil {
		t.Fatalf("existing row should remain")
	}
}

func TestAnalyzeDuplicates_FindsMovieDuplicates(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert duplicate movies
	year := 2005
	root := t.TempDir()
	path1 := filepath.Join(root, "storage1", "Movies", "Robots (2005)", "Robots.mkv")
	path2 := filepath.Join(root, "storage2", "Movies", "Robots (2005)", "Robots.mkv")
	for _, path := range []string{path1, path2} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(path, []byte("video"), 0644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
	}
	_ = db.UpsertMediaFile(&database.MediaFile{
		Path:            path1,
		NormalizedTitle: "robots",
		Year:            &year,
		MediaType:       "movie",
		Size:            4000000000,
		QualityScore:    284,
		Resolution:      "720p",
		SourceType:      "BluRay",
	})
	_ = db.UpsertMediaFile(&database.MediaFile{
		Path:            path2,
		NormalizedTitle: "robots",
		Year:            &year,
		MediaType:       "movie",
		Size:            4400000000,
		QualityScore:    84,
		Resolution:      "unknown",
		SourceType:      "unknown",
	})

	svc := NewCleanupService(db)
	analysis, err := svc.AnalyzeDuplicates()
	if err != nil {
		t.Fatalf("AnalyzeDuplicates failed: %v", err)
	}

	if analysis.TotalGroups != 1 {
		t.Errorf("Expected 1 group, got %d", analysis.TotalGroups)
	}

	if len(analysis.Groups) > 0 && len(analysis.Groups[0].Files) != 2 {
		t.Errorf("Expected 2 files in group, got %d", len(analysis.Groups[0].Files))
	}
}

func TestAnalyzeDuplicates_TieBreaksBestFileBySize(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	year := 2021
	root := t.TempDir()
	small := &database.MediaFile{
		Path:            filepath.Join(root, "storage1", "TV", "Invincible S04E08.mp4"),
		NormalizedTitle: "invincible",
		Year:            &year,
		Season:          intPtr(4),
		Episode:         intPtr(8),
		MediaType:       "episode",
		Size:            447130391,
		QualityScore:    0,
	}
	large := &database.MediaFile{
		Path:            filepath.Join(root, "storage1", "TV", "Invincible S04E08.mkv"),
		NormalizedTitle: "invincible",
		Year:            &year,
		Season:          intPtr(4),
		Episode:         intPtr(8),
		MediaType:       "episode",
		Size:            718249625,
		QualityScore:    0,
	}
	for _, file := range []*database.MediaFile{small, large} {
		if err := os.MkdirAll(filepath.Dir(file.Path), 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(file.Path, []byte("video"), 0644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
	}
	if err := db.UpsertMediaFile(small); err != nil {
		t.Fatalf("failed to insert small file: %v", err)
	}
	if err := db.UpsertMediaFile(large); err != nil {
		t.Fatalf("failed to insert large file: %v", err)
	}

	svc := NewCleanupService(db)
	analysis, err := svc.AnalyzeDuplicates()
	if err != nil {
		t.Fatalf("AnalyzeDuplicates failed: %v", err)
	}
	if analysis.TotalGroups != 1 {
		t.Fatalf("TotalGroups = %d, want 1", analysis.TotalGroups)
	}
	if analysis.Groups[0].BestFileID != large.ID {
		t.Fatalf("BestFileID = %d, want larger file id %d", analysis.Groups[0].BestFileID, large.ID)
	}
	if analysis.Groups[0].ReclaimableBytes != small.Size {
		t.Fatalf("ReclaimableBytes = %d, want smaller file size %d", analysis.Groups[0].ReclaimableBytes, small.Size)
	}
}

func TestAnalyzeDuplicates_IgnoresMissingFilesBeforeChoosingKeeper(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	root := t.TempDir()
	year := 2026
	missingHighQuality := &database.MediaFile{
		Path:            filepath.Join(root, "missing", "F Valentines Day.mkv"),
		NormalizedTitle: "fvalentinesday",
		Year:            &year,
		MediaType:       "movie",
		Size:            5334752016,
		QualityScore:    84,
	}
	existingLowerQuality := &database.MediaFile{
		Path:            filepath.Join(root, "existing", "F Valentines Day.mkv"),
		NormalizedTitle: "fvalentinesday",
		Year:            &year,
		MediaType:       "movie",
		Size:            2162719007,
		QualityScore:    42,
	}
	if err := os.MkdirAll(filepath.Dir(existingLowerQuality.Path), 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	if err := os.WriteFile(existingLowerQuality.Path, []byte("video"), 0644); err != nil {
		t.Fatalf("failed to create existing file: %v", err)
	}
	if err := db.UpsertMediaFile(missingHighQuality); err != nil {
		t.Fatalf("failed to insert missing file row: %v", err)
	}
	if err := db.UpsertMediaFile(existingLowerQuality); err != nil {
		t.Fatalf("failed to insert existing file row: %v", err)
	}

	svc := NewCleanupService(db)
	analysis, err := svc.AnalyzeDuplicates()
	if err != nil {
		t.Fatalf("AnalyzeDuplicates failed: %v", err)
	}
	if analysis.TotalGroups != 0 {
		t.Fatalf("TotalGroups = %d, want 0 because only one duplicate candidate exists on disk", analysis.TotalGroups)
	}
	if got, err := db.GetMediaFileByID(missingHighQuality.ID); err != nil {
		t.Fatalf("GetMediaFileByID failed: %v", err)
	} else if got != nil {
		t.Fatalf("missing file row should be pruned from the database")
	}
	if got, err := db.GetMediaFileByID(existingLowerQuality.ID); err != nil {
		t.Fatalf("GetMediaFileByID failed: %v", err)
	} else if got == nil {
		t.Fatalf("existing file row should remain in the database")
	}
}

func TestDeleteDuplicateFileDeletesMember(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	root := t.TempDir()
	year := 2005
	keepPath := filepath.Join(root, "storage1", "Robots.mkv")
	deletePath := filepath.Join(root, "storage2", "Robots.mkv")
	for _, path := range []string{keepPath, deletePath} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
	}
	if err := os.WriteFile(keepPath, []byte("keep"), 0644); err != nil {
		t.Fatalf("failed to create keep file: %v", err)
	}
	if err := os.WriteFile(deletePath, []byte("delete"), 0644); err != nil {
		t.Fatalf("failed to create duplicate file: %v", err)
	}

	keep := &database.MediaFile{
		Path:            keepPath,
		NormalizedTitle: "robots",
		Year:            &year,
		MediaType:       "movie",
		Size:            4,
		QualityScore:    100,
	}
	duplicate := &database.MediaFile{
		Path:            deletePath,
		NormalizedTitle: "robots",
		Year:            &year,
		MediaType:       "movie",
		Size:            6,
		QualityScore:    50,
	}
	if err := db.UpsertMediaFile(keep); err != nil {
		t.Fatalf("failed to insert keep file: %v", err)
	}
	if err := db.UpsertMediaFile(duplicate); err != nil {
		t.Fatalf("failed to insert duplicate file: %v", err)
	}

	svc := NewCleanupService(db)
	groupID := generateGroupID("robots", &year, nil, nil)
	if err := svc.DeleteDuplicateFile(groupID, duplicate.ID); err != nil {
		t.Fatalf("DeleteDuplicateFile failed: %v", err)
	}
	if _, err := os.Stat(deletePath); !os.IsNotExist(err) {
		t.Fatalf("duplicate file still exists after delete")
	}
	if got, err := db.GetMediaFileByID(duplicate.ID); err != nil {
		t.Fatalf("GetMediaFileByID failed: %v", err)
	} else if got != nil {
		t.Fatalf("duplicate DB row still exists after delete")
	}
}

func TestDeleteDuplicateFileIsIdempotentForStaleDeletedFile(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	year := 2005
	deleted := &database.MediaFile{
		Path:            filepath.Join(t.TempDir(), "Robots.mkv"),
		NormalizedTitle: "robots",
		Year:            &year,
		MediaType:       "movie",
		Size:            6,
		QualityScore:    50,
	}
	if err := db.UpsertMediaFile(deleted); err != nil {
		t.Fatalf("failed to insert deleted file: %v", err)
	}
	if err := db.DeleteMediaFileByID(deleted.ID); err != nil {
		t.Fatalf("failed to simulate prior delete: %v", err)
	}

	svc := NewCleanupService(db)
	groupID := generateGroupID("robots", &year, nil, nil)
	if err := svc.DeleteDuplicateFile(groupID, deleted.ID); err != nil {
		t.Fatalf("DeleteDuplicateFile should be idempotent for stale deleted file, got: %v", err)
	}
}

func TestDeleteDuplicateFileRejectsFileOutsideGroup(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	root := t.TempDir()
	year := 2005
	otherYear := 2006
	for _, file := range []*database.MediaFile{
		{Path: filepath.Join(root, "storage1", "Robots.mkv"), NormalizedTitle: "robots", Year: &year, MediaType: "movie", Size: 4, QualityScore: 100},
		{Path: filepath.Join(root, "storage2", "Robots.mkv"), NormalizedTitle: "robots", Year: &year, MediaType: "movie", Size: 6, QualityScore: 50},
		{Path: filepath.Join(root, "storage3", "Cars.mkv"), NormalizedTitle: "cars", Year: &otherYear, MediaType: "movie", Size: 8, QualityScore: 40},
	} {
		if err := os.MkdirAll(filepath.Dir(file.Path), 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(file.Path, []byte("video"), 0644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
		if err := db.UpsertMediaFile(file); err != nil {
			t.Fatalf("failed to insert media file: %v", err)
		}
	}

	other, err := db.GetMediaFile(filepath.Join(root, "storage3", "Cars.mkv"))
	if err != nil {
		t.Fatalf("GetMediaFile failed: %v", err)
	}
	svc := NewCleanupService(db)
	groupID := generateGroupID("robots", &year, nil, nil)
	if err := svc.DeleteDuplicateFile(groupID, other.ID); err == nil {
		t.Fatalf("DeleteDuplicateFile should reject a file outside the duplicate group")
	}
	if _, err := os.Stat(other.Path); err != nil {
		t.Fatalf("outside-group file should remain on disk: %v", err)
	}
}

func TestAnalyzeScattered_FindsConflicts(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	root := t.TempDir()
	path1 := filepath.Join(root, "storage1", "American Dad (2005)")
	path2 := filepath.Join(root, "storage2", "American Dad! (2005)")
	if err := os.MkdirAll(path1, 0755); err != nil {
		t.Fatalf("failed to create path1: %v", err)
	}
	if err := os.MkdirAll(path2, 0755); err != nil {
		t.Fatalf("failed to create path2: %v", err)
	}
	for _, path := range []string{
		filepath.Join(path1, "American Dad S01E01.mkv"),
		filepath.Join(path1, "American Dad S01E02.mkv"),
		filepath.Join(path2, "American Dad S01E03.mkv"),
	} {
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("failed to create media file: %v", err)
		}
		if err := f.Truncate(minConsolidationFileSize + 1); err != nil {
			f.Close()
			t.Fatalf("failed to size media file: %v", err)
		}
		f.Close()
	}

	// Insert conflicting series in different locations (series table, not media_files)
	// The DetectConflicts function queries series/movies tables for conflicts
	year := 2005
	series1 := &database.Series{
		Title:           "American Dad",
		TitleNormalized: "american dad",
		Year:            year,
		CanonicalPath:   path1,
		LibraryRoot:     filepath.Dir(path1),
		Source:          "filesystem",
		SourcePriority:  50,
		EpisodeCount:    1,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	_, _ = db.UpsertSeries(series1)

	series2 := &database.Series{
		Title:           "American Dad!",
		TitleNormalized: "american dad",
		Year:            year,
		CanonicalPath:   path2,
		LibraryRoot:     filepath.Dir(path2),
		Source:          "filesystem",
		SourcePriority:  50,
		EpisodeCount:    1,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	_, _ = db.UpsertSeries(series2)

	svc := NewCleanupService(db)
	analysis, err := svc.AnalyzeScattered()
	if err != nil {
		t.Fatalf("AnalyzeScattered failed: %v", err)
	}

	if analysis.TotalItems != 1 {
		t.Errorf("Expected 1 scattered item, got %d", analysis.TotalItems)
	}
}

func TestAnalyzeScattered_FiltersMissingConflictLocations(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	root := t.TempDir()
	target := filepath.Join(root, "storage1", "American Dad! (2005)")
	source := filepath.Join(root, "storage2", "American Dad! (2005)")
	missing := filepath.Join(root, "storage3", "American Dad!")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatalf("failed to create target: %v", err)
	}
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}
	for _, path := range []string{
		filepath.Join(target, "American Dad S01E01.mkv"),
		filepath.Join(target, "American Dad S01E02.mkv"),
		filepath.Join(source, "American Dad S01E03.mkv"),
	} {
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("failed to create media file: %v", err)
		}
		if err := f.Truncate(minConsolidationFileSize + 1); err != nil {
			f.Close()
			t.Fatalf("failed to size media file: %v", err)
		}
		f.Close()
	}

	locations, err := json.Marshal([]string{missing, target, source})
	if err != nil {
		t.Fatalf("failed to marshal locations: %v", err)
	}
	_, err = db.DB().Exec(`
		INSERT INTO conflicts (media_type, title, title_normalized, year, locations, resolved, created_at)
		VALUES ('series', 'American Dad', 'americandad', 2005, ?, FALSE, CURRENT_TIMESTAMP)
	`, string(locations))
	if err != nil {
		t.Fatalf("failed to insert conflict: %v", err)
	}

	svc := NewCleanupService(db)
	analysis, err := svc.AnalyzeScattered()
	if err != nil {
		t.Fatalf("AnalyzeScattered failed: %v", err)
	}

	if analysis.TotalItems != 1 {
		t.Fatalf("Expected 1 scattered item, got %d", analysis.TotalItems)
	}
	item := analysis.Items[0]
	for _, loc := range item.Locations {
		if loc == missing {
			t.Fatalf("missing location leaked into API response: %v", item.Locations)
		}
	}
	if item.TargetLocation == missing {
		t.Fatalf("target location must not be missing")
	}
	if item.FilesToMove != 1 {
		t.Fatalf("FilesToMove = %d, want 1", item.FilesToMove)
	}
}
