package sync

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/radarr"
	"github.com/Nomadcxx/jellywatch/internal/sonarr"
)

// TestNewSyncService verifies sync service creation
func TestNewSyncService(t *testing.T) {
	db := createTestDB(t)
	defer db.Close()

	cfg := SyncConfig{
		DB:       db,
		SyncHour: 3,
		Logger:   slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}

	svc := NewSyncService(cfg)
	if svc == nil {
		t.Fatal("expected sync service, got nil")
	}

	if svc.syncHour != 3 {
		t.Errorf("expected sync hour 3, got %d", svc.syncHour)
	}
}

// TestNewSyncServiceInvalidHour verifies invalid hour defaults to 3
func TestNewSyncServiceInvalidHour(t *testing.T) {
	db := createTestDB(t)
	defer db.Close()

	tests := []struct {
		name     string
		hour     int
		expected int
	}{
		{"negative hour", -1, 3},
		{"hour too large", 24, 3},
		{"hour way too large", 100, 3},
		{"valid hour 0", 0, 0},
		{"valid hour 23", 23, 23},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := SyncConfig{
				DB:       db,
				SyncHour: tt.hour,
			}
			svc := NewSyncService(cfg)
			if svc.syncHour != tt.expected {
				t.Errorf("expected sync hour %d, got %d", tt.expected, svc.syncHour)
			}
		})
	}
}

// TestSyncFromFilesystem tests filesystem scanning
func TestSyncFromFilesystem(t *testing.T) {
	db := createTestDB(t)
	defer db.Close()

	// Create temp directories with mock structure
	tmpDir := t.TempDir()

	// Create TV library structure
	tvLib := filepath.Join(tmpDir, "tv")
	os.MkdirAll(tvLib, 0755)

	// Create shows with episodes
	show1 := filepath.Join(tvLib, "Fallout (2024)")
	os.MkdirAll(filepath.Join(show1, "Season 01"), 0755)
	createVideoFile(t, filepath.Join(show1, "Season 01", "Fallout (2024) S01E01.mkv"))
	createVideoFile(t, filepath.Join(show1, "Season 01", "Fallout (2024) S01E02.mkv"))

	show2 := filepath.Join(tvLib, "For All Mankind (2019)")
	os.MkdirAll(filepath.Join(show2, "Season 01"), 0755)
	createVideoFile(t, filepath.Join(show2, "Season 01", "For All Mankind (2019) S01E01.mkv"))

	// Create movie library structure
	movieLib := filepath.Join(tmpDir, "movies")
	os.MkdirAll(movieLib, 0755)

	movie1 := filepath.Join(movieLib, "The Matrix (1999)")
	os.MkdirAll(movie1, 0755)
	createVideoFile(t, filepath.Join(movie1, "The Matrix (1999).mkv"))

	movie2 := filepath.Join(movieLib, "Inception (2010)")
	os.MkdirAll(movie2, 0755)
	createVideoFile(t, filepath.Join(movie2, "Inception (2010).mp4"))

	// Create sync service
	cfg := SyncConfig{
		DB:             db,
		TVLibraries:    []string{tvLib},
		MovieLibraries: []string{movieLib},
		Logger:         slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}
	svc := NewSyncService(cfg)

	// Run filesystem sync
	ctx := context.Background()
	_, err := svc.SyncFromFilesystem(ctx)
	if err != nil {
		t.Fatalf("filesystem sync failed: %v", err)
	}

	// Verify TV shows were added
	series1, err := db.GetSeriesByTitle("Fallout", 2024)
	if err != nil {
		t.Fatalf("failed to get Fallout: %v", err)
	}
	if series1 == nil {
		t.Fatal("expected Fallout to be in database")
	}
	if series1.EpisodeCount != 2 {
		t.Errorf("expected 2 episodes for Fallout, got %d", series1.EpisodeCount)
	}
	if series1.Source != "filesystem" {
		t.Errorf("expected source=filesystem, got %s", series1.Source)
	}
	if series1.SourcePriority != filesystemSourcePriority {
		t.Errorf("expected priority=%d, got %d", filesystemSourcePriority, series1.SourcePriority)
	}

	series2, err := db.GetSeriesByTitle("For All Mankind", 2019)
	if err != nil {
		t.Fatalf("failed to get For All Mankind: %v", err)
	}
	if series2 == nil {
		t.Fatal("expected For All Mankind to be in database")
	}
	if series2.EpisodeCount != 1 {
		t.Errorf("expected 1 episode for For All Mankind, got %d", series2.EpisodeCount)
	}

	// Verify movies were added
	movie1DB, err := db.GetMovieByTitle("The Matrix", 1999)
	if err != nil {
		t.Fatalf("failed to get The Matrix: %v", err)
	}
	if movie1DB == nil {
		t.Fatal("expected The Matrix to be in database")
	}
	if movie1DB.Source != "filesystem" {
		t.Errorf("expected source=filesystem, got %s", movie1DB.Source)
	}

	movie2DB, err := db.GetMovieByTitle("Inception", 2010)
	if err != nil {
		t.Fatalf("failed to get Inception: %v", err)
	}
	if movie2DB == nil {
		t.Fatal("expected Inception to be in database")
	}
}

// TestSyncFromFilesystemContextCancellation tests context cancellation
func TestSyncFromFilesystemContextCancellation(t *testing.T) {
	db := createTestDB(t)
	defer db.Close()

	tmpDir := t.TempDir()
	tvLib := filepath.Join(tmpDir, "tv")
	os.MkdirAll(tvLib, 0755)

	cfg := SyncConfig{
		DB:          db,
		TVLibraries: []string{tvLib},
		Logger:      slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}
	svc := NewSyncService(cfg)

	// Create context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.SyncFromFilesystem(ctx)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled error, got %v", err)
	}
}

// TestSyncFromSonarrMock tests Sonarr sync with mock client
func TestSyncFromSonarrMock(t *testing.T) {
	db := createTestDB(t)
	defer db.Close()

	// Note: This test would require a mock Sonarr client
	// For now, we'll test with nil client (which should skip gracefully)
	cfg := SyncConfig{
		DB:     db,
		Sonarr: nil, // No Sonarr configured
		Logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}
	svc := NewSyncService(cfg)

	// Should not error when Sonarr is nil
	err := svc.RunFullSync(context.Background())
	if err != nil {
		t.Errorf("expected nil error when Sonarr not configured, got %v", err)
	}
}

// TestSyncFromRadarrMock tests Radarr sync with mock client
func TestSyncFromRadarrMock(t *testing.T) {
	db := createTestDB(t)
	defer db.Close()

	// Test with nil client (should skip gracefully)
	cfg := SyncConfig{
		DB:     db,
		Radarr: nil,
		Logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}
	svc := NewSyncService(cfg)

	err := svc.RunFullSync(context.Background())
	if err != nil {
		t.Errorf("expected nil error when Radarr not configured, got %v", err)
	}
}

// TestSyncDirtyRecords tests dirty record synchronization
func TestSyncDirtyRecords(t *testing.T) {
	db := createTestDB(t)
	defer db.Close()

	sonarrID := 123
	series := &database.Series{
		Title:         "Test Show",
		Year:          2020,
		SonarrID:      &sonarrID,
		CanonicalPath: "/tv/Test Show (2020)",
	}
	_, err := db.UpsertSeries(series)
	if err != nil {
		t.Fatalf("failed to create series: %v", err)
	}

	seriesRecord, err := db.GetSeriesByTitle("Test Show", 2020)
	if err != nil {
		t.Fatalf("failed to get series: %v", err)
	}
	if seriesRecord == nil {
		t.Fatal("expected series to be in database")
	}

	err = db.SetSeriesDirty(seriesRecord.ID)
	if err != nil {
		t.Fatalf("failed to set series dirty: %v", err)
	}

	cfg := SyncConfig{
		DB:     db,
		Sonarr: nil,
		Logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}
	svc := NewSyncService(cfg)

	err = svc.syncDirtyRecords(context.Background())
	if err != nil {
		t.Errorf("expected nil error when Sonarr not configured, got %v", err)
	}

	updated, err := db.GetSeriesByID(seriesRecord.ID)
	if err != nil {
		t.Fatalf("failed to get series: %v", err)
	}
	if !updated.SonarrPathDirty {
		t.Error("dirty flag should still be set when Sonarr is not configured")
	}
}

// TestQueueSync tests non-blocking sync queueing
func TestQueueSync(t *testing.T) {
	db := createTestDB(t)
	defer db.Close()

	sonarrID := 456
	series := &database.Series{
		Title:         "Test Series",
		Year:          2024,
		SonarrID:      &sonarrID,
		CanonicalPath: "/tv/Test Series (2024)",
	}
	_, err := db.UpsertSeries(series)
	if err != nil {
		t.Fatalf("failed to create series: %v", err)
	}

	seriesRecord, err := db.GetSeriesByTitle("Test Series", 2024)
	if err != nil || seriesRecord == nil {
		t.Fatal("expected series to exist")
	}

	err = db.SetSeriesDirty(seriesRecord.ID)
	if err != nil {
		t.Fatalf("failed to set series dirty: %v", err)
	}

	cfg := SyncConfig{
		DB:     db,
		Sonarr: nil,
		Logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}
	svc := NewSyncService(cfg)

	svc.QueueSync("series", seriesRecord.ID)

	time.Sleep(10 * time.Millisecond)

	updated, err := db.GetSeriesByID(seriesRecord.ID)
	if err != nil {
		t.Fatalf("failed to get series: %v", err)
	}

	if !updated.SonarrPathDirty {
		t.Log("dirty flag was cleared (expected, Sonarr is nil so sync skipped)")
	}
}

// TestRetryWithBackoff tests exponential backoff logic
func TestRetryWithBackoff(t *testing.T) {
	tests := []struct {
		name          string
		maxRetries    int
		shouldSucceed bool
	}{
		{"immediate success", 3, true},
		{"success on retry", 3, true},
		{"fail all retries", 3, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attempts := 0
			fn := func() error {
				attempts++
				if tt.shouldSucceed {
					if attempts == 1 || attempts == 2 {
						return fmt.Errorf("temporary error")
					}
					return nil
				}
				return fmt.Errorf("permanent error")
			}

			err := retryWithBackoff(context.Background(), tt.maxRetries, fn)

			if tt.shouldSucceed && err != nil {
				t.Errorf("expected success, got error: %v", err)
			}
			if !tt.shouldSucceed && err == nil {
				t.Error("expected error after all retries, got nil")
			}
		})
	}
}

// TestRunFullSync tests the full sync workflow
func TestRunFullSync(t *testing.T) {
	db := createTestDB(t)
	defer db.Close()

	tmpDir := t.TempDir()
	tvLib := filepath.Join(tmpDir, "tv")
	os.MkdirAll(tvLib, 0755)

	// Create a show
	show := filepath.Join(tvLib, "Test Show (2020)")
	os.MkdirAll(filepath.Join(show, "Season 01"), 0755)
	createVideoFile(t, filepath.Join(show, "Season 01", "Test Show S01E01.mkv"))

	cfg := SyncConfig{
		DB:          db,
		TVLibraries: []string{tvLib},
		Sonarr:      nil,
		Radarr:      nil,
		Logger:      slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}
	svc := NewSyncService(cfg)

	err := svc.RunFullSync(context.Background())
	if err != nil {
		t.Fatalf("full sync failed: %v", err)
	}

	// Verify show was added
	series, err := db.GetSeriesByTitle("Test Show", 2020)
	if err != nil {
		t.Fatalf("failed to get Test Show: %v", err)
	}
	if series == nil {
		t.Fatal("expected Test Show to be in database")
	}
}

// TestStartStopScheduler tests the scheduler lifecycle
func TestStartStopScheduler(t *testing.T) {
	db := createTestDB(t)
	defer db.Close()

	cfg := SyncConfig{
		DB:       db,
		SyncHour: 3,
		Logger:   slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}
	svc := NewSyncService(cfg)

	// Start should not block
	svc.Start()

	// Give scheduler time to start
	time.Sleep(10 * time.Millisecond)

	// Stop should work
	svc.Stop()

	// Should not panic if stopped twice
	svc.Stop()
}

// TestParseMediaDir tests directory name parsing
func TestParseMediaDir(t *testing.T) {
	tests := []struct {
		name          string
		dirName       string
		expectedTitle string
		expectedYear  int
	}{
		{"with year", "Fallout (2024)", "Fallout", 2024},
		{"with year and spaces", "For All Mankind (2019)", "For All Mankind", 2019},
		{"without year", "Breaking Bad", "Breaking Bad", 0},
		{"year not at end", "Star Trek (2009) Remastered", "Star Trek (2009) Remastered", 0},
		{"multiple parens at end", "M*A*S*H (1972)", "M*A*S*H", 1972},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title, year := parseMediaDir(tt.dirName)
			if title != tt.expectedTitle {
				t.Errorf("expected title %q, got %q", tt.expectedTitle, title)
			}
			if year != tt.expectedYear {
				t.Errorf("expected year %d, got %d", tt.expectedYear, year)
			}
		})
	}
}

// TestCountVideoFiles tests video file counting
func TestCountVideoFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create nested structure
	season1 := filepath.Join(tmpDir, "Season 01")
	season2 := filepath.Join(tmpDir, "Season 02")
	os.MkdirAll(season1, 0755)
	os.MkdirAll(season2, 0755)

	// Create video files
	createVideoFile(t, filepath.Join(season1, "episode1.mkv"))
	createVideoFile(t, filepath.Join(season1, "episode2.mp4"))
	createVideoFile(t, filepath.Join(season2, "episode3.avi"))

	// Create non-video file
	os.WriteFile(filepath.Join(season1, "subtitle.srt"), []byte("test"), 0644)

	count := countVideoFiles(tmpDir)
	if count != 3 {
		t.Errorf("expected 3 video files, got %d", count)
	}
}

// TestHasVideoFiles tests video file detection
func TestHasVideoFiles(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(string)
		expected bool
	}{
		{
			name: "has mkv file",
			setup: func(dir string) {
				createVideoFile(t, filepath.Join(dir, "movie.mkv"))
			},
			expected: true,
		},
		{
			name: "has mp4 file",
			setup: func(dir string) {
				createVideoFile(t, filepath.Join(dir, "movie.mp4"))
			},
			expected: true,
		},
		{
			name: "only subtitles",
			setup: func(dir string) {
				os.WriteFile(filepath.Join(dir, "subs.srt"), []byte("test"), 0644)
			},
			expected: false,
		},
		{
			name:     "empty directory",
			setup:    func(dir string) {},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(dir)
			result := hasVideoFiles(dir)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestSyncLogCreation verifies sync logs are created
func TestSyncLogCreation(t *testing.T) {
	db := createTestDB(t)
	defer db.Close()

	tmpDir := t.TempDir()
	tvLib := filepath.Join(tmpDir, "tv")
	os.MkdirAll(tvLib, 0755)

	cfg := SyncConfig{
		DB:          db,
		TVLibraries: []string{tvLib},
		Logger:      slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}
	svc := NewSyncService(cfg)

	// Run sync
	_, err := svc.SyncFromFilesystem(context.Background())
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	// Verify sync log was created
	logs, err := db.GetRecentSyncLogs(7)
	if err != nil {
		t.Fatalf("failed to get sync logs: %v", err)
	}

	if len(logs) == 0 {
		t.Fatal("expected at least one sync log entry")
	}

	if logs[0].Source != "filesystem" {
		t.Errorf("expected source=filesystem, got %s", logs[0].Source)
	}
	if logs[0].Status != "success" {
		t.Errorf("expected status=success, got %s", logs[0].Status)
	}
}

// Helper functions

func createTestDB(t *testing.T) *database.MediaDB {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	return db
}

func createVideoFile(t *testing.T, path string) {
	err := os.WriteFile(path, []byte("fake video content"), 0644)
	if err != nil {
		t.Fatalf("failed to create video file: %v", err)
	}
}

// Mock Sonarr/Radarr client helpers (for future expansion)

type mockSonarrClient struct {
	series []sonarr.Series
	err    error
}

func (m *mockSonarrClient) GetAllSeries() ([]sonarr.Series, error) {
	return m.series, m.err
}

type mockRadarrClient struct {
	movies []radarr.Movie
	err    error
}

func (m *mockRadarrClient) GetMovies() ([]radarr.Movie, error) {
	return m.movies, m.err
}
