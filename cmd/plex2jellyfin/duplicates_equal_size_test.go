package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/plans"
)

func TestRunDuplicatesGenerateKeepsEqualSizeDifferentContentForReview(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("SUDO_USER", "")

	db, err := database.Open()
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

	contents := [][]byte{bytes.Repeat([]byte{'a'}, 128), bytes.Repeat([]byte{'b'}, 128)}
	if bytes.Equal(contents[0], contents[1]) {
		t.Fatal("test files must contain different bytes")
	}
	year := 2020
	for i, data := range contents {
		path := filepath.Join(root, fmt.Sprintf("copy-%d.mkv", i))
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("create media file: %v", err)
		}
		if err := db.UpsertMediaFile(&database.MediaFile{
			Path:            path,
			Size:            int64(len(data)),
			MediaType:       "movie",
			NormalizedTitle: "example movie",
			Year:            &year,
			Resolution:      "1080p",
			QualityScore:    50,
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
	if plan != nil && len(plan.Plans) != 0 {
		t.Fatalf("equal-size different-content files entered automatic deletion plan: %#v", plan.Plans)
	}
}
