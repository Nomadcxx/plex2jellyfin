package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/database"
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
