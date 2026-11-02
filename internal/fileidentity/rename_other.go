//go:build !(linux || darwin || windows)

package fileidentity

import (
	"fmt"
)

func renameNoReplace(source, destination string) error {
	return fmt.Errorf("%w on this platform: %s -> %s", ErrRenameNoReplaceUnsupported, source, destination)
}

func exchange(source, destination string) error {
	return fmt.Errorf("%w on this platform: %s <-> %s", ErrExchangeUnsupported, source, destination)
}
