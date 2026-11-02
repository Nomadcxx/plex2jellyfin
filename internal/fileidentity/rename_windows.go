//go:build windows

package fileidentity

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func renameNoReplace(source, destination string) error {
	from, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return fmt.Errorf("encode quarantine source path: %w", err)
	}
	to, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return fmt.Errorf("encode quarantine destination path: %w", err)
	}
	if err := windows.MoveFileEx(from, to, windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return fmt.Errorf("rename without replacement: %w", err)
	}
	return nil
}

func exchange(source, destination string) error {
	return fmt.Errorf("%w on this platform: %s <-> %s", ErrExchangeUnsupported, source, destination)
}
