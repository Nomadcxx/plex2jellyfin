package scanner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/ai"
	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/database"
)

func TestNewFileScanner(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	scanner := NewFileScanner(db)

	if scanner.db == nil {
		t.Error("Expected db to be set")
	}

	if scanner.minMovieSize != 500*1024*1024 {
		t.Errorf("Expected minMovieSize = 500MB, got %d", scanner.minMovieSize)
	}

	if scanner.minEpisodeSize != 50*1024*1024 {
		t.Errorf("Expected minEpisodeSize = 50MB, got %d", scanner.minEpisodeSize)
	}

	if len(scanner.skipPatterns) == 0 {
		t.Error("Expected skip patterns to be set")
	}
}

func TestIsVideoFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/path/to/movie.mkv", true},
		{"/path/to/movie.mp4", true},
		{"/path/to/movie.avi", true},
		{"/path/to/movie.m4v", true},
		{"/path/to/subtitle.srt", false},
		{"/path/to/cover.jpg", false},
		{"/path/to/readme.txt", false},
		{"/path/to/movie.MKV", true}, // Case insensitive
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := isVideoFile(tt.path)
			if result != tt.expected {
				t.Errorf("isVideoFile(%s) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestIsExtraContent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	scanner := NewFileScanner(db)

	tests := []struct {
		path     string
		expected bool
	}{
		{"/media/Movies/Movie (2020)/Movie (2020).mkv", false},
		{"/media/Movies/Movie (2020)/Movie-sample.mkv", true},
		{"/media/Movies/Movie (2020)/trailer.mp4", true},
		{"/media/Movies/Movie (2020)/Extras/deleted-scene.mkv", true},
		{"/media/Movies/Movie (2020)/Featurettes/behind.mkv", true},
		{"/media/Movies/Movie (2020)/cover.jpg", true},
		{"/media/TV/Show/Season 01/Show S01E01.mkv", false},
		{"/media/TV/Show/Season 01/RARBG.txt", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := scanner.isExtraContent(tt.path)
			if result != tt.expected {
				t.Errorf("isExtraContent(%s) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestShouldIncludeFile(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	scanner := NewFileScanner(db)

	tests := []struct {
		name      string
		path      string
		size      int64
		mediaType string
		expected  bool
	}{
		{
			name:      "Valid movie",
			path:      "/media/Movies/Movie (2020)/Movie (2020).mkv",
			size:      600 * 1024 * 1024, // 600MB
			mediaType: "movie",
			expected:  true,
		},
		{
			name:      "Movie too small",
			path:      "/media/Movies/Movie (2020)/Movie (2020).mkv",
			size:      100 * 1024 * 1024, // 100MB
			mediaType: "movie",
			expected:  false,
		},
		{
			name:      "Valid episode",
			path:      "/media/TV/Show/Season 01/Show S01E01.mkv",
			size:      100 * 1024 * 1024, // 100MB
			mediaType: "episode",
			expected:  true,
		},
		{
			name:      "Episode too small",
			path:      "/media/TV/Show/Season 01/Show S01E01.mkv",
			size:      10 * 1024 * 1024, // 10MB
			mediaType: "episode",
			expected:  false,
		},
		{
			name:      "Sample file",
			path:      "/media/Movies/Movie (2020)/sample.mkv",
			size:      600 * 1024 * 1024,
			mediaType: "movie",
			expected:  false,
		},
		{
			name:      "Trailer",
			path:      "/media/Movies/Movie (2020)/trailer.mp4",
			size:      600 * 1024 * 1024,
			mediaType: "movie",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := scanner.shouldIncludeFile(tt.path, tt.size, tt.mediaType)
			if result != tt.expected {
				t.Errorf("shouldIncludeFile() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestProcessFile_Movie(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	scanner := NewFileScanner(db)

	// Create a temporary test file
	tempDir := t.TempDir()
	movieDir := filepath.Join(tempDir, "Interstellar (2014)")
	err := os.MkdirAll(movieDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	moviePath := filepath.Join(movieDir, "Interstellar (2014).mkv")
	err = os.WriteFile(moviePath, []byte("fake video content"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(moviePath)
	if err != nil {
		t.Fatal(err)
	}

	// Process the file
	result := &ScanResult{}
	err = scanner.processFile(moviePath, info, tempDir, "movie", result)
	if err != nil {
		t.Fatalf("processFile failed: %v", err)
	}

	// Verify it was added to database
	file, err := db.GetMediaFile(moviePath)
	if err != nil {
		t.Fatalf("Failed to get media file: %v", err)
	}

	if file == nil {
		t.Fatal("Expected file to be in database")
	}

	if file.MediaType != "movie" {
		t.Errorf("Expected media_type = 'movie', got '%s'", file.MediaType)
	}

	if file.NormalizedTitle != "interstellar" {
		t.Errorf("Expected normalized_title = 'interstellar', got '%s'", file.NormalizedTitle)
	}

	if file.Year == nil || *file.Year != 2014 {
		t.Errorf("Expected year = 2014, got %v", file.Year)
	}

	if !file.IsJellyfinCompliant {
		t.Error("Expected compliant file")
	}
}

func TestProcessFile_Episode(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	scanner := NewFileScanner(db)

	// Create a temporary test file
	tempDir := t.TempDir()
	showDir := filepath.Join(tempDir, "Silo (2023)", "Season 01")
	err := os.MkdirAll(showDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	episodePath := filepath.Join(showDir, "Silo (2023) S01E01.mkv")
	err = os.WriteFile(episodePath, []byte("fake video content"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(episodePath)
	if err != nil {
		t.Fatal(err)
	}

	// Process the file
	result := &ScanResult{}
	err = scanner.processFile(episodePath, info, tempDir, "episode", result)
	if err != nil {
		t.Fatalf("processFile failed: %v", err)
	}

	// Verify it was added to database
	file, err := db.GetMediaFile(episodePath)
	if err != nil {
		t.Fatalf("Failed to get media file: %v", err)
	}

	if file == nil {
		t.Fatal("Expected file to be in database")
	}

	if file.MediaType != "episode" {
		t.Errorf("Expected media_type = 'episode', got '%s'", file.MediaType)
	}

	if file.NormalizedTitle != "silo" {
		t.Errorf("Expected normalized_title = 'silo', got '%s'", file.NormalizedTitle)
	}

	if file.Year == nil || *file.Year != 2023 {
		t.Errorf("Expected year = 2023, got %v", file.Year)
	}

	if file.Season == nil || *file.Season != 1 {
		t.Errorf("Expected season = 1, got %v", file.Season)
	}

	if file.Episode == nil || *file.Episode != 1 {
		t.Errorf("Expected episode = 1, got %v", file.Episode)
	}

	if !file.IsJellyfinCompliant {
		t.Error("Expected compliant file")
	}
}

func TestProcessFile_ObfuscatedEpisodeWithoutEpisodeMarkerIndexesForReview(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	scanner := NewFileScanner(db)

	tempDir := t.TempDir()
	tvRoot := filepath.Join(tempDir, "TVSHOWS")
	showDir := filepath.Join(tvRoot, "The Daily Show (1996)", "Season 2020")
	if err := os.MkdirAll(showDir, 0755); err != nil {
		t.Fatal(err)
	}

	episodePath := filepath.Join(showDir, "248fc0bc4f6f454b89a8158018a398a6.mkv")
	if err := os.WriteFile(episodePath, []byte("fake video content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(episodePath, 100*1024*1024); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(episodePath)
	if err != nil {
		t.Fatal(err)
	}

	result := &ScanResult{}
	if err := scanner.processFile(episodePath, info, tvRoot, "episode", result); err != nil {
		t.Fatalf("processFile failed: %v", err)
	}

	file, err := db.GetMediaFile(episodePath)
	if err != nil {
		t.Fatalf("Failed to get media file: %v", err)
	}
	if file == nil {
		t.Fatal("Expected file to be in database")
	}
	if file.MediaType != "episode" {
		t.Fatalf("Expected media_type = episode, got %q", file.MediaType)
	}
	if file.NormalizedTitle != "thedailyshow" {
		t.Fatalf("Expected normalized_title = thedailyshow, got %q", file.NormalizedTitle)
	}
	if file.Year == nil || *file.Year != 1996 {
		t.Fatalf("Expected year = 1996, got %v", file.Year)
	}
	if file.Season == nil || *file.Season != 2020 {
		t.Fatalf("Expected season = 2020, got %v", file.Season)
	}
	if file.Episode != nil {
		t.Fatalf("Expected unknown episode to stay nil, got %v", *file.Episode)
	}
	if !file.NeedsReview {
		t.Fatal("Expected file with unknown episode to need review")
	}
	if file.ParseMethod != "folder" {
		t.Fatalf("Expected parse_method = folder, got %q", file.ParseMethod)
	}
}

func TestProcessFile_ObfuscatedWithoutEpisodeContextDoesNotTriggerAI(t *testing.T) {
	calls := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "AI should not be called for obfuscated files with no episode context", http.StatusInternalServerError)
	}))
	defer mockServer.Close()

	db := setupTestDB(t)
	defer db.Close()

	aiCfg := config.AIConfig{
		Enabled:              true,
		OllamaEndpoint:       mockServer.URL,
		Model:                "minimax-m2.5:cloud",
		ConfidenceThreshold:  0.8,
		AutoTriggerThreshold: 0.6,
		TimeoutSeconds:       5,
		CacheEnabled:         false,
	}
	matcher, err := ai.NewMatcher(aiCfg)
	if err != nil {
		t.Fatal(err)
	}
	scanner := NewFileScannerWithAI(db, NewAIHelper(aiCfg, db.DB(), matcher))

	tempDir := t.TempDir()
	tvRoot := filepath.Join(tempDir, "TVSHOWS")
	showDir := filepath.Join(tvRoot, "The Daily Show (1996)", "Season 2020")
	if err := os.MkdirAll(showDir, 0755); err != nil {
		t.Fatal(err)
	}

	episodePath := filepath.Join(showDir, "248fc0bc4f6f454b89a8158018a398a6.mkv")
	if err := os.WriteFile(episodePath, []byte("fake video content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(episodePath, 100*1024*1024); err != nil {
		t.Fatal(err)
	}

	result, err := scanner.ScanPath(context.Background(), episodePath, tvRoot, "episode")
	if err != nil {
		t.Fatalf("ScanPath failed: %v", err)
	}
	if result.AITriggered != 0 || result.AIFailed != 0 || calls != 0 {
		t.Fatalf("AI should not be used for obfuscated file without episode context, triggered=%d failed=%d calls=%d", result.AITriggered, result.AIFailed, calls)
	}

	file, err := db.GetMediaFile(episodePath)
	if err != nil {
		t.Fatalf("Failed to get media file: %v", err)
	}
	if file.Year == nil || *file.Year != 1996 {
		t.Fatalf("expected folder-derived year 1996 to be preserved, got %v", file.Year)
	}
	if file.ParseMethod != "folder" {
		t.Fatalf("expected parse_method folder, got %q", file.ParseMethod)
	}
	if !file.NeedsReview {
		t.Fatal("obfuscated file without episode context should still need review")
	}
}

func TestProcessFile_DeterministicTVParseDoesNotTriggerAI(t *testing.T) {
	calls := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "AI should not be called for deterministic TV parses", http.StatusInternalServerError)
	}))
	defer mockServer.Close()

	db := setupTestDB(t)
	defer db.Close()

	aiCfg := config.AIConfig{
		Enabled:              true,
		OllamaEndpoint:       mockServer.URL,
		Model:                "minimax-m2.5:cloud",
		ConfidenceThreshold:  0.8,
		AutoTriggerThreshold: 0.6,
		TimeoutSeconds:       5,
		CacheEnabled:         false,
	}
	matcher, err := ai.NewMatcher(aiCfg)
	if err != nil {
		t.Fatal(err)
	}
	scanner := NewFileScannerWithAI(db, NewAIHelper(aiCfg, db.DB(), matcher))

	tempDir := t.TempDir()
	tvRoot := filepath.Join(tempDir, "TVSHOWS")
	showDir := filepath.Join(tvRoot, "Universal Basic Guys (2024)", "Season 01")
	if err := os.MkdirAll(showDir, 0755); err != nil {
		t.Fatal(err)
	}
	episodePath := filepath.Join(showDir, "Universal Basic Guys S01E01.mkv")
	if err := os.WriteFile(episodePath, []byte("fake video content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(episodePath, 100*1024*1024); err != nil {
		t.Fatal(err)
	}

	result, err := scanner.ScanPath(context.Background(), episodePath, tvRoot, "episode")
	if err != nil {
		t.Fatalf("ScanPath failed: %v", err)
	}
	if result.AITriggered != 0 || result.AIFailed != 0 || calls != 0 {
		t.Fatalf("AI should not be used for deterministic parse, triggered=%d failed=%d calls=%d", result.AITriggered, result.AIFailed, calls)
	}

	file, err := db.GetMediaFile(episodePath)
	if err != nil {
		t.Fatal(err)
	}
	if file.ParseMethod != "regex" {
		t.Fatalf("parse_method = %q, want regex", file.ParseMethod)
	}
	if file.Confidence < 0.8 {
		t.Fatalf("confidence = %.2f, want >= 0.80", file.Confidence)
	}
	if file.NeedsReview {
		t.Fatal("deterministic parse should not need review")
	}
}

func TestProcessFile_NonCompliant(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	scanner := NewFileScanner(db)

	// Create a non-compliant file (has release markers)
	tempDir := t.TempDir()
	movieDir := filepath.Join(tempDir, "Interstellar (2014)")
	err := os.MkdirAll(movieDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	moviePath := filepath.Join(movieDir, "Interstellar.2014.1080p.BluRay.x264.mkv")
	err = os.WriteFile(moviePath, []byte("fake video content"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(moviePath)
	if err != nil {
		t.Fatal(err)
	}

	// Process the file
	result := &ScanResult{}
	err = scanner.processFile(moviePath, info, tempDir, "movie", result)
	if err != nil {
		t.Fatalf("processFile failed: %v", err)
	}

	// Verify compliance issues were recorded
	file, err := db.GetMediaFile(moviePath)
	if err != nil {
		t.Fatalf("Failed to get media file: %v", err)
	}

	if file.IsJellyfinCompliant {
		t.Error("Expected non-compliant file")
	}

	if len(file.ComplianceIssues) == 0 {
		t.Error("Expected compliance issues to be recorded")
	}
}

func TestScanPath_Integration(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	scanner := NewFileScanner(db)

	// Create test directory structure
	tempDir := t.TempDir()

	// Create multiple movie files
	movies := []struct {
		folder   string
		filename string
		size     int64
	}{
		{"Interstellar (2014)", "Interstellar (2014).mkv", 600 * 1024 * 1024},
		{"Her (2013)", "Her (2013).mkv", 700 * 1024 * 1024},
		{"Her (2013)", "sample.mkv", 50 * 1024 * 1024}, // Should be skipped (sample)
	}

	for _, m := range movies {
		dir := filepath.Join(tempDir, m.folder)
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			t.Fatal(err)
		}

		path := filepath.Join(dir, m.filename)
		// Create file with specified size
		content := make([]byte, m.size)
		err = os.WriteFile(path, content, 0644)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Scan the directory
	ctx := context.Background()
	result := &ScanResult{}
	err := scanner.scanPath(ctx, tempDir, "movie", result)
	if err != nil {
		t.Fatalf("scanPath failed: %v", err)
	}

	// Verify results
	if result.FilesScanned != 3 {
		t.Errorf("Expected FilesScanned = 3, got %d", result.FilesScanned)
	}

	// Should skip sample file and small file
	if result.FilesSkipped != 1 {
		t.Errorf("Expected FilesSkipped = 1, got %d", result.FilesSkipped)
	}

	if result.FilesAdded != 2 {
		t.Errorf("Expected FilesAdded = 2, got %d", result.FilesAdded)
	}

	// Verify files are in database
	files, err := db.GetMediaFilesByNormalizedKey("interstellar", 2014, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Errorf("Expected 1 Interstellar file, got %d", len(files))
	}

	files, err = db.GetMediaFilesByNormalizedKey("her", 2013, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Errorf("Expected 1 Her file, got %d", len(files))
	}
}

func TestScanPath_MovieUpdatesParentAndBestFile(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	scanner := NewFileScanner(db)

	root := t.TempDir()
	movieDir := filepath.Join(root, "Movies", "F Valentines Day (2026)")
	if err := os.MkdirAll(movieDir, 0755); err != nil {
		t.Fatal(err)
	}
	moviePath := filepath.Join(movieDir, "F Valentines Day (2026).mkv")
	content := make([]byte, 600*1024*1024)
	if err := os.WriteFile(moviePath, content, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := scanner.ScanPath(context.Background(), movieDir, filepath.Join(root, "Movies"), "movie")
	if err != nil {
		t.Fatalf("ScanPath failed: %v", err)
	}
	if result.FilesAdded != 1 {
		t.Fatalf("FilesAdded = %d, want 1", result.FilesAdded)
	}

	file, err := db.GetMediaFile(moviePath)
	if err != nil {
		t.Fatalf("GetMediaFile failed: %v", err)
	}
	if file == nil {
		t.Fatalf("media file was not indexed")
	}
	if file.ParentMovieID == nil {
		t.Fatalf("ParentMovieID should be set")
	}

	var bestFileID int64
	err = db.DB().QueryRow(`SELECT best_file_id FROM movies WHERE id = ?`, *file.ParentMovieID).Scan(&bestFileID)
	if err != nil {
		t.Fatalf("query best_file_id failed: %v", err)
	}
	if bestFileID != file.ID {
		t.Fatalf("best_file_id = %d, want media file id %d", bestFileID, file.ID)
	}
}

func TestScanLibraries(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	scanner := NewFileScanner(db)

	// Create test directories
	tempDir := t.TempDir()
	tvDir := filepath.Join(tempDir, "TV")
	movieDir := filepath.Join(tempDir, "Movies")

	// Create TV show
	showDir := filepath.Join(tvDir, "Silo (2023)", "Season 01")
	err := os.MkdirAll(showDir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	episodePath := filepath.Join(showDir, "Silo (2023) S01E01.mkv")
	content := make([]byte, 100*1024*1024) // 100MB
	err = os.WriteFile(episodePath, content, 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create movie
	err = os.MkdirAll(filepath.Join(movieDir, "Her (2013)"), 0755)
	if err != nil {
		t.Fatal(err)
	}
	moviePath := filepath.Join(movieDir, "Her (2013)", "Her (2013).mkv")
	content = make([]byte, 600*1024*1024) // 600MB
	err = os.WriteFile(moviePath, content, 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Scan both libraries
	ctx := context.Background()
	result, err := scanner.ScanLibraries(ctx, []string{tvDir}, []string{movieDir})
	if err != nil {
		t.Fatalf("ScanLibraries failed: %v", err)
	}

	if result.FilesScanned != 2 {
		t.Errorf("Expected FilesScanned = 2, got %d", result.FilesScanned)
	}

	if result.FilesAdded != 2 {
		t.Errorf("Expected FilesAdded = 2, got %d", result.FilesAdded)
	}

	if result.Duration == 0 {
		t.Error("Expected duration to be recorded")
	}
}

func TestParseInt(t *testing.T) {
	tests := []struct {
		input       string
		expected    int
		expectError bool
	}{
		{"2014", 2014, false},
		{"2023", 2023, false},
		{"1", 1, false},
		{"abc", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := parseInt(tt.input)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("parseInt(%s) = %d, want %d", tt.input, result, tt.expected)
			}
		})
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

func TestScanPathSkipsQuarantineDirs(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	scanner := NewFileScanner(db)

	tempDir := t.TempDir()
	writeSized := func(path string, size int64) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := f.Truncate(size); err != nil {
			t.Fatal(err)
		}
	}

	writeSized(filepath.Join(tempDir, "Her (2013)", "Her (2013).mkv"), 600*1024*1024)
	writeSized(filepath.Join(tempDir, "_jellywatch_quarantine_20260607", "Her (2013)", "Her (2013).mkv"), 600*1024*1024)
	writeSized(filepath.Join(tempDir, "_plex2jellyfin_quarantine_20260701", "Other (2020)", "Other (2020).mkv"), 600*1024*1024)

	result := &ScanResult{}
	if err := scanner.scanPath(context.Background(), tempDir, "movie", result); err != nil {
		t.Fatalf("scanPath failed: %v", err)
	}
	if result.FilesAdded != 1 {
		t.Fatalf("FilesAdded = %d, want 1 (quarantine dirs must be skipped); result=%+v", result.FilesAdded, result)
	}
}
