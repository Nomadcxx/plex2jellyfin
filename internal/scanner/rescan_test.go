package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
)

func TestFullRescanEmitsProgressAndHonorsCancel(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.mkv"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.mkv"), []byte("y"), 0644); err != nil {
		t.Fatal(err)
	}

	mdb, err := database.OpenPath(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer mdb.Close()
	s := NewFileScanner(mdb)

	progress := make(chan database.ProgressEvent, 64)
	done := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { done <- s.FullRescan(ctx, []RescanRoot{{Path: dir, MediaType: "movie"}}, true, progress) }()

	var saw bool
	for ev := range progress {
		if ev.Phase == "indexing" {
			saw = true
		}
		if ev.Phase == "complete" {
			break
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !saw {
		t.Error("no indexing events emitted")
	}
}

func TestFullRescanReportsIndexErrors(t *testing.T) {
	dir := t.TempDir()
	broken := filepath.Join(dir, "broken.mkv")
	if err := os.Symlink(filepath.Join(dir, "missing-source.mkv"), broken); err != nil {
		t.Fatal(err)
	}

	mdb, err := database.OpenPath(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer mdb.Close()
	s := NewFileScanner(mdb)

	progress := make(chan database.ProgressEvent, 64)
	done := make(chan error, 1)
	go func() { done <- s.FullRescan(context.Background(), []RescanRoot{{Path: dir, MediaType: "movie"}}, false, progress) }()
	for ev := range progress {
		if ev.Phase == "complete" {
			break
		}
	}
	if err := <-done; err == nil {
		t.Fatal("expected full rescan to report indexing error")
	}
}

// TestFullRescanTypedRootPreventsObfuscatedTVAsMovie reproduces the bogus
// "movie" conflicts: an obfuscated TV episode file (no SxxExx in filename)
// under a TV root must be indexed as episode, not movie.
func TestFullRescanTypedRootPreventsObfuscatedTVAsMovie(t *testing.T) {
	tvRoot := t.TempDir()
	seriesDir := filepath.Join(tvRoot, "Some Show (2020)", "Season 01")
	if err := os.MkdirAll(seriesDir, 0755); err != nil {
		t.Fatal(err)
	}
	obfuscated := filepath.Join(seriesDir, "1eec0cfe1a2645608b09c8b99ccf2960.mkv")
	if err := os.WriteFile(obfuscated, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	mdb, err := database.OpenPath(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer mdb.Close()
	s := NewFileScanner(mdb)

	progress := make(chan database.ProgressEvent, 64)
	if err := s.FullRescan(context.Background(), []RescanRoot{{Path: tvRoot, MediaType: "episode"}}, false, progress); err != nil {
		t.Fatal(err)
	}
	for ev := range progress {
		if ev.Phase == "complete" {
			break
		}
	}

	files, err := mdb.GetMediaFilesByLibrary(tvRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.Path != obfuscated {
			continue
		}
		if f.MediaType != "episode" {
			t.Errorf("obfuscated TV file under TV root: media_type=%q want episode (path=%s)", f.MediaType, f.Path)
		}
		return
	}
	t.Error("obfuscated TV file not indexed at all")
}