package config

import (
	"os"
	"path/filepath"
	"sync"
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

func looksLikeOneWriter(s string) bool {
	for i := 0; i < 16; i++ {
		if s == "writer"+string(rune('a'+i)) {
			return true
		}
	}
	return false
}
