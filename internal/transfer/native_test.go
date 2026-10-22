package transfer

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeCopyReportsVerifiedChecksum(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.mkv")
	dst := filepath.Join(dir, "library", "movie.mkv")
	content := []byte("checksum this transfer")
	if err := os.WriteFile(src, content, 0644); err != nil {
		t.Fatal(err)
	}

	opts := DefaultOptions()
	opts.Checksum = true
	opts.SkipHealthCheck = true
	opts.RetryAttempts = 0
	result, err := NewNativeTransferer(1024).Copy(src, dst, opts)
	if err != nil {
		t.Fatalf("Copy: %v", err)
	}
	want := sha256.Sum256(content)
	if result.Checksum != hex.EncodeToString(want[:]) {
		t.Fatalf("checksum = %q, want %x", result.Checksum, want)
	}
}

func TestNativeChecksumMismatchPreservesExistingDestination(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.mkv")
	dst := filepath.Join(dir, "library", "movie.mkv")
	if err := os.WriteFile(src, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("keep old"), 0644); err != nil {
		t.Fatal(err)
	}

	opts := DefaultOptions()
	opts.Checksum = true
	opts.DeletePartial = true
	opts.SkipHealthCheck = true
	opts.RetryAttempts = 0
	opts.Progress = func(_, _ int64) {
		if err := os.WriteFile(src, []byte("modified"), 0644); err != nil {
			t.Errorf("modify source during transfer: %v", err)
		}
	}
	_, err := NewNativeTransferer(1024).Copy(src, dst, opts)
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("Copy error = %v, want %v", err, ErrChecksumMismatch)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("existing destination was not preserved: %v", err)
	}
	if string(got) != "keep old" {
		t.Fatalf("existing destination = %q, want %q", got, "keep old")
	}
}

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

func TestNativeMoveKeepsSourceWhenPermissionsFail(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.mkv")
	dst := filepath.Join(dir, "library", "movie.mkv")
	content := []byte("source must survive")
	if err := os.WriteFile(src, content, 0644); err != nil {
		t.Fatal(err)
	}

	opts := DefaultOptions()
	opts.SkipHealthCheck = true
	opts.RetryAttempts = 0
	opts.FileMode = 0644
	opts.Progress = func(_, _ int64) {
		entries, err := os.ReadDir(filepath.Dir(dst))
		if err != nil {
			t.Errorf("read destination directory: %v", err)
			return
		}
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), "."+filepath.Base(dst)+".tmp-") {
				continue
			}
			tempPath := filepath.Join(filepath.Dir(dst), entry.Name())
			if err := os.Remove(tempPath); err != nil {
				t.Errorf("remove temporary destination: %v", err)
				return
			}
			if err := os.Symlink(filepath.Join(dir, "missing-target"), tempPath); err != nil {
				t.Errorf("replace temporary destination with broken symlink: %v", err)
			}
			return
		}
		t.Error("temporary destination not found")
	}

	result, err := NewNativeTransferer(1024).Move(src, dst, opts)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Move error = %v, want permissions error wrapping %v", err, os.ErrNotExist)
	}
	if result == nil || result.Success || result.SourceRemoved {
		t.Fatalf("Move result = %+v, want unsuccessful result with source retained", result)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source was not preserved: %v", err)
	}
}
