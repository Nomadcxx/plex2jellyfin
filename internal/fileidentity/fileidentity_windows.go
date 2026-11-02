//go:build windows

package fileidentity

import (
	"fmt"
	"os"
	"syscall"
)

func FromPath(path string) (string, bool) {
	return fileIdentity(path, 0)
}

func fromLstatPath(path string, _ os.FileInfo) (string, bool) {
	return fileIdentity(path, syscall.FILE_FLAG_OPEN_REPARSE_POINT)
}

func fileIdentity(path string, extraFlags uint32) (string, bool) {
	flags := uint32(syscall.FILE_FLAG_BACKUP_SEMANTICS) | extraFlags
	handle, err := syscall.CreateFile(syscall.StringToUTF16Ptr(path), 0, 0, nil, syscall.OPEN_EXISTING, flags, 0)
	if err != nil {
		return "", false
	}
	defer syscall.CloseHandle(handle)
	var stat syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(handle, &stat); err != nil {
		return "", false
	}
	if extraFlags == 0 && stat.FileAttributes&syscall.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return "", false
	}
	inode := uint64(stat.FileIndexHigh)<<32 | uint64(stat.FileIndexLow)
	size := uint64(stat.FileSizeHigh)<<32 | uint64(stat.FileSizeLow)
	return fmt.Sprintf("%x:%x:%x:%x", stat.VolumeSerialNumber, inode, size, stat.LastWriteTime.Nanoseconds()), true
}
