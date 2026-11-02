package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
)

func TestRepairZeroByteMediaCommandDefaultsToDryRun(t *testing.T) {
	root := t.TempDir()
	db, err := database.OpenPath(filepath.Join(root, "media.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer db.Close()

	path := filepath.Join(root, "empty.mkv")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("create empty file: %v", err)
	}
	file := &database.MediaFile{Path: path, Size: 0, MediaType: "movie", NormalizedTitle: "empty film", Year: intPtr(2024)}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("UpsertMediaFile: %v", err)
	}

	var output bytes.Buffer
	if err := runRepairZeroByteMediaWithDB(db, false, &output, 10); err != nil {
		t.Fatalf("dry-run command: %v", err)
	}
	if !strings.Contains(output.String(), "DRY RUN") || !strings.Contains(output.String(), path) {
		t.Fatalf("dry-run output = %q, want mode and candidate path", output.String())
	}
	stored, err := db.GetMediaFile(path)
	if err != nil || stored == nil || stored.NeedsReview {
		t.Fatalf("dry run changed database row: file=%+v err=%v", stored, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("dry run changed source file: %v", err)
	}

	output.Reset()
	if err := runRepairZeroByteMediaWithDB(db, true, &output, 10); err != nil {
		t.Fatalf("execute command: %v", err)
	}
	if !strings.Contains(output.String(), "EXECUTED") {
		t.Fatalf("execute output = %q, want executed mode", output.String())
	}
	stored, err = db.GetMediaFile(path)
	if err != nil || stored == nil || !stored.NeedsReview {
		t.Fatalf("execute did not mark retained row for review: file=%+v err=%v", stored, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("execute removed source file: %v", err)
	}
}
