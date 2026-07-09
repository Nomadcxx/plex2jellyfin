package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
)

// openTestParseDB opens a temp DB for use in parses command tests.
// It returns the DB, the DB path, and a cleanup func.
func openTestParseDB(t *testing.T) (*database.MediaDB, string, func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := database.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	return db, dbPath, func() { db.Close() }
}

// reopenDB opens a second handle to the same DB file.
func reopenDB(t *testing.T, path string) *database.MediaDB {
	t.Helper()
	db, err := database.OpenPath(path)
	if err != nil {
		t.Fatalf("reopenDB: %v", err)
	}
	return db
}

func insertTestDecision(t *testing.T, db *database.MediaDB, d database.ParseDecision) int64 {
	t.Helper()
	id, err := db.InsertDecision(d)
	if err != nil {
		t.Fatalf("InsertDecision: %v", err)
	}
	return id
}

func runParsesCmd(t *testing.T, db *database.MediaDB, args ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	openDB := func() (*database.MediaDB, error) { return db, nil }
	cmd := newParsesCmdWithDeps(openDB, &stdout, &stderr)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func TestParsesCmd_Failures(t *testing.T) {
	db, _, cleanup := openTestParseDB(t)
	defer cleanup()

	now := time.Now()
	insertTestDecision(t, db, database.ParseDecision{
		SourcePath:     "/watch/tv/FailShow.S01E01.mkv",
		SourceFilename: "FailShow.S01E01.mkv",
		EventAt:        now,
		AutoLabel:      "FAIL",
	})
	insertTestDecision(t, db, database.ParseDecision{
		SourcePath:     "/watch/tv/OkShow.S01E02.mkv",
		SourceFilename: "OkShow.S01E02.mkv",
		EventAt:        now,
		AutoLabel:      "OK",
	})

	stdout, _, err := runParsesCmd(t, db, "--failures")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "FailShow.S01E01.mkv") {
		t.Errorf("expected FailShow in output, got: %s", stdout)
	}
	if strings.Contains(stdout, "OkShow.S01E02.mkv") {
		t.Errorf("expected OkShow to be filtered out, but found in output: %s", stdout)
	}
}

func TestParsesCmd_Drift(t *testing.T) {
	db, _, cleanup := openTestParseDB(t)
	defer cleanup()

	now := time.Now()
	insertTestDecision(t, db, database.ParseDecision{
		SourcePath:     "/watch/tv/DriftShow.S01E01.mkv",
		SourceFilename: "DriftShow.S01E01.mkv",
		EventAt:        now,
		AutoLabel:      "DRIFT",
	})
	insertTestDecision(t, db, database.ParseDecision{
		SourcePath:     "/watch/tv/OkShow.S01E02.mkv",
		SourceFilename: "OkShow.S01E02.mkv",
		EventAt:        now,
		AutoLabel:      "OK",
	})

	stdout, _, err := runParsesCmd(t, db, "--drift")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "DriftShow.S01E01.mkv") {
		t.Errorf("expected DriftShow in output, got: %s", stdout)
	}
	if strings.Contains(stdout, "OkShow.S01E02.mkv") {
		t.Errorf("expected OkShow to be filtered out, but found in output: %s", stdout)
	}
}

func TestParsesCmd_SourceContains(t *testing.T) {
	db, _, cleanup := openTestParseDB(t)
	defer cleanup()

	now := time.Now()
	insertTestDecision(t, db, database.ParseDecision{
		SourcePath:     "/watch/tv/Tracker.S02E19.mkv",
		SourceFilename: "Tracker.S02E19.mkv",
		EventAt:        now,
	})
	insertTestDecision(t, db, database.ParseDecision{
		SourcePath:     "/watch/movies/Example.Movie.2024.mkv",
		SourceFilename: "Example.Movie.2024.mkv",
		EventAt:        now,
	})

	stdout, _, err := runParsesCmd(t, db, "--source-contains", "Tracker")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "Tracker.S02E19.mkv") {
		t.Errorf("expected Tracker in output, got: %s", stdout)
	}
	if strings.Contains(stdout, "Example.Movie.2024.mkv") {
		t.Errorf("expected movie to be filtered out, but found in output: %s", stdout)
	}
}

func TestParsesCmd_Override(t *testing.T) {
	// Seed via dedicated connection, then close it before command runs.
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "override.db")
	seedDB, err := database.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("OpenPath seed: %v", err)
	}

	now := time.Now()
	id := insertTestDecision(t, seedDB, database.ParseDecision{
		SourcePath:     "/watch/tv/FailShow.S01E01.mkv",
		SourceFilename: "FailShow.S01E01.mkv",
		EventAt:        now,
		AutoLabel:      "FAIL",
	})
	seedDB.Close()

	// Command opens its own connection (and closes it via defer in RunE).
	cmdDB, err := database.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("reopen for command: %v", err)
	}
	var stdout, stderr bytes.Buffer
	openDB := func() (*database.MediaDB, error) { return cmdDB, nil }
	cmd := newParsesCmdWithDeps(openDB, &stdout, &stderr)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--override", itoa(id), "--label", "wrong"})
	if execErr := cmd.Execute(); execErr != nil {
		t.Fatalf("unexpected error: %v", execErr)
	}

	// Re-open for assertion (cmdDB was closed by the command's defer).
	assertDB := reopenDB(t, dbPath)
	defer assertDB.Close()

	got, err := assertDB.GetDecision(id)
	if err != nil {
		t.Fatalf("GetDecision: %v", err)
	}
	if got == nil {
		t.Fatal("GetDecision returned nil")
	}
	if got.HumanLabelOverride != "wrong" {
		t.Errorf("HumanLabelOverride = %q, want %q", got.HumanLabelOverride, "wrong")
	}
}

func TestParsesCmd_InvalidLabel(t *testing.T) {
	db, _, cleanup := openTestParseDB(t)
	defer cleanup()

	now := time.Now()
	id := insertTestDecision(t, db, database.ParseDecision{
		SourcePath:     "/watch/tv/SomeShow.S01E01.mkv",
		SourceFilename: "SomeShow.S01E01.mkv",
		EventAt:        now,
	})

	_, _, err := runParsesCmd(t, db, "--override", itoa(id), "--label", "notvalid")
	if err == nil {
		t.Fatal("expected error for invalid label, got nil")
	}
	if !strings.Contains(err.Error(), "invalid label") {
		t.Errorf("expected 'invalid label' in error, got: %v", err)
	}
}

func itoa(n int64) string {
	return fmt.Sprintf("%d", n)
}
