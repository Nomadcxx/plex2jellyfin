//go:build !(aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris || windows)

package fileidentity

import "os"

func FromPath(string) (string, bool) {
	return "", false
}

func fromLstatPath(string, os.FileInfo) (string, bool) {
	return "", false
}
