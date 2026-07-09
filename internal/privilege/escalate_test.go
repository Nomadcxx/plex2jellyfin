package privilege

import (
	"os"
	"testing"
)

func TestNeedsRoot_AsNonRoot(t *testing.T) {
	// Skip if actually running as root
	if os.Geteuid() == 0 {
		t.Skip("Test requires non-root user")
	}

	if !NeedsRoot() {
		t.Error("NeedsRoot() = false, want true when not root")
	}
}

func TestNeedsRoot_TestBypass(t *testing.T) {
	t.Setenv("PLEX2JELLYFIN_TEST_NO_ESCALATE", "1")

	if NeedsRoot() {
		t.Error("NeedsRoot() = true, want false when test bypass is enabled")
	}
}

func TestNeedsRoot_AsRoot(t *testing.T) {
	// This test only makes sense when running as root
	if os.Geteuid() != 0 {
		t.Skip("Test requires root user")
	}

	if NeedsRoot() {
		t.Error("NeedsRoot() = true, want false when root")
	}
}
