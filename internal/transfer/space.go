package transfer

import (
	"os"
	"syscall"
)

// UsableFreeBytes is the space this process can allocate.
// statfs Bavail excludes the filesystem reserved pool. On ext4 that pool
// is about 5% of the disk, so df reports 0 available while root can still
// write it. The daemon runs as root and must count Bfree.
func UsableFreeBytes(stat syscall.Statfs_t) int64 {
	return usableFreeBytes(stat, os.Geteuid() == 0)
}

func usableFreeBytes(stat syscall.Statfs_t, root bool) int64 {
	blocks := stat.Bavail
	if root {
		blocks = stat.Bfree
	}
	return int64(blocks) * int64(stat.Bsize)
}
