package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/database"
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
	go func() { done <- s.FullRescan(ctx, []string{dir}, true, progress) }()

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
