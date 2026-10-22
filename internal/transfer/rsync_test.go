package transfer

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func TestRsyncCopyReportsVerifiedChecksum(t *testing.T) {
	rsPath, err := exec.LookPath("rsync")
	if err != nil {
		t.Skip("rsync is not installed")
	}

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
	result, err := NewRsyncTransferer(rsPath).Copy(src, dst, opts)
	if err != nil {
		t.Fatalf("Copy: %v", err)
	}
	want := sha256.Sum256(content)
	if result.Checksum != hex.EncodeToString(want[:]) {
		t.Fatalf("checksum = %q, want %x", result.Checksum, want)
	}
}

func TestBuildArgsAddsNoTimesByDefault(t *testing.T) {
	r := &RsyncTransferer{}
	args := r.buildArgs(TransferOptions{})
	if !slices.Contains(args, "--no-times") {
		t.Fatalf("expected --no-times in default args, got %v", args)
	}
}

func TestBuildArgsOmitsNoTimesWhenPreserveTimes(t *testing.T) {
	r := &RsyncTransferer{}
	args := r.buildArgs(TransferOptions{PreserveTimes: true})
	if slices.Contains(args, "--no-times") {
		t.Fatalf("expected no --no-times when PreserveTimes=true, got %v", args)
	}
}
