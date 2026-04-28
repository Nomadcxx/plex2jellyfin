package transfer

import (
	"path/filepath"
	"sync"
	"syscall"
)

// VolumeLimiter caps the number of concurrent transfers per destination
// volume (mount point). Heavy concurrent rsync to the same disk causes
// I/O contention that triggered the false-positive "destination disk
// unhealthy" cascade in the original health check, and produces long
// no-progress timeouts on legitimately large transfers.
//
// The limiter detects the mount root by walking up parent directories
// until the device number changes (standard Linux mount detection).
// Each mount root gets its own buffered semaphore.
type VolumeLimiter struct {
	cap int

	mu    sync.Mutex
	sems  map[string]chan struct{}
	roots map[string]string // path -> resolved mount root (cache)
}

// NewVolumeLimiter returns a limiter that allows up to `concurrent` parallel
// transfers per destination mount point. A value <= 0 disables limiting.
func NewVolumeLimiter(concurrent int) *VolumeLimiter {
	return &VolumeLimiter{
		cap:   concurrent,
		sems:  make(map[string]chan struct{}),
		roots: make(map[string]string),
	}
}

// Acquire blocks until a slot is available for the volume containing dst,
// then returns a release function. Callers must invoke release exactly once
// (typically `defer release()`). When the limiter is disabled, release is a
// no-op and Acquire returns immediately.
func (v *VolumeLimiter) Acquire(dst string) func() {
	if v == nil || v.cap <= 0 {
		return func() {}
	}
	root := v.mountRoot(dst)
	sem := v.semFor(root)
	sem <- struct{}{}
	return func() { <-sem }
}

func (v *VolumeLimiter) semFor(root string) chan struct{} {
	v.mu.Lock()
	defer v.mu.Unlock()
	if s, ok := v.sems[root]; ok {
		return s
	}
	s := make(chan struct{}, v.cap)
	v.sems[root] = s
	return s
}

// mountRoot walks up from `path` until it finds the directory whose parent
// resides on a different filesystem (different syscall.Stat_t.Dev). On any
// stat error or when reaching "/", it returns the deepest path inspected.
// Results are cached per input path.
func (v *VolumeLimiter) mountRoot(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	v.mu.Lock()
	if cached, ok := v.roots[abs]; ok {
		v.mu.Unlock()
		return cached
	}
	v.mu.Unlock()

	cur := abs
	curDev, ok := devOf(cur)
	if !ok {
		// stat failed: try parent until we find an existing dir.
		cur = filepath.Dir(cur)
		curDev, ok = devOf(cur)
		if !ok {
			v.cacheRoot(abs, cur)
			return cur
		}
	}
	for {
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		parentDev, ok := devOf(parent)
		if !ok || parentDev != curDev {
			break
		}
		cur = parent
	}
	v.cacheRoot(abs, cur)
	return cur
}

func (v *VolumeLimiter) cacheRoot(key, root string) {
	v.mu.Lock()
	v.roots[key] = root
	v.mu.Unlock()
}

func devOf(path string) (uint64, bool) {
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		return 0, false
	}
	return uint64(st.Dev), true
}

// VolumeLimitedTransferer wraps a Transferer and enforces a per-destination
// volume concurrency cap on Move/Copy operations. The underlying transferer
// is invoked unchanged once a slot is acquired.
type VolumeLimitedTransferer struct {
	inner   Transferer
	limiter *VolumeLimiter
}

// NewVolumeLimitedTransferer wraps `inner` with the supplied limiter. If
// limiter is nil or its cap is <= 0, this is effectively a passthrough.
func NewVolumeLimitedTransferer(inner Transferer, limiter *VolumeLimiter) *VolumeLimitedTransferer {
	return &VolumeLimitedTransferer{inner: inner, limiter: limiter}
}

func (t *VolumeLimitedTransferer) Name() string {
	return "volume-limited(" + t.inner.Name() + ")"
}

func (t *VolumeLimitedTransferer) CanResume() bool {
	return t.inner.CanResume()
}

func (t *VolumeLimitedTransferer) Move(src, dst string, opts TransferOptions) (*TransferResult, error) {
	release := t.limiter.Acquire(dst)
	defer release()
	return t.inner.Move(src, dst, opts)
}

func (t *VolumeLimitedTransferer) Copy(src, dst string, opts TransferOptions) (*TransferResult, error) {
	release := t.limiter.Acquire(dst)
	defer release()
	return t.inner.Copy(src, dst, opts)
}
