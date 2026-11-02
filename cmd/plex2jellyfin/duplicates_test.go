package main

import (
	"bytes"
	"fmt"
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

func TestDeleteDuplicateKeepsDatabaseRowWhenRemovalFails(t *testing.T) {
	db, err := database.OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	path := filepath.Join(t.TempDir(), "nonempty-directory")
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, "keep.txt"), []byte("keep"), 0644); err != nil {
		t.Fatalf("Failed to create directory entry: %v", err)
	}
	if err := db.UpsertMediaFile(&database.MediaFile{Path: path, MediaType: "movie", Size: 4}); err != nil {
		t.Fatalf("Failed to insert media file: %v", err)
	}

	if err := deleteDuplicateFile(db, path, -1, -1); err == nil {
		t.Fatal("expected removal error for non-empty directory")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("media path should remain after failed removal: %v", err)
	}
	if file, err := db.GetMediaFile(path); err != nil || file == nil {
		t.Fatalf("database row should remain after failed removal, file=%v err=%v", file, err)
	}
}

func TestDeleteDuplicateReportsDatabaseCleanupFailure(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := database.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

	path := filepath.Join(tmpDir, "duplicate.mkv")
	if err := os.WriteFile(path, []byte("video"), 0644); err != nil {
		t.Fatalf("create duplicate: %v", err)
	}
	if err := db.UpsertMediaFile(&database.MediaFile{Path: path, MediaType: "movie", Size: 5}); err != nil {
		t.Fatalf("insert media file: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	if err := deleteDuplicateFile(db, path, -1, -1); err == nil {
		t.Fatal("expected database cleanup failure")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file should be removed before the database error, stat err=%v", err)
	}

	checkDB, err := database.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	defer checkDB.Close()
	file, err := checkDB.GetMediaFile(path)
	if err != nil {
		t.Fatalf("read media file row: %v", err)
	}
	if file == nil {
		t.Fatal("expected failed cleanup to leave the row for later reconciliation")
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

func TestRunDuplicatesGenerateSkipsLowConfidenceGroups(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("SUDO_USER", "")

	db, err := database.Open()
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

	year := 2020
	for i, size := range []int64{100, 80} {
		path := filepath.Join(root, fmt.Sprintf("copy-%d.mkv", i))
		if err := os.WriteFile(path, bytes.Repeat([]byte{'x'}, int(size)), 0644); err != nil {
			t.Fatalf("create media file: %v", err)
		}
		if err := db.UpsertMediaFile(&database.MediaFile{
			Path:            path,
			Size:            size,
			MediaType:       "movie",
			NormalizedTitle: "example movie",
			Year:            &year,
			Resolution:      "1080p",
		}); err != nil {
			t.Fatalf("insert media file: %v", err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	if err := runDuplicatesGenerate(); err != nil {
		t.Fatalf("generate duplicate plan: %v", err)
	}

	plan, err := plans.LoadDuplicatePlans()
	if err != nil {
		t.Fatalf("load duplicate plan: %v", err)
	}
	if plan != nil {
		t.Fatalf("low-confidence group produced a deletable plan: %#v", plan.Plans)
	}
}

func TestRunDuplicatesExecuteRefusesLowConfidencePlan(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("SUDO_USER", "")
	t.Setenv("PLEX2JELLYFIN_TEST_NO_ESCALATE", "1")

	db, err := database.Open()
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	year := 2020
	paths := []string{filepath.Join(root, "keep.mkv"), filepath.Join(root, "delete.mkv")}
	for i, size := range []int64{100, 80} {
		if err := os.WriteFile(paths[i], bytes.Repeat([]byte{'x'}, int(size)), 0644); err != nil {
			t.Fatalf("create media file: %v", err)
		}
		if err := db.UpsertMediaFile(&database.MediaFile{
			Path:            paths[i],
			Size:            size,
			MediaType:       "movie",
			NormalizedTitle: "example movie",
			Year:            &year,
			Resolution:      "1080p",
		}); err != nil {
			t.Fatalf("insert media file: %v", err)
		}
	}

	analysis, err := service.NewCleanupService(db).AnalyzeDuplicates()
	if err != nil {
		t.Fatalf("analyze duplicates: %v", err)
	}
	if len(analysis.Groups) != 1 || len(analysis.Groups[0].Files) != 2 {
		t.Fatalf("expected one two-file duplicate group, got %#v", analysis.Groups)
	}
	group := analysis.Groups[0]
	plan := &plans.DuplicatePlan{
		Plans: []plans.DuplicateGroup{{
			GroupID:   group.ID,
			Title:     group.Title,
			Year:      group.Year,
			MediaType: group.MediaType,
			Keep:      plans.FileInfo{ID: group.Files[0].ID, Path: group.Files[0].Path, Size: group.Files[0].Size},
			Delete:    plans.FileInfo{ID: group.Files[1].ID, Path: group.Files[1].Path, Size: group.Files[1].Size},
		}},
	}
	if err := plans.SaveDuplicatePlans(plan); err != nil {
		t.Fatalf("save duplicate plan: %v", err)
	}

	oldStdin := os.Stdin
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	os.Stdin = stdinR
	t.Cleanup(func() { os.Stdin = oldStdin })
	if _, err := stdinW.WriteString("y\n"); err != nil {
		t.Fatalf("write confirmation: %v", err)
	}
	_ = stdinW.Close()

	if err := runDuplicatesExecute(db, nil); err != nil {
		t.Fatalf("execute duplicate plan: %v", err)
	}
	if _, err := os.Stat(paths[1]); err != nil {
		t.Fatalf("low-confidence duplicate was deleted: %v", err)
	}
}

func TestDuplicatePlanCurrentIssuesRejectsChangedFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))

	db, err := database.Open()
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	group := addHighConfidenceDuplicatePlanGroup(t, db, root, "changed", false)
	before, err := os.Stat(group.Delete.Path)
	if err != nil {
		t.Fatalf("stat planned delete file: %v", err)
	}
	changedContents := bytes.Repeat([]byte{'n'}, int(group.Delete.Size))
	if err := os.WriteFile(group.Delete.Path, changedContents, 0o644); err != nil {
		t.Fatalf("replace planned delete file: %v", err)
	}
	changedTime := before.ModTime().Add(time.Hour)
	if err := os.Chtimes(group.Delete.Path, changedTime, changedTime); err != nil {
		t.Fatalf("change planned file modification time: %v", err)
	}

	issues, err := duplicatePlanCurrentIssues(db, nil, &plans.DuplicatePlan{Plans: []plans.DuplicateGroup{group}})
	if err != nil {
		t.Fatalf("revalidate changed duplicate plan: %v", err)
	}
	if len(issues) == 0 {
		t.Fatal("changed planned file was accepted for deletion")
	}
	if err := deletePlannedDuplicateFile(db, group, -1, -1); err == nil {
		t.Fatal("changed planned file passed the final deletion check")
	}
	if _, err := os.Stat(group.Delete.Path); err != nil {
		t.Fatalf("changed planned file was deleted: %v", err)
	}
	if _, err := db.GetMediaFile(group.Delete.Path); err != nil {
		t.Fatalf("changed planned file inventory row was removed: %v", err)
	}
}

func TestDeletePlannedDuplicateFileRejectsChangedKeeper(t *testing.T) {
	root := t.TempDir()
	db, err := database.OpenPath(filepath.Join(root, "media.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	group := addHighConfidenceDuplicatePlanGroup(t, db, root, "changed-keeper", false)
	before, err := os.Stat(group.Keep.Path)
	if err != nil {
		t.Fatalf("stat planned keeper: %v", err)
	}
	if err := os.WriteFile(group.Keep.Path, bytes.Repeat([]byte{'n'}, int(group.Keep.Size)), 0o644); err != nil {
		t.Fatalf("replace planned keeper: %v", err)
	}
	changedTime := before.ModTime().Add(time.Hour)
	if err := os.Chtimes(group.Keep.Path, changedTime, changedTime); err != nil {
		t.Fatalf("change keeper modification time: %v", err)
	}

	if err := deletePlannedDuplicateFile(db, group, -1, -1); err == nil {
		t.Fatal("delete candidate was removed after its keeper changed")
	}
	if _, err := os.Stat(group.Delete.Path); err != nil {
		t.Fatalf("delete candidate was removed: %v", err)
	}
	if _, err := db.GetMediaFile(group.Delete.Path); err != nil {
		t.Fatalf("delete candidate inventory row was removed: %v", err)
	}
}

func addHighConfidenceDuplicatePlanGroup(t *testing.T, db *database.MediaDB, root, title string, blockDelete bool) plans.DuplicateGroup {
	t.Helper()
	if blockDelete && os.Geteuid() == 0 {
		t.Skip("read-only-directory delete failure cannot be simulated as root")
	}

	deleteDir := root
	if blockDelete {
		deleteDir = filepath.Join(root, "readonly-"+title)
		if err := os.Mkdir(deleteDir, 0755); err != nil {
			t.Fatalf("create read-only directory: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(deleteDir, 0755) })
	}

	files := []struct {
		path  string
		data  []byte
		score int
	}{
		{path: filepath.Join(root, title+"-keep.mkv"), data: bytes.Repeat([]byte{'k'}, 100), score: 10},
		{path: filepath.Join(deleteDir, title+"-delete.mkv"), data: bytes.Repeat([]byte{'d'}, 80), score: 0},
	}
	for _, file := range files {
		if err := os.WriteFile(file.path, file.data, 0644); err != nil {
			t.Fatalf("create duplicate media file: %v", err)
		}
		if err := db.UpsertMediaFile(&database.MediaFile{
			Path:            file.path,
			Size:            int64(len(file.data)),
			MediaType:       "movie",
			NormalizedTitle: title,
			Year:            intPtr(2020),
			Resolution:      "1080p",
			QualityScore:    file.score,
		}); err != nil {
			t.Fatalf("insert duplicate media row: %v", err)
		}
	}
	if blockDelete {
		if err := os.Chmod(deleteDir, 0555); err != nil {
			t.Fatalf("make delete directory read-only: %v", err)
		}
	}

	svc := service.NewCleanupService(db)
	analysis, err := svc.AnalyzeDuplicates()
	if err != nil {
		t.Fatalf("analyze duplicate group: %v", err)
	}
	for _, group := range analysis.Groups {
		if group.Title != title {
			continue
		}
		entry, reason := buildSafeDuplicatePlanEntry(group, svc, nil)
		if reason != "" {
			t.Fatalf("expected high-confidence plan group, got %q", reason)
		}
		return entry
	}
	t.Fatalf("duplicate group %q not found", title)
	return plans.DuplicateGroup{}
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

	group := addHighConfidenceDuplicatePlanGroup(t, db, tmpDir, "example", true)

	plan := &plans.DuplicatePlan{
		CreatedAt: time.Now(),
		Command:   "duplicates generate",
		Summary: plans.DuplicateSummary{
			TotalGroups:      1,
			FilesToDelete:    1,
			SpaceReclaimable: group.Delete.Size,
		},
		Plans: []plans.DuplicateGroup{group},
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

	deletedGroup := addHighConfidenceDuplicatePlanGroup(t, db, tmpDir, "deleted", false)
	failedGroup := addHighConfidenceDuplicatePlanGroup(t, db, tmpDir, "failed", true)

	plan := &plans.DuplicatePlan{
		CreatedAt: time.Now(),
		Command:   "duplicates generate",
		Summary: plans.DuplicateSummary{
			TotalGroups:      2,
			FilesToDelete:    2,
			SpaceReclaimable: deletedGroup.Delete.Size + failedGroup.Delete.Size,
		},
		Plans: []plans.DuplicateGroup{deletedGroup, failedGroup},
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
