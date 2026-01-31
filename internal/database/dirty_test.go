package database

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestGetDirtySeries(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	series1 := &Series{
		Title:         "Test Show 1",
		Year:          2020,
		CanonicalPath: "/tv/Test Show 1 (2020)",
		LibraryRoot:   "/tv",
		Source:        "jellywatch",
	}
	_, err := db.UpsertSeries(series1)
	if err != nil {
		t.Fatalf("UpsertSeries failed: %v", err)
	}

	err = db.SetSeriesDirty(series1.ID)
	if err != nil {
		t.Fatalf("SetSeriesDirty failed: %v", err)
	}

	series2 := &Series{
		Title:         "Test Show 2",
		Year:          2021,
		CanonicalPath: "/tv/Test Show 2 (2021)",
		LibraryRoot:   "/tv",
		Source:        "jellywatch",
	}
	_, err = db.UpsertSeries(series2)
	if err != nil {
		t.Fatalf("UpsertSeries failed: %v", err)
	}

	dirtySeries, err := db.GetDirtySeries()
	if err != nil {
		t.Fatalf("GetDirtySeries failed: %v", err)
	}

	if len(dirtySeries) != 1 {
		t.Errorf("expected 1 dirty series, got %d", len(dirtySeries))
	}

	if len(dirtySeries) > 0 && dirtySeries[0].ID != series1.ID {
		t.Errorf("expected dirty series ID %d, got %d", series1.ID, dirtySeries[0].ID)
	}
}

func TestGetDirtyMovies(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	movie1 := &Movie{
		Title:         "Test Movie 1",
		Year:          2020,
		CanonicalPath: "/movies/Test Movie 1 (2020)",
		LibraryRoot:   "/movies",
		Source:        "jellywatch",
	}
	_, err := db.UpsertMovie(movie1)
	if err != nil {
		t.Fatalf("UpsertMovie failed: %v", err)
	}

	err = db.SetMovieDirty(movie1.ID)
	if err != nil {
		t.Fatalf("SetMovieDirty failed: %v", err)
	}

	movie2 := &Movie{
		Title:         "Test Movie 2",
		Year:          2021,
		CanonicalPath: "/movies/Test Movie 2 (2021)",
		LibraryRoot:   "/movies",
		Source:        "jellywatch",
	}
	_, err = db.UpsertMovie(movie2)
	if err != nil {
		t.Fatalf("UpsertMovie failed: %v", err)
	}

	dirtyMovies, err := db.GetDirtyMovies()
	if err != nil {
		t.Fatalf("GetDirtyMovies failed: %v", err)
	}

	if len(dirtyMovies) != 1 {
		t.Errorf("expected 1 dirty movie, got %d", len(dirtyMovies))
	}

	if len(dirtyMovies) > 0 && dirtyMovies[0].ID != movie1.ID {
		t.Errorf("expected dirty movie ID %d, got %d", movie1.ID, dirtyMovies[0].ID)
	}
}

func TestMarkSeriesSynced(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	series := &Series{
		Title:         "Test Show",
		Year:          2020,
		CanonicalPath: "/tv/Test Show (2020)",
		LibraryRoot:   "/tv",
		Source:        "jellywatch",
	}
	_, err := db.UpsertSeries(series)
	if err != nil {
		t.Fatalf("UpsertSeries failed: %v", err)
	}

	err = db.SetSeriesDirty(series.ID)
	if err != nil {
		t.Fatalf("SetSeriesDirty failed: %v", err)
	}

	retrieved, err := db.GetSeriesByID(series.ID)
	if err != nil {
		t.Fatalf("GetSeriesByID failed: %v", err)
	}
	if !retrieved.SonarrPathDirty {
		t.Error("expected series to be dirty before marking synced")
	}

	err = db.MarkSeriesSynced(series.ID)
	if err != nil {
		t.Fatalf("MarkSeriesSynced failed: %v", err)
	}

	retrieved, err = db.GetSeriesByID(series.ID)
	if err != nil {
		t.Fatalf("GetSeriesByID after sync failed: %v", err)
	}
	if retrieved.SonarrPathDirty {
		t.Error("expected series dirty flag to be cleared after marking synced")
	}
	if retrieved.SonarrSyncedAt == nil {
		t.Error("expected SonarrSyncedAt to be set after marking synced")
	}
}

func TestMarkMovieSynced(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	movie := &Movie{
		Title:         "Test Movie",
		Year:          2020,
		CanonicalPath: "/movies/Test Movie (2020)",
		LibraryRoot:   "/movies",
		Source:        "jellywatch",
	}
	_, err := db.UpsertMovie(movie)
	if err != nil {
		t.Fatalf("UpsertMovie failed: %v", err)
	}

	err = db.SetMovieDirty(movie.ID)
	if err != nil {
		t.Fatalf("SetMovieDirty failed: %v", err)
	}

	retrieved, err := db.GetMovieByID(movie.ID)
	if err != nil {
		t.Fatalf("GetMovieByID failed: %v", err)
	}
	if !retrieved.RadarrPathDirty {
		t.Error("expected movie to be dirty before marking synced")
	}

	err = db.MarkMovieSynced(movie.ID)
	if err != nil {
		t.Fatalf("MarkMovieSynced failed: %v", err)
	}

	retrieved, err = db.GetMovieByID(movie.ID)
	if err != nil {
		t.Fatalf("GetMovieByID after sync failed: %v", err)
	}
	if retrieved.RadarrPathDirty {
		t.Error("expected movie dirty flag to be cleared after marking synced")
	}
	if retrieved.RadarrSyncedAt == nil {
		t.Error("expected RadarrSyncedAt to be set after marking synced")
	}
}

func TestGetSeriesByID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	series := &Series{
		Title:         "Test Show",
		Year:          2020,
		CanonicalPath: "/tv/Test Show (2020)",
		LibraryRoot:   "/tv",
		Source:        "jellywatch",
	}
	_, err := db.UpsertSeries(series)
	if err != nil {
		t.Fatalf("UpsertSeries failed: %v", err)
	}

	retrieved, err := db.GetSeriesByID(series.ID)
	if err != nil {
		t.Fatalf("GetSeriesByID failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("expected series to be found")
	}

	if retrieved.Title != "Test Show" {
		t.Errorf("expected title 'Test Show', got '%s'", retrieved.Title)
	}

	nonExistent, err := db.GetSeriesByID(999999)
	if err != nil {
		t.Fatalf("GetSeriesByID with non-existent ID failed: %v", err)
	}
	if nonExistent != nil {
		t.Error("expected nil for non-existent series")
	}
}

func TestGetMovieByID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	movie := &Movie{
		Title:         "Test Movie",
		Year:          2020,
		CanonicalPath: "/movies/Test Movie (2020)",
		LibraryRoot:   "/movies",
		Source:        "jellywatch",
	}
	_, err := db.UpsertMovie(movie)
	if err != nil {
		t.Fatalf("UpsertMovie failed: %v", err)
	}

	retrieved, err := db.GetMovieByID(movie.ID)
	if err != nil {
		t.Fatalf("GetMovieByID failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("expected movie to be found")
	}

	if retrieved.Title != "Test Movie" {
		t.Errorf("expected title 'Test Movie', got '%s'", retrieved.Title)
	}

	nonExistent, err := db.GetMovieByID(999999)
	if err != nil {
		t.Fatalf("GetMovieByID with non-existent ID failed: %v", err)
	}
	if nonExistent != nil {
		t.Error("expected nil for non-existent movie")
	}
}

// TestDirtyFlags_DatabaseClosed verifies error when db is closed
func TestDirtyFlags_DatabaseClosed(t *testing.T) {
	db := setupTestDB(t)

	// Create a series first
	series := &Series{
		Title:         "Test Show",
		Year:          2020,
		CanonicalPath: "/tv/Test Show (2020)",
		LibraryRoot:   "/tv",
		Source:        "jellywatch",
	}
	_, err := db.UpsertSeries(series)
	if err != nil {
		t.Fatalf("UpsertSeries failed: %v", err)
	}

	// Close the database
	db.Close()

	// Test SetSeriesDirty on closed database
	err = db.SetSeriesDirty(1)
	if err == nil {
		t.Error("SetSeriesDirty: expected error when database closed, got nil")
	}

	// Test SetMovieDirty on closed database
	err = db.SetMovieDirty(1)
	if err == nil {
		t.Error("SetMovieDirty: expected error when database closed, got nil")
	}

	// Test GetDirtySeries on closed database
	_, err = db.GetDirtySeries()
	if err == nil {
		t.Error("GetDirtySeries: expected error when database closed, got nil")
	}

	// Test GetDirtyMovies on closed database
	_, err = db.GetDirtyMovies()
	if err == nil {
		t.Error("GetDirtyMovies: expected error when database closed, got nil")
	}

	// Test MarkSeriesSynced on closed database
	err = db.MarkSeriesSynced(1)
	if err == nil {
		t.Error("MarkSeriesSynced: expected error when database closed, got nil")
	}

	// Test MarkMovieSynced on closed database
	err = db.MarkMovieSynced(1)
	if err == nil {
		t.Error("MarkMovieSynced: expected error when database closed, got nil")
	}

	// Test GetSeriesByID on closed database
	_, err = db.GetSeriesByID(1)
	if err == nil {
		t.Error("GetSeriesByID: expected error when database closed, got nil")
	}

	// Test GetMovieByID on closed database
	_, err = db.GetMovieByID(1)
	if err == nil {
		t.Error("GetMovieByID: expected error when database closed, got nil")
	}
}

// TestDirtyFlags_InvalidID verifies error handling for invalid IDs
func TestDirtyFlags_InvalidID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create a series first
	series := &Series{
		Title:         "Test Show",
		Year:          2020,
		CanonicalPath: "/tv/Test Show (2020)",
		LibraryRoot:   "/tv",
		Source:        "jellywatch",
	}
	_, err := db.UpsertSeries(series)
	if err != nil {
		t.Fatalf("UpsertSeries failed: %v", err)
	}

	// Create a movie
	movie := &Movie{
		Title:         "Test Movie",
		Year:          2021,
		CanonicalPath: "/movies/Test Movie (2021)",
		LibraryRoot:   "/movies",
		Source:        "jellywatch",
	}
	_, err = db.UpsertMovie(movie)
	if err != nil {
		t.Fatalf("UpsertMovie failed: %v", err)
	}

	// Test with ID 0 - should return nil, nil (no rows found, not an error)
	seriesResult, err := db.GetSeriesByID(0)
	if err != nil {
		t.Errorf("GetSeriesByID(0): unexpected error: %v", err)
	}
	if seriesResult != nil {
		t.Error("GetSeriesByID(0): expected nil result for ID 0")
	}

	movieResult, err := db.GetMovieByID(0)
	if err != nil {
		t.Errorf("GetMovieByID(0): unexpected error: %v", err)
	}
	if movieResult != nil {
		t.Error("GetMovieByID(0): expected nil result for ID 0")
	}

	// Test with negative ID - should return nil, nil
	seriesResult, err = db.GetSeriesByID(-1)
	if err != nil {
		t.Errorf("GetSeriesByID(-1): unexpected error: %v", err)
	}
	if seriesResult != nil {
		t.Error("GetSeriesByID(-1): expected nil result for negative ID")
	}

	movieResult, err = db.GetMovieByID(-1)
	if err != nil {
		t.Errorf("GetMovieByID(-1): unexpected error: %v", err)
	}
	if movieResult != nil {
		t.Error("GetMovieByID(-1): expected nil result for negative ID")
	}

	// Test with non-existent ID (999999)
	seriesResult, err = db.GetSeriesByID(999999)
	if err != nil {
		t.Errorf("GetSeriesByID(999999): unexpected error: %v", err)
	}
	if seriesResult != nil {
		t.Error("GetSeriesByID(999999): expected nil result for non-existent ID")
	}

	movieResult, err = db.GetMovieByID(999999)
	if err != nil {
		t.Errorf("GetMovieByID(999999): unexpected error: %v", err)
	}
	if movieResult != nil {
		t.Error("GetMovieByID(999999): expected nil result for non-existent ID")
	}

	// Test SetSeriesDirty with non-existent ID - should not error (just updates 0 rows)
	err = db.SetSeriesDirty(999999)
	if err != nil {
		t.Errorf("SetSeriesDirty(999999): unexpected error: %v", err)
	}

	// Test SetMovieDirty with non-existent ID
	err = db.SetMovieDirty(999999)
	if err != nil {
		t.Errorf("SetMovieDirty(999999): unexpected error: %v", err)
	}

	// Test MarkSeriesSynced with non-existent ID
	err = db.MarkSeriesSynced(999999)
	if err != nil {
		t.Errorf("MarkSeriesSynced(999999): unexpected error: %v", err)
	}

	// Test MarkMovieSynced with non-existent ID
	err = db.MarkMovieSynced(999999)
	if err != nil {
		t.Errorf("MarkMovieSynced(999999): unexpected error: %v", err)
	}
}

// TestDirtyFlags_ConcurrentWrites verifies thread safety with concurrent SetSeriesDirty calls
func TestDirtyFlags_ConcurrentWrites(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create multiple series
	seriesIDs := make([]int64, 5)
	for i := 0; i < 5; i++ {
		series := &Series{
			Title:         fmt.Sprintf("Test Show %d", i),
			Year:          2020 + i,
			CanonicalPath: fmt.Sprintf("/tv/Test Show %d (%d)", i, 2020+i),
			LibraryRoot:   "/tv",
			Source:        "jellywatch",
		}
		_, err := db.UpsertSeries(series)
		if err != nil {
			t.Fatalf("UpsertSeries %d failed: %v", i, err)
		}
		seriesIDs[i] = series.ID
	}

	// Spawn 10 goroutines doing concurrent SetSeriesDirty calls
	var wg sync.WaitGroup
	errors := make(chan error, 100)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				// Alternate between different series IDs
				seriesID := seriesIDs[j%5]
				if err := db.SetSeriesDirty(seriesID); err != nil {
					errors <- fmt.Errorf("worker %d: SetSeriesDirty failed: %w", workerID, err)
				}
			}
		}(i)
	}

	// Also do concurrent reads
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				if _, err := db.GetDirtySeries(); err != nil {
					errors <- fmt.Errorf("worker %d: GetDirtySeries failed: %w", workerID, err)
				}
			}
		}(i)
	}

	// Wait for all goroutines
	wg.Wait()
	close(errors)

	// Check for any errors
	errorCount := 0
	for err := range errors {
		t.Errorf("Concurrent operation error: %v", err)
		errorCount++
		if errorCount >= 10 {
			t.Log("Too many errors, stopping error reporting")
			break
		}
	}

	if errorCount > 0 {
		t.Fatalf("Found %d errors during concurrent operations", errorCount)
	}

	// Verify all series are marked dirty
	dirtySeries, err := db.GetDirtySeries()
	if err != nil {
		t.Fatalf("GetDirtySeries failed: %v", err)
	}

	if len(dirtySeries) != 5 {
		t.Errorf("expected 5 dirty series after concurrent writes, got %d", len(dirtySeries))
	}
}

// TestGetAllSeries_DatabaseLocked verifies timeout handling when database is locked
func TestGetAllSeries_DatabaseLocked(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert some series
	for i := 0; i < 3; i++ {
		series := &Series{
			Title:         fmt.Sprintf("Locked Test Show %d", i),
			Year:          2020 + i,
			CanonicalPath: fmt.Sprintf("/tv/Locked Test Show %d (%d)", i, 2020+i),
			LibraryRoot:   "/tv",
			Source:        "jellywatch",
		}
		if _, err := db.UpsertSeries(series); err != nil {
			t.Fatalf("UpsertSeries %d failed: %v", i, err)
		}
	}

	// Start a write transaction to lock the database
	tx, err := db.DB().Begin()
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}

	// Insert within the transaction but don't commit yet
	_, err = tx.Exec(`INSERT INTO series (title, title_normalized, year, canonical_path, library_root, source, source_priority) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"Transaction Series", "transactionseries", 2025, "/tv/Transaction", "/tv", "test", 100)
	if err != nil {
		t.Fatalf("failed to insert in transaction: %v", err)
	}

	// Try to read while transaction is open - should handle gracefully
	done := make(chan error, 1)
	go func() {
		_, err := db.GetAllSeries()
		done <- err
	}()

	// Wait with timeout
	select {
	case err := <-done:
		if err != nil {
			t.Logf("GetAllSeries returned error during concurrent transaction (expected): %v", err)
		} else {
			t.Log("GetAllSeries completed successfully during concurrent transaction")
		}
	case <-time.After(10 * time.Second):
		t.Error("GetAllSeries timed out - database may be deadlocked")
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		t.Errorf("failed to commit transaction: %v", err)
	}

	// Verify we can read after commit
	series, err := db.GetAllSeries()
	if err != nil {
		t.Fatalf("GetAllSeries failed after commit: %v", err)
	}

	// Should have original 3 plus the transaction series
	if len(series) != 4 {
		t.Errorf("expected 4 series after commit, got %d", len(series))
	}
}

// TestSetSeriesDirty_MultipleCalls verifies idempotency - calling SetSeriesDirty multiple times doesn't cause issues
func TestSetSeriesDirty_MultipleCalls(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	series := &Series{
		Title:         "Test Show",
		Year:          2020,
		CanonicalPath: "/tv/Test Show (2020)",
		LibraryRoot:   "/tv",
		Source:        "jellywatch",
	}
	_, err := db.UpsertSeries(series)
	if err != nil {
		t.Fatalf("UpsertSeries failed: %v", err)
	}

	// Set dirty 10 times - should not error
	for i := 0; i < 10; i++ {
		err = db.SetSeriesDirty(series.ID)
		if err != nil {
			t.Errorf("SetSeriesDirty call %d failed: %v", i+1, err)
		}
	}

	// Verify dirty flag is still set
	retrieved, err := db.GetSeriesByID(series.ID)
	if err != nil {
		t.Fatalf("GetSeriesByID failed: %v", err)
	}
	if !retrieved.SonarrPathDirty {
		t.Error("dirty flag should be set after multiple calls")
	}
}

// TestSetMovieDirty_MultipleCalls verifies idempotency for movies
func TestSetMovieDirty_MultipleCalls(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	movie := &Movie{
		Title:         "Test Movie",
		Year:          2020,
		CanonicalPath: "/movies/Test Movie (2020)",
		LibraryRoot:   "/movies",
		Source:        "jellywatch",
	}
	_, err := db.UpsertMovie(movie)
	if err != nil {
		t.Fatalf("UpsertMovie failed: %v", err)
	}

	// Set dirty 10 times
	for i := 0; i < 10; i++ {
		err = db.SetMovieDirty(movie.ID)
		if err != nil {
			t.Errorf("SetMovieDirty call %d failed: %v", i+1, err)
		}
	}

	retrieved, err := db.GetMovieByID(movie.ID)
	if err != nil {
		t.Fatalf("GetMovieByID failed: %v", err)
	}
	if !retrieved.RadarrPathDirty {
		t.Error("dirty flag should be set after multiple calls")
	}
}

// TestMarkSeriesSynced_ClearsBothFlags verifies both sonarr and radarr dirty flags are cleared
func TestMarkSeriesSynced_ClearsBothFlags(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	series := &Series{
		Title:         "Test Show",
		Year:          2020,
		CanonicalPath: "/tv/Test Show (2020)",
		LibraryRoot:   "/tv",
		Source:        "jellywatch",
	}
	_, err := db.UpsertSeries(series)
	if err != nil {
		t.Fatalf("UpsertSeries failed: %v", err)
	}

	// Set both flags manually
	_, err = db.DB().Exec(`UPDATE series SET sonarr_path_dirty = 1, radarr_path_dirty = 1 WHERE id = ?`, series.ID)
	if err != nil {
		t.Fatalf("failed to set dirty flags manually: %v", err)
	}

	// Mark synced
	err = db.MarkSeriesSynced(series.ID)
	if err != nil {
		t.Fatalf("MarkSeriesSynced failed: %v", err)
	}

	retrieved, err := db.GetSeriesByID(series.ID)
	if err != nil {
		t.Fatalf("GetSeriesByID failed: %v", err)
	}
	if retrieved.SonarrPathDirty {
		t.Error("sonarr_path_dirty should be cleared")
	}
	if retrieved.RadarrPathDirty {
		t.Error("radarr_path_dirty should be cleared")
	}
	if retrieved.SonarrSyncedAt == nil {
		t.Error("SonarrSyncedAt should be set")
	}
	if retrieved.RadarrSyncedAt == nil {
		t.Error("RadarrSyncedAt should be set")
	}
}

// TestMarkMovieSynced_UpdatesTimestamp verifies timestamp is actually updated on subsequent syncs
func TestMarkMovieSynced_UpdatesTimestamp(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	movie := &Movie{
		Title:         "Test Movie",
		Year:          2020,
		CanonicalPath: "/movies/Test Movie (2020)",
		LibraryRoot:   "/movies",
		Source:        "jellywatch",
	}
	_, err := db.UpsertMovie(movie)
	if err != nil {
		t.Fatalf("UpsertMovie failed: %v", err)
	}

	// First sync
	err = db.SetMovieDirty(movie.ID)
	if err != nil {
		t.Fatalf("SetMovieDirty failed: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	err = db.MarkMovieSynced(movie.ID)
	if err != nil {
		t.Fatalf("MarkMovieSynced (first) failed: %v", err)
	}

	retrieved1, err := db.GetMovieByID(movie.ID)
	if err != nil {
		t.Fatalf("GetMovieByID (first) failed: %v", err)
	}

	// Second sync
	time.Sleep(10 * time.Millisecond)
	err = db.SetMovieDirty(movie.ID)
	if err != nil {
		t.Fatalf("SetMovieDirty (second) failed: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	err = db.MarkMovieSynced(movie.ID)
	if err != nil {
		t.Fatalf("MarkMovieSynced (second) failed: %v", err)
	}

	retrieved2, err := db.GetMovieByID(movie.ID)
	if err != nil {
		t.Fatalf("GetMovieByID (second) failed: %v", err)
	}

	// Verify timestamps are different (second sync should be later)
	if retrieved2.RadarrSyncedAt.Before(*retrieved1.RadarrSyncedAt) || retrieved2.RadarrSyncedAt.Equal(*retrieved1.RadarrSyncedAt) {
		t.Error("second sync timestamp should be later than first")
	}
}

// TestGetDirtySeries_OrderedByPriority verifies ORDER BY source_priority DESC works
func TestGetDirtySeries_OrderedByPriority(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create series with different priorities
	priorities := []int{50, 100, 75}

	for i, priority := range priorities {
		series := &Series{
			Title:          fmt.Sprintf("Test Show %d", i),
			Year:           2020 + i,
			CanonicalPath:  fmt.Sprintf("/tv/Test Show %d (%d)", i, 2020+i),
			LibraryRoot:    "/tv",
			Source:         "test",
			SourcePriority: priority,
		}
		_, err := db.UpsertSeries(series)
		if err != nil {
			t.Fatalf("UpsertSeries %d failed: %v", i, err)
		}
		_ = db.SetSeriesDirty(series.ID)
	}

	dirtySeries, err := db.GetDirtySeries()
	if err != nil {
		t.Fatalf("GetDirtySeries failed: %v", err)
	}

	if len(dirtySeries) != 3 {
		t.Fatalf("expected 3 dirty series, got %d", len(dirtySeries))
	}

	// Verify order: 100, 75, 50 (DESC)
	expectedOrder := []int{100, 75, 50}
	for i, series := range dirtySeries {
		if series.SourcePriority != expectedOrder[i] {
			t.Errorf("position %d: expected priority %d, got %d",
				i, expectedOrder[i], series.SourcePriority)
		}
	}
}

// TestGetDirtyMovies_OrderedByPriority verifies ordering for movies
func TestGetDirtyMovies_OrderedByPriority(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	priorities := []int{50, 100, 75}

	for i, priority := range priorities {
		movie := &Movie{
			Title:          fmt.Sprintf("Test Movie %d", i),
			Year:           2020 + i,
			CanonicalPath:  fmt.Sprintf("/movies/Test Movie %d (%d)", i, 2020+i),
			LibraryRoot:    "/movies",
			Source:         "test",
			SourcePriority: priority,
		}
		_, err := db.UpsertMovie(movie)
		if err != nil {
			t.Fatalf("UpsertMovie %d failed: %v", i, err)
		}
		_ = db.SetMovieDirty(movie.ID)
	}

	dirtyMovies, err := db.GetDirtyMovies()
	if err != nil {
		t.Fatalf("GetDirtyMovies failed: %v", err)
	}

	if len(dirtyMovies) != 3 {
		t.Fatalf("expected 3 dirty movies, got %d", len(dirtyMovies))
	}

	expectedOrder := []int{100, 75, 50}
	for i, movie := range dirtyMovies {
		if movie.SourcePriority != expectedOrder[i] {
			t.Errorf("position %d: expected priority %d, got %d",
				i, expectedOrder[i], movie.SourcePriority)
		}
	}
}

// TestDirtyFlags_DefaultToFalse verifies new records have dirty=0 by default
func TestDirtyFlags_DefaultToFalse(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create series without setting dirty
	series := &Series{
		Title:         "Test Show",
		Year:          2020,
		CanonicalPath: "/tv/Test Show (2020)",
		LibraryRoot:   "/tv",
		Source:        "jellywatch",
	}
	_, err := db.UpsertSeries(series)
	if err != nil {
		t.Fatalf("UpsertSeries failed: %v", err)
	}

	retrieved, err := db.GetSeriesByID(series.ID)
	if err != nil {
		t.Fatalf("GetSeriesByID failed: %v", err)
	}

	if retrieved.SonarrPathDirty {
		t.Error("sonarr_path_dirty should default to false")
	}
	if retrieved.RadarrPathDirty {
		t.Error("radarr_path_dirty should default to false")
	}
	if retrieved.SonarrSyncedAt != nil {
		t.Error("sonarr_synced_at should default to nil")
	}
	if retrieved.RadarrSyncedAt != nil {
		t.Error("radarr_synced_at should default to nil")
	}

	// Same for movies
	movie := &Movie{
		Title:         "Test Movie",
		Year:          2020,
		CanonicalPath: "/movies/Test Movie (2020)",
		LibraryRoot:   "/movies",
		Source:        "jellywatch",
	}
	_, err = db.UpsertMovie(movie)
	if err != nil {
		t.Fatalf("UpsertMovie failed: %v", err)
	}

	retrievedMovie, err := db.GetMovieByID(movie.ID)
	if err != nil {
		t.Fatalf("GetMovieByID failed: %v", err)
	}

	if retrievedMovie.RadarrPathDirty {
		t.Error("radarr_path_dirty should default to false for movies")
	}
	if retrievedMovie.RadarrSyncedAt != nil {
		t.Error("radarr_synced_at should default to nil for movies")
	}
}

// TestDirtyFlags_SurvivesRestart verifies dirty flags persist across DB close/reopen
func TestDirtyFlags_SurvivesRestart(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	// First session: create and set dirty
	db1, err := OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB first time: %v", err)
	}

	series := &Series{
		Title:         "Test Show",
		Year:          2020,
		CanonicalPath: "/tv/Test Show (2020)",
		LibraryRoot:   "/tv",
		Source:        "jellywatch",
	}
	_, err = db1.UpsertSeries(series)
	if err != nil {
		db1.Close()
		t.Fatalf("UpsertSeries failed: %v", err)
	}

	err = db1.SetSeriesDirty(series.ID)
	if err != nil {
		db1.Close()
		t.Fatalf("SetSeriesDirty failed: %v", err)
	}

	seriesID := series.ID
	db1.Close()

	// Second session: reopen and verify dirty flag persists
	db2, err := OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen DB: %v", err)
	}
	defer db2.Close()

	retrieved, err := db2.GetSeriesByID(seriesID)
	if err != nil {
		t.Fatalf("GetSeriesByID failed: %v", err)
	}
	if !retrieved.SonarrPathDirty {
		t.Error("dirty flag should persist across DB restart")
	}
}

// TestDirtyFlags_ScanError verifies error handling when table doesn't exist
func TestDirtyFlags_ScanError(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Corrupt the database by dropping the series table
	_, err := db.DB().Exec(`DROP TABLE series`)
	if err != nil {
		t.Fatalf("failed to drop series table: %v", err)
	}

	// Try to get dirty series - should error
	_, err = db.GetDirtySeries()
	if err == nil {
		t.Error("expected error when series table doesn't exist")
	}

	// Drop movies table too
	_, err = db.DB().Exec(`DROP TABLE movies`)
	if err != nil {
		t.Fatalf("failed to drop movies table: %v", err)
	}

	// Try to get dirty movies - should also error
	_, err = db.GetDirtyMovies()
	if err == nil {
		t.Error("expected error when movies table doesn't exist")
	}
}
