package jellyfin

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestLockUnlock(t *testing.T) {
	mgr := NewPlaybackLockManager()
	path := "/media/Movies/The Matrix (1999)/The Matrix (1999).mkv"
	info := PlaybackInfo{UserName: "alice", DeviceName: "Shield", ClientName: "Jellyfin Web", ItemID: "123"}

	mgr.Lock(path, info)

	locked, got := mgr.IsLocked(path)
	if !locked {
		t.Fatalf("expected path to be locked")
	}
	if got == nil || got.UserName != "alice" || got.DeviceName != "Shield" {
		t.Fatalf("unexpected lock info: %+v", got)
	}

	mgr.Unlock(path)
	locked, got = mgr.IsLocked(path)
	if locked || got != nil {
		t.Fatalf("expected path to be unlocked")
	}
}

func TestMultiplePaths(t *testing.T) {
	mgr := NewPlaybackLockManager()
	mgr.Lock("/a.mkv", PlaybackInfo{UserName: "u1"})
	mgr.Lock("/b.mkv", PlaybackInfo{UserName: "u2"})

	if count := mgr.Count(); count != 2 {
		t.Fatalf("expected count 2, got %d", count)
	}

	locks := mgr.GetAllLocks()
	if len(locks) != 2 {
		t.Fatalf("expected 2 locks, got %d", len(locks))
	}
}

func TestUnlockNonexistent(t *testing.T) {
	mgr := NewPlaybackLockManager()
	mgr.Unlock("/does-not-exist.mkv")

	if count := mgr.Count(); count != 0 {
		t.Fatalf("expected count 0, got %d", count)
	}
}

func TestGetAllLocksSnapshot(t *testing.T) {
	mgr := NewPlaybackLockManager()
	mgr.Lock("/a.mkv", PlaybackInfo{UserName: "u1"})

	snapshot := mgr.GetAllLocks()
	snapshot["/b.mkv"] = PlaybackInfo{UserName: "mutated"}
	delete(snapshot, "/a.mkv")

	if count := mgr.Count(); count != 1 {
		t.Fatalf("expected internal map to be immutable from snapshot, got count %d", count)
	}
}

func TestConcurrentLocks(t *testing.T) {
	mgr := NewPlaybackLockManager()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			path := fmt.Sprintf("/p/%d.mkv", i)
			mgr.Lock(path, PlaybackInfo{UserName: "u", StartedAt: time.Now()})
			if locked, _ := mgr.IsLocked(path); !locked {
				t.Errorf("expected path %s to be locked", path)
			}
			mgr.Unlock(path)
		}()
	}

	wg.Wait()

	if count := mgr.Count(); count != 0 {
		t.Fatalf("expected all locks removed, got %d", count)
	}
}
