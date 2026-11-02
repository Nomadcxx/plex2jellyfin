//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package fileidentity

import "os"

func FromPath(path string) (string, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	return FromInfo(info)
}

func fromLstatPath(_ string, info os.FileInfo) (string, bool) {
	return FromInfo(info)
}
