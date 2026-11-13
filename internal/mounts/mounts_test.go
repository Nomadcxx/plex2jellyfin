package mounts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testFstab = `# /etc/fstab
UUID=aaaa  /             ext4  rw  0 1
UUID=bbbb  /mnt/STORAGE1 auto  nosuid,nodev,nofail 0 0
UUID=cccc  /mnt/STORAGE2 ext4  defaults 0 0
UUID=dddd  /mnt/My\040Disk ext4 defaults 0 0
UUID=eeee  none          swap  sw 0 0
`

const testMountinfo = `22 1 259:2 / / rw,relatime shared:1 - ext4 /dev/mapper/root rw
40 22 8:130 / /mnt/STORAGE2 rw,relatime shared:20 - ext4 /dev/sdi1 rw
41 22 8:17 / /mnt/My\040Disk rw,relatime shared:21 - ext4 /dev/sdb1 rw
`

func writeFixtures(t *testing.T, fstab, mountinfo string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	fstabPath := filepath.Join(dir, "fstab")
	mountinfoPath := filepath.Join(dir, "mountinfo")
	if err := os.WriteFile(fstabPath, []byte(fstab), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mountinfoPath, []byte(mountinfo), 0o644); err != nil {
		t.Fatal(err)
	}
	return fstabPath, mountinfoPath
}

func TestCheckReportsPathUnderUnmountedFstabEntry(t *testing.T) {
	fstabPath, mountinfoPath := writeFixtures(t, testFstab, testMountinfo)

	err := check("/mnt/STORAGE1/TVSHOWS", fstabPath, mountinfoPath)
	if err == nil {
		t.Fatal("expected an error for a library under an unmounted fstab entry")
	}
	if !strings.Contains(err.Error(), "/mnt/STORAGE1") {
		t.Fatalf("error %q does not name the mount point", err)
	}
	if err := check("/mnt/STORAGE1", fstabPath, mountinfoPath); err == nil {
		t.Fatal("expected an error for the unmounted mount point itself")
	}
}

func TestCheckAcceptsMountedAndUnconfiguredPaths(t *testing.T) {
	fstabPath, mountinfoPath := writeFixtures(t, testFstab, testMountinfo)

	for _, path := range []string{
		"/mnt/STORAGE2/TVSHOWS",
		"/mnt/My Disk/MOVIES",
		"/mnt/STORAGE10/TVSHOWS", // no fstab entry: lives on /
		"/mnt/STORAGE1X/TVSHOWS", // prefix of an unmounted entry, not under it
		"/home/user/media",
	} {
		if err := check(path, fstabPath, mountinfoPath); err != nil {
			t.Errorf("check(%q) = %v, want nil", path, err)
		}
	}
}

func TestCheckIgnoresMissingFstab(t *testing.T) {
	_, mountinfoPath := writeFixtures(t, testFstab, testMountinfo)

	missing := filepath.Join(t.TempDir(), "no-fstab")
	if err := check("/mnt/STORAGE1/TVSHOWS", missing, mountinfoPath); err != nil {
		t.Fatalf("check without fstab = %v, want nil", err)
	}
}
