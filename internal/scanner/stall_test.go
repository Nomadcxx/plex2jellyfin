package scanner

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunLibraryScanWithGuardHeartbeatAndStall(t *testing.T) {
	ctx := context.Background()
	var heartbeats atomic.Int32

	err := runLibraryScanWithGuard(ctx, 80*time.Millisecond, 20*time.Millisecond,
		func(libCtx context.Context, touch func()) error {
			touch()
			// Never touch again — simulate hung mount after first entry.
			<-libCtx.Done()
			return libCtx.Err()
		},
		func() { heartbeats.Add(1) },
	)
	if !errors.Is(err, ErrLibraryStalled) {
		t.Fatalf("got %v, want ErrLibraryStalled", err)
	}
	if heartbeats.Load() < 1 {
		t.Fatalf("expected at least one heartbeat, got %d", heartbeats.Load())
	}
}

func TestRunLibraryScanWithGuardCompletes(t *testing.T) {
	ctx := context.Background()
	err := runLibraryScanWithGuard(ctx, time.Second, 50*time.Millisecond,
		func(_ context.Context, touch func()) error {
			touch()
			return nil
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunLibraryScanWithGuardParentCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(40 * time.Millisecond)
		cancel()
	}()
	err := runLibraryScanWithGuard(ctx, time.Minute, 20*time.Millisecond,
		func(libCtx context.Context, touch func()) error {
			touch()
			<-libCtx.Done()
			return libCtx.Err()
		},
		nil,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}
