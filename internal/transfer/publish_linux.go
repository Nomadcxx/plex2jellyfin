//go:build linux

package transfer

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func publishNoReplace(stagedPath, destinationPath string) error {
	err := unix.Renameat2(unix.AT_FDCWD, stagedPath, unix.AT_FDCWD, destinationPath, unix.RENAME_NOREPLACE)
	if err == nil {
		return nil
	}
	if !errors.Is(err, unix.ENOSYS) && !errors.Is(err, unix.EINVAL) && !errors.Is(err, unix.EOPNOTSUPP) {
		return fmt.Errorf("publish without replacing destination: %w", err)
	}
	if err := os.Link(stagedPath, destinationPath); err != nil {
		return fmt.Errorf("publish without replacing destination: %w", err)
	}
	// ponytail: leave a hidden stage link if unlink fails; final publication is safe.
	_ = os.Remove(stagedPath)
	return nil
}
