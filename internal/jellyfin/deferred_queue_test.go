package jellyfin

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestAddAndGet(t *testing.T) {
	q := NewDeferredQueue()
	path := "/m/file.mkv"
	op := DeferredOp{Type: "organize_movie", SourcePath: path, DeferredAt: time.Now()}

	q.Add(path, op)

	ops := q.GetForPath(path)
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	if ops[0].Type != "organize_movie" {
		t.Fatalf("unexpected op type: %s", ops[0].Type)
	}
}

func TestRemoveForPath(t *testing.T) {
	q := NewDeferredQueue()
	path := "/m/file.mkv"
	q.Add(path, DeferredOp{Type: "organize_movie", SourcePath: path, DeferredAt: time.Now()})

	removed := q.RemoveForPath(path)
	if len(removed) != 1 {
		t.Fatalf("expected 1 removed op, got %d", len(removed))
	}

	if left := q.GetForPath(path); len(left) != 0 {
		t.Fatalf("expected no ops left, got %d", len(left))
	}
}

func TestMultipleOpsPerPath(t *testing.T) {
	q := NewDeferredQueue()
	path := "/m/file.mkv"
	q.Add(path, DeferredOp{Type: "organize_movie", SourcePath: path, DeferredAt: time.Now()})
	q.Add(path, DeferredOp{Type: "delete_duplicate", SourcePath: path, DeferredAt: time.Now()})

	ops := q.GetForPath(path)
	if len(ops) != 2 {
		t.Fatalf("expected 2 ops, got %d", len(ops))
	}
}

func TestCount(t *testing.T) {
	q := NewDeferredQueue()
	q.Add("/a.mkv", DeferredOp{Type: "organize_movie", SourcePath: "/a.mkv", DeferredAt: time.Now()})
	q.Add("/a.mkv", DeferredOp{Type: "delete_duplicate", SourcePath: "/a.mkv", DeferredAt: time.Now()})
	q.Add("/b.mkv", DeferredOp{Type: "organize_tv", SourcePath: "/b.mkv", DeferredAt: time.Now()})

	if got := q.Count(); got != 3 {
		t.Fatalf("expected count 3, got %d", got)
	}

	q.RemoveForPath("/a.mkv")
	if got := q.Count(); got != 1 {
		t.Fatalf("expected count 1 after remove, got %d", got)
	}
}

func TestConcurrentAccess(t *testing.T) {
	q := NewDeferredQueue()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			path := fmt.Sprintf("/p/%d.mkv", i%5)
			q.Add(path, DeferredOp{Type: "organize_movie", SourcePath: path, DeferredAt: time.Now()})
			_ = q.GetForPath(path)
		}()
	}

	wg.Wait()
	if q.Count() == 0 {
		t.Fatalf("expected deferred ops to be added")
	}
}
