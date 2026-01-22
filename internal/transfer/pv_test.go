package transfer

import (
	"testing"
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
