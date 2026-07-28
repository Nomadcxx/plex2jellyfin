package transfer

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestNewTransferer(t *testing.T) {
	tr, err := New(BackendAuto)
	if err != nil {
		t.Fatalf("New(BackendAuto) failed: %v", err)
	}
	if tr == nil {
		t.Fatal("New(BackendAuto) returned nil")
	}
	t.Logf("Using backend: %s", tr.Name())
}

func TestRsyncTransfererInterface(t *testing.T) {
	tr := NewRsyncTransferer("/usr/bin/rsync")
	var _ Transferer = tr
	if tr.Name() != "rsync" {
		t.Errorf("expected name 'rsync', got '%s'", tr.Name())
	}
	if !tr.CanResume() {
		t.Error("rsync should support resume")
	}
}

func TestRsyncMoveDoesNotDeleteSourceBeforeAtomicPublish(t *testing.T) {
	tr := NewRsyncTransferer("/usr/bin/rsync")
	if args := tr.buildArgs(DefaultOptions()); slices.Contains(args, "--remove-source-files") {
		t.Fatalf("rsync move deletes the source before Plex2Jellyfin publishes the final name: %v", args)
	}
	dst := filepath.Join("/library", "Movie (2026)", "Movie (2026).mkv")
	staged := tr.stagingPath("/downloads/release-a/movie.mkv", dst)
	if filepath.Ext(staged) == filepath.Ext(dst) || filepath.Dir(staged) != filepath.Dir(dst) {
		t.Fatalf("staging path %q must be non-media in the destination directory", staged)
	}
	if other := tr.stagingPath("/downloads/release-b/movie.mkv", dst); other == staged {
		t.Fatalf("different sources share staging path %q", staged)
	}
}

func TestNativeTransfererInterface(t *testing.T) {
	tr := NewNativeTransferer(1024 * 1024)
	var _ Transferer = tr
	if tr.Name() != "native" {
		t.Errorf("expected name 'native', got '%s'", tr.Name())
	}
	if tr.CanResume() {
		t.Error("native should not support resume")
	}
}

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()
	if opts.Timeout != 5*time.Minute {
		t.Errorf("expected Timeout 5m, got %v", opts.Timeout)
	}
	if opts.RetryAttempts != 3 {
		t.Errorf("expected RetryAttempts 3, got %d", opts.RetryAttempts)
	}
}

func TestCheckDiskHealth(t *testing.T) {
	health, err := CheckDiskHealth(os.TempDir(), 5*time.Second)
	if err != nil {
		t.Fatalf("CheckDiskHealth failed: %v", err)
	}
	if !health.IsHealthy() {
		t.Errorf("temp dir should be healthy: %+v", health)
	}
}

func TestStatWithTimeout(t *testing.T) {
	info, err := StatWithTimeout(os.TempDir(), 5*time.Second)
	if err != nil {
		t.Fatalf("StatWithTimeout failed: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected temp dir to be a directory")
	}
}

func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.txt")
	dstPath := filepath.Join(tmpDir, "dest.txt")

	testContent := []byte("test content for transfer")
	if err := os.WriteFile(srcPath, testContent, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	tr, err := New(BackendAuto)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	opts := DefaultOptions()
	opts.Timeout = 30 * time.Second

	result, err := tr.Copy(srcPath, dstPath, opts)
	if err != nil {
		t.Fatalf("Copy failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Copy reported failure: %v", result.Error)
	}
	if result.BytesCopied != int64(len(testContent)) {
		t.Errorf("expected %d bytes copied, got %d", len(testContent), result.BytesCopied)
	}

	dstContent, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("failed to read destination: %v", err)
	}
	if string(dstContent) != string(testContent) {
		t.Errorf("content mismatch: expected %q, got %q", testContent, dstContent)
	}
}

func TestMoveFile(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.txt")
	dstPath := filepath.Join(tmpDir, "dest.txt")

	testContent := []byte("test content for move")
	if err := os.WriteFile(srcPath, testContent, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	tr, err := New(BackendAuto)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	opts := DefaultOptions()
	opts.Timeout = 30 * time.Second

	result, err := tr.Move(srcPath, dstPath, opts)
	if err != nil {
		t.Fatalf("Move failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Move reported failure: %v", result.Error)
	}

	if _, err := os.Stat(srcPath); !os.IsNotExist(err) {
		t.Error("source file should be removed after Move")
	}

	dstContent, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("failed to read destination: %v", err)
	}
	if string(dstContent) != string(testContent) {
		t.Errorf("content mismatch: expected %q, got %q", testContent, dstContent)
	}
}

func TestSourceNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "nonexistent.txt")
	dstPath := filepath.Join(tmpDir, "dest.txt")

	tr, err := New(BackendAuto)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	opts := DefaultOptions()
	opts.RetryAttempts = 0

	_, err = tr.Copy(srcPath, dstPath, opts)
	if err == nil {
		t.Error("expected error for nonexistent source")
	}
}
