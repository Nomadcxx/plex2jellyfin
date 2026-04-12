package transfer

import (
	"fmt"
	"log"
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
				log.Printf("[permissions] warning: chown skipped for %s (not running as root, euid=%d): target uid=%d gid=%d",
					path, os.Geteuid(), uid, gid)
				return nil
			}
			return fmt.Errorf("chown failed: %w", err)
		}
	}

	return nil
}
