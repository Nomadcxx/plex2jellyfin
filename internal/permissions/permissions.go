package permissions

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// CanDelete checks if current process has permission to delete a file
func CanDelete(path string) (bool, error) {
	// Check if file exists
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	// Check if we have write permission on parent directory
	dir := filepath.Dir(path)
	dirInfo, err := os.Stat(dir)
	if err != nil {
		return false, err
	}

	// Get directory permissions
	dirMode := dirInfo.Mode().Perm()

	// Check if directory is writable
	if dirMode&0200 == 0 {
		return false, nil
	}

	// Check if file is writable (needed on some systems)
	fileMode := info.Mode().Perm()
	if fileMode&0200 == 0 {
		return false, nil
	}

	return true, nil
}

func FixPermissions(path string, uid, gid int) error {
	if err := os.Chmod(path, 0644); err != nil {
		return fmt.Errorf("failed to chmod: %w", err)
	}

	if uid >= 0 || gid >= 0 {
		currentUID, currentGID, err := GetFileOwnership(path)
		if err != nil {
			return fmt.Errorf("failed to get current ownership: %w", err)
		}

		if uid < 0 {
			uid = currentUID
		}
		if gid < 0 {
			gid = currentGID
		}

		if err := os.Chown(path, uid, gid); err != nil {
			return fmt.Errorf("failed to chown (may need sudo): %w", err)
		}
	}

	return nil
}

// GetFileOwnership returns UID and GID of a file
func GetFileOwnership(path string) (int, int, error) {
	info, err := os.Stat(path)
	if err != nil {
		return -1, -1, err
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return -1, -1, fmt.Errorf("failed to get file stat")
	}

	return int(stat.Uid), int(stat.Gid), nil
}

// NeedsOwnershipChange checks if file ownership differs from target
func NeedsOwnershipChange(path string, targetUID, targetGID int) (bool, error) {
	if targetUID < 0 && targetGID < 0 {
		return false, nil // No target ownership specified
	}

	currentUID, currentGID, err := GetFileOwnership(path)
	if err != nil {
		return false, err
	}

	if targetUID >= 0 && currentUID != targetUID {
		return true, nil
	}
	if targetGID >= 0 && currentGID != targetGID {
		return true, nil
	}

	return false, nil
}

// PermissionError represents a permission-related error with helpful guidance
type PermissionError struct {
	Path    string
	Op      string
	Err     error
	NeedUID int
	NeedGID int
}

func (e *PermissionError) Error() string {
	msg := fmt.Sprintf("permission denied: cannot %s %s: %v", e.Op, e.Path, e.Err)

	if e.NeedUID >= 0 || e.NeedGID >= 0 {
		msg += fmt.Sprintf("\n\nTo fix this, run:\n  sudo chown")
		if e.NeedUID >= 0 && e.NeedGID >= 0 {
			msg += fmt.Sprintf(" %d:%d", e.NeedUID, e.NeedGID)
		} else if e.NeedUID >= 0 {
			msg += fmt.Sprintf(" %d", e.NeedUID)
		} else {
			msg += fmt.Sprintf(" :%d", e.NeedGID)
		}
		msg += fmt.Sprintf(" %s", e.Path)
	} else {
		msg += fmt.Sprintf("\n\nTo fix this, run:\n  sudo chmod 644 %s", e.Path)
	}

	return msg
}

// NewPermissionError creates a helpful permission error
func NewPermissionError(path, op string, err error, uid, gid int) error {
	return &PermissionError{
		Path:    path,
		Op:      op,
		Err:     err,
		NeedUID: uid,
		NeedGID: gid,
	}
}
