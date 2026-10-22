package transfer

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyChecksumRejectsDifferentFiles(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source")
	dst := filepath.Join(dir, "destination")
	if err := os.WriteFile(src, []byte("source"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("corrupt"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := verifyChecksum(src, dst); !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("verifyChecksum error = %v, want %v", err, ErrChecksumMismatch)
	}
}
