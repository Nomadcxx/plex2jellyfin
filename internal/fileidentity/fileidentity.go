package fileidentity

import (
	"errors"
	"fmt"
	"os"
	"reflect"
)

var ErrRenameNoReplaceUnsupported = errors.New("atomic no-replace rename is unsupported")
var ErrExchangeUnsupported = errors.New("atomic path exchange is unsupported")

// RenameNoReplace atomically renames source to destination without replacing
// an existing destination path.
func RenameNoReplace(source, destination string) error {
	return renameNoReplace(source, destination)
}

// Exchange atomically swaps two paths on filesystems that support it.
func Exchange(source, destination string) error {
	return exchange(source, destination)
}

// FromInfo returns a stable identity for a file when the platform exposes
// device and inode fields through FileInfo.Sys.
func FromInfo(info os.FileInfo) (string, bool) {
	if info == nil || info.Sys() == nil {
		return "", false
	}

	stat := reflect.ValueOf(info.Sys())
	if stat.Kind() == reflect.Pointer {
		if stat.IsNil() {
			return "", false
		}
		stat = stat.Elem()
	}
	if stat.Kind() != reflect.Struct {
		return "", false
	}
	device, ok := unsignedField(stat.FieldByName("Dev"))
	if !ok {
		return "", false
	}
	inode, ok := unsignedField(stat.FieldByName("Ino"))
	if !ok {
		return "", false
	}
	// ponytail: device/inode plus size/mtime protects against ordinary inode reuse; use native generation IDs if a real collision is observed.
	return fmt.Sprintf("%x:%x:%x:%x", device, inode, uint64(info.Size()), uint64(info.ModTime().UnixNano())), true
}

func MatchesPath(path, identity string) bool {
	current, ok := FromPath(path)
	return ok && identity != "" && current == identity
}

func FromLstat(path string) (string, bool) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", false
	}
	return fromLstatPath(path, info)
}

func MatchesLstat(path, identity string) bool {
	current, ok := FromLstat(path)
	return ok && identity != "" && current == identity
}

func unsignedField(field reflect.Value) (uint64, bool) {
	if !field.IsValid() {
		return 0, false
	}
	switch field.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return field.Uint(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value := field.Int()
		if value < 0 {
			return 0, false
		}
		return uint64(value), true
	default:
		return 0, false
	}
}
