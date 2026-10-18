package database

import (
	"context"
	"errors"
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

	if err := ResetDatabase(context.Background(), mdb, nil, progress); err != nil {
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
	if err := ResetDatabase(context.Background(), mdb, []string{"sync_log"}, progress); err != nil {
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

func TestResetDatabaseCancellationRollsBackEarlierDeletes(t *testing.T) {
	mdb, err := OpenPath(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer mdb.Close()
	db := mdb.SQL()
	resetTables := []string{"reset_first", "reset_second", "reset_third"}
	for _, table := range resetTables {
		if _, err := db.Exec(`CREATE TABLE ` + `"` + table + `"` + ` (value TEXT)`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO "` + table + `" (value) VALUES ('kept')`); err != nil {
			t.Fatal(err)
		}
	}

	tables, err := listUserTables(db)
	if err != nil {
		t.Fatal(err)
	}
	preserve := make([]string, 0, len(tables)-len(resetTables))
	for _, table := range tables {
		if table != "reset_first" && table != "reset_second" && table != "reset_third" {
			preserve = append(preserve, table)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	progress := make(chan ProgressEvent)
	done := make(chan error, 1)
	go func() {
		done <- ResetDatabase(ctx, mdb, preserve, progress)
		close(progress)
	}()
	truncating := 0
	for event := range progress {
		if event.Phase == "truncating" {
			truncating++
			if truncating == 2 {
				cancel()
			}
		}
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("ResetDatabase error = %v, want context cancellation", err)
	}

	for _, table := range resetTables {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM "` + table + `"`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("%s row count = %d, want 1 after cancellation", table, count)
		}
	}
}
