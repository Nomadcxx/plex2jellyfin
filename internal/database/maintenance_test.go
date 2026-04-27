package database

import (
	"context"
	"path/filepath"
	"testing"
)

func TestResetDatabaseTruncatesUserTables(t *testing.T) {
	dir := t.TempDir()
	mdb, err := OpenPath(filepath.Join(dir, "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer mdb.Close()

	db := mdb.SQL()
	if _, err := db.Exec(`INSERT INTO media_files (path, size, media_type, normalized_title) VALUES ('/x', 0, 'movie', 'x')`); err != nil {
		t.Fatal(err)
	}

	progress := make(chan ProgressEvent, 64)
	go func() {
		for range progress {
		}
	}()

	if err := ResetDatabase(context.Background(), db, nil, progress); err != nil {
		t.Fatal(err)
	}
	close(progress)

	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM media_files").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("media_files count = %d, want 0", n)
	}
}

func TestResetDatabasePreservesNamedTable(t *testing.T) {
	dir := t.TempDir()
	mdb, err := OpenPath(filepath.Join(dir, "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer mdb.Close()
	db := mdb.SQL()
	if _, err := db.Exec(`INSERT INTO sync_log (source, started_at, status) VALUES ('boot', CURRENT_TIMESTAMP, 'running')`); err != nil {
		t.Skip("sync_log table not present in schema; preserve-list test skipped")
	}
	progress := make(chan ProgressEvent, 64)
	go func() {
		for range progress {
		}
	}()
	if err := ResetDatabase(context.Background(), db, []string{"sync_log"}, progress); err != nil {
		t.Fatal(err)
	}
	close(progress)
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM sync_log").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("sync_log preserved count = %d, want 1", n)
	}
}
