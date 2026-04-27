package watcher

import (
	"os"
	"path/filepath"
	"testing"
)

type noopHandler struct{}

func (noopHandler) HandleFileEvent(FileEvent) error { return nil }
func (noopHandler) IsMediaFile(string) bool         { return false }

func TestReplaceWatchPathsRemovesOldWatchesAndAddsNewOnes(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old")
	newPath := filepath.Join(dir, "new")
	t.Cleanup(func() {})

	if err := os.MkdirAll(oldPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newPath, 0o755); err != nil {
		t.Fatal(err)
	}

	w, err := NewWatcher(noopHandler{}, false, WithRecursive(false))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	if err := w.Watch([]string{oldPath}); err != nil {
		t.Fatal(err)
	}
	if err := w.ReplaceWatchPaths([]string{newPath}); err != nil {
		t.Fatal(err)
	}

	watches := w.fsWatcher.WatchList()
	if len(watches) != 1 {
		t.Fatalf("watch count = %d, want 1 (%v)", len(watches), watches)
	}
	if watches[0] != newPath {
		t.Fatalf("watched path = %q, want %q", watches[0], newPath)
	}
}
