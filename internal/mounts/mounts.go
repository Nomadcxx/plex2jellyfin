// Package mounts detects library paths whose configured filesystem is not
// mounted. An unmounted disk leaves an empty mountpoint on the parent
// filesystem, and tools such as Docker recreate missing bind-mount sources
// there, so a readable directory alone does not prove the library is present.
package mounts

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Check returns an error when path lies under an /etc/fstab mount point that
// is not currently mounted. Paths without an fstab entry are not constrained.
func Check(path string) error {
	return check(path, "/etc/fstab", "/proc/self/mountinfo")
}

func check(path, fstabPath, mountinfoPath string) error {
	configured, err := readMountPoints(fstabPath, 1)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", fstabPath, err)
	}

	path = filepath.Clean(path)
	expected := ""
	for _, mp := range configured {
		if within(path, mp) && len(mp) > len(expected) {
			expected = mp
		}
	}
	if expected == "" || expected == "/" {
		return nil
	}

	mounted, err := readMountPoints(mountinfoPath, 4)
	if err != nil {
		return fmt.Errorf("read %s: %w", mountinfoPath, err)
	}
	for _, mp := range mounted {
		if mp == expected {
			return nil
		}
	}
	return fmt.Errorf("%s is configured in %s but not mounted", expected, fstabPath)
}

// readMountPoints returns the absolute mount points found in the given
// whitespace-separated field of a fstab- or mountinfo-format file.
func readMountPoints(path string, field int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var points []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) <= field {
			continue
		}
		mp := unescape(fields[field])
		if filepath.IsAbs(mp) {
			points = append(points, filepath.Clean(mp))
		}
	}
	return points, scanner.Err()
}

func within(path, mountPoint string) bool {
	return path == mountPoint || strings.HasPrefix(path, strings.TrimSuffix(mountPoint, "/")+"/")
}

// unescape decodes the octal escapes (e.g. \040 for a space) used in fstab
// and mountinfo fields.
func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(v))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
