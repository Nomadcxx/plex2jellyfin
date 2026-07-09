package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/plans"
	"github.com/Nomadcxx/plex2jellyfin/internal/service"
)

func TestDeleteDuplicateWithPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := database.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	testFile := filepath.Join(t.TempDir(), "duplicate.mkv")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	file := &database.MediaFile{
		Path:      testFile,
		MediaType: "movie",
		Size:      4,
	}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}

	err = deleteDuplicateFile(db, testFile, -1, -1)

	if err != nil {
		t.Errorf("Delete failed even after permission fix: %v", err)
	}

	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("File still exists after delete")
	}

	dbFile, err := db.GetMediaFile(testFile)
	if err == nil && dbFile != nil {
		t.Error("Database entry still exists after successful delete")
	}
}

func TestDeleteDuplicateDatabaseCleanupOnFailure(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := database.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	testFile := "/nonexistent/duplicate.mkv"

	file := &database.MediaFile{
		Path:      testFile,
		MediaType: "movie",
		Size:      100,
	}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}

	err = deleteDuplicateFile(db, testFile, -1, -1)

	if err != nil {
		t.Logf("Expected error for non-existent file: %v", err)
	}

	dbFile, err := db.GetMediaFile(testFile)
	if err == nil && dbFile != nil {
		t.Error("Database entry should be removed even when file doesn't exist")
	}
}

func TestDuplicatePlanRootIssuesBlocksPathsOutsideConfiguredLibraries(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Libraries.Movies = []string{"/library-movies"}
	cfg.Libraries.TV = nil
	plan := &plans.DuplicatePlan{
		Plans: []plans.DuplicateGroup{{
			Keep:   plans.FileInfo{Path: "/library-movies/Movie (2020)/Movie (2020).mkv"},
			Delete: plans.FileInfo{Path: "/tmp/evil.mkv"},
		}},
	}

	issues := duplicatePlanRootIssues(plan, cfg)
	if !containsIssue(issues, "outside configured library roots") {
		t.Fatalf("expected root safety issue, got %#v", issues)
	}
}

func TestDuplicatePlanRootIssuesAllowsConfiguredLibraries(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Libraries.Movies = []string{"/library-movies"}
	cfg.Libraries.TV = nil
	plan := &plans.DuplicatePlan{
		Plans: []plans.DuplicateGroup{{
			Keep:   plans.FileInfo{Path: "/library-movies/Movie (2020)/Movie (2020).mkv"},
			Delete: plans.FileInfo{Path: "/library-movies/Movie (2020)/Movie (2020) duplicate.mkv"},
		}},
	}

	if issues := duplicatePlanRootIssues(plan, cfg); len(issues) != 0 {
		t.Fatalf("expected no root safety issues, got %#v", issues)
	}
}

func TestDuplicateGroupLooksUnsafeTVMovie(t *testing.T) {
	root := t.TempDir()
	tvRoot := filepath.Join(root, "TVSHOWS")
	movieRoot := filepath.Join(root, "MOVIES")
	cfg := &config.Config{}
	cfg.Libraries.TV = []string{tvRoot}

	unsafe := service.DuplicateGroup{
		MediaType: "movie",
		Files: []service.MediaFile{
			{Path: filepath.Join(tvRoot, "The Daily Show (1996)", "Season 2020", "hash.mkv")},
		},
	}
	if !duplicateGroupLooksUnsafeTVMovie(unsafe, cfg) {
		t.Fatal("expected movie duplicate under TV root to be unsafe")
	}

	safeMovie := service.DuplicateGroup{
		MediaType: "movie",
		Files: []service.MediaFile{
			{Path: filepath.Join(movieRoot, "Robots (2005)", "Robots (2005).mkv")},
		},
	}
	if duplicateGroupLooksUnsafeTVMovie(safeMovie, cfg) {
		t.Fatal("expected movie duplicate outside TV roots to be safe")
	}

	episode := service.DuplicateGroup{
		MediaType: "series",
		Files: []service.MediaFile{
			{Path: filepath.Join(tvRoot, "Show (2020)", "Season 01", "Show S01E01.mkv")},
		},
	}
	if duplicateGroupLooksUnsafeTVMovie(episode, cfg) {
		t.Fatal("expected episode duplicate under TV root to be safe")
	}
}

func TestRunDuplicatesExecuteReportsDeleteFailures(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("PLEX2JELLYFIN_TEST_NO_ESCALATE", "1")

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := database.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	nonEmptyDir := filepath.Join(tmpDir, "not-a-file")
	if err := os.Mkdir(nonEmptyDir, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nonEmptyDir, "child.txt"), []byte("x"), 0644); err != nil {
		t.Fatalf("Failed to create child file: %v", err)
	}

	plan := &plans.DuplicatePlan{
		CreatedAt: time.Now(),
		Command:   "duplicates generate",
		Summary: plans.DuplicateSummary{
			TotalGroups:      1,
			FilesToDelete:    1,
			SpaceReclaimable: 1,
		},
		Plans: []plans.DuplicateGroup{{
			GroupID:   "g1",
			Title:     "Example",
			MediaType: "movie",
			Delete: plans.FileInfo{
				Path: nonEmptyDir,
				Size: 1,
			},
		}},
	}
	if err := plans.SaveDuplicatePlans(plan); err != nil {
		t.Fatalf("SaveDuplicatePlans failed: %v", err)
	}

	oldStdin := os.Stdin
	oldStdout := os.Stdout
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	os.Stdin = stdinR
	os.Stdout = stdoutW
	t.Cleanup(func() {
		os.Stdin = oldStdin
		os.Stdout = oldStdout
	})

	if _, err := stdinW.WriteString("y\n"); err != nil {
		t.Fatalf("write confirmation: %v", err)
	}
	_ = stdinW.Close()

	err = runDuplicatesExecute(db, nil)
	_ = stdoutW.Close()
	var output bytes.Buffer
	_, _ = io.Copy(&output, stdoutR)
	if err != nil {
		t.Fatalf("runDuplicatesExecute failed: %v", err)
	}

	if !strings.Contains(output.String(), "1 files failed to delete") {
		t.Fatalf("expected failure count in output, got:\n%s", output.String())
	}
}

func TestRunDuplicatesExecuteArchivesPlanOnPartialFailure(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("PLEX2JELLYFIN_TEST_NO_ESCALATE", "1")

	db, err := database.OpenPath(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	deletable := filepath.Join(tmpDir, "duplicate.mkv")
	if err := os.WriteFile(deletable, []byte("x"), 0644); err != nil {
		t.Fatalf("Failed to create duplicate file: %v", err)
	}
	nonEmptyDir := filepath.Join(tmpDir, "not-a-file")
	if err := os.Mkdir(nonEmptyDir, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nonEmptyDir, "child.txt"), []byte("x"), 0644); err != nil {
		t.Fatalf("Failed to create child file: %v", err)
	}

	plan := &plans.DuplicatePlan{
		CreatedAt: time.Now(),
		Command:   "duplicates generate",
		Summary: plans.DuplicateSummary{
			TotalGroups:      2,
			FilesToDelete:    2,
			SpaceReclaimable: 2,
		},
		Plans: []plans.DuplicateGroup{
			{
				GroupID:   "g1",
				Title:     "Deleted",
				MediaType: "movie",
				Delete: plans.FileInfo{
					Path: deletable,
					Size: 1,
				},
			},
			{
				GroupID:   "g2",
				Title:     "Failed",
				MediaType: "movie",
				Delete: plans.FileInfo{
					Path: nonEmptyDir,
					Size: 1,
				},
			},
		},
	}
	if err := plans.SaveDuplicatePlans(plan); err != nil {
		t.Fatalf("SaveDuplicatePlans failed: %v", err)
	}

	oldStdin := os.Stdin
	oldStdout := os.Stdout
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	os.Stdin = stdinR
	os.Stdout = stdoutW
	t.Cleanup(func() {
		os.Stdin = oldStdin
		os.Stdout = oldStdout
	})

	if _, err := stdinW.WriteString("y\n"); err != nil {
		t.Fatalf("write confirmation: %v", err)
	}
	_ = stdinW.Close()

	if err := runDuplicatesExecute(db, nil); err != nil {
		t.Fatalf("runDuplicatesExecute failed: %v", err)
	}
	_ = stdoutW.Close()
	_, _ = io.Copy(io.Discard, stdoutR)

	plansDir, err := plans.GetPlansDir()
	if err != nil {
		t.Fatalf("GetPlansDir: %v", err)
	}
	planPath := filepath.Join(plansDir, "duplicates.json")
	if _, err := os.Stat(planPath); !os.IsNotExist(err) {
		t.Fatalf("active plan should be archived after partial failure, stat err=%v", err)
	}
	if _, err := os.Stat(planPath + ".old"); err != nil {
		t.Fatalf("archived plan should exist after partial failure: %v", err)
	}
}
