package transfer

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPVImplementation(t *testing.T) {
	var _ Transferer = &PVTransferer{}
}

func TestPVCopyReportsVerifiedChecksum(t *testing.T) {
	pvPath, err := exec.LookPath("pv")
	if err != nil {
		t.Skip("pv is not installed")
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "source.mkv")
	dst := filepath.Join(dir, "library", "movie.mkv")
	content := []byte("checksum this transfer")
	require.NoError(t, os.WriteFile(src, content, 0644))

	opts := DefaultOptions()
	opts.Checksum = true
	opts.SkipHealthCheck = true
	opts.RetryAttempts = 0
	result, err := NewPVTransferer(pvPath).Copy(src, dst, opts)
	require.NoError(t, err)
	want := sha256.Sum256(content)
	assert.Equal(t, hex.EncodeToString(want[:]), result.Checksum)
}

func TestPVChecksumMismatchPreservesExistingDestination(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.mkv")
	dst := filepath.Join(dir, "library", "movie.mkv")
	fakePV := filepath.Join(dir, "pv")
	require.NoError(t, os.WriteFile(src, []byte("original"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0755))
	require.NoError(t, os.WriteFile(dst, []byte("keep old"), 0644))
	require.NoError(t, os.WriteFile(fakePV, []byte("#!/bin/sh\nprintf modified\n"), 0755))

	opts := DefaultOptions()
	opts.Checksum = true
	opts.DeletePartial = true
	opts.SkipHealthCheck = true
	opts.RetryAttempts = 0
	_, err := NewPVTransferer(fakePV).Copy(src, dst, opts)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrChecksumMismatch), "error should preserve checksum mismatch")

	data, readErr := os.ReadFile(dst)
	require.NoError(t, readErr)
	assert.Equal(t, "keep old", string(data), "checksum failure must preserve the existing destination")
}

func TestPVDetection(t *testing.T) {
	transferer, err := New(BackendPV)
	if err != nil {
		t.Logf("PV not available: %v", err)
		return
	}

	if transferer.Name() != "pv" {
		t.Errorf("Expected backend name 'pv', got '%s'", transferer.Name())
	}
}

func TestPV_PreservesExistingFileOnFailure(t *testing.T) {
	falsePath, err := exec.LookPath("false")
	require.NoError(t, err)
	transferer := NewPVTransferer(falsePath)

	srcDir := t.TempDir()
	dstDir := t.TempDir()

	srcFile := filepath.Join(srcDir, "source.mkv")
	require.NoError(t, os.WriteFile(srcFile, []byte("new content"), 0644))

	existingFile := filepath.Join(dstDir, "target.mkv")
	require.NoError(t, os.WriteFile(existingFile, []byte("original content"), 0644))

	_, err = transferer.Copy(srcFile, existingFile, TransferOptions{})
	require.Error(t, err, "should fail when pv exits unsuccessfully")

	data, readErr := os.ReadFile(existingFile)
	require.NoError(t, readErr)
	assert.Equal(t, "original content", string(data),
		"existing file content must be preserved when transfer fails")
}

func TestBackendString(t *testing.T) {
	if BackendPV.String() != "pv" {
		t.Errorf("Expected 'pv', got '%s'", BackendPV.String())
	}

	if BackendRsync.String() != "rsync" {
		t.Errorf("Expected 'rsync', got '%s'", BackendRsync.String())
	}

	if BackendNative.String() != "native" {
		t.Errorf("Expected 'native', got '%s'", BackendNative.String())
	}

	if BackendAuto.String() != "auto" {
		t.Errorf("Expected 'auto', got '%s'", BackendAuto.String())
	}
}
