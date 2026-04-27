package logging

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeFile writes s to path, failing the test on any error.
func writeFile(t *testing.T, path, s string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(s), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestRotateFiles_NoCompress_ShiftAndEvict(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "app.log")

	writeFile(t, base, "current")
	writeFile(t, filepath.Join(dir, "app.1.log"), "one")
	writeFile(t, filepath.Join(dir, "app.2.log"), "two")

	if err := rotateFiles(base, 2, false); err != nil {
		t.Fatalf("rotateFiles: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "app.3.log")); !os.IsNotExist(err) {
		t.Fatalf("expected app.3.log evicted, stat err=%v", err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "app.1.log")); err != nil || string(got) != "current" {
		t.Fatalf("app.1.log wrong: got=%q err=%v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "app.2.log")); err != nil || string(got) != "one" {
		t.Fatalf("app.2.log wrong: got=%q err=%v", got, err)
	}
}

func TestRotateFiles_Compress_CreatesGz(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "app.log")
	writeFile(t, base, "hello world")

	if err := rotateFiles(base, 5, true); err != nil {
		t.Fatalf("rotateFiles: %v", err)
	}

	gzPath := filepath.Join(dir, "app.1.log.gz")
	if _, err := os.Stat(gzPath); err != nil {
		t.Fatalf("expected app.1.log.gz, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "app.1.log")); !os.IsNotExist(err) {
		t.Fatalf("expected plaintext app.1.log removed after compression")
	}

	// Decompress and verify content round-trips.
	f, err := os.Open(gzPath)
	if err != nil {
		t.Fatalf("open gz: %v", err)
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer gr.Close()
	content, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("read gz: %v", err)
	}
	if string(content) != "hello world" {
		t.Fatalf("decompressed content mismatch: got %q", content)
	}
}

func TestRotateFiles_ShiftsGzBackups(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "app.log")
	writeFile(t, base, "current")
	// Existing gz backup from a previous rotation.
	writeFile(t, filepath.Join(dir, "app.1.log.gz"), "fakegz")

	if err := rotateFiles(base, 5, true); err != nil {
		t.Fatalf("rotateFiles: %v", err)
	}

	// The old gz should have shifted to .2, and the new current should be
	// compressed at .1.
	if _, err := os.Stat(filepath.Join(dir, "app.2.log.gz")); err != nil {
		t.Fatalf("expected app.2.log.gz after shift, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "app.1.log.gz")); err != nil {
		t.Fatalf("expected new app.1.log.gz, got err=%v", err)
	}
}

func TestPruneOldBackups_RemovesStale(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "app.log")

	old := filepath.Join(dir, "app.1.log")
	fresh := filepath.Join(dir, "app.2.log")
	writeFile(t, old, "old")
	writeFile(t, fresh, "fresh")

	// Backdate old to 10 days ago.
	past := time.Now().Add(-10 * 24 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	if err := pruneOldBackups(base, 7); err != nil {
		t.Fatalf("pruneOldBackups: %v", err)
	}

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("expected old backup removed, stat err=%v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("fresh backup should survive, err=%v", err)
	}
}

func TestPruneOldBackups_ZeroDisabled(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "app.log")
	old := filepath.Join(dir, "app.1.log")
	writeFile(t, old, "old")

	past := time.Now().Add(-365 * 24 * time.Hour)
	os.Chtimes(old, past, past)

	if err := pruneOldBackups(base, 0); err != nil {
		t.Fatalf("pruneOldBackups(0): %v", err)
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatalf("zero MaxAgeDays should disable pruning, but backup was removed: %v", err)
	}
}

func TestRotateFiles_EvictsCompressedPastLimit(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "app.log")
	writeFile(t, base, "current")
	writeFile(t, filepath.Join(dir, "app.1.log.gz"), "a")
	writeFile(t, filepath.Join(dir, "app.2.log.gz"), "b")

	// maxBackups=2 means anything shifting to .3+ is evicted.
	if err := rotateFiles(base, 2, true); err != nil {
		t.Fatalf("rotateFiles: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	for _, n := range names {
		if strings.Contains(n, ".3.") {
			t.Fatalf("expected no .3 backup after eviction, got %v", names)
		}
	}
}
