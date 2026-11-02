//go:build darwin

package fileidentity

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func renameNoReplace(source, destination string) error {
	if err := unix.RenamexNp(source, destination, unix.RENAME_EXCL); err != nil {
		return fmt.Errorf("rename without replacement: %w", err)
	}
	return nil
}

func exchange(source, destination string) error {
	if err := unix.RenamexNp(source, destination, unix.RENAME_SWAP); err != nil {
		return fmt.Errorf("exchange %s and %s: %w", source, destination, err)
	}
	return nil
}
