//go:build !(aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris)

package database

func processIsRunning(pid int) bool {
	// ponytail: without portable signal-0 checks, retain positive-PID reservations; add a platform process probe to auto-recover them.
	return pid > 0
}
