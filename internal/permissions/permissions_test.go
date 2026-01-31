package permissions

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCanDelete_OwnedByUs(t *testing.T) {
	testFile := filepath.Join(t.TempDir(), "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	canDelete, err := CanDelete(testFile)
	if err != nil {
		t.Errorf("CanDelete failed: %v", err)
	}
	if !canDelete {
		t.Error("Should be able to delete file we own")
	}
}

func TestFixPermissions_MakeDeletable(t *testing.T) {
	testFile := filepath.Join(t.TempDir(), "readonly.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0444); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Make readonly
	if err := os.Chmod(testFile, 0444); err != nil {
		t.Fatalf("Failed to chmod: %v", err)
	}

	// Fix permissions
	if err := FixPermissions(testFile, -1, -1); err != nil {
		t.Errorf("FixPermissions failed: %v", err)
	}

	// Should now be deletable
	canDelete, err := CanDelete(testFile)
	if err != nil {
		t.Errorf("CanDelete failed: %v", err)
	}
	if !canDelete {
		t.Error("File should be deletable after FixPermissions")
	}
}

func TestGetFileOwnership(t *testing.T) {
	testFile := filepath.Join(t.TempDir(), "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	uid, gid, err := GetFileOwnership(testFile)
	if err != nil {
		t.Errorf("GetFileOwnership failed: %v", err)
	}

	// Should return valid UIDs (non-negative)
	if uid < 0 || gid < 0 {
		t.Errorf("Invalid ownership: uid=%d, gid=%d", uid, gid)
	}
}

func TestPermissionError_MessageFormat(t *testing.T) {
	err := NewPermissionError(
		"/path/to/file.mkv",
		"delete",
		fmt.Errorf("permission denied"),
		1000,
		1000,
	)

	msg := err.Error()

	expectedContains := []string{
		"permission denied: cannot delete /path/to/file.mkv: permission denied",
		"\n\nTo fix this, run:\n  sudo chown 1000:1000 /path/to/file.mkv",
	}

	for _, expected := range expectedContains {
		if !strings.Contains(msg, expected) {
			t.Errorf("Error message missing expected text:\nGot: %s\nExpected to contain: %s", msg, expected)
		}
	}
}

func TestPermissionError_ChmodOnly(t *testing.T) {
	err := NewPermissionError(
		"/path/to/file.mkv",
		"delete",
		fmt.Errorf("permission denied"),
		-1,
		-1,
	)

	msg := err.Error()

	if !strings.Contains(msg, "sudo chmod") {
		t.Error("Should not suggest sudo chown when no ownership change needed")
	}
	if !strings.Contains(msg, "chmod 644") {
		t.Error("Should suggest chmod 644 when no ownership change")
	}
}
