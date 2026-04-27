package ipc

import (
	"context"
	"testing"
	"time"
)

func TestRegistryStartAndLookup(t *testing.T) {
	reg := NewOpRegistry()
	ctx, cancel := context.WithCancel(context.Background())
	op, err := reg.Start("op-1", "RESCAN", cancel)
	if err != nil {
		t.Fatal(err)
	}
	if op.ID != "op-1" {
		t.Errorf("got id %q", op.ID)
	}
	got, ok := reg.Get("op-1")
	if !ok {
		t.Fatal("not found")
	}
	if got.Cmd != "RESCAN" {
		t.Errorf("cmd %q", got.Cmd)
	}
	_ = ctx
}

func TestRegistryStartConflictsWithMutator(t *testing.T) {
	reg := NewOpRegistry()
	_, _ = reg.Start("a", "RESCAN", func() {})
	if _, err := reg.Start("b", "RESCAN", func() {}); err == nil {
		t.Error("expected conflict error")
	}
}

func TestRegistryStatusAlwaysAllowed(t *testing.T) {
	reg := NewOpRegistry()
	_, _ = reg.Start("a", "RESCAN", func() {})
	if _, err := reg.Start("b", "STATUS", func() {}); err != nil {
		t.Errorf("STATUS should not conflict: %v", err)
	}
}

func TestRegistryFinishCleansUp(t *testing.T) {
	reg := NewOpRegistry()
	op, _ := reg.Start("a", "RESCAN", func() {})
	reg.Finish(op.ID, "done", nil)
	if _, ok := reg.Get(op.ID); ok {
		// We keep it for replay window; assert TTL works.
	}
	got, ok := reg.Get(op.ID)
	if !ok {
		t.Fatal("op vanished before TTL")
	}
	if got.Final == nil || got.Final.State != "done" {
		t.Errorf("final = %+v", got.Final)
	}
	reg.evictBefore(time.Now().Add(time.Hour))
	if _, ok := reg.Get(op.ID); ok {
		t.Error("expected eviction")
	}
}
