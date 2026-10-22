package wizard

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/service"
)

func TestDuplicatePromptDoesNotOfferUnimplementedSwap(t *testing.T) {
	w := &Wizard{reader: bufio.NewReader(strings.NewReader("y\nskip\n"))}
	analysis := &service.DuplicateAnalysis{Groups: []service.DuplicateGroup{{
		Title:      "Example",
		BestFileID: 1,
		Files:      []service.MediaFile{{ID: 1}, {ID: 2}},
	}}}

	oldStdout := os.Stdout
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	os.Stdout = stdoutW
	handleErr := w.handleDuplicates(analysis)
	_ = stdoutW.Close()
	os.Stdout = oldStdout
	output, err := io.ReadAll(stdoutR)
	_ = stdoutR.Close()
	if err != nil {
		t.Fatalf("read captured output: %v", err)
	}
	if handleErr != nil {
		t.Fatalf("handleDuplicates: %v", handleErr)
	}
	if strings.Contains(string(output), "[S]wap") || strings.Contains(string(output), "Swap not implemented") {
		t.Fatalf("duplicate prompt still offers the unsupported swap action:\n%s", output)
	}
}

func TestHandleDuplicatesReportsUnreconciledDatabaseFailures(t *testing.T) {
	for _, action := range []string{"k", "a"} {
		t.Run(action, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "test.db")
			db, err := database.OpenPath(dbPath)
			if err != nil {
				t.Fatalf("open database: %v", err)
			}

			path := filepath.Join(t.TempDir(), "duplicate.mkv")
			if err := os.WriteFile(path, []byte("video"), 0644); err != nil {
				t.Fatalf("create duplicate: %v", err)
			}
			file := &database.MediaFile{Path: path, MediaType: "movie", Size: 5}
			if err := db.UpsertMediaFile(file); err != nil {
				t.Fatalf("insert media file: %v", err)
			}
			if err := db.Close(); err != nil {
				t.Fatalf("close database: %v", err)
			}

			w := &Wizard{
				db:      db,
				service: service.NewCleanupService(db),
				reader:  bufio.NewReader(strings.NewReader("y\n" + action + "\n")),
			}
			analysis := &service.DuplicateAnalysis{Groups: []service.DuplicateGroup{{
				Title:      "Example",
				BestFileID: file.ID + 1,
				Files:      []service.MediaFile{{ID: file.ID, Path: path, Size: file.Size}},
			}}}

			oldStdout := os.Stdout
			stdoutR, stdoutW, err := os.Pipe()
			if err != nil {
				t.Fatalf("create stdout pipe: %v", err)
			}
			os.Stdout = stdoutW
			handleErr := w.handleDuplicates(analysis)
			_ = stdoutW.Close()
			os.Stdout = oldStdout
			output, err := io.ReadAll(stdoutR)
			_ = stdoutR.Close()
			if err != nil {
				t.Fatalf("read captured output: %v", err)
			}

			if handleErr == nil {
				t.Fatal("expected database cleanup and reconciliation failure")
			}
			if !strings.Contains(handleErr.Error(), path) || !strings.Contains(handleErr.Error(), "reconcil") {
				t.Fatalf("error should identify the path and failed reconciliation, got %v", handleErr)
			}
			if strings.Contains(string(output), "✅ Deleted:") || strings.Contains(string(output), "Deleted 1 files") {
				t.Fatalf("unreconciled file was reported as successfully deleted:\n%s", output)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("file should be removed before database failure, stat err=%v", err)
			}

			checkDB, err := database.OpenPath(dbPath)
			if err != nil {
				t.Fatalf("reopen database: %v", err)
			}
			defer checkDB.Close()
			remaining, err := checkDB.GetMediaFile(path)
			if err != nil {
				t.Fatalf("read media file row: %v", err)
			}
			if remaining == nil {
				t.Fatal("expected stale row to remain for a later scan")
			}
		})
	}
}
