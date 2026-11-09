package transfer

import (
	"syscall"
	"testing"
)

func TestUsableFreeBytesCountsReservedPoolForRoot(t *testing.T) {
	stat := syscall.Statfs_t{Bsize: 4096, Bavail: 10, Bfree: 1000}

	if got := usableFreeBytes(stat, false); got != 10*4096 {
		t.Fatalf("unprivileged free = %d, want %d", got, 10*4096)
	}
	if got := usableFreeBytes(stat, true); got != 1000*4096 {
		t.Fatalf("root free = %d, want %d", got, 1000*4096)
	}
}
