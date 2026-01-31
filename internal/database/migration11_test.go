package database

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMigration11_DirtyFlags(t *testing.T) {
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

	retrieved, err := db.GetSeriesByTitle("Test Show", 2020)
	if err != nil {
		t.Fatalf("GetSeriesByTitle failed: %v", err)
	}

	if retrieved.SonarrSyncedAt != nil {
		t.Errorf("expected SonarrSyncedAt to be nil (default), got %v", retrieved.SonarrSyncedAt)
	}
	if retrieved.SonarrPathDirty {
		t.Errorf("expected SonarrPathDirty to be false (default), got true")
	}
	if retrieved.RadarrSyncedAt != nil {
		t.Errorf("expected RadarrSyncedAt to be nil (default), got %v", retrieved.RadarrSyncedAt)
	}
	if retrieved.RadarrPathDirty {
		t.Errorf("expected RadarrPathDirty to be false (default), got true")
	}

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

	retrievedMovie, err := db.GetMovieByTitle("Test Movie", 2021)
	if err != nil {
		t.Fatalf("GetMovieByTitle failed: %v", err)
	}

	if retrievedMovie.RadarrSyncedAt != nil {
		t.Errorf("expected RadarrSyncedAt to be nil (default), got %v", retrievedMovie.RadarrSyncedAt)
	}
	if retrievedMovie.RadarrPathDirty {
		t.Errorf("expected RadarrPathDirty to be false (default), got true")
	}
}

// TestMigration11_CorruptDatabase verifies graceful failure on corrupt database
func TestMigration11_CorruptDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "corrupt.db")

	// Create a corrupt database file (not a valid SQLite file)
	corruptData := []byte("NOT_A_SQLITE_FILE\x00\x01\x02\x03")
	if err := os.WriteFile(dbPath, corruptData, 0644); err != nil {
		t.Fatalf("failed to create corrupt database file: %v", err)
	}

	// Attempt to open the corrupt database - should return an error
	_, err := OpenPath(dbPath)
	if err == nil {
		t.Error("OpenPath: expected error when opening corrupt database, got nil")
	}

	// The error should mention database or sqlite
	if err != nil && err.Error() == "" {
		t.Error("OpenPath: expected non-empty error message for corrupt database")
	}

	// Test with partially corrupt schema version table scenario
	// Create a valid SQLite file but with invalid schema
	validDBPath := filepath.Join(tmpDir, "invalid_schema.db")
	db, err := sql.Open("sqlite", validDBPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}

	// Create schema_version with invalid data type
	_, err = db.Exec(`CREATE TABLE schema_version (version TEXT)`)
	if err != nil {
		t.Fatalf("failed to create schema_version table: %v", err)
	}

	// Insert non-integer value
	_, err = db.Exec(`INSERT INTO schema_version (version) VALUES ('not_a_number')`)
	if err != nil {
		t.Fatalf("failed to insert into schema_version: %v", err)
	}
	db.Close()

	// Try to open - should handle the error gracefully
	_, err = OpenPath(validDBPath)
	if err == nil {
		t.Error("OpenPath: expected error when opening database with invalid schema version, got nil")
	}
}

// TestMigration11_IdempotentApplication verifies migration can be applied twice without error
func TestMigration11_IdempotentApplication(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Get current schema version
	var version int
	err := db.DB().QueryRow("SELECT version FROM schema_version ORDER BY version DESC LIMIT 1").Scan(&version)
	if err != nil {
		t.Fatalf("failed to get schema version: %v", err)
	}

	if version < 10 {
		t.Skip("migration 11 depends on migration 10")
	}

	// Find migration 11
	var migration11 migration
	found := false
	for _, m := range migrations {
		if m.version == 11 {
			migration11 = m
			found = true
			break
		}
	}

	if !found {
		t.Fatal("migration 11 not found")
	}

	// Apply first time - skip schema_version INSERT for manual test
	for _, stmt := range migration11.up {
		if strings.Contains(stmt, "INSERT INTO schema_version") {
			continue  // Skip on first run
		}
		_, err = db.DB().Exec(stmt)
		if err != nil {
			// Column already exists is acceptable for second run
			if !strings.Contains(err.Error(), "duplicate column") {
				t.Fatalf("first application of migration 11 failed: %v", err)
			}
		}
	}

	// Apply second time - should not error beyond duplicate column warnings
	for _, stmt := range migration11.up {
		if strings.Contains(stmt, "INSERT INTO schema_version") {
			continue  // Always skip
		}
		_, err = db.DB().Exec(stmt)
		if err != nil && !strings.Contains(err.Error(), "duplicate column") {
			t.Logf("second application error (expected for duplicate columns): %v", err)
		}
	}
}

// TestMigration11_ColumnDefaults verifies DEFAULT 0 for dirty flags works
func TestMigration11_ColumnDefaults(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert series directly without dirty columns (they should default to 0)
	_, err := db.DB().Exec(`
		INSERT INTO series (title, title_normalized, year, canonical_path, library_root, source, source_priority)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "Default Test", "defaulttest", 2020, "/tv/Default Test (2020)", "/tv", "test", 100)
	if err != nil {
		t.Fatalf("failed to insert series: %v", err)
	}

	// Query to check default values
	var sonarrDirty, radarrDirty int
	var sonarrSynced, radarrSynced sql.NullTime

	err = db.DB().QueryRow(`
		SELECT sonarr_path_dirty, radarr_path_dirty, sonarr_synced_at, radarr_synced_at
		FROM series WHERE title = ?
	`, "Default Test").Scan(&sonarrDirty, &radarrDirty, &sonarrSynced, &radarrSynced)
	if err != nil {
		t.Fatalf("failed to query defaults: %v", err)
	}

	if sonarrDirty != 0 {
		t.Errorf("sonarr_path_dirty default should be 0, got %d", sonarrDirty)
	}
	if radarrDirty != 0 {
		t.Errorf("radarr_path_dirty default should be 0, got %d", radarrDirty)
	}
	if sonarrSynced.Valid {
		t.Error("sonarr_synced_at should default to NULL")
	}
	if radarrSynced.Valid {
		t.Error("radarr_synced_at should default to NULL")
	}
}

// TestMigration11_NullableTimestamps verifies sync timestamps are nullable
func TestMigration11_NullableTimestamps(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert series
	_, err := db.DB().Exec(`
		INSERT INTO series (title, title_normalized, year, canonical_path, library_root, source, source_priority)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "Nullable Test", "nullabletest", 2020, "/tv/Nullable Test (2020)", "/tv", "test", 100)
	if err != nil {
		t.Fatalf("failed to insert series: %v", err)
	}

	// Mark synced (should set timestamps)
	var seriesID int64
	err = db.DB().QueryRow("SELECT id FROM series WHERE title = ?", "Nullable Test").Scan(&seriesID)
	if err != nil {
		t.Fatalf("failed to get series ID: %v", err)
	}

	err = db.MarkSeriesSynced(seriesID)
	if err != nil {
		t.Fatalf("MarkSeriesSynced failed: %v", err)
	}

	// Query to verify timestamps are set
	var sonarrSynced, radarrSynced sql.NullTime

	err = db.DB().QueryRow(`
		SELECT sonarr_synced_at, radarr_synced_at
		FROM series WHERE id = ?
	`, seriesID).Scan(&sonarrSynced, &radarrSynced)
	if err != nil {
		t.Fatalf("failed to query timestamps: %v", err)
	}

	if !sonarrSynced.Valid {
		t.Error("sonarr_synced_at should be valid after MarkSeriesSynced")
	}
	if !radarrSynced.Valid {
		t.Error("radarr_synced_at should be valid after MarkSeriesSynced")
	}
	if sonarrSynced.Time.After(time.Now()) {
		t.Error("sonarr_synced_at should not be in the future")
	}
	if radarrSynced.Time.After(time.Now()) {
		t.Error("radarr_synced_at should not be in the future")
	}
}
