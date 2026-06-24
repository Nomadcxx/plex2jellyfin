package transfer

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPVImplementation(t *testing.T) {
	var _ Transferer = &PVTransferer{}
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
	transferer, err := New(BackendPV)
	if err != nil {
		t.Skipf("PV not available: %v", err)
	}

	srcDir := t.TempDir()
	dstDir := t.TempDir()

	srcFile := filepath.Join(srcDir, "source.mkv")
	require.NoError(t, os.WriteFile(srcFile, make([]byte, 10*1024*1024), 0644))

	existingFile := filepath.Join(dstDir, "target.mkv")
	require.NoError(t, os.WriteFile(existingFile, []byte("original content"), 0644))

	_, err = transferer.Copy(srcFile, existingFile, TransferOptions{Timeout: 1 * time.Millisecond})
	require.Error(t, err, "should fail due to timeout")

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
