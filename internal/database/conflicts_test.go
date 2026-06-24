package database

import (
	"testing"
	"time"
)

// dropUniqueConstraint recreates the given table without the UNIQUE constraint
// so we can insert duplicate (title_normalized, year) rows to simulate conflicts.
func dropUniqueConstraint(t *testing.T, db *MediaDB, table string) {
	t.Helper()

	// SQLite cannot DROP CONSTRAINT, so we recreate the table.
	// 1. Rename original
	_, err := db.db.Exec("ALTER TABLE " + table + " RENAME TO " + table + "_orig")
	if err != nil {
		t.Fatalf("rename %s failed: %v", table, err)
	}

	// 2. Create new table without UNIQUE constraint
	var createSQL string
	switch table {
	case "movies":
		createSQL = `CREATE TABLE movies (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			title_normalized TEXT NOT NULL,
			year INTEGER,
			tmdb_id INTEGER, imdb_id TEXT, radarr_id INTEGER,
			canonical_path TEXT NOT NULL, library_root TEXT NOT NULL,
			source TEXT NOT NULL DEFAULT 'jellywatch',
			source_priority INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_synced_at DATETIME
		)`
	case "series":
		createSQL = `CREATE TABLE series (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			title_normalized TEXT NOT NULL,
			year INTEGER,
			tvdb_id INTEGER, imdb_id TEXT, sonarr_id INTEGER,
			canonical_path TEXT NOT NULL, library_root TEXT NOT NULL,
			source TEXT NOT NULL DEFAULT 'jellywatch',
			source_priority INTEGER NOT NULL DEFAULT 0,
			episode_count INTEGER DEFAULT 0,
			last_episode_added DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_synced_at DATETIME
		)`
	}
	_, err = db.db.Exec(createSQL)
	if err != nil {
		t.Fatalf("recreate %s failed: %v", table, err)
	}

	// 3. Copy data back (empty in fresh test DB, but keeps schema consistent)
	_, err = db.db.Exec("INSERT INTO " + table + " SELECT * FROM " + table + "_orig")
	if err != nil {
		// OK if orig is empty
	}
	// 4. Drop original
	_, _ = db.db.Exec("DROP TABLE " + table + "_orig")
}

// insertConflictRow inserts a raw row into movies or series table to simulate
// a conflict (same title_normalized + year, different canonical_path).
// Requires dropUniqueConstraint to have been called first.
func insertConflictRow(t *testing.T, db *MediaDB, table, title, normTitle string, year int, path string) {
	t.Helper()
	_, err := db.db.Exec(
		`INSERT INTO `+table+` (title, title_normalized, year, canonical_path, library_root, source, source_priority, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		title, normTitle, year, path, "/library", "test", 50, time.Now(), time.Now(),
	)
	if err != nil {
		t.Fatalf("insert into %s failed: %v", table, err)
	}
}

func TestDetectConflicts_FindsMovieAndSeriesConflicts(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Bypass UNIQUE(title_normalized, year) to simulate conflicts
	dropUniqueConstraint(t, db, "movies")
	dropUniqueConstraint(t, db, "series")

	// Movie conflict: same title_normalized+year, two different paths
	insertConflictRow(t, db, "movies", "Dracula", "dracula", 2020, "/storage1/Dracula (2020)")
	insertConflictRow(t, db, "movies", "Dracula", "dracula", 2020, "/storage5/Dracula (2020)")

	// Series conflict: same title_normalized+year, two different paths
	insertConflictRow(t, db, "series", "Rome", "rome", 2005, "/storage2/Rome (2005)")
	insertConflictRow(t, db, "series", "Rome", "rome", 2005, "/storage7/Rome (2005)")

	// Non-conflict: unique movie (should NOT appear)
	insertConflictRow(t, db, "movies", "Inception", "inception", 2010, "/storage1/Inception (2010)")

	conflicts, err := db.DetectConflicts()
	if err != nil {
		t.Fatalf("DetectConflicts failed: %v", err)
	}

	if len(conflicts) != 2 {
		t.Fatalf("expected 2 conflicts, got %d: %+v", len(conflicts), conflicts)
	}

	var movieConflict, seriesConflict *Conflict
	for i := range conflicts {
		switch conflicts[i].MediaType {
		case "movie":
			movieConflict = &conflicts[i]
		case "series":
			seriesConflict = &conflicts[i]
		}
	}

	if movieConflict == nil {
		t.Fatal("expected a movie conflict, got none")
	}
	if movieConflict.Title != "Dracula" {
		t.Errorf("movie conflict title: got %q, want %q", movieConflict.Title, "Dracula")
	}
	if movieConflict.TitleNormalized != "dracula" {
		t.Errorf("movie conflict norm: got %q, want %q", movieConflict.TitleNormalized, "dracula")
	}
	if len(movieConflict.Locations) != 2 {
		t.Errorf("movie conflict locations: got %d, want 2", len(movieConflict.Locations))
	}

	if seriesConflict == nil {
		t.Fatal("expected a series conflict, got none")
	}
	if seriesConflict.Title != "Rome" {
		t.Errorf("series conflict title: got %q, want %q", seriesConflict.Title, "Rome")
	}
	if seriesConflict.TitleNormalized != "rome" {
		t.Errorf("series conflict norm: got %q, want %q", seriesConflict.TitleNormalized, "rome")
	}
	if len(seriesConflict.Locations) != 2 {
		t.Errorf("series conflict locations: got %d, want 2", len(seriesConflict.Locations))
	}
}

func TestDetectConflicts_NoConflictsReturnsEmpty(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Single movie, single series — no conflicts (UNIQUE constraint allows these)
	insertConflictRow(t, db, "movies", "Inception", "inception", 2010, "/storage1/Inception (2010)")
	insertConflictRow(t, db, "series", "Rome", "rome", 2005, "/storage2/Rome (2005)")

	conflicts, err := db.DetectConflicts()
	if err != nil {
		t.Fatalf("DetectConflicts failed: %v", err)
	}

	if len(conflicts) != 0 {
		t.Fatalf("expected 0 conflicts, got %d: %+v", len(conflicts), conflicts)
	}
}

func TestDetectConflicts_SameTitleDifferentYearNoConflict(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Same title, different years = different media items, not a conflict
	insertConflictRow(t, db, "movies", "Dracula", "dracula", 2020, "/storage1/Dracula (2020)")
	insertConflictRow(t, db, "movies", "Dracula", "dracula", 2025, "/storage5/Dracula (2025)")

	conflicts, err := db.DetectConflicts()
	if err != nil {
		t.Fatalf("DetectConflicts failed: %v", err)
	}

	if len(conflicts) != 0 {
		t.Fatalf("expected 0 conflicts (different years), got %d: %+v", len(conflicts), conflicts)
	}
}
