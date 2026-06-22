package transfer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNativeCopyFailurePreservesExistingDestination(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "source-dir.mkv")
	if err := os.Mkdir(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "movie.mkv")
	if err := os.WriteFile(dst, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}

	n := NewNativeTransferer(1024)
	if _, err := n.Copy(srcDir, dst, TransferOptions{RetryAttempts: 0}); err == nil {
		t.Fatal("expected copy from directory to fail")
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original" {
		t.Fatalf("destination was modified after failed copy: %q", string(got))
	}
}
