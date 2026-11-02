//go:build linux

package fileidentity

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

func renameNoReplace(source, destination string) error {
	if err := unix.Renameat2(unix.AT_FDCWD, source, unix.AT_FDCWD, destination, unix.RENAME_NOREPLACE); err != nil {
		return renameNoReplaceError(err)
	}
	return nil
}

func renameNoReplaceError(err error) error {
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.EOPNOTSUPP) {
		return fmt.Errorf("%w: %v", ErrRenameNoReplaceUnsupported, err)
	}
	return fmt.Errorf("rename without replacement: %w", err)
}

func exchange(source, destination string) error {
	if err := unix.Renameat2(unix.AT_FDCWD, source, unix.AT_FDCWD, destination, unix.RENAME_EXCHANGE); err != nil {
		if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.EOPNOTSUPP) {
			return fmt.Errorf("%w: %v", ErrExchangeUnsupported, err)
		}
		return fmt.Errorf("exchange %s and %s: %w", source, destination, err)
	}
	return nil
}
