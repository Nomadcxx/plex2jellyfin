//go:build !linux

package transfer

import (
	"fmt"
	"os"
)

func publishNoReplace(stagedPath, destinationPath string) error {
	if err := os.Link(stagedPath, destinationPath); err != nil {
		return fmt.Errorf("publish without replacing destination: %w", err)
	}
	// ponytail: leave a hidden stage link if unlink fails; final publication is safe.
	_ = os.Remove(stagedPath)
	return nil
}
