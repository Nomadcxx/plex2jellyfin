package transfer

import (
	"fmt"
	"os"
)

// ApplyPermissions sets file mode and ownership based on TransferOptions.
// Requires root privileges for chown operations.
func ApplyPermissions(path string, opts TransferOptions) error {
	if opts.FileMode != 0 {
		if err := os.Chmod(path, opts.FileMode); err != nil {
			return fmt.Errorf("chmod failed: %w", err)
		}
	}

	if opts.TargetUID >= 0 || opts.TargetGID >= 0 {
		uid := opts.TargetUID
		gid := opts.TargetGID
		if uid < 0 {
			uid = -1
		}
		if gid < 0 {
			gid = -1
		}
		if err := os.Chown(path, uid, gid); err != nil {
			if os.Geteuid() != 0 {
				return fmt.Errorf("chown failed (not running as root): target uid=%d gid=%d, current euid=%d: %w",
					uid, gid, os.Geteuid(), err)
			}
			return fmt.Errorf("chown failed: %w", err)
		}
	}

	return nil
}
