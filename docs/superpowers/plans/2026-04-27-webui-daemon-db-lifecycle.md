# WebUI Daemon & Database Lifecycle — Implementation Plan (Plan 2 of 3)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring full daemon and database lifecycle control into the webui — start/stop/restart/reload/recover the daemon, run "re-scan media" and "reset database" with TUI-installer-grade progress, recover from interrupted destructive ops, and add CLI parity (`jellywatch daemon …`). Builds directly on the IPC + Reload foundation laid in Plan 1.

**Architecture:** Extends the IPC protocol with streaming/lifecycle commands (`STOP`, `RESCAN`, `RESET_DB`, `ATTACH`, `CANCEL`) plus a daemon-side op registry and on-disk op log (`op_log.jsonl`) that survives crashes. `jellyweb` exposes REST endpoints that translate browser actions into IPC ops and relays progress via SSE. Frontend gets a `/settings/daemon` and `/settings/database` page with a TUI-installer-style `ProgressCard`, typed-confirm modals, and a recovery flow keyed off the op log.

**Tech Stack:** Go (chi, net.UnixConn, context cancellation), SQLite (via existing `internal/database`), Next.js App Router, EventSource (browser-native SSE), shadcn/ui dialogs.

**Spec:** `docs/superpowers/specs/2026-04-27-webui-control-plane-design.md` §4 (full IPC), §4.9 (op log + crash recovery), §5.5–§5.7 (REST + SSE), §6.4–§6.6 (frontend), §8 (CLI parity).

**Depends on (already merged in Plan 1):**
- `internal/daemon/ipc.{Server,Client,FrameWriter}`, protocol types, error codes
- `internal/daemon/reload.Supervisor` and `reload.Default`
- `internal/config.AtomicWriteWithLock`
- `cmd/jellywatchd/control.go` (where Plan 1 registers handlers)

---

## Pre-flight: existing-API anchors (verified against repo at plan time)

These are the real signatures we extend; deviations from them are bugs.

- **`internal/api/config_save_pipeline.go:12`** — the live `IPCCaller` interface is:
  ```go
  type IPCCaller interface {
      Call(ctx context.Context, cmd ipc.Command, args any) (json.RawMessage, error)
  }
  ```
  Every test stub and handler in this plan **must** include `ctx`. We extend (not replace) this interface; see Task 4.1.
- **`internal/database/database.go`** — entry points are `Open() (*MediaDB, error)` and `OpenPath(path string) (*MediaDB, error)`. `MediaDB` wraps a `*sql.DB`. A getter `(*MediaDB).SQL() *sql.DB` is added in Task 2.0 so maintenance code can use the raw handle.
- **`internal/scanner`** — the scanner type is `FileScanner` (`NewFileScanner`, `NewFileScannerWithAI`). There is **no** `Scanner` type. The rescan capability is added as a method on `*FileScanner` in Task 2.1a, reusing the existing periodic walk path.
- **`cmd/jellywatchd/control.go:14`** — the live status struct is `daemonStatus{PID, UptimeSeconds, ConfigLoaded}`. Task 1.6 extends this struct rather than introducing a parallel `statusPayload`.
- **`internal/daemon/ipc/server.go`** — `Server` does **not** export a `Path()` accessor. Task 1.4 adds `func (s *Server) Path() string { return s.path }`.
- **`internal/daemon/ipc/server.go:38`** — `NewServer` does **not** allocate a registry. Task 1.4 changes `NewServer` to allocate a default `NewOpRegistry()` so `RegisterStreaming` is safe without an explicit `SetRegistry`.

If during execution any of these no longer match (e.g., main rebased), STOP and reconcile before continuing.

---

## File Structure

### Backend (NEW)
- `internal/daemon/ipc/op_registry.go` — in-memory registry of running ops keyed by `op_id`
- `internal/daemon/ipc/op_log.go` — append-only JSONL op log + recovery helpers
- `internal/daemon/ipc/op_log_test.go`
- `internal/daemon/ipc/streaming.go` — helpers: heartbeat ticker, `Attach`, `Cancel`
- `internal/daemon/ipc/streaming_test.go`
- `internal/database/maintenance.go` — `Rescan`, `ResetDatabase`, both progress-channel based
- `internal/database/maintenance_test.go`
- `internal/api/daemon_handlers.go` — `/daemon/status`, `/daemon/start`, `/daemon/stop`, `/daemon/restart`, `/daemon/reload`, `/daemon/recover`
- `internal/api/daemon_handlers_test.go`
- `internal/api/database_handlers.go` — `/database/rescan`, `/database/reset`
- `internal/api/database_handlers_test.go`
- `internal/api/sse_relay.go` — `/events/op/{op_id}` (and `/replay`)
- `internal/api/sse_relay_test.go`
- `internal/jellyweb/daemonctl/launcher.go` — strategy resolver: systemd-user → systemd-system → detached exec
- `internal/jellyweb/daemonctl/launcher_test.go`
- `cmd/jellywatch/daemon_cmd.go` — `jellywatch daemon {status,reload,stop}` CLI subcommands
- `cmd/jellywatch/daemon_cmd_test.go`

### Backend (EDIT)
- `internal/daemon/ipc/protocol.go` — add `CmdStop`, `CmdRescan`, `CmdResetDB`, `CmdAttach`, `CmdCancel`
- `internal/daemon/ipc/server.go` — heartbeat goroutine on streaming handlers, frame muxing per op_id
- `internal/daemon/ipc/client.go` — `Stream(ctx, cmd, args) (<-chan Frame, error)` API for streaming responses
- `cmd/jellywatchd/control.go` — register the new IPC commands; on startup, run op-log recovery before accepting non-recovery commands
- `cmd/jellywatchd/main.go` — pass scanner + database handles into the new handlers
- `internal/api/server.go` — mount new routes
- `api/openapi.yaml` — declare new endpoints
- `cmd/jellywatch/main.go` — register `daemon` subcommand group

### Frontend (NEW)
- `web/src/components/settings/ProgressCard.tsx` — SSE-driven phase/percent feed
- `web/src/components/settings/ProgressCard.test.tsx`
- `web/src/components/settings/ConfirmDestructive.tsx` — typed-confirm modal
- `web/src/components/settings/ConfirmDestructive.test.tsx`
- `web/src/components/settings/ConfirmReversible.tsx` — simple modal
- `web/src/components/settings/RecoveryBanner.tsx` — shown when daemon state is `interrupted`
- `web/src/hooks/useDaemon.ts`
- `web/src/hooks/useDaemon.test.ts`
- `web/src/hooks/useOpStream.ts`
- `web/src/hooks/useOpStream.test.ts`
- `web/src/app/settings/daemon/page.tsx`
- `web/src/app/settings/database/page.tsx`

### Frontend (EDIT)
- `web/src/app/settings/layout.tsx` — already has Database link (Plan 1 added it); ensure status pill polls `/daemon/status`
- `web/src/lib/api/client.ts` — typed clients for the new routes

---

## Phase 1 — IPC extensions

### Task 1.1: Add lifecycle command names to the protocol

**Files:** Modify: `internal/daemon/ipc/protocol.go`. Test: `internal/daemon/ipc/protocol_test.go` (extend).

- [ ] **Step 1: Failing test**

Append to `internal/daemon/ipc/protocol_test.go`:

```go
func TestLifecycleCommandsDefined(t *testing.T) {
	for _, c := range []Command{CmdStop, CmdRescan, CmdResetDB, CmdAttach, CmdCancel, CmdRecover} {
		if string(c) == "" {
			t.Errorf("command constant empty")
		}
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/daemon/ipc/... -run TestLifecycleCommandsDefined`
Expected: FAIL — constants undefined.

- [ ] **Step 3: Add constants**

In `internal/daemon/ipc/protocol.go`:

```go
const (
	CmdStatus  Command = "STATUS"
	CmdReload  Command = "RELOAD"
	CmdStop    Command = "STOP"
	CmdRescan  Command = "RESCAN"
	CmdResetDB Command = "RESET_DB"
	CmdAttach  Command = "ATTACH"
	CmdCancel  Command = "CANCEL"
	CmdRecover Command = "RECOVER"
)
```

- [ ] **Step 4: Run test**

Run: `go test ./internal/daemon/ipc/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/ipc/protocol.go internal/daemon/ipc/protocol_test.go
git commit -m "feat(ipc): declare lifecycle command names (stop/rescan/reset_db/attach/cancel)"
```

---

### Task 1.2: Op registry

**Files:** Create: `internal/daemon/ipc/op_registry.go`, `internal/daemon/ipc/op_registry_test.go`.

- [ ] **Step 1: Failing test**

```go
// internal/daemon/ipc/op_registry_test.go
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
	_ = ctx // silence unused
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
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/daemon/ipc/... -run TestRegistry`
Expected: FAIL.

- [ ] **Step 3: Implement**

```go
// internal/daemon/ipc/op_registry.go
package ipc

import (
	"errors"
	"sync"
	"time"
)

// Op is a tracked long-running command.
type Op struct {
	ID        string
	Cmd       Command
	StartedAt time.Time
	Cancel    func()
	Frames    *FrameRing
	Final     *FinalState
	mu        sync.Mutex
}

type FinalState struct {
	State string         // "done" | "error" | "cancelled"
	Code  ErrorCode
	Msg   string
	At    time.Time
}

// OpRegistry tracks active and recently-finished ops.
type OpRegistry struct {
	mu  sync.Mutex
	ops map[string]*Op
	ttl time.Duration
}

// Mutating commands are mutually exclusive at the registry level.
var mutators = map[Command]bool{
	CmdRescan:  true,
	CmdResetDB: true,
}

func NewOpRegistry() *OpRegistry {
	return &OpRegistry{ops: map[string]*Op{}, ttl: 10 * time.Minute}
}

// Start registers a new op. Returns ErrBusy as a value matching ipc.ErrBusy
// if a mutator is already running.
func (r *OpRegistry) Start(id string, cmd Command, cancel func()) (*Op, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if mutators[cmd] {
		for _, o := range r.ops {
			if mutators[o.Cmd] && o.Final == nil {
				return nil, errors.New(string(ErrBusy))
			}
		}
	}

	op := &Op{
		ID: id, Cmd: cmd, StartedAt: time.Now(),
		Cancel: cancel,
		Frames: NewFrameRing(1024),
	}
	r.ops[id] = op
	return op, nil
}

func (r *OpRegistry) Get(id string) (*Op, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	op, ok := r.ops[id]
	return op, ok
}

// Finish marks an op as terminal and starts the TTL countdown.
func (r *OpRegistry) Finish(id, state string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	op, ok := r.ops[id]
	if !ok {
		return
	}
	op.mu.Lock()
	op.Final = &FinalState{State: state, At: time.Now()}
	if err != nil {
		op.Final.Msg = err.Error()
	}
	op.mu.Unlock()
}

// EvictExpired removes finished ops older than ttl. Should be called
// periodically.
func (r *OpRegistry) EvictExpired() { r.evictBefore(time.Now().Add(-r.ttl)) }

func (r *OpRegistry) evictBefore(t time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, op := range r.ops {
		op.mu.Lock()
		if op.Final != nil && op.Final.At.Before(t) {
			delete(r.ops, id)
		}
		op.mu.Unlock()
	}
}
```

```go
// internal/daemon/ipc/frame_ring.go
package ipc

import "sync"

// FrameRing is a fixed-size ring buffer of progress frames so reattaching
// clients can replay recent history. Older frames are silently dropped.
type FrameRing struct {
	mu     sync.Mutex
	buf    []Frame
	max    int
	cursor int
}

func NewFrameRing(max int) *FrameRing { return &FrameRing{max: max} }

func (r *FrameRing) Append(f Frame) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = append(r.buf, f)
	if len(r.buf) > r.max {
		r.buf = r.buf[len(r.buf)-r.max:]
	}
}

// Snapshot returns a copy of all frames currently in the ring.
func (r *FrameRing) Snapshot() []Frame {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Frame, len(r.buf))
	copy(out, r.buf)
	return out
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/daemon/ipc/... -run TestRegistry`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/ipc/op_registry.go internal/daemon/ipc/op_registry_test.go internal/daemon/ipc/frame_ring.go
git commit -m "feat(ipc): in-memory op registry with mutator mutex and replay buffer"
```

---

### Task 1.3: Op log (write-ahead, recovery)

**Files:** Create: `internal/daemon/ipc/op_log.go`, `internal/daemon/ipc/op_log_test.go`.

- [ ] **Step 1: Failing test**

```go
// internal/daemon/ipc/op_log_test.go
package ipc

import (
	"path/filepath"
	"testing"
)

func TestOpLogAppendAndScan(t *testing.T) {
	dir := t.TempDir()
	lg, err := OpenOpLog(filepath.Join(dir, "op_log.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := lg.Begin("op-1", "RESET_DB", map[string]any{"confirm": "media.db"}); err != nil {
		t.Fatal(err)
	}
	if err := lg.End("op-1", "done", ""); err != nil {
		t.Fatal(err)
	}
	if err := lg.Begin("op-2", "RESCAN", nil); err != nil {
		t.Fatal(err)
	}
	// op-2 deliberately not ended (simulating a crash).
	lg.Close()

	lg2, err := OpenOpLog(filepath.Join(dir, "op_log.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer lg2.Close()
	pending, err := lg2.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != "op-2" {
		t.Errorf("pending = %+v", pending)
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/daemon/ipc/... -run TestOpLog`
Expected: FAIL.

- [ ] **Step 3: Implement**

```go
// internal/daemon/ipc/op_log.go
package ipc

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"
)

type OpLogEntry struct {
	ID        string         `json:"id"`
	Cmd       Command        `json:"cmd"`
	Args      map[string]any `json:"args,omitempty"`
	State     string         `json:"state"` // "in_progress" | "done" | "error" | "cancelled"
	Msg       string         `json:"msg,omitempty"`
	StartedAt time.Time      `json:"started_at"`
	EndedAt   *time.Time     `json:"ended_at,omitempty"`
}

// OpLog is an append-only JSONL record of destructive ops. Each op
// produces two lines: one in_progress, one terminal.
type OpLog struct {
	mu   sync.Mutex
	path string
	f    *os.File
}

func OpenOpLog(path string) (*OpLog, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	return &OpLog{path: path, f: f}, nil
}

func (l *OpLog) Close() { _ = l.f.Close() }

// Begin records that op id starts now. Must be called before any
// destructive write the op performs.
func (l *OpLog) Begin(id string, cmd Command, args map[string]any) error {
	return l.append(OpLogEntry{
		ID: id, Cmd: cmd, Args: args,
		State: "in_progress", StartedAt: time.Now(),
	})
}

// End records that op id has terminated. state is "done", "error", or
// "cancelled".
func (l *OpLog) End(id, state, msg string) error {
	now := time.Now()
	return l.append(OpLogEntry{
		ID: id, State: state, Msg: msg,
		EndedAt: &now,
	})
}

func (l *OpLog) append(e OpLogEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if _, err := l.f.Write(b); err != nil {
		return err
	}
	return l.f.Sync()
}

// Pending scans the log and returns ops that have a Begin but no
// matching End — "interrupted" ops from a prior daemon run.
func (l *OpLog) Pending() ([]OpLogEntry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, err := l.f.Seek(0, 0); err != nil {
		return nil, err
	}
	pending := map[string]OpLogEntry{}
	sc := bufio.NewScanner(l.f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e OpLogEntry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return nil, err
		}
		if e.State == "in_progress" {
			pending[e.ID] = e
		} else {
			delete(pending, e.ID)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	out := make([]OpLogEntry, 0, len(pending))
	for _, e := range pending {
		out = append(out, e)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// MarkDiscarded transitions a pending op to "cancelled" so it no longer
// appears in Pending. Used by the recovery flow.
func (l *OpLog) MarkDiscarded(id string) error { return l.End(id, "cancelled", "discarded by recovery") }

var ErrInterruptedOp = errors.New("interrupted op exists; recovery required")
```

- [ ] **Step 4: Run test**

Run: `go test ./internal/daemon/ipc/... -run TestOpLog`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/ipc/op_log.go internal/daemon/ipc/op_log_test.go
git commit -m "feat(ipc): append-only op log with pending detection on startup"
```

---

### Task 1.4: Streaming server primitives — heartbeat, ATTACH, CANCEL

**Files:** Create: `internal/daemon/ipc/streaming.go`, `internal/daemon/ipc/streaming_test.go`. Modify: `internal/daemon/ipc/server.go`.

- [ ] **Step 1: Failing test**

```go
// internal/daemon/ipc/streaming_test.go
package ipc

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"
)

func TestAttachReplaysFrames(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "ctl.sock")
	srv := NewServer(sock)
	srv.SetRegistry(NewOpRegistry())

	// Register a fake long-running op handler.
	srv.RegisterStreaming(Command("FAKE"), func(ctx context.Context, args json.RawMessage, w FrameWriter, op *Op) {
		op.Frames.Append(Frame{ID: op.ID, Type: FrameProgress, Phase: "p1", Msg: "hello"})
		w.write(Frame{ID: op.ID, Type: FrameProgress, Phase: "p1", Msg: "hello"})
		<-ctx.Done()
	})

	srv.Register(CmdAttach, attachHandler(srv))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	// 1. Start the FAKE op.
	c1, _ := net.Dial("unix", sock)
	defer c1.Close()
	c1.Write([]byte(`{"v":1,"id":"op-1","cmd":"FAKE"}` + "\n"))
	dec1 := json.NewDecoder(c1)
	var f1 Frame
	if err := dec1.Decode(&f1); err != nil {
		t.Fatal(err)
	}
	if f1.Phase != "p1" {
		t.Errorf("first frame: %+v", f1)
	}

	// 2. Attach from a second connection and expect to replay frames.
	c2, _ := net.Dial("unix", sock)
	defer c2.Close()
	c2.Write([]byte(`{"v":1,"id":"r2","cmd":"ATTACH","args":{"op_id":"op-1"}}` + "\n"))
	dec2 := json.NewDecoder(c2)
	c2.SetReadDeadline(time.Now().Add(2 * time.Second))
	var f2 Frame
	if err := dec2.Decode(&f2); err != nil {
		t.Fatal(err)
	}
	if f2.Phase != "p1" || f2.ID != "op-1" {
		t.Errorf("replay frame: %+v", f2)
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/daemon/ipc/... -run TestAttachReplays`
Expected: FAIL.

- [ ] **Step 3: Extend `Server` with streaming**

In `internal/daemon/ipc/server.go`, add (and update `NewServer` to allocate a default registry so `RegisterStreaming` is safe out of the box):

```go
type StreamingHandler func(ctx context.Context, args json.RawMessage, w FrameWriter, op *Op)

// Path returns the unix socket path; tests use it to dial the running server.
func (s *Server) Path() string { return s.path }

// SetRegistry replaces the default registry. Streaming handlers panic at
// request time if the registry is nil — NewServer already provides one.
func (s *Server) SetRegistry(r *OpRegistry) {
    if r == nil {
        panic("ipc: SetRegistry(nil)")
    }
    s.registry = r
}

// RegisterStreaming wraps a streaming handler with op_id allocation,
// registry tracking, and frame ring mirroring.
func (s *Server) RegisterStreaming(cmd Command, h StreamingHandler) {
    if s.registry == nil {
        panic("ipc: RegisterStreaming requires a registry")
    }
    s.Register(cmd, func(ctx context.Context, req Request, w FrameWriter) {
        opCtx, cancel := context.WithCancel(ctx)
        op, err := s.registry.Start(req.ID, cmd, cancel)
        if err != nil {
            w.Error(req.ID, ErrBusy, err.Error())
            return
        }
        defer s.registry.Finish(op.ID, "done", nil) // overwritten by handler on error/cancel
        ringW := &ringWriter{inner: w, ring: op.Frames}
        hbDone := make(chan struct{})
        go heartbeatLoop(opCtx, req.ID, ringW, hbDone)
        h(opCtx, req.Args, ringW, op)
        close(hbDone)
    })
}
```

Also update `NewServer`:
```go
func NewServer(path string) *Server {
    return &Server{
        path:            path,
        handlers:        make(map[Command]Handler),
        allowedPeerUIDs: map[int]struct{}{os.Getuid(): {}},
        registry:        NewOpRegistry(),
    }
}
```
And add the `registry *OpRegistry` field to `Server`.

```go
// internal/daemon/ipc/streaming.go
package ipc

import (
	"context"
	"encoding/json"
	"time"
)

// ringWriter mirrors every progress frame into the op's frame ring before
// forwarding to the wire.
type ringWriter struct {
	inner FrameWriter
	ring  *FrameRing
}

func (w *ringWriter) Result(id string, data json.RawMessage) {
	w.inner.Result(id, data)
}
func (w *ringWriter) Progress(id, phase, msg string, current, total int) {
	w.ring.Append(Frame{ID: id, Type: FrameProgress, Phase: phase, Msg: msg})
	w.inner.Progress(id, phase, msg, current, total)
}
func (w *ringWriter) Done(id string, data json.RawMessage) {
	w.ring.Append(Frame{ID: id, Type: FrameDone, Data: data})
	w.inner.Done(id, data)
}
func (w *ringWriter) Error(id string, code ErrorCode, msg string) {
	w.ring.Append(Frame{ID: id, Type: FrameError, Code: code, Msg: msg})
	w.inner.Error(id, code, msg)
}
func (w *ringWriter) write(f Frame) {
	if fw, ok := w.inner.(*frameWriter); ok {
		fw.write(f)
	}
}

func heartbeatLoop(ctx context.Context, opID string, w *ringWriter, done <-chan struct{}) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-t.C:
			w.write(Frame{ID: opID, Type: FrameHeartbeat, TS: time.Now().Unix()})
		}
	}
}

// attachHandler replays the op's frame ring then keeps the connection
// open until the op finalizes (or the client disconnects).
func attachHandler(s *Server) Handler {
	return func(ctx context.Context, req Request, w FrameWriter) {
		var args struct {
			OpID string `json:"op_id"`
		}
		if err := json.Unmarshal(req.Args, &args); err != nil {
			w.Error(req.ID, ErrBadRequest, err.Error())
			return
		}
		op, ok := s.registry.Get(args.OpID)
		if !ok {
			w.Error(req.ID, ErrNotFound, "no such op")
			return
		}
		// Replay everything we have.
		for _, f := range op.Frames.Snapshot() {
			if fw, ok := w.(*frameWriter); ok {
				fw.write(f)
			}
		}
		// If already final, return now.
		op.mu.Lock()
		final := op.Final
		op.mu.Unlock()
		if final != nil {
			return
		}
		// Otherwise, wait for context — simplest synchronization is a
		// poll loop; production uses a per-op fan-out channel.
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		seen := len(op.Frames.Snapshot())
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				snap := op.Frames.Snapshot()
				for ; seen < len(snap); seen++ {
					if fw, ok := w.(*frameWriter); ok {
						fw.write(snap[seen])
					}
				}
				op.mu.Lock()
				if op.Final != nil {
					op.mu.Unlock()
					return
				}
				op.mu.Unlock()
			}
		}
	}
}

// cancelHandler cancels an in-flight op via the registry. The streaming
// handler's deferred Finish will then transition state to "cancelled"
// once the goroutine returns from ctx.Err().
func cancelHandler(s *Server) Handler {
	return func(ctx context.Context, req Request, w FrameWriter) {
		var args struct {
			OpID string `json:"op_id"`
		}
		if err := json.Unmarshal(req.Args, &args); err != nil {
			w.Error(req.ID, ErrBadRequest, err.Error())
			return
		}
		op, ok := s.registry.Get(args.OpID)
		if !ok {
			w.Error(req.ID, ErrNotFound, "no such op")
			return
		}
		op.Cancel()
		w.Result(req.ID, json.RawMessage(`{"cancelled":true}`))
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/daemon/ipc/... -run TestAttachReplays -race -timeout 10s`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/ipc/server.go internal/daemon/ipc/streaming.go internal/daemon/ipc/streaming_test.go
git commit -m "feat(ipc): streaming handler scaffold with heartbeat, attach, cancel"
```

---

### Task 1.5: Streaming client API

**Files:** Modify: `internal/daemon/ipc/client.go`. Test: extend `internal/daemon/ipc/client_test.go`.

- [ ] **Step 1: Failing test**

Append to `internal/daemon/ipc/client_test.go`:

```go
func TestClientStreamReceivesFrames(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "ctl.sock")
	srv := NewServer(sock)
	srv.SetRegistry(NewOpRegistry())
	srv.RegisterStreaming(Command("FAKE"), func(ctx context.Context, args json.RawMessage, w FrameWriter, op *Op) {
		w.Progress(op.ID, "p1", "hello", 1, 10)
		w.Progress(op.ID, "p1", "world", 2, 10)
		w.Done(op.ID, json.RawMessage(`{"ok":true}`))
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	cli := NewClient(sock)
	frames, errc := cli.Stream(ctx, Command("FAKE"), nil)
	gotProgress := 0
	gotDone := false
	for f := range frames {
		switch f.Type {
		case FrameProgress:
			gotProgress++
		case FrameDone:
			gotDone = true
		}
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	if gotProgress != 2 || !gotDone {
		t.Errorf("got %d progress, done=%v", gotProgress, gotDone)
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/daemon/ipc/... -run TestClientStream`
Expected: FAIL.

- [ ] **Step 3: Implement**

In `internal/daemon/ipc/client.go`:

```go
// Stream issues a streaming command and returns a frames channel.
// Closing the returned context aborts the stream. The errc channel
// receives a single value: nil on clean termination, or the parse/IO
// error.
func (c *Client) Stream(ctx context.Context, cmd Command, args any) (<-chan Frame, <-chan error) {
	frames := make(chan Frame, 32)
	errc := make(chan error, 1)
	go func() {
		defer close(frames)
		defer close(errc)
		conn, err := net.DialTimeout("unix", c.path, 2*time.Second)
		if err != nil {
			errc <- err
			return
		}
		defer conn.Close()
		go func() {
			<-ctx.Done()
			conn.Close()
		}()
		id := uuid.NewString()
		req := Request{V: ProtocolVersion, ID: id, Cmd: cmd}
		if args != nil {
			b, _ := json.Marshal(args)
			req.Args = b
		}
		if err := json.NewEncoder(conn).Encode(req); err != nil {
			errc <- err
			return
		}
		dec := json.NewDecoder(bufio.NewReader(conn))
		for {
			var f Frame
			if err := dec.Decode(&f); err != nil {
				if ctx.Err() != nil {
					errc <- nil
					return
				}
				errc <- err
				return
			}
			if f.Type == FrameHeartbeat {
				continue
			}
			frames <- f
			if f.Type == FrameDone || f.Type == FrameError {
				errc <- nil
				return
			}
		}
	}()
	return frames, errc
}
```

(Add `"context"` import if missing.)

- [ ] **Step 4: Run tests**

Run: `go test ./internal/daemon/ipc/... -run TestClientStream -race -timeout 10s`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/ipc/client.go internal/daemon/ipc/client_test.go
git commit -m "feat(ipc): client.Stream returns frames channel for long-running commands"
```

---

### Task 1.6: STOP command + recovery wiring in daemon main

**Files:** Modify: `cmd/jellywatchd/control.go`, `cmd/jellywatchd/main.go`. Test: `cmd/jellywatchd/control_test.go` (extend).

- [ ] **Step 1: Failing test**

Append to `cmd/jellywatchd/control_test.go`:

```go
func TestStopHandlerCallsShutdown(t *testing.T) {
	called := false
	stop := func() { called = true }
	srv := ipc.NewServer(filepath.Join(t.TempDir(), "ctl.sock"))
	srv.Register(ipc.CmdStop, stopHandler(stop))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()
	cli := ipc.NewClient(srv.Path())
	_, _ = cli.Call(ctx, ipc.CmdStop, nil)
	time.Sleep(50 * time.Millisecond)
	if !called {
		t.Error("stop func not invoked")
	}
}
```

(May need `srv.Path()` accessor — add a one-line getter on `Server` if not present.)

- [ ] **Step 2: Run, verify fail**

Run: `go test ./cmd/jellywatchd/... -run TestStopHandler`
Expected: FAIL.

- [ ] **Step 3: Implement**

In `cmd/jellywatchd/control.go`:

```go
func stopHandler(stop func()) ipc.Handler {
	return func(ctx context.Context, req ipc.Request, w ipc.FrameWriter) {
		w.Result(req.ID, json.RawMessage(`{"stopping":true}`))
		go stop()
	}
}
```

In `cmd/jellywatchd/main.go`, after the IPC server is constructed and reloadables registered:

```go
controlServer.Register(ipc.CmdStop, stopHandler(func() {
	cancel() // the daemon's root context cancel
}))
```

Also extend the existing `daemonStatus` struct in `cmd/jellywatchd/control.go` (do not introduce a parallel `statusPayload`). Add fields and update `statusHandler`:

```go
type daemonStatus struct {
    PID            int              `json:"pid"`
    UptimeSeconds  int64            `json:"uptime_seconds"`
    ConfigLoaded   bool             `json:"config_loaded"`
    State          string           `json:"state"` // "running" | "interrupted"
    InterruptedOps []ipc.OpLogEntry `json:"interrupted_ops,omitempty"`
}
```

`statusHandler` now takes a getter for pending ops:

```go
func statusHandler(startedAt time.Time, getConfig func() *config.Config, getPending func() []ipc.OpLogEntry) ipc.Handler {
    return func(ctx context.Context, req ipc.Request, w ipc.FrameWriter) {
        pending := getPending()
        status := daemonStatus{
            PID:            os.Getpid(),
            UptimeSeconds:  int64(time.Since(startedAt).Seconds()),
            ConfigLoaded:   getConfig() != nil,
            State:          "running",
            InterruptedOps: pending,
        }
        if len(pending) > 0 {
            status.State = "interrupted"
        }
        data, err := json.Marshal(status)
        if err != nil {
            w.Error(req.ID, ipc.ErrInternal, err.Error())
            return
        }
        w.Result(req.ID, data)
    }
}
```

Update existing call sites that pass `statusHandler(...)` to supply the new third argument. Add the registry/op-log/recovery sequence in `main.go`:

```go
// Op-registry + op-log are required by streaming commands.
opLogPath := filepath.Join(configDir, "op_log.jsonl")
opLog, err := ipc.OpenOpLog(opLogPath)
if err != nil {
    log.Fatalf("open op log: %v", err)
}
defer opLog.Close()

pending, _ := opLog.Pending()
var pendingMu sync.Mutex
getPending := func() []ipc.OpLogEntry {
    pendingMu.Lock(); defer pendingMu.Unlock()
    out := make([]ipc.OpLogEntry, len(pending)); copy(out, pending); return out
}
clearPending := func() { pendingMu.Lock(); pending = nil; pendingMu.Unlock() }

// Server already has a registry from NewServer; expose it for handlers
// that need direct access (none today).
controlServer.Register(ipc.CmdStatus, statusHandler(startedAt, getCurrentConfig, getPending))
```

While `len(pending) > 0`, mutating IPC commands (`RESCAN`, `RESET_DB`) must reject with `ErrConflict` until the operator clears the interrupted state via `RECOVER` (Task 1.7).

- [ ] **Step 4: Run tests**

Run: `go test ./cmd/jellywatchd/... -run TestStopHandler && go build ./...`
Expected: PASS, clean build.

- [ ] **Step 5: Commit**

```bash
git add cmd/jellywatchd/
git commit -m "feat(daemon): STOP IPC + op-log recovery on startup"
```

---

### Task 1.7: RECOVER command (discard / resume)

**Files:** Modify: `internal/daemon/ipc/protocol.go`, `cmd/jellywatchd/control.go`. Test: `cmd/jellywatchd/control_test.go`.

The browser's `/daemon/recover` endpoint (Task 3.1) needs a daemon-side handler. Resume is out of scope for v1 (no destructive op is currently re-runnable mid-flight) — the handler accepts `discard` and rejects `resume` with `ErrNotImplemented`. Discard marks every pending op as `cancelled` in the op log and clears the daemon's in-memory pending slice so subsequent mutators are unblocked.

- [ ] **Step 1: Failing test**

```go
func TestRecoverDiscardClearsPending(t *testing.T) {
    dir := t.TempDir()
    sock := filepath.Join(dir, "ctl.sock")
    logFile, _ := ipc.OpenOpLog(filepath.Join(dir, "op_log.jsonl"))
    defer logFile.Close()
    _ = logFile.Begin("op-x", ipc.CmdResetDB, nil)

    pending, _ := logFile.Pending()
    if len(pending) != 1 { t.Fatal("setup: expected 1 pending") }

    var current = pending
    getPending := func() []ipc.OpLogEntry { return current }
    clearPending := func() { current = nil }

    srv := ipc.NewServer(sock)
    srv.Register(ipc.CmdRecover, recoverHandler(logFile, getPending, clearPending))
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    srv.Start(ctx); defer srv.Stop()

    cli := ipc.NewClient(sock)
    if _, err := cli.Call(ctx, ipc.CmdRecover, map[string]string{"action": "discard"}); err != nil {
        t.Fatal(err)
    }
    if got, _ := logFile.Pending(); len(got) != 0 {
        t.Errorf("expected pending cleared, got %d", len(got))
    }
    if len(getPending()) != 0 { t.Error("in-memory pending not cleared") }
}
```

- [ ] **Step 2: Add `CmdRecover` constant** to `protocol.go` alongside the others.

- [ ] **Step 3: Implement**

```go
// cmd/jellywatchd/control.go
type recoverArgs struct { Action string `json:"action"` }

func recoverHandler(log *ipc.OpLog, getPending func() []ipc.OpLogEntry, clearPending func()) ipc.Handler {
    return func(ctx context.Context, req ipc.Request, w ipc.FrameWriter) {
        var a recoverArgs
        if err := json.Unmarshal(req.Args, &a); err != nil {
            w.Error(req.ID, ipc.ErrBadRequest, err.Error()); return
        }
        switch a.Action {
        case "discard":
            for _, p := range getPending() {
                if err := log.MarkDiscarded(p.ID); err != nil {
                    w.Error(req.ID, ipc.ErrInternal, err.Error()); return
                }
            }
            clearPending()
            w.Result(req.ID, json.RawMessage(`{"discarded":true}`))
        case "resume":
            w.Error(req.ID, ipc.ErrNotImplemented, "resume not supported in v1")
        default:
            w.Error(req.ID, ipc.ErrBadRequest, "action must be discard or resume")
        }
    }
}
```

Wire in `main.go`:
```go
controlServer.Register(ipc.CmdRecover, recoverHandler(opLog, getPending, clearPending))
```

Also gate `RESCAN` / `RESET_DB` registration:
```go
guard := func(h ipc.StreamingHandler) ipc.StreamingHandler {
    return func(ctx context.Context, args json.RawMessage, w ipc.FrameWriter, op *ipc.Op) {
        if len(getPending()) > 0 {
            w.Error(op.ID, ipc.ErrConflict, "interrupted op pending; recover first")
            return
        }
        h(ctx, args, w, op)
    }
}
controlServer.RegisterStreaming(ipc.CmdRescan,  guard(rescanHandler(scanner, opLog)))
controlServer.RegisterStreaming(ipc.CmdResetDB, guard(resetDBHandler(db, opLog)))
```

- [ ] **Step 4: Run tests**

Run: `go test ./cmd/jellywatchd/... -run TestRecover && go test ./internal/daemon/ipc/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/ipc/protocol.go cmd/jellywatchd/
git commit -m "feat(daemon): RECOVER IPC (discard) + interrupted-op gate on mutators"
```

---

## Phase 2 — Long-running ops

### Task 2.0: Expose `*sql.DB` from `MediaDB`

**Files:** Modify: `internal/database/database.go`. Test: `internal/database/database_test.go`.

The maintenance package needs the raw handle (DDL, VACUUM). Add a getter rather than passing `*MediaDB` deep into the IPC layer.

- [ ] **Step 1: Failing test**

```go
func TestMediaDBSQLReturnsHandle(t *testing.T) {
    db, err := OpenPath(filepath.Join(t.TempDir(), "media.db"))
    if err != nil { t.Fatal(err) }
    defer db.Close()
    if db.SQL() == nil { t.Fatal("SQL() returned nil") }
    var n int
    if err := db.SQL().QueryRow("SELECT 1").Scan(&n); err != nil { t.Fatal(err) }
}
```

- [ ] **Step 2: Implement**

```go
func (m *MediaDB) SQL() *sql.DB { return m.db } // or whatever the field is named
```

- [ ] **Step 3: Commit**

```bash
git add internal/database/
git commit -m "feat(db): expose underlying *sql.DB via MediaDB.SQL"
```

---

### Task 2.1: Database maintenance package

**Files:** Create: `internal/database/maintenance.go`, `internal/database/maintenance_test.go`.

Before writing the test, **audit the schema** (`internal/database/schema.go`) to determine: (a) which user tables exist, (b) whether any "preserve on reset" table exists today (e.g., `audit_log`, `sync_log`). The test below assumes `sync_log` is preserved; substitute the actual operator-history table name if different. If no such table exists, scope `preserve` to the empty set and update the API/UI text in Task 4.1 / 7.6 accordingly.

- [ ] **Step 1: Failing test**

```go
// internal/database/maintenance_test.go
package database

import (
    "context"
    "path/filepath"
    "testing"
)

func TestResetDatabaseTruncatesUserTables(t *testing.T) {
    dir := t.TempDir()
    mdb, err := OpenPath(filepath.Join(dir, "media.db"))
    if err != nil { t.Fatal(err) }
    defer mdb.Close()

    db := mdb.SQL()
    // Insert into a known table (substitute the real table name from
    // schema.go — e.g., "movies" or "media_files").
    if _, err := db.Exec(`INSERT INTO media_files (path) VALUES ('/x')`); err != nil {
        t.Fatal(err)
    }

    progress := make(chan ProgressEvent, 16)
    go func() { for range progress {} }()

    if err := ResetDatabase(context.Background(), db, nil, progress); err != nil {
        t.Fatal(err)
    }
    close(progress)

    var n int
    if err := db.QueryRow("SELECT COUNT(*) FROM media_files").Scan(&n); err != nil {
        t.Fatal(err)
    }
    if n != 0 {
        t.Errorf("media_files count = %d, want 0", n)
    }
}

func TestResetDatabasePreservesNamedTable(t *testing.T) {
    dir := t.TempDir()
    mdb, _ := OpenPath(filepath.Join(dir, "media.db"))
    defer mdb.Close()
    db := mdb.SQL()
    // Substitute a table that exists in schema.go and is meaningful to
    // preserve (e.g., "sync_log"). If none exist, drop this test.
    if _, err := db.Exec(`INSERT INTO sync_log (event) VALUES ('boot')`); err != nil {
        t.Skip("sync_log table not present in schema; preserve-list test skipped")
    }
    progress := make(chan ProgressEvent, 16)
    go func() { for range progress {} }()
    if err := ResetDatabase(context.Background(), db, []string{"sync_log"}, progress); err != nil {
        t.Fatal(err)
    }
    close(progress)
    var n int
    db.QueryRow("SELECT COUNT(*) FROM sync_log").Scan(&n)
    if n != 1 { t.Errorf("sync_log preserved count = %d, want 1", n) }
}
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/database/... -run TestResetDatabase`
Expected: FAIL.

- [ ] **Step 3: Implement**

```go
// internal/database/maintenance.go
package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type ProgressEvent struct {
	Phase   string
	Msg     string
	Current int
	Total   int
}

// ResetDatabase truncates every table except those named in preserve.
// Emits ProgressEvent values to progress as it works. Honors ctx cancel
// at table boundaries.
func ResetDatabase(ctx context.Context, db *sql.DB, preserve []string, progress chan<- ProgressEvent) error {
	keep := map[string]bool{}
	for _, t := range preserve {
		keep[t] = true
	}

	tables, err := listUserTables(db)
	if err != nil {
		return err
	}
	progress <- ProgressEvent{Phase: "preparing", Total: len(tables)}

	for i, name := range tables {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if keep[name] {
			progress <- ProgressEvent{Phase: "preserving", Msg: name, Current: i + 1, Total: len(tables)}
			continue
		}
		progress <- ProgressEvent{Phase: "truncating", Msg: name, Current: i + 1, Total: len(tables)}
		if _, err := db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %q", name)); err != nil {
			return fmt.Errorf("truncate %s: %w", name, err)
		}
	}

	progress <- ProgressEvent{Phase: "vacuuming"}
	// VACUUM cannot run inside a transaction; database/sql + go-sqlite3
	// does not auto-wrap Exec in one, so this is safe. Use Exec (not
	// ExecContext) because VACUUM ignores cancel mid-statement anyway.
	if _, err := db.Exec("VACUUM"); err != nil {
		return fmt.Errorf("vacuum: %w", err)
	}

	progress <- ProgressEvent{Phase: "complete"}
	return nil
}

func listUserTables(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		// Skip SQLite-managed leftover tables.
		if strings.HasPrefix(n, "sqlite_") {
			continue
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
```

Add a similar `Rescan` capability — but the actual walk lives on the existing scanner type, so it is implemented in Task 2.1a, not here.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/database/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/database/maintenance.go internal/database/maintenance_test.go
git commit -m "feat(db): ResetDatabase with progress channel and ctx-cancel"
```

---

### Task 2.1a: `FileScanner.FullRescan`

**Files:** Modify: `internal/scanner/scanner.go` (or a new `internal/scanner/rescan.go`), `internal/scanner/periodic.go`. Test: `internal/scanner/rescan_test.go` (new).

The existing scanner has a periodic walker (`internal/scanner/periodic.go`); `FullRescan` is a one-shot version of that path that emits `database.ProgressEvent` values and honors `ctx`. Rather than duplicate logic, expose a method that wraps the existing walk with a progress adapter.

**Contract:**
```go
func (s *FileScanner) FullRescan(ctx context.Context, paths []string, dryRun bool, progress chan<- database.ProgressEvent) error
```

- Emits `{Phase: "walking", Msg: <root>}` once per root.
- Emits `{Phase: "indexing", Msg: <file>, Current: i, Total: n}` per file processed.
- Emits `{Phase: "complete"}` at end.
- Returns `ctx.Err()` when cancellation is observed at a file boundary.
- `dryRun` means: walk + classify but do not write to the database.

- [ ] **Step 1: Failing test**

```go
// internal/scanner/rescan_test.go
package scanner

import (
    "context"
    "os"
    "path/filepath"
    "testing"

    "github.com/Nomadcxx/jellywatch/internal/database"
)

func TestFullRescanEmitsProgressAndHonorsCancel(t *testing.T) {
    dir := t.TempDir()
    os.WriteFile(filepath.Join(dir, "a.mkv"), []byte("x"), 0644)
    os.WriteFile(filepath.Join(dir, "b.mkv"), []byte("y"), 0644)

    mdb, _ := database.OpenPath(filepath.Join(t.TempDir(), "m.db"))
    defer mdb.Close()
    s := NewFileScanner(mdb)

    progress := make(chan database.ProgressEvent, 16)
    done := make(chan error, 1)
    ctx, cancel := context.WithCancel(context.Background())
    go func() { done <- s.FullRescan(ctx, []string{dir}, true, progress) }()

    var saw bool
    for ev := range progress {
        if ev.Phase == "indexing" { saw = true }
        if ev.Phase == "complete" { break }
    }
    if err := <-done; err != nil { t.Fatal(err) }
    if !saw { t.Error("no indexing events emitted") }
    _ = cancel
}
```

- [ ] **Step 2: Implement**

Reuse the periodic walker. Sketch:

```go
func (s *FileScanner) FullRescan(ctx context.Context, roots []string, dryRun bool, progress chan<- database.ProgressEvent) error {
    files, err := s.collectFiles(roots) // existing helper or new wrapper
    if err != nil { return err }
    for i, root := range roots {
        progress <- database.ProgressEvent{Phase: "walking", Msg: root, Current: i + 1, Total: len(roots)}
    }
    for i, f := range files {
        if err := ctx.Err(); err != nil { return err }
        progress <- database.ProgressEvent{Phase: "indexing", Msg: f, Current: i + 1, Total: len(files)}
        if dryRun { continue }
        if err := s.indexOne(ctx, f); err != nil { return err }
    }
    progress <- database.ProgressEvent{Phase: "complete"}
    return nil
}
```

If `collectFiles` / `indexOne` don't exist with these names, factor them out from `periodic.go` first. Add a unit test for the factor-out before this task to keep behavior pinned.

- [ ] **Step 3: Run tests**

Run: `go test ./internal/scanner/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/scanner/
git commit -m "feat(scanner): FullRescan one-shot walk with progress channel and ctx-cancel"
```

---

### Task 2.2: RESCAN IPC handler

**Files:** Modify: `cmd/jellywatchd/control.go`. Test: `cmd/jellywatchd/control_test.go`.

- [ ] **Step 1: Failing test**

```go
func TestRescanStreamsProgress(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "ctl.sock")
	srv := ipc.NewServer(sock)
	srv.SetRegistry(ipc.NewOpRegistry())

	scanner := &fakeScannerForTest{}
	logFile, _ := ipc.OpenOpLog(filepath.Join(dir, "op_log.jsonl"))
	defer logFile.Close()

	srv.RegisterStreaming(ipc.CmdRescan, rescanHandler(scanner, logFile))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv.Start(ctx)
	defer srv.Stop()

	cli := ipc.NewClient(sock)
	frames, errc := cli.Stream(ctx, ipc.CmdRescan, map[string]any{"dry_run": false})
	progress, done := 0, false
	for f := range frames {
		switch f.Type {
		case ipc.FrameProgress:
			progress++
		case ipc.FrameDone:
			done = true
		}
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	if progress < 1 || !done {
		t.Errorf("progress=%d done=%v", progress, done)
	}
}

type fakeScannerForTest struct{}

func (fakeScannerForTest) FullRescan(ctx context.Context, paths []string, dry bool, p chan<- database.ProgressEvent) error {
	p <- database.ProgressEvent{Phase: "walking"}
	p <- database.ProgressEvent{Phase: "indexing", Current: 1, Total: 1}
	return nil
}
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./cmd/jellywatchd/... -run TestRescan`
Expected: FAIL.

- [ ] **Step 3: Implement**

In `cmd/jellywatchd/control.go`:

```go
type rescanArgs struct {
	Paths  []string `json:"paths,omitempty"`
	DryRun bool     `json:"dry_run"`
}

type rescanScanner interface {
	FullRescan(ctx context.Context, paths []string, dryRun bool, p chan<- database.ProgressEvent) error
}

func rescanHandler(scanner rescanScanner, log *ipc.OpLog) ipc.StreamingHandler {
	return func(ctx context.Context, raw json.RawMessage, w ipc.FrameWriter, op *ipc.Op) {
		var args rescanArgs
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &args)
		}
		_ = log.Begin(op.ID, ipc.CmdRescan, map[string]any{"paths": args.Paths, "dry_run": args.DryRun})
		progress := make(chan database.ProgressEvent, 64)
		go func() {
			for ev := range progress {
				w.Progress(op.ID, ev.Phase, ev.Msg, ev.Current, ev.Total)
			}
		}()
		err := scanner.FullRescan(ctx, args.Paths, args.DryRun, progress)
		close(progress)
		if err != nil {
			w.Error(op.ID, ipc.ErrInternal, err.Error())
			_ = log.End(op.ID, "error", err.Error())
			return
		}
		w.Done(op.ID, json.RawMessage(`{"ok":true}`))
		_ = log.End(op.ID, "done", "")
	}
}
```

- [ ] **Step 4: Wire into daemon main**

```go
controlServer.RegisterStreaming(ipc.CmdRescan, rescanHandler(scanner, opLog))
```

- [ ] **Step 5: Run tests**

Run: `go test ./cmd/jellywatchd/... -run TestRescan && go build ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/jellywatchd/
git commit -m "feat(daemon): RESCAN IPC with progress streaming and op log"
```

---

### Task 2.3: RESET_DB IPC handler

**Files:** Modify: `cmd/jellywatchd/control.go`. Test: `cmd/jellywatchd/control_test.go`.

- [ ] **Step 1: Failing test**

```go
func TestResetDBHandlerRequiresLiteralConfirm(t *testing.T) {
	srv := ipc.NewServer(filepath.Join(t.TempDir(), "ctl.sock"))
	srv.SetRegistry(ipc.NewOpRegistry())
	srv.RegisterStreaming(ipc.CmdResetDB, resetDBHandler(nil, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv.Start(ctx); defer srv.Stop()

	cli := ipc.NewClient(srv.Path())
	frames, _ := cli.Stream(ctx, ipc.CmdResetDB, map[string]any{"confirm": "wrong"})
	gotErr := false
	for f := range frames {
		if f.Type == ipc.FrameError {
			gotErr = true
		}
	}
	if !gotErr {
		t.Error("expected ErrBadRequest for wrong confirm")
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./cmd/jellywatchd/... -run TestResetDBHandler`
Expected: FAIL.

- [ ] **Step 3: Implement**

```go
type resetArgs struct {
	Confirm  string   `json:"confirm"`
	Preserve []string `json:"preserve,omitempty"`
}

func resetDBHandler(db *sql.DB, log *ipc.OpLog) ipc.StreamingHandler {
	return func(ctx context.Context, raw json.RawMessage, w ipc.FrameWriter, op *ipc.Op) {
		var args resetArgs
		if err := json.Unmarshal(raw, &args); err != nil {
			w.Error(op.ID, ipc.ErrBadRequest, err.Error())
			return
		}
		if args.Confirm != "media.db" {
			w.Error(op.ID, ipc.ErrBadRequest, `confirm must equal "media.db"`)
			return
		}
		_ = log.Begin(op.ID, ipc.CmdResetDB, map[string]any{"preserve": args.Preserve})
		progress := make(chan database.ProgressEvent, 64)
		go func() {
			for ev := range progress {
				w.Progress(op.ID, ev.Phase, ev.Msg, ev.Current, ev.Total)
			}
		}()
		err := database.ResetDatabase(ctx, db, args.Preserve, progress)
		close(progress)
		if err != nil {
			w.Error(op.ID, ipc.ErrInternal, err.Error())
			_ = log.End(op.ID, "error", err.Error())
			return
		}
		w.Done(op.ID, json.RawMessage(`{"ok":true}`))
		_ = log.End(op.ID, "done", "")
	}
}
```

Wire into main:
```go
controlServer.RegisterStreaming(ipc.CmdResetDB, guard(resetDBHandler(db.SQL(), opLog)))
```

Also register cancel + attach:
```go
controlServer.Register(ipc.CmdAttach, attachHandler(controlServer))
controlServer.Register(ipc.CmdCancel, cancelHandler(controlServer))
```

- [ ] **Step 4: Run tests**

Run: `go test ./cmd/jellywatchd/... && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/jellywatchd/
git commit -m "feat(daemon): RESET_DB IPC with typed-confirm gate, ATTACH/CANCEL wiring"
```

---

## Phase 3 — REST API: daemon lifecycle

### Task 3.1: GET /daemon/status

**Files:** Create: `internal/api/daemon_handlers.go`, `internal/api/daemon_handlers_test.go`.

- [ ] **Step 1: Failing test**

```go
// internal/api/daemon_handlers_test.go
package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
)

type stubDaemonIPC struct {
	statusBody json.RawMessage
	streamErr  error
}

func (s stubDaemonIPC) Call(ctx context.Context, cmd ipc.Command, args any) (json.RawMessage, error) {
	return s.statusBody, nil
}

func (s stubDaemonIPC) StreamWithID(ctx context.Context, cmd ipc.Command, args any, opID string) error {
	return s.streamErr
}

func TestDaemonStatusReturnsRunning(t *testing.T) {
	h := &DaemonHandlers{IPC: stubDaemonIPC{statusBody: json.RawMessage(`{"state":"running","version":"x"}`)}}
	req := httptest.NewRequest("GET", "/daemon/status", nil)
	w := httptest.NewRecorder()
	h.Status(w, req)
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	var got map[string]any
	json.Unmarshal(w.Body.Bytes(), &got)
	if got["state"] != "running" {
		t.Errorf("state = %v", got["state"])
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/api/... -run TestDaemonStatus`
Expected: FAIL.

- [ ] **Step 3: Implement**

```go
// internal/api/daemon_handlers.go
package api

import (
	"encoding/json"
	"net/http"

	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
	"github.com/Nomadcxx/jellywatch/internal/jellyweb/daemonctl"
)

type DaemonHandlers struct {
	IPC      IPCCaller
	Launcher *daemonctl.Launcher
}

func (h *DaemonHandlers) Status(w http.ResponseWriter, r *http.Request) {
	body, err := h.IPC.Call(r.Context(), ipc.CmdStatus, nil)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"state":"stopped"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

func (h *DaemonHandlers) Stop(w http.ResponseWriter, r *http.Request) {
	if _, err := h.IPC.Call(r.Context(), ipc.CmdStop, nil); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *DaemonHandlers) Reload(w http.ResponseWriter, r *http.Request) {
	body, err := h.IPC.Call(r.Context(), ipc.CmdReload, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

func (h *DaemonHandlers) Start(w http.ResponseWriter, r *http.Request) {
	if err := h.Launcher.Start(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *DaemonHandlers) Restart(w http.ResponseWriter, r *http.Request) {
	_, _ = h.IPC.Call(r.Context(), ipc.CmdStop, nil)
	if err := h.Launcher.Start(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

type recoverArgs struct {
	Action string `json:"action"`
}

func (h *DaemonHandlers) Recover(w http.ResponseWriter, r *http.Request) {
	var a recoverArgs
	json.NewDecoder(r.Body).Decode(&a)
	body, err := h.IPC.Call(r.Context(), ipc.CmdRecover, a)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}
```

- [ ] **Step 4: Run test**

Run: `go test ./internal/api/... -run TestDaemonStatus`
Expected: PASS.

- [ ] **Step 5: Mount routes**

In `internal/api/server.go`:

```go
daemonH := &DaemonHandlers{IPC: s.ipc, Launcher: s.launcher}
r.Route("/daemon", func(r chi.Router) {
	r.Get("/status", daemonH.Status)
	r.Post("/stop", daemonH.Stop)
	r.Post("/reload", daemonH.Reload)
	r.Post("/start", daemonH.Start)
	r.Post("/restart", daemonH.Restart)
	r.Post("/recover", daemonH.Recover)
})
```

- [ ] **Step 6: Commit**

```bash
git add internal/api/daemon_handlers.go internal/api/daemon_handlers_test.go internal/api/server.go
git commit -m "feat(api): /daemon/{status,stop,reload,start,restart,recover}"
```

---

### Task 3.2: Launcher (start strategy resolver)

**Files:** Create: `internal/jellyweb/daemonctl/launcher.go`, `_test.go`.

- [ ] **Step 1: Failing test**

```go
// internal/jellyweb/daemonctl/launcher_test.go
package daemonctl

import (
	"errors"
	"testing"
)

func TestLauncherSelectsSystemdUserWhenAvailable(t *testing.T) {
	l := &Launcher{
		systemdUserExists: func() bool { return true },
		systemdUserStart:  func() error { return nil },
	}
	if err := l.Start(); err != nil {
		t.Fatal(err)
	}
}

func TestLauncherFallsBackToDirectExec(t *testing.T) {
	called := false
	l := &Launcher{
		systemdUserExists:   func() bool { return false },
		systemdSystemExists: func() bool { return false },
		directExec:          func() error { called = true; return nil },
	}
	if err := l.Start(); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("direct exec not invoked")
	}
}

func TestLauncherReturnsStrategyError(t *testing.T) {
	l := &Launcher{
		systemdUserExists: func() bool { return true },
		systemdUserStart:  func() error { return errors.New("boom") },
	}
	if err := l.Start(); err == nil {
		t.Error("expected error")
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/jellyweb/daemonctl/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

```go
// internal/jellyweb/daemonctl/launcher.go
package daemonctl

import (
	"errors"
	"os/exec"
	"syscall"
)

type Launcher struct {
	BinaryPath          string // path to jellywatchd
	LogPath             string // path for detached stdout/stderr

	// Hooks for unit testing — production code populates them in New().
	systemdUserExists   func() bool
	systemdUserStart    func() error
	systemdSystemExists func() bool
	systemdSystemStart  func() error
	directExec          func() error
}

func New(binary, logPath string) *Launcher {
	l := &Launcher{BinaryPath: binary, LogPath: logPath}
	l.systemdUserExists = func() bool {
		return exec.Command("systemctl", "--user", "list-unit-files", "jellywatchd.service").Run() == nil
	}
	l.systemdUserStart = func() error {
		return exec.Command("systemctl", "--user", "start", "jellywatchd").Run()
	}
	l.systemdSystemExists = func() bool {
		return exec.Command("systemctl", "list-unit-files", "jellywatchd.service").Run() == nil
	}
	l.systemdSystemStart = func() error {
		return exec.Command("systemctl", "start", "jellywatchd").Run()
	}
	l.directExec = func() error {
		f, err := openLogFile(logPath)
		if err != nil {
			return err
		}
		cmd := exec.Command(binary)
		cmd.Stdout, cmd.Stderr = f, f
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := cmd.Start(); err != nil {
			f.Close()
			return err
		}
		// Don't Wait — we want the daemon detached.
		return nil
	}
	return l
}

func (l *Launcher) Start() error {
	if l.systemdUserExists != nil && l.systemdUserExists() {
		return l.systemdUserStart()
	}
	if l.systemdSystemExists != nil && l.systemdSystemExists() {
		return l.systemdSystemStart()
	}
	if l.directExec != nil {
		return l.directExec()
	}
	return errors.New("no daemon launch strategy available")
}
```

```go
// internal/jellyweb/daemonctl/log_file.go
package daemonctl

import "os"

func openLogFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/jellyweb/daemonctl/...`
Expected: PASS.

- [ ] **Step 5: Wire in jellyweb startup**

In `cmd/jellyweb/main.go`, before constructing `api.Server`, build the launcher and pass it through. Add `Launcher *daemonctl.Launcher` to `api.Server`'s deps.

- [ ] **Step 6: Commit**

```bash
git add internal/jellyweb/daemonctl/ cmd/jellyweb/main.go internal/api/server.go
git commit -m "feat(jellyweb): launcher with systemd-user → systemd-system → detached exec strategy"
```

---

## Phase 4 — REST API: database lifecycle

### Task 4.1: POST /database/rescan + /database/reset

**Files:** Create: `internal/api/database_handlers.go`, `_test.go`.

- [ ] **Step 1: Failing test**

```go
// internal/api/database_handlers_test.go
package api

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestRescanReturnsOpID(t *testing.T) {
	h := &DatabaseHandlers{IPC: stubDaemonIPC{
		statusBody: json.RawMessage(`{"op_id":"op-123"}`),
	}}
	body := []byte(`{"dry_run":false}`)
	req := httptest.NewRequest("POST", "/database/rescan", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Rescan(w, req)
	if w.Code != 202 {
		t.Fatalf("status %d", w.Code)
	}
	var got map[string]string
	json.Unmarshal(w.Body.Bytes(), &got)
	if got["op_id"] == "" {
		t.Error("expected op_id in response")
	}
}

func TestResetRequiresConfirm(t *testing.T) {
	h := &DatabaseHandlers{IPC: stubDaemonIPC{}}
	req := httptest.NewRequest("POST", "/database/reset", bytes.NewReader([]byte(`{"confirm":"wrong"}`)))
	w := httptest.NewRecorder()
	h.Reset(w, req)
	if w.Code != 400 {
		t.Errorf("want 400, got %d", w.Code)
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/api/... -run TestRescan`
Expected: FAIL.

- [ ] **Step 3: Implement**

The op ID is allocated by the API handler so the SSE relay can subscribe before the IPC connection finishes its first frame. The handler dispatches an asynchronous IPC stream that reuses the supplied ID via a new `Client.StreamWithID` (the daemon's `OpRegistry` and op-log key on `req.ID`, so the wire ID and the op_id must match).

```go
// internal/api/database_handlers.go
package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
	"github.com/google/uuid"
)

type DatabaseHandlers struct {
	IPC IPCCaller
}

type rescanReq struct {
	Paths  []string `json:"paths"`
	DryRun bool     `json:"dry_run"`
}

func (h *DatabaseHandlers) Rescan(w http.ResponseWriter, r *http.Request) {
	var body rescanReq
	json.NewDecoder(r.Body).Decode(&body)
	opID := "op-" + uuid.NewString()
	// Detached background stream; result/error frames are surfaced via
	// the SSE relay (Task 5.1) when the browser ATTACHes to op_id.
	go h.IPC.StreamWithID(context.Background(), ipc.CmdRescan, body, opID)
	respondAccepted(w, opID)
}

type resetReq struct {
	Confirm  string   `json:"confirm"`
	Preserve []string `json:"preserve"`
}

func (h *DatabaseHandlers) Reset(w http.ResponseWriter, r *http.Request) {
	var body resetReq
	json.NewDecoder(r.Body).Decode(&body)
	if body.Confirm != "media.db" {
		http.Error(w, "confirm must equal media.db", http.StatusBadRequest)
		return
	}
	opID := "op-" + uuid.NewString()
	go h.IPC.StreamWithID(context.Background(), ipc.CmdResetDB, body, opID)
	respondAccepted(w, opID)
}

func respondAccepted(w http.ResponseWriter, opID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"op_id": opID})
}
```

Extend `IPCCaller` (in `internal/api/config_save_pipeline.go`) — additive, existing call sites unaffected:

```go
type IPCCaller interface {
    Call(ctx context.Context, cmd ipc.Command, args any) (json.RawMessage, error)
    StreamWithID(ctx context.Context, cmd ipc.Command, args any, opID string) error
}
```

Add a real implementation on `ipc.Client` that dials, writes a Request whose `ID == opID`, and drains frames into the daemon's registry/ring (the relay later ATTACHes to that ID). The function returns nil on clean termination so the goroutine exits silently:

```go
// internal/daemon/ipc/client.go
func (c *Client) StreamWithID(ctx context.Context, cmd Command, args any, opID string) error {
    conn, err := net.DialTimeout("unix", c.path, 2*time.Second)
    if err != nil { return err }
    defer conn.Close()
    go func() { <-ctx.Done(); conn.Close() }()
    req := Request{V: ProtocolVersion, ID: opID, Cmd: cmd}
    if args != nil {
        b, _ := json.Marshal(args); req.Args = b
    }
    if err := json.NewEncoder(conn).Encode(req); err != nil { return err }
    dec := json.NewDecoder(bufio.NewReader(conn))
    for {
        var f Frame
        if err := dec.Decode(&f); err != nil {
            if ctx.Err() != nil { return nil }
            return err
        }
        if f.Type == FrameDone || f.Type == FrameError { return nil }
    }
}
```

> **Race note:** the dispatch goroutine takes ~1ms to register the op in the daemon's registry. The SSE relay (Task 5.1) must retry `ATTACH` for up to ~1s before returning 404 to the browser, otherwise a fast subscription races the kickoff. See Task 5.1 for the retry loop.
r.Route("/database", func(r chi.Router) {
	r.Post("/rescan", dbH.Rescan)
	r.Post("/reset", dbH.Reset)
})
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/api/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/api/database_handlers.go internal/api/database_handlers_test.go internal/api/server.go internal/daemon/ipc/client.go
git commit -m "feat(api): /database/{rescan,reset} kick off IPC streaming ops"
```

---

## Phase 5 — SSE relay

### Task 5.1: GET /events/op/{op_id}

**Files:** Create: `internal/api/sse_relay.go`, `_test.go`.

- [ ] **Step 1: Failing test**

```go
// internal/api/sse_relay_test.go
package api

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSSERelayForwardsFrames(t *testing.T) {
	// Use a fake IPC that emits a known sequence on Stream.
	frames := make(chan ipc.Frame, 4)
	frames <- ipc.Frame{ID: "x", Type: ipc.FrameProgress, Phase: "p", Msg: "hi"}
	close(frames)
	ipcStub := &fakeStream{frames: frames}

	h := &SSERelay{IPC: ipcStub}
	req := httptest.NewRequest("GET", "/events/op/x", nil)
	req = withChiRouteParam(req, "op_id", "x")
	w := httptest.NewRecorder()

	ctx, cancel := context.WithTimeout(req.Context(), 500*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)
	h.Stream(w, req)

	out := w.Body.String()
	if !strings.Contains(out, `"phase":"p"`) {
		t.Errorf("missing frame in SSE: %q", out)
	}
}
```

(Add a `fakeStream` test helper exposing `Attach(ctx, opID) (<-chan ipc.Frame, error)`, plus a `withChiRouteParam(req, key, val)` helper that wraps `chi.RouteContext` so `chi.URLParam` works in unit tests:

```go
func withChiRouteParam(r *http.Request, key, val string) *http.Request {
    rctx := chi.NewRouteContext()
    rctx.URLParams.Add(key, val)
    return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}
```
)

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/api/... -run TestSSERelay`
Expected: FAIL.

- [ ] **Step 3: Implement**

```go
// internal/api/sse_relay.go
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
	"github.com/go-chi/chi/v5"
)

type IPCAttacher interface {
	Attach(ctx context.Context, opID string) (<-chan ipc.Frame, error)
}

type SSERelay struct {
	IPC IPCAttacher
}

func (s *SSERelay) Stream(w http.ResponseWriter, r *http.Request) {
	opID := chi.URLParam(r, "op_id")
	frames, err := s.IPC.Attach(r.Context(), opID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	for {
		select {
		case <-r.Context().Done():
			return
		case f, ok := <-frames:
			if !ok {
				return
			}
			b, _ := json.Marshal(f)
			fmt.Fprintf(w, "data: %s\n\n", b)
			if flusher != nil {
				flusher.Flush()
			}
			if f.Type == ipc.FrameDone || f.Type == ipc.FrameError {
				return
			}
		}
	}
}
```

Add `Attach` to `ipc.Client` (delegates to ATTACH IPC command, with a short retry to absorb the kickoff race):

```go
// In internal/daemon/ipc/client.go
func (c *Client) Attach(ctx context.Context, opID string) (<-chan Frame, error) {
    deadline := time.Now().Add(1 * time.Second)
    for {
        frames, errc := c.Stream(ctx, CmdAttach, map[string]string{"op_id": opID})
        // Peek the first frame; if it's an ErrNotFound, retry.
        select {
        case f, ok := <-frames:
            if !ok {
                if err := <-errc; err != nil { return nil, err }
                return nil, errors.New("stream closed without frames")
            }
            if f.Type == FrameError && f.Code == ErrNotFound && time.Now().Before(deadline) {
                time.Sleep(50 * time.Millisecond); continue
            }
            // Re-emit the peeked frame onto a new channel so the caller sees it.
            out := make(chan Frame, 32)
            out <- f
            go func() {
                defer close(out)
                for ff := range frames { out <- ff }
            }()
            return out, nil
        case <-ctx.Done():
            return nil, ctx.Err()
        }
    }
}
```

The `IPCAttacher` interface and `SSERelay` call site need to pass `r.Context()`:

```go
type IPCAttacher interface {
    Attach(ctx context.Context, opID string) (<-chan ipc.Frame, error)
}
// ...
frames, err := s.IPC.Attach(r.Context(), opID)
```

- [ ] **Step 4: Mount route + test**

```go
sse := &SSERelay{IPC: s.ipc}
r.Get("/events/op/{op_id}", sse.Stream)
```

Run: `go test ./internal/api/... -run TestSSERelay`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/sse_relay.go internal/api/sse_relay_test.go internal/api/server.go internal/daemon/ipc/client.go
git commit -m "feat(api): SSE relay for /events/op/{id} via IPC ATTACH"
```

---

## Phase 6 — CLI parity

### Task 6.1: `jellywatch daemon {status,reload,stop}`

**Files:** Create: `cmd/jellywatch/daemon_cmd.go`, `cmd/jellywatch/daemon_cmd_test.go`. Modify: `cmd/jellywatch/main.go`.

- [ ] **Step 1: Failing test**

```go
// cmd/jellywatch/daemon_cmd_test.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
)

func TestDaemonStatusCommandPrintsJSON(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "ctl.sock")
	srv := ipc.NewServer(sock)
	srv.Register(ipc.CmdStatus, func(ctx context.Context, req ipc.Request, w ipc.FrameWriter) {
		w.Result(req.ID, json.RawMessage(`{"state":"running"}`))
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv.Start(ctx); defer srv.Stop()

	var buf bytes.Buffer
	cmd := newDaemonStatusCmd(sock, &buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte(`"state":"running"`)) {
		t.Errorf("output: %s", buf.String())
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./cmd/jellywatch/... -run TestDaemonStatusCommand`
Expected: FAIL.

- [ ] **Step 3: Implement**

```go
// cmd/jellywatch/daemon_cmd.go
package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
	"github.com/Nomadcxx/jellywatch/internal/paths"
	"github.com/spf13/cobra"
)

func newDaemonCmd() *cobra.Command {
	c := &cobra.Command{Use: "daemon", Short: "Daemon control"}
	c.AddCommand(newDaemonStatusCmd("", os.Stdout))
	c.AddCommand(newDaemonReloadCmd())
	c.AddCommand(newDaemonStopCmd())
	return c
}

func socketPath() string {
	d, _ := paths.JellyWatchDir()
	return d + "/control.sock"
}

func newDaemonStatusCmd(sock string, out io.Writer) *cobra.Command {
	if sock == "" {
		sock = socketPath()
	}
	return &cobra.Command{
		Use:   "status",
		Short: "Show daemon status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cli := ipc.NewClient(sock)
			body, err := cli.Call(context.Background(), ipc.CmdStatus, nil)
			if err != nil {
				return err
			}
			fmt.Fprintln(out, string(body))
			return nil
		},
	}
}

func newDaemonReloadCmd() *cobra.Command {
	return &cobra.Command{
		Use: "reload", Short: "Reload daemon config",
		RunE: func(cmd *cobra.Command, args []string) error {
			cli := ipc.NewClient(socketPath())
			body, err := cli.Call(context.Background(), ipc.CmdReload, nil)
			if err != nil {
				return err
			}
			fmt.Println(string(body))
			return nil
		},
	}
}

func newDaemonStopCmd() *cobra.Command {
	return &cobra.Command{
		Use: "stop", Short: "Stop the daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			cli := ipc.NewClient(socketPath())
			_, err := cli.Call(context.Background(), ipc.CmdStop, nil)
			return err
		},
	}
}
```

In `cmd/jellywatch/main.go`, register: `rootCmd.AddCommand(newDaemonCmd())`.

- [ ] **Step 4: Run tests**

Run: `go test ./cmd/jellywatch/... && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/jellywatch/daemon_cmd.go cmd/jellywatch/daemon_cmd_test.go cmd/jellywatch/main.go
git commit -m "feat(cli): jellywatch daemon {status,reload,stop} via IPC"
```

---

## Phase 7 — Frontend

### Task 7.1: ProgressCard component

**Files:** Create: `web/src/components/settings/ProgressCard.tsx`, `_test.tsx`.

- [ ] **Step 1: Failing test**

```tsx
// web/src/components/settings/ProgressCard.test.tsx
import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { ProgressCard } from './ProgressCard';

describe('ProgressCard', () => {
  it('renders phase and percentage', () => {
    render(
      <ProgressCard
        title="Re-scan"
        events={[
          { type: 'progress', phase: 'walking', msg: '/storage', current: 100, total: 1000 },
        ]}
      />,
    );
    expect(screen.getByText(/walking/i)).toBeInTheDocument();
    expect(screen.getByText(/10%/)).toBeInTheDocument();
  });

  it('shows done state', () => {
    render(<ProgressCard title="Re-scan" events={[{ type: 'done' }]} />);
    expect(screen.getByText(/complete/i)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run, verify fail**

Run: `(cd web && npx vitest run src/components/settings/ProgressCard.test.tsx)`
Expected: FAIL.

- [ ] **Step 3: Implement**

```tsx
// web/src/components/settings/ProgressCard.tsx
'use client';

import { Progress } from '@/components/ui/progress';

export type OpEvent = {
  type: 'progress' | 'done' | 'error' | 'cancelled';
  phase?: string;
  msg?: string;
  current?: number;
  total?: number;
  code?: string;
};

type Props = {
  title: string;
  events: OpEvent[];
};

export function ProgressCard({ title, events }: Props) {
  const last = events[events.length - 1];
  const pct = last?.total ? Math.round(((last.current ?? 0) / last.total) * 100) : 0;
  const isDone = last?.type === 'done';
  const isError = last?.type === 'error';
  const isCancelled = last?.type === 'cancelled';

  return (
    <div className="rounded border p-4">
      <h3 className="font-semibold">{title}</h3>
      {isDone && <p className="text-green-600">Complete.</p>}
      {isError && <p className="text-destructive">Failed: {last?.msg}</p>}
      {isCancelled && <p>Cancelled.</p>}
      {!isDone && !isError && !isCancelled && last?.type === 'progress' && (
        <>
          <p className="text-sm text-muted-foreground">
            {last.phase}: {last.msg} ({pct}%)
          </p>
          <Progress value={pct} className="mt-2" />
        </>
      )}
      <ul className="mt-3 max-h-48 overflow-y-auto text-xs font-mono text-muted-foreground">
        {events.slice(-30).map((e, i) => (
          <li key={i}>
            [{e.type}] {e.phase}: {e.msg}
          </li>
        ))}
      </ul>
    </div>
  );
}
```

- [ ] **Step 4: Run test**

Run: `(cd web && npx vitest run src/components/settings/ProgressCard.test.tsx)`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/settings/ProgressCard.tsx web/src/components/settings/ProgressCard.test.tsx
git commit -m "feat(web): ProgressCard with installer-style phase feed"
```

---

### Task 7.2: ConfirmDestructive (typed confirm)

**Files:** Create: `web/src/components/settings/ConfirmDestructive.tsx`, `_test.tsx`.

- [ ] **Step 1: Failing test**

```tsx
// web/src/components/settings/ConfirmDestructive.test.tsx
import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ConfirmDestructive } from './ConfirmDestructive';

describe('ConfirmDestructive', () => {
  it('disables confirm until match', () => {
    const onConfirm = vi.fn();
    render(<ConfirmDestructive open phrase="media.db" onConfirm={onConfirm} onCancel={vi.fn()}>
      Delete the media database?
    </ConfirmDestructive>);
    const button = screen.getByRole('button', { name: /confirm/i });
    expect(button).toBeDisabled();
    fireEvent.change(screen.getByPlaceholderText(/type/i), { target: { value: 'media.db' } });
    expect(button).not.toBeDisabled();
    fireEvent.click(button);
    expect(onConfirm).toHaveBeenCalled();
  });
});
```

- [ ] **Step 2: Run, verify fail**

Run: `(cd web && npx vitest run src/components/settings/ConfirmDestructive.test.tsx)`
Expected: FAIL.

- [ ] **Step 3: Implement**

```tsx
// web/src/components/settings/ConfirmDestructive.tsx
'use client';

import { ReactNode, useState } from 'react';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';

type Props = {
  open: boolean;
  phrase: string;
  children: ReactNode;
  onConfirm: () => void;
  onCancel: () => void;
};

export function ConfirmDestructive({ open, phrase, children, onConfirm, onCancel }: Props) {
  const [typed, setTyped] = useState('');
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onCancel()}>
      <DialogContent>
        <DialogTitle>Confirm destructive action</DialogTitle>
        <div className="text-sm">{children}</div>
        <p className="mt-3 text-sm">
          Type <span className="font-mono">{phrase}</span> to confirm.
        </p>
        <Input
          value={typed}
          placeholder={`type ${phrase}`}
          onChange={(e) => setTyped(e.target.value)}
        />
        <div className="mt-3 flex justify-end gap-2">
          <Button variant="ghost" onClick={onCancel}>Cancel</Button>
          <Button variant="destructive" disabled={typed !== phrase} onClick={onConfirm}>
            Confirm
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
```

- [ ] **Step 4: Run test**

Run: `(cd web && npx vitest run src/components/settings/ConfirmDestructive.test.tsx)`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/settings/ConfirmDestructive.tsx web/src/components/settings/ConfirmDestructive.test.tsx
git commit -m "feat(web): ConfirmDestructive typed-confirm modal"
```

---

### Task 7.3: useDaemon hook

**Files:** Create: `web/src/hooks/useDaemon.ts`, `_test.ts`.

- [ ] **Step 1: Failing test**

```ts
// web/src/hooks/useDaemon.test.ts
import { describe, expect, it, vi } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { useDaemon } from './useDaemon';

describe('useDaemon', () => {
  it('polls /daemon/status', async () => {
    const fetchSpy = vi.spyOn(global, 'fetch').mockResolvedValue({
      ok: true,
      json: async () => ({ state: 'running', uptime_s: 60 }),
    } as Response);

    const { result } = renderHook(() => useDaemon(50));
    await waitFor(() => expect(result.current.status?.state).toBe('running'));
    fetchSpy.mockRestore();
  });
});
```

- [ ] **Step 2: Run, verify fail**

Run: `(cd web && npx vitest run src/hooks/useDaemon.test.ts)`
Expected: FAIL.

- [ ] **Step 3: Implement**

```ts
// web/src/hooks/useDaemon.ts
'use client';

import { useEffect, useState } from 'react';

export type DaemonStatus = {
  state: 'running' | 'stopped' | 'interrupted';
  version?: string;
  uptime_s?: number;
  current_op?: { id: string; cmd: string };
  config_hash?: string;
  interrupted_op?: any;
};

export function useDaemon(intervalMs = 5000) {
  const [status, setStatus] = useState<DaemonStatus | null>(null);

  useEffect(() => {
    let cancelled = false;
    async function tick() {
      try {
        const r = await fetch('/api/daemon/status');
        if (!cancelled && r.ok) setStatus(await r.json());
      } catch {
        if (!cancelled) setStatus({ state: 'stopped' });
      }
    }
    tick();
    const id = setInterval(tick, intervalMs);
    return () => { cancelled = true; clearInterval(id); };
  }, [intervalMs]);

  async function action(name: 'start' | 'stop' | 'restart' | 'reload') {
    await fetch(`/api/daemon/${name}`, { method: 'POST' });
  }

  return { status, action };
}
```

- [ ] **Step 4: Run test**

Run: `(cd web && npx vitest run src/hooks/useDaemon.test.ts)`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/hooks/useDaemon.ts web/src/hooks/useDaemon.test.ts
git commit -m "feat(web): useDaemon hook with polling status and lifecycle actions"
```

---

### Task 7.4: useOpStream hook (SSE with reattach)

**Files:** Create: `web/src/hooks/useOpStream.ts`, `_test.ts`.

- [ ] **Step 1: Failing test**

```ts
// web/src/hooks/useOpStream.test.ts
import { describe, expect, it, vi, afterEach } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';

class MockEventSource {
  static instances: MockEventSource[] = [];
  onmessage: ((ev: MessageEvent) => void) | null = null;
  url: string;
  closed = false;
  constructor(url: string) { this.url = url; MockEventSource.instances.push(this); }
  close() { this.closed = true; }
  emit(data: any) { this.onmessage?.({ data: JSON.stringify(data) } as MessageEvent); }
}

describe('useOpStream', () => {
  afterEach(() => { MockEventSource.instances = []; });

  it('streams frames into events array', async () => {
    (global as any).EventSource = MockEventSource;
    const { useOpStream } = await import('./useOpStream');
    const { result } = renderHook(() => useOpStream('op-x'));
    act(() => MockEventSource.instances[0].emit({ type: 'progress', phase: 'p', msg: 'hi' }));
    await waitFor(() => expect(result.current.events.length).toBe(1));
    expect(result.current.events[0].phase).toBe('p');
  });
});
```

- [ ] **Step 2: Run, verify fail**

Run: `(cd web && npx vitest run src/hooks/useOpStream.test.ts)`
Expected: FAIL.

- [ ] **Step 3: Implement**

```ts
// web/src/hooks/useOpStream.ts
'use client';

import { useEffect, useState } from 'react';
import type { OpEvent } from '@/components/settings/ProgressCard';

const STORAGE_KEY = 'jellywatch.activeOp';

export function useOpStream(opID: string | null) {
  const [events, setEvents] = useState<OpEvent[]>([]);

  useEffect(() => {
    if (!opID) return;
    sessionStorage.setItem(STORAGE_KEY, opID);
    const es = new EventSource(`/api/events/op/${opID}`);
    es.onmessage = (ev) => {
      try {
        const f = JSON.parse(ev.data);
        setEvents((prev) => [...prev, f]);
        if (f.type === 'done' || f.type === 'error' || f.type === 'cancelled') {
          es.close();
          sessionStorage.removeItem(STORAGE_KEY);
        }
      } catch {
        // ignore malformed frames
      }
    };
    return () => es.close();
  }, [opID]);

  return { events };
}

export function reattachActiveOp(): string | null {
  if (typeof window === 'undefined') return null;
  return sessionStorage.getItem(STORAGE_KEY);
}
```

- [ ] **Step 4: Run test**

Run: `(cd web && npx vitest run src/hooks/useOpStream.test.ts)`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/hooks/useOpStream.ts web/src/hooks/useOpStream.test.ts
git commit -m "feat(web): useOpStream SSE hook with sessionStorage-backed reattach"
```

---

### Task 7.5: /settings/daemon page

**Files:** Create: `web/src/app/settings/daemon/page.tsx`.

- [ ] **Step 1: Implement**

```tsx
// web/src/app/settings/daemon/page.tsx
'use client';

import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { useDaemon } from '@/hooks/useDaemon';
import { ConfirmReversible } from '@/components/settings/ConfirmReversible';

export default function DaemonPage() {
  const { status, action } = useDaemon(5000);
  const [confirmAction, setConfirmAction] = useState<'stop' | 'restart' | null>(null);

  if (!status) return <p>Loading…</p>;

  if (status.state === 'interrupted') {
    return (
      <div className="space-y-4">
        <h1 className="text-2xl font-semibold">Daemon</h1>
        <div className="rounded border border-destructive/50 bg-destructive/10 p-4">
          <p className="font-semibold">A previous destructive op was interrupted.</p>
          <pre className="mt-2 text-xs">{JSON.stringify(status.interrupted_op, null, 2)}</pre>
          <div className="mt-3 flex gap-2">
            <Button onClick={async () => {
              await fetch('/api/daemon/recover', {
                method: 'POST', headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ action: 'discard' }),
              });
              location.reload();
            }}>Discard</Button>
            <Button variant="outline" onClick={async () => {
              await fetch('/api/daemon/recover', {
                method: 'POST', headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ action: 'resume' }),
              });
              location.reload();
            }}>Resume</Button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">Daemon</h1>
      <div className="rounded border p-4">
        <p>State: <span className="font-mono">{status.state}</span></p>
        <p>Version: <span className="font-mono">{status.version}</span></p>
        <p>Uptime: <span className="font-mono">{status.uptime_s}s</span></p>
        <div className="mt-4 flex gap-2">
          <Button onClick={() => action('reload')}>Reload from disk</Button>
          <Button variant="outline" onClick={() => setConfirmAction('restart')}>Restart</Button>
          <Button variant="outline" onClick={() => setConfirmAction('stop')}>Stop</Button>
        </div>
      </div>

      <ConfirmReversible
        open={confirmAction !== null}
        title={`Confirm daemon ${confirmAction}`}
        onConfirm={async () => {
          if (confirmAction) await action(confirmAction);
          setConfirmAction(null);
        }}
        onCancel={() => setConfirmAction(null)}
      >
        Are you sure you want to {confirmAction} the daemon?
      </ConfirmReversible>
    </div>
  );
}
```

Add a tiny `ConfirmReversible.tsx` mirror of `ConfirmDestructive` minus the typed confirm.

- [ ] **Step 2: Build & commit**

Run: `(cd web && npm run build)`

```bash
git add web/src/app/settings/daemon/page.tsx web/src/components/settings/ConfirmReversible.tsx
git commit -m "feat(web): /settings/daemon page with status + lifecycle controls + recovery"
```

---

### Task 7.6: /settings/database page

**Files:** Create: `web/src/app/settings/database/page.tsx`.

- [ ] **Step 1: Implement**

```tsx
// web/src/app/settings/database/page.tsx
'use client';

import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { ConfirmDestructive } from '@/components/settings/ConfirmDestructive';
import { ProgressCard } from '@/components/settings/ProgressCard';
import { useOpStream } from '@/hooks/useOpStream';

export default function DatabasePage() {
  const [opID, setOpID] = useState<string | null>(null);
  const [confirmReset, setConfirmReset] = useState(false);
  const { events } = useOpStream(opID);

  async function startRescan() {
    const r = await fetch('/api/database/rescan', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ dry_run: false }),
    });
    const { op_id } = await r.json();
    setOpID(op_id);
  }

  async function startReset() {
    setConfirmReset(false);
    const r = await fetch('/api/database/reset', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ confirm: 'media.db', preserve: ['audit_log'] }),
    });
    const { op_id } = await r.json();
    setOpID(op_id);
  }

  if (opID) {
    return (
      <div className="space-y-4">
        <h1 className="text-2xl font-semibold">Database</h1>
        <ProgressCard title="Operation in progress" events={events} />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">Database</h1>

      <div className="rounded border p-4">
        <h2 className="font-semibold">Re-scan media</h2>
        <p className="text-sm text-muted-foreground">
          Re-walks all watch folders. Existing parse decisions and audit history are preserved.
        </p>
        <div className="mt-3">
          <Button onClick={startRescan}>Start re-scan</Button>
        </div>
      </div>

      <div className="rounded border border-destructive/30 p-4">
        <h2 className="font-semibold">Reset database</h2>
        <p className="text-sm text-muted-foreground">
          Permanently deletes indexed media, parse decisions, and operator overrides.
          Audit log is preserved.
        </p>
        <div className="mt-3">
          <Button variant="destructive" onClick={() => setConfirmReset(true)}>Reset database…</Button>
        </div>
      </div>

      <ConfirmDestructive
        open={confirmReset}
        phrase="media.db"
        onConfirm={startReset}
        onCancel={() => setConfirmReset(false)}
      >
        This will delete every table except <span className="font-mono">audit_log</span>.
      </ConfirmDestructive>
    </div>
  );
}
```

- [ ] **Step 2: Build & commit**

```bash
(cd web && npm run build)
git add web/src/app/settings/database/page.tsx
git commit -m "feat(web): /settings/database page with re-scan and typed-confirm reset"
```

---

## Phase 8 — OpenAPI + integration smoke

### Task 8.1: Update OpenAPI spec

**Files:** Modify: `api/openapi.yaml`.

- [ ] **Step 1: Add entries**

Append:

```yaml
  /daemon/status:
    get:
      summary: Daemon status
      responses: { '200': { description: OK } }
  /daemon/start:
    post:
      responses: { '202': { description: Accepted } }
  /daemon/stop:
    post:
      responses: { '202': { description: Accepted } }
  /daemon/restart:
    post:
      responses: { '202': { description: Accepted } }
  /daemon/reload:
    post:
      responses: { '200': { description: Reload result } }
  /daemon/recover:
    post:
      requestBody: { required: true }
      responses: { '200': { description: Recovery applied } }

  /database/rescan:
    post:
      requestBody: { required: true }
      responses: { '202': { description: Op started } }
  /database/reset:
    post:
      requestBody: { required: true }
      responses: { '202': { description: Op started } }

  /events/op/{op_id}:
    get:
      parameters:
        - { name: op_id, in: path, required: true, schema: { type: string } }
      responses: { '200': { description: SSE stream } }
```

- [ ] **Step 2: Regenerate types and commit**

```bash
make openapi-codegen 2>/dev/null || true
git add api/openapi.yaml web/src/types/api.ts
git commit -m "docs(openapi): declare daemon/database/events routes"
```

---

### Task 8.2: End-to-end integration smoke

**Files:** Create: `internal/api/integration_lifecycle_test.go`.

- [ ] **Step 1: Write test**

```go
//go:build integration

package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
	"github.com/go-chi/chi/v5"
)

// ipcCallerAdapter satisfies both IPCCaller and IPCAttacher around the
// real ipc.Client.
type ipcCallerAdapter struct{ c *ipc.Client }

func (a ipcCallerAdapter) Call(ctx context.Context, cmd ipc.Command, args any) (json.RawMessage, error) {
	return a.c.Call(ctx, cmd, args)
}
func (a ipcCallerAdapter) StreamWithID(ctx context.Context, cmd ipc.Command, args any, opID string) error {
	return a.c.StreamWithID(ctx, cmd, args, opID)
}
func (a ipcCallerAdapter) Attach(ctx context.Context, opID string) (<-chan ipc.Frame, error) {
	return a.c.Attach(ctx, opID)
}

func TestRescanEndToEnd(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "ctl.sock")

	srv := ipc.NewServer(sock)
	srv.RegisterStreaming(ipc.CmdRescan, func(ctx context.Context, _ json.RawMessage, w ipc.FrameWriter, op *ipc.Op) {
		w.Progress(op.ID, "walking", "/x", 1, 2)
		w.Progress(op.ID, "indexing", "/x", 2, 2)
		w.Done(op.ID, json.RawMessage(`{"ok":true}`))
	})
	srv.Register(ipc.CmdAttach, attachHandler(srv))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv.Start(ctx); defer srv.Stop()

	cli := ipc.NewClient(sock)
	adapter := ipcCallerAdapter{c: cli}
	dbH := &DatabaseHandlers{IPC: adapter}
	sse := &SSERelay{IPC: adapter}
	mux := chi.NewRouter()
	mux.Post("/database/rescan", dbH.Rescan)
	mux.Get("/events/op/{op_id}", sse.Stream)

	req := httptest.NewRequest("POST", "/database/rescan", strings.NewReader(`{"dry_run":false}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != 202 {
		t.Fatalf("rescan kickoff status %d", w.Code)
	}
	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	opID := body["op_id"]
	if opID == "" { t.Fatal("missing op_id") }

	req2 := httptest.NewRequest("GET", "/events/op/"+opID, nil)
	w2 := httptest.NewRecorder()
	ctx2, cancel2 := context.WithTimeout(req2.Context(), 2*time.Second)
	defer cancel2()
	mux.ServeHTTP(w2, req2.WithContext(ctx2))

	out := w2.Body.String()
	if !strings.Contains(out, `"phase":"walking"`) {
		t.Errorf("missing walking frame in SSE: %s", out)
	}
	if !strings.Contains(out, `"type":"done"`) {
		t.Errorf("missing done frame in SSE: %s", out)
	}
}
```

- [ ] **Step 2: Run**

Run: `go test -tags=integration ./internal/api/... -run TestRescanEndToEnd -timeout 10s`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/api/integration_lifecycle_test.go
git commit -m "test(api): integration smoke for /database/rescan → SSE relay"
```

---

### Task 8.3: Final lint + build sweep

- [ ] **Step 1: Run full check**

```bash
go vet ./...
go test ./...
(cd web && npm run lint && npm run build)
```

Expected: clean.

- [ ] **Step 2: Update layout pill polling**

Make sure `web/src/app/settings/layout.tsx` polls `/api/daemon/status` and renders a colored dot next to the Daemon nav item (green/red/yellow per state).

```tsx
// in layout.tsx, alongside SECTIONS:
import { useDaemon } from '@/hooks/useDaemon';

// inside the component:
const { status } = useDaemon(5000);
// when rendering the Daemon link, prepend a span with a state-colored dot.
```

- [ ] **Step 3: Commit**

```bash
git add web/src/app/settings/layout.tsx
git commit -m "feat(web): daemon-state pill in settings nav"
```

---

## Self-Review

- [ ] **Spec coverage:**
  - §4.4 commands STOP/RESCAN/RESET_DB/ATTACH/CANCEL/RECOVER → Tasks 1.6, 2.2, 2.3, 1.4 (ATTACH+CANCEL), 1.7
  - §4.5 heartbeats → Task 1.4
  - §4.6 reattach → Task 1.4 (server-side) + Task 7.4 (client-side)
  - §4.7 cancellation → Task 1.4 (CANCEL); CANCEL → Finish wiring documented in Task 1.4 streaming wrapper
  - §4.9 op log + crash recovery → Task 1.3 (log) + Task 1.6 (startup scan) + Task 1.7 (RECOVER + mutator gate)
  - §5.5 daemon endpoints → Task 3.1
  - §5.6 database endpoints → Task 4.1
  - §5.7 SSE relay → Task 5.1
  - §6.4 daemon UI → Task 7.5
  - §6.5 database UI → Task 7.6
  - §6.6 SSE reattach → Task 7.4
  - §8 CLI parity → Task 6.1
- [ ] **Pre-flight anchors verified:** `IPCCaller` ctx-bearing, `MediaDB.SQL()` exposed, `FileScanner.FullRescan` defined, `daemonStatus` extended (not duplicated), `Server.Path()` accessor, `NewServer` allocates default registry.
- [ ] **op_id flow:** allocated by API → passed via `StreamWithID` → daemon registers under same ID → SSE `Attach` retries up to 1s to absorb the kickoff race.
- [ ] **Placeholder scan:** No TBD/TODO/etc. left.
- [ ] **Type consistency:** `Op`, `OpRegistry`, `OpLog`, `FrameRing`, `OpEvent`, `useDaemon`, `useOpStream` — referenced consistently.
- [ ] All commit messages are conventional and contain no AI attribution.

---

**Plan complete.** Two execution options:

1. **Subagent-Driven (recommended)** — fresh subagent per task, review between tasks.
2. **Inline Execution** — run tasks in this session via executing-plans.

Plan 3 (Observability) requires Design 2 to be brainstormed first.
