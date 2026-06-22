package config

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"testing"
)

func TestAtomicWriteWithLockWritesWholeFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := AtomicWriteWithLock(path, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Errorf("got %q", string(got))
	}
}

func TestAtomicWriteWithLockSerializesParallelWriters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			content := []byte("writer" + string(rune('a'+i)))
			_ = AtomicWriteWithLock(path, content, 0600)
		}()
	}
	wg.Wait()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !looksLikeOneWriter(string(got)) {
		t.Errorf("got partial/garbled content %q", string(got))
	}
}

func TestAtomicWriteWithLockPreservesExistingOwnership(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root to verify ownership preservation")
	}

	u, err := user.Lookup("nomadx")
	if err != nil {
		t.Skip("nomadx user not available")
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		t.Fatal(err)
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(path, uid, gid); err != nil {
		t.Fatal(err)
	}

	if err := AtomicWriteWithLock(path, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	st := info.Sys().(*syscall.Stat_t)
	if int(st.Uid) != uid || int(st.Gid) != gid {
		t.Fatalf("ownership changed to %d:%d, want %d:%d", st.Uid, st.Gid, uid, gid)
	}
}

func looksLikeOneWriter(s string) bool {
	for i := 0; i < 16; i++ {
		if s == "writer"+string(rune('a'+i)) {
			return true
		}
	}
	return false
}
