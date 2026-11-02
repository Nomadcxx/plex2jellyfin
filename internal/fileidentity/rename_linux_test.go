//go:build linux

package fileidentity

import (
	"errors"
	"testing"

	"golang.org/x/sys/unix"
)

func TestRenameNoReplaceMapsUnsupportedKernelAndFilesystemErrors(t *testing.T) {
	for _, err := range []error{unix.ENOSYS, unix.EINVAL, unix.EOPNOTSUPP} {
		if got := renameNoReplaceError(err); !errors.Is(got, ErrRenameNoReplaceUnsupported) {
			t.Errorf("renameNoReplaceError(%v) = %v, want unsupported sentinel", err, got)
		}
	}
	if got := renameNoReplaceError(unix.EEXIST); errors.Is(got, ErrRenameNoReplaceUnsupported) {
		t.Fatalf("destination-exists error was classified as unsupported: %v", got)
	}
}
