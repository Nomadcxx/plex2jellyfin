package database

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// setupTestDB creates a temporary database for testing
func setupTestDB(t *testing.T) *MediaDB {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	return db
}

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"For All Mankind (2019)", "forallmankind"},
		{"M*A*S*H", "mash"},
		{"The Office (2005)", "theoffice"},
		{"Star Trek: Deep Space Nine (1993)", "startrekdeepspacenine"},
		{"Friends", "friends"},
		{"It's Always Sunny", "itsalwayssunny"},
		{"Mr. Robot", "mrrobot"},
		{"The Big Bang Theory", "thebigbangtheory"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := NormalizeTitle(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeTitle(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestOpenPathAppliesSQLiteConcurrencyPragmas(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	var busyTimeout int
	if err := db.DB().QueryRow(`PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
		t.Fatalf("query busy_timeout: %v", err)
	}
	if busyTimeout < 30000 {
		t.Fatalf("busy_timeout = %d, want at least 30000", busyTimeout)
	}

	var journalMode string
	if err := db.DB().QueryRow(`PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}
}

func TestExtractYear(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"For All Mankind (2019)", 2019},
		{"The Office (2005)", 2005},
		{"Friends", 0},
		{"Star Trek (1966)", 1966},
		{"Invalid (9999)", 0}, // Out of range
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ExtractYear(tt.input)
			if result != tt.expected {
				t.Errorf("ExtractYear(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDatabaseOpenClose(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Verify database was created
	if _, err := os.Stat(db.Path()); os.IsNotExist(err) {
		t.Errorf("database file was not created at %s", db.Path())
	}

	// Verify schema_version table exists
	var version int
	err := db.DB().QueryRow("SELECT version FROM schema_version ORDER BY version DESC LIMIT 1").Scan(&version)
	if err != nil {
		t.Fatalf("failed to query schema_version: %v", err)
	}

	if version != currentSchemaVersion {
		t.Errorf("schema version = %d, want %d", version, currentSchemaVersion)
	}
}

func TestSeriesUpsert(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert new series
	series := &Series{
		Title:          "For All Mankind",
		Year:           2019,
		CanonicalPath:  "/mnt/STORAGE5/TVSHOWS/For All Mankind (2019)",
		LibraryRoot:    "/mnt/STORAGE5/TVSHOWS",
		Source:         "filesystem",
		SourcePriority: 50,
		EpisodeCount:   30,
	}

	shouldUpdate, err := db.UpsertSeries(series)
	if err != nil {
		t.Fatalf("failed to insert series: %v", err)
	}

	if shouldUpdate {
		t.Error("shouldUpdate should be false for new insert")
	}

	if series.ID == 0 {
		t.Error("series ID should be set after insert")
	}

	if series.TitleNormalized != "forallmankind" {
		t.Errorf("title_normalized = %q, want 'forallmankind'", series.TitleNormalized)
	}

	// Retrieve and verify
	retrieved, err := db.GetSeriesByTitle("For All Mankind", 2019)
	if err != nil {
		t.Fatalf("failed to retrieve series: %v", err)
	}

	if retrieved == nil {
		t.Fatal("retrieved series is nil")
	}

	if retrieved.Title != series.Title {
		t.Errorf("title = %q, want %q", retrieved.Title, series.Title)
	}

	if retrieved.EpisodeCount != 30 {
		t.Errorf("episode_count = %d, want 30", retrieved.EpisodeCount)
	}
}

func TestSeriesSourcePriority(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert with sonarr source (priority 25)
	sonarrSeries := &Series{
		Title:          "Fallout",
		Year:           2024,
		CanonicalPath:  "/mnt/STORAGE5/TVSHOWS/Fallout (2024)",
		LibraryRoot:    "/mnt/STORAGE5/TVSHOWS",
		Source:         "sonarr",
		SourcePriority: 25,
		EpisodeCount:   2,
	}

	_, err := db.UpsertSeries(sonarrSeries)
	if err != nil {
		t.Fatalf("failed to insert sonarr series: %v", err)
	}

	// Try to update with jellywatch source (priority 100) - different path
	jellywatchSeries := &Series{
		Title:          "Fallout",
		Year:           2024,
		CanonicalPath:  "/mnt/STORAGE7/TVSHOWS/Fallout (2024)",
		LibraryRoot:    "/mnt/STORAGE7/TVSHOWS",
		Source:         "jellywatch",
		SourcePriority: 100,
		EpisodeCount:   9,
	}

	shouldUpdate, err := db.UpsertSeries(jellywatchSeries)
	if err != nil {
		t.Fatalf("failed to update with jellywatch: %v", err)
	}

	// JellyWatch priority wins, path changed, so should signal update
	// Note: shouldUpdate would be true if sonarr_id was set
	// For this test we didn't set it, so it should be false
	if shouldUpdate {
		t.Error("shouldUpdate should be false when sonarr_id is nil")
	}

	// Verify the path was updated to jellywatch path
	retrieved, err := db.GetSeriesByTitle("Fallout", 2024)
	if err != nil {
		t.Fatalf("failed to retrieve series: %v", err)
	}

	if retrieved.CanonicalPath != jellywatchSeries.CanonicalPath {
		t.Errorf("canonical_path = %q, want %q (jellywatch should win)",
			retrieved.CanonicalPath, jellywatchSeries.CanonicalPath)
	}

	if retrieved.Source != "jellywatch" {
		t.Errorf("source = %q, want 'jellywatch'", retrieved.Source)
	}

	// Try to downgrade with lower priority - should be ignored
	filesystemSeries := &Series{
		Title:          "Fallout",
		Year:           2024,
		CanonicalPath:  "/mnt/STORAGE1/TVSHOWS/Fallout (2024)",
		LibraryRoot:    "/mnt/STORAGE1/TVSHOWS",
		Source:         "filesystem",
		SourcePriority: 50,
		EpisodeCount:   0,
	}

	_, err = db.UpsertSeries(filesystemSeries)
	if err != nil {
		t.Fatalf("failed to attempt filesystem update: %v", err)
	}

	// Verify path didn't change (jellywatch priority still wins)
	retrieved, err = db.GetSeriesByTitle("Fallout", 2024)
	if err != nil {
		t.Fatalf("failed to retrieve series: %v", err)
	}

	if retrieved.CanonicalPath != jellywatchSeries.CanonicalPath {
		t.Errorf("canonical_path changed to %q, should remain %q",
			retrieved.CanonicalPath, jellywatchSeries.CanonicalPath)
	}
}

func TestIncrementEpisodeCount(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert series
	series := &Series{
		Title:          "The Office",
		Year:           2005,
		CanonicalPath:  "/mnt/STORAGE8/TVSHOWS/The Office (2005)",
		LibraryRoot:    "/mnt/STORAGE8/TVSHOWS",
		Source:         "jellywatch",
		SourcePriority: 100,
		EpisodeCount:   0,
	}

	_, err := db.UpsertSeries(series)
	if err != nil {
		t.Fatalf("failed to insert series: %v", err)
	}

	// Increment episode count
	err = db.IncrementEpisodeCount(series.ID)
	if err != nil {
		t.Fatalf("failed to increment episode count: %v", err)
	}

	// Verify count increased
	retrieved, err := db.GetSeriesByTitle("The Office", 2005)
	if err != nil {
		t.Fatalf("failed to retrieve series: %v", err)
	}

	if retrieved.EpisodeCount != 1 {
		t.Errorf("episode_count = %d, want 1", retrieved.EpisodeCount)
	}

	if retrieved.LastEpisodeAdded == nil {
		t.Error("last_episode_added should be set")
	}
}

func TestGetAllSeriesInLibrary(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	library := "/mnt/STORAGE1/TVSHOWS"

	// Insert multiple series in same library
	series1 := &Series{
		Title:          "Breaking Bad",
		Year:           2008,
		CanonicalPath:  library + "/Breaking Bad (2008)",
		LibraryRoot:    library,
		Source:         "jellywatch",
		SourcePriority: 100,
	}

	series2 := &Series{
		Title:          "Better Call Saul",
		Year:           2015,
		CanonicalPath:  library + "/Better Call Saul (2015)",
		LibraryRoot:    library,
		Source:         "jellywatch",
		SourcePriority: 100,
	}

	// Insert series in different library
	series3 := &Series{
		Title:          "The Wire",
		Year:           2002,
		CanonicalPath:  "/mnt/STORAGE2/TVSHOWS/The Wire (2002)",
		LibraryRoot:    "/mnt/STORAGE2/TVSHOWS",
		Source:         "jellywatch",
		SourcePriority: 100,
	}

	for _, s := range []*Series{series1, series2, series3} {
		if _, err := db.UpsertSeries(s); err != nil {
			t.Fatalf("failed to insert series: %v", err)
		}
	}

	// Get series from library 1
	results, err := db.GetAllSeriesInLibrary(library)
	if err != nil {
		t.Fatalf("failed to get series in library: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("got %d series, want 2", len(results))
	}

	// Verify order (should be alphabetical)
	if len(results) == 2 {
		if results[0].Title != "Better Call Saul" {
			t.Errorf("first series = %q, want 'Better Call Saul'", results[0].Title)
		}
		if results[1].Title != "Breaking Bad" {
			t.Errorf("second series = %q, want 'Breaking Bad'", results[1].Title)
		}
	}
}

func TestMovieOperations(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert movie
	movie := &Movie{
		Title:          "Inception",
		Year:           2010,
		CanonicalPath:  "/mnt/STORAGE2/MOVIES/Inception (2010)",
		LibraryRoot:    "/mnt/STORAGE2/MOVIES",
		Source:         "radarr",
		SourcePriority: 25,
	}

	shouldUpdate, err := db.UpsertMovie(movie)
	if err != nil {
		t.Fatalf("failed to insert movie: %v", err)
	}

	if shouldUpdate {
		t.Error("shouldUpdate should be false for new insert")
	}

	if movie.ID == 0 {
		t.Error("movie ID should be set after insert")
	}

	// Retrieve
	retrieved, err := db.GetMovieByTitle("Inception", 2010)
	if err != nil {
		t.Fatalf("failed to retrieve movie: %v", err)
	}

	if retrieved == nil {
		t.Fatal("retrieved movie is nil")
	}

	if retrieved.Title != "Inception" {
		t.Errorf("title = %q, want 'Inception'", retrieved.Title)
	}
}

func TestGetStats(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert some series and movies
	series := &Series{
		Title:          "Test Series",
		Year:           2020,
		CanonicalPath:  "/test/series",
		LibraryRoot:    "/test",
		Source:         "jellywatch",
		SourcePriority: 100,
	}

	movie := &Movie{
		Title:          "Test Movie",
		Year:           2020,
		CanonicalPath:  "/test/movie",
		LibraryRoot:    "/test",
		Source:         "jellywatch",
		SourcePriority: 100,
	}

	db.UpsertSeries(series)
	db.UpsertMovie(movie)

	// Get stats
	stats, err := db.GetStats()
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}

	if stats.SeriesCount != 1 {
		t.Errorf("series_count = %d, want 1", stats.SeriesCount)
	}

	if stats.MoviesCount != 1 {
		t.Errorf("movies_count = %d, want 1", stats.MoviesCount)
	}
}

func TestConcurrentAccess(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Test concurrent reads and writes
	done := make(chan bool)

	for i := 0; i < 5; i++ {
		go func(n int) {
			series := &Series{
				Title:          "Concurrent Series",
				Year:           2020 + n,
				CanonicalPath:  fmt.Sprintf("/test/Concurrent Series (%d)", 2020+n),
				LibraryRoot:    "/test",
				Source:         "jellywatch",
				SourcePriority: 100,
			}

			_, err := db.UpsertSeries(series)
			if err != nil {
				t.Errorf("concurrent insert failed: %v", err)
			}

			// Also do some reads
			db.GetSeriesByTitle("Concurrent Series", 2020+n)

			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 5; i++ {
		<-done
	}

	// Verify all were inserted
	stats, _ := db.GetStats()
	if stats.SeriesCount != 5 {
		t.Errorf("got %d series after concurrent inserts, want 5", stats.SeriesCount)
	}
}

func TestYearlessLookup(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert two series with same name but different years
	series1 := &Series{
		Title:          "The Office",
		Year:           2001, // UK version
		CanonicalPath:  "/mnt/STORAGE1/TVSHOWS/The Office (2001)",
		LibraryRoot:    "/mnt/STORAGE1/TVSHOWS",
		Source:         "jellywatch",
		SourcePriority: 100,
		EpisodeCount:   14,
	}

	series2 := &Series{
		Title:          "The Office",
		Year:           2005, // US version
		CanonicalPath:  "/mnt/STORAGE2/TVSHOWS/The Office (2005)",
		LibraryRoot:    "/mnt/STORAGE2/TVSHOWS",
		Source:         "jellywatch",
		SourcePriority: 100,
		EpisodeCount:   201,
	}

	db.UpsertSeries(series1)
	db.UpsertSeries(series2)

	// Lookup without year - should return US version (higher episode count)
	retrieved, err := db.GetSeriesByTitle("The Office", 0)
	if err != nil {
		t.Fatalf("failed to retrieve series: %v", err)
	}

	if retrieved == nil {
		t.Fatal("retrieved series is nil")
	}

	if retrieved.Year != 2005 {
		t.Errorf("year = %d, want 2005 (should prefer higher episode count)", retrieved.Year)
	}

	// Lookup with specific year
	retrievedUK, err := db.GetSeriesByTitle("The Office", 2001)
	if err != nil {
		t.Fatalf("failed to retrieve UK series: %v", err)
	}

	if retrievedUK.Year != 2001 {
		t.Errorf("year = %d, want 2001", retrievedUK.Year)
	}
}

// TestConflictDetection tests that conflicts are recorded when same series/movie
// is found in different locations with same priority
func TestConflictDetection(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// First filesystem scan finds Silo in library1
	series1 := &Series{
		Title:          "Silo",
		Year:           2023,
		CanonicalPath:  "/library1/Silo (2023)",
		LibraryRoot:    "/library1",
		Source:         "filesystem",
		SourcePriority: 50,
		EpisodeCount:   10,
	}

	_, err := db.UpsertSeries(series1)
	if err != nil {
		t.Fatalf("failed to upsert series1: %v", err)
	}

	// Verify no conflicts yet
	conflicts, err := db.GetUnresolvedConflicts()
	if err != nil {
		t.Fatalf("failed to get conflicts: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected 0 conflicts initially, got %d", len(conflicts))
	}

	// Second filesystem scan finds Silo in library2 (same priority!)
	series2 := &Series{
		Title:          "Silo",
		Year:           2023,
		CanonicalPath:  "/library2/Silo (2023)",
		LibraryRoot:    "/library2",
		Source:         "filesystem",
		SourcePriority: 50, // Same priority as filesystem
		EpisodeCount:   10,
	}

	_, err = db.UpsertSeries(series2)
	if err != nil {
		t.Fatalf("failed to upsert series2: %v", err)
	}

	// NOW there should be a conflict recorded
	conflicts, err = db.GetUnresolvedConflicts()
	if err != nil {
		t.Fatalf("failed to get conflicts after second upsert: %v", err)
	}

	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}

	conflict := conflicts[0]
	if conflict.MediaType != "series" {
		t.Errorf("MediaType = %s, want series", conflict.MediaType)
	}
	if conflict.Title != "Silo" {
		t.Errorf("Title = %s, want Silo", conflict.Title)
	}
	if len(conflict.Locations) != 2 {
		t.Errorf("expected 2 locations, got %d", len(conflict.Locations))
	}

	// Verify both paths are in the conflict
	paths := make(map[string]bool)
	for _, loc := range conflict.Locations {
		paths[loc] = true
	}
	if !paths["/library1/Silo (2023)"] || !paths["/library2/Silo (2023)"] {
		t.Errorf("conflict locations don't contain both paths: %v", conflict.Locations)
	}

	// Third scan finds it in library3 - should add to existing conflict
	series3 := &Series{
		Title:          "Silo",
		Year:           2023,
		CanonicalPath:  "/library3/Silo (2023)",
		LibraryRoot:    "/library3",
		Source:         "filesystem",
		SourcePriority: 50,
		EpisodeCount:   10,
	}

	_, err = db.UpsertSeries(series3)
	if err != nil {
		t.Fatalf("failed to upsert series3: %v", err)
	}

	// Should still be 1 conflict, but now with 3 locations
	conflicts, err = db.GetUnresolvedConflicts()
	if err != nil {
		t.Fatalf("failed to get conflicts after third upsert: %v", err)
	}

	if len(conflicts) != 1 {
		t.Errorf("expected still 1 conflict, got %d", len(conflicts))
	}

	if len(conflicts[0].Locations) != 3 {
		t.Errorf("expected 3 locations, got %d: %v", len(conflicts[0].Locations), conflicts[0].Locations)
	}

	t.Logf("Conflict recorded successfully with locations: %v", conflicts[0].Locations)
}

// TestConflictDetectionMovie tests conflict detection for movies
func TestConflictDetectionMovie(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// First scan finds movie in library1
	movie1 := &Movie{
		Title:          "The Matrix",
		Year:           1999,
		CanonicalPath:  "/movies1/The Matrix (1999)",
		LibraryRoot:    "/movies1",
		Source:         "filesystem",
		SourcePriority: 50,
	}

	_, err := db.UpsertMovie(movie1)
	if err != nil {
		t.Fatalf("failed to upsert movie1: %v", err)
	}

	// Second scan finds same movie in library2
	movie2 := &Movie{
		Title:          "The Matrix",
		Year:           1999,
		CanonicalPath:  "/movies2/The Matrix (1999)",
		LibraryRoot:    "/movies2",
		Source:         "filesystem",
		SourcePriority: 50,
	}

	_, err = db.UpsertMovie(movie2)
	if err != nil {
		t.Fatalf("failed to upsert movie2: %v", err)
	}

	// Should have recorded a conflict
	conflicts, err := db.GetUnresolvedConflicts()
	if err != nil {
		t.Fatalf("failed to get conflicts: %v", err)
	}

	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}

	if conflicts[0].MediaType != "movie" {
		t.Errorf("MediaType = %s, want movie", conflicts[0].MediaType)
	}

	if len(conflicts[0].Locations) != 2 {
		t.Errorf("expected 2 locations, got %d", len(conflicts[0].Locations))
	}

	t.Logf("Movie conflict recorded: %v", conflicts[0].Locations)
}

func TestUpsertConflictWithoutUniqueConstraint(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	year := 2005
	first := Conflict{
		MediaType:       "series",
		Title:           "Example",
		TitleNormalized: "example",
		Year:            &year,
		Locations:       []string{"/library/a/Example", "/library/b/Example"},
	}
	if err := db.upsertConflict(first); err != nil {
		t.Fatalf("first upsertConflict failed: %v", err)
	}

	second := first
	second.Locations = append(second.Locations, "/library/c/Example")
	if err := db.upsertConflict(second); err != nil {
		t.Fatalf("second upsertConflict failed: %v", err)
	}

	conflicts, err := db.GetUnresolvedConflicts()
	if err != nil {
		t.Fatalf("GetUnresolvedConflicts failed: %v", err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("expected one unresolved conflict, got %d", len(conflicts))
	}
	if len(conflicts[0].Locations) != 3 {
		t.Fatalf("expected updated conflict locations, got %v", conflicts[0].Locations)
	}
}

// TestNoConflictOnHigherPriorityUpdate verifies no conflict recorded when
// higher priority source overwrites lower priority (this is expected behavior)
func TestNoConflictOnHigherPriorityUpdate(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Sonarr provides initial location (low priority)
	series1 := &Series{
		Title:          "Fallout",
		Year:           2024,
		CanonicalPath:  "/sonarr-path/Fallout (2024)",
		LibraryRoot:    "/sonarr-path",
		Source:         "sonarr",
		SourcePriority: 25, // Low priority
		EpisodeCount:   8,
	}

	_, err := db.UpsertSeries(series1)
	if err != nil {
		t.Fatalf("failed to upsert sonarr series: %v", err)
	}

	// JellyWatch learns the correct location (high priority)
	series2 := &Series{
		Title:          "Fallout",
		Year:           2024,
		CanonicalPath:  "/jellywatch-path/Fallout (2024)",
		LibraryRoot:    "/jellywatch-path",
		Source:         "jellywatch",
		SourcePriority: 100, // High priority - this is authoritative
		EpisodeCount:   8,
	}

	_, err = db.UpsertSeries(series2)
	if err != nil {
		t.Fatalf("failed to upsert jellywatch series: %v", err)
	}

	// This should still record a conflict! The paths are different.
	// Even though jellywatch is authoritative, we still want to know
	// the show exists in multiple places.
	conflicts, err := db.GetUnresolvedConflicts()
	if err != nil {
		t.Fatalf("failed to get conflicts: %v", err)
	}

	// We record conflicts ANY time paths differ, regardless of priority
	if len(conflicts) != 1 {
		t.Errorf("expected 1 conflict (paths differ), got %d", len(conflicts))
	}

	t.Logf("Conflict recorded for priority override: %v", conflicts)
}

func TestMediaDBSQLReturnsHandle(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if db.SQL() == nil {
		t.Fatal("SQL() returned nil")
	}
	var n int
	if err := db.SQL().QueryRow("SELECT 1").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("got %d, want 1", n)
	}
}
