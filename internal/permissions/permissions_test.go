package permissions

import (
	"os"
	"path/filepath"
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
