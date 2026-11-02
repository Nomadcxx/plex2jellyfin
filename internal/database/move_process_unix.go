//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package database

import (
	"errors"
	"syscall"
)

func processIsRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	// ponytail: PID reuse can delay recovery; persist process start ticks if this is observed.
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
