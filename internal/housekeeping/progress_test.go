package housekeeping

import (
	"errors"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/daemon/ipc"
)

func TestTaskProgressFinishMarksRegistryOpDone(t *testing.T) {
	reg := ipc.NewOpRegistry()
	engine := &Engine{}
	engine.SetOpRegistry(reg)

	progress := engine.startTaskOp(42, "/src/Upload", "/dst/Upload (2020)", 2, 2048)
	progress.finish(nil)

	ops := reg.List()
	if len(ops) != 1 {
		t.Fatalf("expected one op, got %d", len(ops))
	}
	if ops[0].State != "done" {
		t.Fatalf("expected op state done, got %q", ops[0].State)
	}
}

func TestTaskProgressFinishMarksRegistryOpError(t *testing.T) {
	reg := ipc.NewOpRegistry()
	engine := &Engine{}
	engine.SetOpRegistry(reg)

	progress := engine.startTaskOp(43, "/src/Upload", "/dst/Upload (2020)", 2, 2048)
	progress.finish(errors.New("size mismatch"))

	ops := reg.List()
	if len(ops) != 1 {
		t.Fatalf("expected one op, got %d", len(ops))
	}
	if ops[0].State != "error" {
		t.Fatalf("expected op state error, got %q", ops[0].State)
	}
	if ops[0].FinalMsg != "size mismatch" {
		t.Fatalf("expected final error message, got %q", ops[0].FinalMsg)
	}
}
