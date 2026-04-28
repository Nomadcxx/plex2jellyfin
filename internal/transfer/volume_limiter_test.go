package transfer

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type countingTransferer struct {
	mu      sync.Mutex
	active  int32
	maxSeen int32
	delay   time.Duration
}

func (c *countingTransferer) Name() string    { return "counting" }
func (c *countingTransferer) CanResume() bool { return false }

func (c *countingTransferer) trace() {
	n := atomic.AddInt32(&c.active, 1)
	c.mu.Lock()
	if n > c.maxSeen {
		c.maxSeen = n
	}
	c.mu.Unlock()
	time.Sleep(c.delay)
	atomic.AddInt32(&c.active, -1)
}

func (c *countingTransferer) Move(_, _ string, _ TransferOptions) (*TransferResult, error) {
	c.trace()
	return &TransferResult{Success: true}, nil
}
func (c *countingTransferer) Copy(_, _ string, _ TransferOptions) (*TransferResult, error) {
	c.trace()
	return &TransferResult{Success: true}, nil
}

func TestVolumeLimiter_CapsConcurrencyPerVolume(t *testing.T) {
	limiter := NewVolumeLimiter(2)
	inner := &countingTransferer{delay: 50 * time.Millisecond}
	wrapped := NewVolumeLimitedTransferer(inner, limiter)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = wrapped.Move("/tmp/src", "/tmp/dst", TransferOptions{SkipHealthCheck: true})
		}()
	}
	wg.Wait()

	if inner.maxSeen > 2 {
		t.Fatalf("expected max concurrent <=2, observed %d", inner.maxSeen)
	}
	if inner.maxSeen == 0 {
		t.Fatal("expected at least one transfer to run")
	}
}

func TestVolumeLimiter_ZeroCapDisables(t *testing.T) {
	limiter := NewVolumeLimiter(0)
	inner := &countingTransferer{delay: 20 * time.Millisecond}
	wrapped := NewVolumeLimitedTransferer(inner, limiter)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = wrapped.Move("/tmp/src", "/tmp/dst", TransferOptions{SkipHealthCheck: true})
		}()
	}
	wg.Wait()

	if inner.maxSeen < 2 {
		t.Fatalf("expected unlimited concurrency to allow >=2, saw %d", inner.maxSeen)
	}
}

func TestVolumeLimiter_MountRootCachesResolution(t *testing.T) {
	v := NewVolumeLimiter(1)
	root1 := v.mountRoot("/tmp")
	root2 := v.mountRoot("/tmp")
	if root1 != root2 {
		t.Fatalf("expected stable mount root, got %q vs %q", root1, root2)
	}
}
