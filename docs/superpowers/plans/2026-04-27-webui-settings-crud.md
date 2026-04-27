# WebUI Settings CRUD — Implementation Plan (Plan 1 of 3)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every section of `~/.config/jellywatch/config.toml` editable from the webui, with hot reload over IPC, atomic write safety, validation/connection-test affordances, and route-per-section UI.

**Architecture:** `jellyweb` becomes the only runtime config writer (atomic write + `flock`). Web saves use one canonical pipeline: re-read config, apply change, write a backup, atomically write the candidate config, trigger daemon `RELOAD`, and restore the previous config if reload fails. New `internal/daemon/ipc` package establishes a Unix-socket control plane carrying `STATUS` and `RELOAD` only (other commands ship in Plan 2). Each daemon subsystem implements a two-phase `Reloadable` interface so reload either fully applies or fully rolls back. Frontend gains a left-rail settings layout with one route per section and a shared `SettingsForm` save pipeline.

**Tech Stack:** Go (chi router, gorilla/websocket replaced by raw net for IPC), TOML via existing `internal/config`, Next.js App Router, React Query, shadcn/ui, Vitest + msw for tests.

**Spec:** `docs/superpowers/specs/2026-04-27-webui-control-plane-design.md` §3, §4 (subset: STATUS, RELOAD only), §5.1–§5.4, §6.

---

## File Structure

### Backend (NEW)
- `internal/daemon/ipc/protocol.go` — message types, command names, error codes
- `internal/daemon/ipc/server.go` — listener, accept loop, dispatch
- `internal/daemon/ipc/client.go` — client (used by jellyweb + future CLI)
- `internal/daemon/ipc/peercred_linux.go` — `SO_PEERCRED` UID check
- `internal/daemon/reload/supervisor.go` — two-phase prepare/commit runner
- `internal/daemon/reload/registry.go` — global registration of `Reloadable` implementations
- `internal/daemon/reload/scanner_reloadable.go` — adapter for the scanner subsystem
- `internal/daemon/reload/ai_reloadable.go` — adapter for AI matcher
- `internal/daemon/reload/logging_reloadable.go` — adapter for logging level
- `internal/api/settings_handlers.go` — generic per-section read/write
- `internal/api/config_save_pipeline.go` — canonical save + reload + restore helper
- `internal/api/paths_handlers.go` — array CRUD for paths/libraries
- `internal/api/test_handlers.go` — connection-test endpoints
- `internal/api/preflight_handlers.go` — `/paths/preflight`
- `internal/config/save.go` — atomic write + `flock`
- `internal/config/sections.go` — section registry, mapping name → struct field

### Backend (EDIT)
- `cmd/jellyweb/main.go` — construct IPC client and pass it into API server
- `cmd/jellywatchd/main.go` — start IPC server; register reloadables
- `internal/api/server.go` — mount new routes
- `internal/config/config.go` — add `Save()` flock+atomic; `secret:"true"` struct tags
- `api/openapi.yaml` — declare new routes

### Frontend (NEW)
- `web/src/app/settings/layout.tsx` — left-rail nav + overview poller
- `web/src/app/settings/paths/page.tsx`
- `web/src/app/settings/libraries/page.tsx`
- `web/src/app/settings/sonarr/page.tsx`
- `web/src/app/settings/radarr/page.tsx`
- `web/src/app/settings/jellyfin/page.tsx`
- `web/src/app/settings/options/page.tsx`
- `web/src/app/settings/logging/page.tsx`
- `web/src/app/settings/permissions/page.tsx`
- `web/src/components/settings/SettingsForm.tsx`
- `web/src/components/settings/PathListEditor.tsx`
- `web/src/components/settings/SecretField.tsx`
- `web/src/components/settings/TestConnectionButton.tsx`
- `web/src/components/settings/SubsystemReloadStatus.tsx`
- `web/src/hooks/usePathPreflight.ts`

### Frontend (EDIT)
- `web/src/app/settings/page.tsx` — overview cards (replaces current monolithic page)
- `web/src/app/settings/ai/page.tsx` — adopt SettingsForm
- `web/src/hooks/useSettings.ts` — per-section CRUD hooks
- `web/src/lib/api/client.ts` — typed clients for new routes

---

## Phase 1 — IPC foundation

### Task 1.1: Define IPC protocol types

**Files:** Create: `internal/daemon/ipc/protocol.go`. Test: `internal/daemon/ipc/protocol_test.go`.

- [ ] **Step 1: Write the failing test**

```go
// internal/daemon/ipc/protocol_test.go
package ipc

import (
	"encoding/json"
	"testing"
)

func TestRequestRoundtrip(t *testing.T) {
	r := Request{V: 1, ID: "abc", Cmd: CmdStatus}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var got Request
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got != r {
		t.Errorf("roundtrip mismatch: %+v != %+v", got, r)
	}
}

func TestErrorCodeIsString(t *testing.T) {
	if string(ErrBusy) != "BUSY" {
		t.Errorf("ErrBusy = %q, want BUSY", ErrBusy)
	}
}
```

- [ ] **Step 2: Run test, verify fail**

Run: `go test ./internal/daemon/ipc/...`
Expected: FAIL — package does not compile (types undefined).

- [ ] **Step 3: Implement protocol types**

```go
// internal/daemon/ipc/protocol.go
package ipc

import "encoding/json"

const ProtocolVersion = 1

type Command string

const (
	CmdStatus Command = "STATUS"
	CmdReload Command = "RELOAD"
)

type Request struct {
	V    int             `json:"v"`
	ID   string          `json:"id"`
	Cmd  Command         `json:"cmd"`
	Args json.RawMessage `json:"args,omitempty"`
}

type FrameType string

const (
	FrameResult    FrameType = "result"
	FrameProgress  FrameType = "progress"
	FrameHeartbeat FrameType = "heartbeat"
	FrameDone      FrameType = "done"
	FrameError     FrameType = "error"
)

type Frame struct {
	ID    string          `json:"id"`
	Type  FrameType       `json:"type"`
	Code  ErrorCode       `json:"code,omitempty"`
	Msg   string          `json:"msg,omitempty"`
	Phase string          `json:"phase,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
	TS    int64           `json:"ts,omitempty"`
}

type ErrorCode string

const (
	ErrBusy            ErrorCode = "BUSY"
	ErrBadRequest      ErrorCode = "BAD_REQUEST"
	ErrVersionMismatch ErrorCode = "VERSION_MISMATCH"
	ErrUnauthorized    ErrorCode = "UNAUTHORIZED"
	ErrNotFound        ErrorCode = "NOT_FOUND"
	ErrConflict        ErrorCode = "CONFLICT"
	ErrInterrupted    ErrorCode = "INTERRUPTED"
	ErrCancelled       ErrorCode = "CANCELLED"
	ErrTimeout         ErrorCode = "TIMEOUT"
	ErrInternal        ErrorCode = "INTERNAL"
	ErrNotImplemented  ErrorCode = "NOT_IMPLEMENTED"
)
```

- [ ] **Step 4: Run test, verify pass**

Run: `go test ./internal/daemon/ipc/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/ipc/protocol.go internal/daemon/ipc/protocol_test.go
git commit -m "feat(ipc): protocol types and error taxonomy"
```

---

### Task 1.2: SO_PEERCRED UID check (Linux)

**Files:** Create: `internal/daemon/ipc/peercred_linux.go`. Test: `internal/daemon/ipc/peercred_linux_test.go`.

- [ ] **Step 1: Write the failing test**

```go
// internal/daemon/ipc/peercred_linux_test.go
//go:build linux

package ipc

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestPeerUIDMatchesSelf(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	go func() {
		c, _ := net.Dial("unix", sockPath)
		defer c.Close()
	}()

	conn, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	uid, err := peerUID(conn.(*net.UnixConn))
	if err != nil {
		t.Fatal(err)
	}
	if uid != os.Getuid() {
		t.Errorf("peer UID %d != self UID %d", uid, os.Getuid())
	}
}
```

- [ ] **Step 2: Run test, verify fail**

Run: `go test ./internal/daemon/ipc/... -run TestPeerUID`
Expected: FAIL — `peerUID` undefined.

- [ ] **Step 3: Implement**

```go
// internal/daemon/ipc/peercred_linux.go
//go:build linux

package ipc

import (
	"errors"
	"net"
	"syscall"
)

// peerUID returns the UID of the peer connected via a Unix domain socket.
// Linux-only via SO_PEERCRED.
func peerUID(c *net.UnixConn) (int, error) {
	rc, err := c.SyscallConn()
	if err != nil {
		return -1, err
	}
	var ucred *syscall.Ucred
	var sockErr error
	if err := rc.Control(func(fd uintptr) {
		ucred, sockErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return -1, err
	}
	if sockErr != nil {
		return -1, sockErr
	}
	if ucred == nil {
		return -1, errors.New("ucred unavailable")
	}
	return int(ucred.Uid), nil
}
```

- [ ] **Step 4: Run test**

Run: `go test ./internal/daemon/ipc/... -run TestPeerUID`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/ipc/peercred_linux.go internal/daemon/ipc/peercred_linux_test.go
git commit -m "feat(ipc): SO_PEERCRED uid check on accepted connections"
```

---

### Task 1.3: IPC server skeleton (listener, accept loop, dispatch)

**Files:** Create: `internal/daemon/ipc/server.go`. Test: `internal/daemon/ipc/server_test.go`.

- [ ] **Step 1: Write the failing test**

```go
// internal/daemon/ipc/server_test.go
package ipc

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"
)

func TestServerHandlesStatus(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "control.sock")

	srv := NewServer(sockPath)
	srv.Register(CmdStatus, func(ctx context.Context, req Request, w FrameWriter) {
		w.Result(req.ID, json.RawMessage(`{"state":"running"}`))
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	c, err := net.DialTimeout("unix", sockPath, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if _, err := c.Write([]byte(`{"v":1,"id":"r1","cmd":"STATUS"}` + "\n")); err != nil {
		t.Fatal(err)
	}

	dec := json.NewDecoder(c)
	var f Frame
	if err := dec.Decode(&f); err != nil {
		t.Fatal(err)
	}
	if f.ID != "r1" || f.Type != FrameResult {
		t.Errorf("unexpected frame: %+v", f)
	}
}
```

- [ ] **Step 2: Run test, verify fail**

Run: `go test ./internal/daemon/ipc/... -run TestServerHandlesStatus`
Expected: FAIL — `NewServer`, `FrameWriter` undefined.

- [ ] **Step 3: Implement**

```go
// internal/daemon/ipc/server.go
package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"sync"
	"time"
)

const (
	maxFrameBytes  = 64 * 1024
	idleTimeout    = 5 * time.Minute
	heartbeatEvery = 5 * time.Second
)

type Handler func(ctx context.Context, req Request, w FrameWriter)

type FrameWriter interface {
	Result(id string, data json.RawMessage)
	Progress(id string, phase, msg string, current, total int)
	Done(id string, data json.RawMessage)
	Error(id string, code ErrorCode, msg string)
}

type Server struct {
	path            string
	handlers        map[Command]Handler
	allowedPeerUIDs map[int]struct{}
	listener        *net.UnixListener
	wg              sync.WaitGroup
	mu              sync.Mutex
	stopped         bool
}

func NewServer(path string) *Server {
	return &Server{
		path:            path,
		handlers:        map[Command]Handler{},
		allowedPeerUIDs: map[int]struct{}{os.Getuid(): {}},
	}
}

func (s *Server) Register(c Command, h Handler) { s.handlers[c] = h }

// AddAllowedPeerUID permits a specific local UID to connect. This is used for
// root/system daemon mode where jellyweb runs as the installing user.
func (s *Server) AddAllowedPeerUID(uid int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.allowedPeerUIDs[uid] = struct{}{}
}

func (s *Server) Start(ctx context.Context) error {
	// stale socket cleanup
	if _, err := os.Stat(s.path); err == nil {
		if c, derr := net.DialTimeout("unix", s.path, 200*time.Millisecond); derr == nil {
			c.Close()
			return errors.New("socket already in use by a live peer")
		}
		_ = os.Remove(s.path)
	}
	addr, err := net.ResolveUnixAddr("unix", s.path)
	if err != nil {
		return err
	}
	l, err := net.ListenUnix("unix", addr)
	if err != nil {
		return err
	}
	if err := os.Chmod(s.path, 0600); err != nil {
		l.Close()
		return err
	}
	s.listener = l

	s.wg.Add(1)
	go s.acceptLoop(ctx)
	return nil
}

func (s *Server) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	s.mu.Unlock()
	if s.listener != nil {
		s.listener.Close()
	}
	s.wg.Wait()
	_ = os.Remove(s.path)
}

func (s *Server) acceptLoop(ctx context.Context) {
	defer s.wg.Done()
	for {
		s.listener.SetDeadline(time.Now().Add(time.Second))
		c, err := s.listener.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return
		}
		s.wg.Add(1)
		go s.handleConn(ctx, c)
	}
}

func (s *Server) handleConn(ctx context.Context, c *net.UnixConn) {
	defer s.wg.Done()
	defer c.Close()

	uid, err := peerUID(c)
	if err != nil || !s.peerAllowed(uid) {
		return
	}

	w := newFrameWriter(c)
	r := bufio.NewReaderSize(c, maxFrameBytes)
	for {
		c.SetReadDeadline(time.Now().Add(idleTimeout))
		line, err := r.ReadBytes('\n')
		if err != nil {
			return
		}
		if len(line) > maxFrameBytes {
			return
		}
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			w.Error("", ErrBadRequest, "invalid json")
			continue
		}
		if req.V != ProtocolVersion {
			w.Error(req.ID, ErrVersionMismatch, "unsupported protocol version")
			continue
		}
		h, ok := s.handlers[req.Cmd]
		if !ok {
			w.Error(req.ID, ErrNotImplemented, string(req.Cmd))
			continue
		}
		go h(ctx, req, w)
	}
}

func (s *Server) peerAllowed(uid int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.allowedPeerUIDs[uid]
	return ok
}

type frameWriter struct {
	mu sync.Mutex
	c  net.Conn
}

func newFrameWriter(c net.Conn) *frameWriter { return &frameWriter{c: c} }

func (w *frameWriter) write(f Frame) {
	w.mu.Lock()
	defer w.mu.Unlock()
	enc := json.NewEncoder(w.c)
	_ = enc.Encode(f)
}

func (w *frameWriter) Result(id string, data json.RawMessage) {
	w.write(Frame{ID: id, Type: FrameResult, Data: data})
}
func (w *frameWriter) Progress(id, phase, msg string, current, total int) {
	d, _ := json.Marshal(map[string]int{"current": current, "total": total})
	w.write(Frame{ID: id, Type: FrameProgress, Phase: phase, Msg: msg, Data: d})
}
func (w *frameWriter) Done(id string, data json.RawMessage) {
	w.write(Frame{ID: id, Type: FrameDone, Data: data})
}
func (w *frameWriter) Error(id string, code ErrorCode, msg string) {
	w.write(Frame{ID: id, Type: FrameError, Code: code, Msg: msg})
}
```

- [ ] **Step 4: Run test**

Run: `go test ./internal/daemon/ipc/... -run TestServerHandlesStatus`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/ipc/server.go internal/daemon/ipc/server_test.go
git commit -m "feat(ipc): unix-socket server with frame-writer dispatch"
```

---

### Task 1.4: IPC client (used by jellyweb + future CLI)

**Files:** Create: `internal/daemon/ipc/client.go`, `internal/daemon/ipc/client_test.go`.

- [ ] **Step 1: Write the failing test**

```go
// internal/daemon/ipc/client_test.go
package ipc

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestClientCallStatus(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "ctl.sock")
	srv := NewServer(sock)
	srv.Register(CmdStatus, func(ctx context.Context, req Request, w FrameWriter) {
		w.Result(req.ID, json.RawMessage(`{"state":"running"}`))
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	cli := NewClient(sock)
	res, err := cli.Call(ctx, CmdStatus, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal(res, &got); err != nil {
		t.Fatal(err)
	}
	if got["state"] != "running" {
		t.Errorf("state = %q", got["state"])
	}
}
```

- [ ] **Step 2: Run test, verify fail**

Run: `go test ./internal/daemon/ipc/... -run TestClientCallStatus`
Expected: FAIL — `NewClient` undefined.

- [ ] **Step 3: Implement**

```go
// internal/daemon/ipc/client.go
package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/google/uuid"
)

type Client struct {
	path string
}

func NewClient(path string) *Client { return &Client{path: path} }

// Call performs a single-request, single-response RPC. For streaming commands
// use Stream.
func (c *Client) Call(ctx context.Context, cmd Command, args any) (json.RawMessage, error) {
	conn, err := net.DialTimeout("unix", c.path, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("dial ipc: %w", err)
	}
	defer conn.Close()

	id := uuid.NewString()
	req := Request{V: ProtocolVersion, ID: id, Cmd: cmd}
	if args != nil {
		b, err := json.Marshal(args)
		if err != nil {
			return nil, err
		}
		req.Args = b
	}
	enc := json.NewEncoder(conn)
	if err := enc.Encode(req); err != nil {
		return nil, err
	}

	dec := json.NewDecoder(bufio.NewReader(conn))
	deadline, hasDeadline := ctx.Deadline()
	if !hasDeadline {
		deadline = time.Now().Add(10 * time.Second)
	}
	conn.SetReadDeadline(deadline)
	for {
		var f Frame
		if err := dec.Decode(&f); err != nil {
			return nil, err
		}
		switch f.Type {
		case FrameResult:
			return f.Data, nil
		case FrameError:
			return nil, fmt.Errorf("ipc error %s: %s", f.Code, f.Msg)
		case FrameHeartbeat:
			continue
		default:
			return nil, errors.New("unexpected frame type for Call")
		}
	}
}
```

- [ ] **Step 4: Run test**

Run: `go test ./internal/daemon/ipc/... -run TestClientCallStatus`
Expected: PASS.

- [ ] **Step 5: Add `github.com/google/uuid` if missing**

Run: `go mod tidy && go build ./...`
Expected: clean build.

- [ ] **Step 6: Commit**

```bash
git add internal/daemon/ipc/client.go internal/daemon/ipc/client_test.go go.mod go.sum
git commit -m "feat(ipc): client used by jellyweb and CLI"
```

---

### Task 1.5: Reload supervisor (two-phase prepare/commit)

**Files:** Create: `internal/daemon/reload/supervisor.go`, `internal/daemon/reload/registry.go`, `internal/daemon/reload/supervisor_test.go`.

- [ ] **Step 1: Write the failing test**

```go
// internal/daemon/reload/supervisor_test.go
package reload

import (
	"context"
	"errors"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/config"
)

type fake struct {
	name      string
	prepErr   error
	commitErr error
	committed bool
	rolledBack bool
}

func (f *fake) Name() string { return f.name }
func (f *fake) Prepare(ctx context.Context, oldCfg, newCfg *config.Config) (Commit, Rollback, error) {
	if f.prepErr != nil {
		return nil, nil, f.prepErr
	}
	return func() error {
			if f.commitErr != nil {
				return f.commitErr
			}
			f.committed = true
			return nil
		}, func() {
			f.rolledBack = true
		}, nil
}

func TestSupervisorAllSucceed(t *testing.T) {
	a := &fake{name: "a"}
	b := &fake{name: "b"}
	sup := NewSupervisor()
	sup.Register(a)
	sup.Register(b)
	res := sup.Reload(context.Background(), &config.Config{}, &config.Config{})
	if !res.OK {
		t.Fatalf("expected OK, got %+v", res)
	}
	if !a.committed || !b.committed {
		t.Errorf("subsystems not committed")
	}
}

func TestSupervisorRollbackOnPrepareFailure(t *testing.T) {
	a := &fake{name: "a"}
	b := &fake{name: "b", prepErr: errors.New("nope")}
	sup := NewSupervisor()
	sup.Register(a)
	sup.Register(b)
	res := sup.Reload(context.Background(), &config.Config{}, &config.Config{})
	if res.OK {
		t.Fatal("expected failure")
	}
	if a.committed {
		t.Error("a should not have committed")
	}
	if !a.rolledBack {
		t.Error("a should have rolled back")
	}
}
```

- [ ] **Step 2: Run test, verify fail**

Run: `go test ./internal/daemon/reload/...`
Expected: FAIL — types undefined.

- [ ] **Step 3: Implement**

```go
// internal/daemon/reload/supervisor.go
package reload

import (
	"context"
	"sync"

	"github.com/Nomadcxx/jellywatch/internal/config"
)

type Commit func() error
type Rollback func()

type Reloadable interface {
	Name() string
	Prepare(ctx context.Context, oldCfg, newCfg *config.Config) (Commit, Rollback, error)
}

type Result struct {
	OK       bool
	Reloaded []string
	Failed   []FailedSubsystem
}

type FailedSubsystem struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

type Supervisor struct {
	mu  sync.Mutex
	all []Reloadable
}

func NewSupervisor() *Supervisor { return &Supervisor{} }

func (s *Supervisor) Register(r Reloadable) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.all = append(s.all, r)
}

func (s *Supervisor) Reload(ctx context.Context, oldCfg, newCfg *config.Config) Result {
	s.mu.Lock()
	subs := append([]Reloadable(nil), s.all...)
	s.mu.Unlock()

	type prepared struct {
		name     string
		commit   Commit
		rollback Rollback
	}
	var ok []prepared
	for _, r := range subs {
		commit, rollback, err := r.Prepare(ctx, oldCfg, newCfg)
		if err != nil {
			for i := len(ok) - 1; i >= 0; i-- {
				ok[i].rollback()
			}
			return Result{
				OK:     false,
				Failed: []FailedSubsystem{{Name: r.Name(), Error: err.Error()}},
			}
		}
		ok = append(ok, prepared{name: r.Name(), commit: commit, rollback: rollback})
	}

	res := Result{OK: true}
	for _, p := range ok {
		if err := p.commit(); err != nil {
			res.OK = false
			res.Failed = append(res.Failed, FailedSubsystem{Name: p.name, Error: err.Error()})
			continue
		}
		res.Reloaded = append(res.Reloaded, p.name)
	}
	return res
}
```

```go
// internal/daemon/reload/registry.go
package reload

var Default = NewSupervisor()

func Register(r Reloadable) { Default.Register(r) }
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/daemon/reload/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/reload/
git commit -m "feat(reload): two-phase supervisor with prepare/commit/rollback"
```

---

### Task 1.6: Wire IPC server into daemon main

**Files:** Modify: `cmd/jellywatchd/main.go`. Test: `cmd/jellywatchd/main_test.go` (smoke test).

- [ ] **Step 1: Write smoke test**

```go
// cmd/jellywatchd/ipc_smoke_test.go
package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
)

func TestStatusHandlerReturnsState(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "control.sock")
	srv := ipc.NewServer(sock)
	registerStatusHandler(srv, "0.0.0-test")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	cli := ipc.NewClient(sock)
	res, err := cli.Call(ctx, ipc.CmdStatus, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(res, &got); err != nil {
		t.Fatal(err)
	}
	if got["state"] != "running" {
		t.Errorf("state = %v", got["state"])
	}
	if got["version"] != "0.0.0-test" {
		t.Errorf("version = %v", got["version"])
	}
}
```

- [ ] **Step 2: Run test, verify fail**

Run: `go test ./cmd/jellywatchd/... -run TestStatusHandlerReturnsState`
Expected: FAIL — `registerStatusHandler` undefined.

- [ ] **Step 3: Implement handler registration helper**

Add to `cmd/jellywatchd/main.go`:

```go
import (
	// ... existing imports ...
	"context"
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
	"github.com/Nomadcxx/jellywatch/internal/daemon/reload"
	"github.com/Nomadcxx/jellywatch/internal/paths"
)

var daemonStartTime = time.Now()
var loadedConfig *config.Config

func registerStatusHandler(srv *ipc.Server, version string) {
	srv.Register(ipc.CmdStatus, func(ctx context.Context, req ipc.Request, w ipc.FrameWriter) {
		payload := map[string]any{
			"state":     "running",
			"version":   version,
			"uptime_s":  int(time.Since(daemonStartTime).Seconds()),
		}
		b, _ := json.Marshal(payload)
		w.Result(req.ID, b)
	})
}

func registerReloadHandler(srv *ipc.Server) {
	srv.Register(ipc.CmdReload, func(ctx context.Context, req ipc.Request, w ipc.FrameWriter) {
		// loaded earlier in main; for now reload reads from disk
		oldCfg := loadedConfig // package-level var set in main
		newCfg, err := config.Load()
		if err != nil {
			w.Error(req.ID, ipc.ErrInternal, err.Error())
			return
		}
		res := reload.Default.Reload(ctx, oldCfg, newCfg)
		if res.OK {
			loadedConfig = newCfg
		}
		b, _ := json.Marshal(map[string]any{
			"ok":        res.OK,
			"reloaded":  res.Reloaded,
			"failed":    res.Failed,
		})
		w.Result(req.ID, b)
	})
}

func startIPCServer(ctx context.Context, version string) (*ipc.Server, error) {
	configDir, err := paths.JellyWatchDir()
	if err != nil {
		return nil, err
	}
	sockPath := filepath.Join(configDir, "control.sock")
	srv := ipc.NewServer(sockPath)
	registerStatusHandler(srv, version)
	registerReloadHandler(srv)
	return srv, srv.Start(ctx)
}
```

In `runDaemon`, set `loadedConfig = cfg` immediately after config load. Start the IPC server after `ctx, cancel := context.WithCancel(context.Background())` is created and before the watcher/health/periodic goroutines start:

```go
ipcSrv, err := startIPCServer(ctx, Version)
if err != nil {
	log.Fatalf("ipc start: %v", err)
}
defer ipcSrv.Stop()
```

- [ ] **Step 4: Run test**

Run: `go test ./cmd/jellywatchd/... -run TestStatusHandlerReturnsState && go build ./...`
Expected: PASS, clean build.

- [ ] **Step 5: Commit**

```bash
git add cmd/jellywatchd/
git commit -m "feat(daemon): start IPC server and register STATUS+RELOAD handlers"
```

---

### Task 1.7: Logging Reloadable (simplest subsystem first)

**Files:** Create: `internal/daemon/reload/logging_reloadable.go`, `internal/daemon/reload/logging_reloadable_test.go`.

- [ ] **Step 1: Write failing test**

```go
// internal/daemon/reload/logging_reloadable_test.go
package reload

import (
	"context"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/config"
)

func TestLoggingReloadableSwapsLevel(t *testing.T) {
	current := "info"
	target := &current
	r := NewLoggingReloadable(target)
	old := &config.Config{Logging: config.LoggingConfig{Level: "info"}}
	new := &config.Config{Logging: config.LoggingConfig{Level: "debug"}}
	commit, _, err := r.Prepare(context.Background(), old, new)
	if err != nil {
		t.Fatal(err)
	}
	if current != "info" {
		t.Errorf("level swapped before commit: %q", current)
	}
	if err := commit(); err != nil {
		t.Fatal(err)
	}
	if current != "debug" {
		t.Errorf("level after commit = %q", current)
	}
}
```

- [ ] **Step 2: Run test, verify fail**

Run: `go test ./internal/daemon/reload/... -run TestLoggingReloadable`
Expected: FAIL.

- [ ] **Step 3: Implement**

```go
// internal/daemon/reload/logging_reloadable.go
package reload

import (
	"context"
	"errors"

	"github.com/Nomadcxx/jellywatch/internal/config"
)

type loggingReloadable struct {
	level *string
}

func NewLoggingReloadable(level *string) Reloadable {
	return &loggingReloadable{level: level}
}

func (r *loggingReloadable) Name() string { return "logging" }

func (r *loggingReloadable) Prepare(ctx context.Context, oldCfg, newCfg *config.Config) (Commit, Rollback, error) {
	target := newCfg.Logging.Level
	switch target {
	case "trace", "debug", "info", "warn", "error":
	default:
		return nil, nil, errors.New("invalid log level: " + target)
	}
	prev := *r.level
	commit := func() error { *r.level = target; return nil }
	rollback := func() { *r.level = prev }
	return commit, rollback, nil
}
```

- [ ] **Step 4: Run test**

Run: `go test ./internal/daemon/reload/...`
Expected: PASS.

- [ ] **Step 5: Wire into daemon main**

Edit `cmd/jellywatchd/main.go`, in the same place IPC starts, register the logging reloadable:

```go
reload.Register(reload.NewLoggingReloadable(&loadedConfig.Logging.Level))
```

(`loadedConfig` is the package-level config var.)

- [ ] **Step 6: Build**

Run: `go build ./...`
Expected: clean build.

- [ ] **Step 7: Commit**

```bash
git add internal/daemon/reload/logging_reloadable.go internal/daemon/reload/logging_reloadable_test.go cmd/jellywatchd/main.go
git commit -m "feat(reload): logging-level reloadable wired into daemon"
```

---

### Task 1.8: Scanner Reloadable (rebuild watcher set)

**Files:** Create: `internal/daemon/reload/scanner_reloadable.go`, `internal/daemon/reload/scanner_reloadable_test.go`.

- [ ] **Step 1: Failing test**

```go
// internal/daemon/reload/scanner_reloadable_test.go
package reload

import (
	"context"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/config"
)

type fakeScanner struct {
	current []string
}

func (f *fakeScanner) ReplaceWatchPaths(paths []string) error {
	f.current = paths
	return nil
}

func TestScannerReloadableRebuildsPaths(t *testing.T) {
	s := &fakeScanner{current: []string{"/old"}}
	r := NewScannerReloadable(s)
	old := &config.Config{Watch: config.WatchConfig{Movies: []string{"/old"}}}
	new := &config.Config{Watch: config.WatchConfig{Movies: []string{"/a"}, TV: []string{"/b"}}}
	commit, _, err := r.Prepare(context.Background(), old, new)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.current; got[0] != "/old" {
		t.Errorf("scanner mutated before commit")
	}
	if err := commit(); err != nil {
		t.Fatal(err)
	}
	if len(s.current) != 2 || s.current[0] != "/a" || s.current[1] != "/b" {
		t.Errorf("commit produced %v", s.current)
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/daemon/reload/... -run TestScannerReloadable`
Expected: FAIL.

- [ ] **Step 3: Implement**

```go
// internal/daemon/reload/scanner_reloadable.go
package reload

import (
	"context"

	"github.com/Nomadcxx/jellywatch/internal/config"
)

type ScannerLike interface {
	ReplaceWatchPaths(paths []string) error
}

type scannerReloadable struct {
	s ScannerLike
}

func NewScannerReloadable(s ScannerLike) Reloadable {
	return &scannerReloadable{s: s}
}

func (r *scannerReloadable) Name() string { return "scanner" }

func (r *scannerReloadable) Prepare(ctx context.Context, oldCfg, newCfg *config.Config) (Commit, Rollback, error) {
	prev := append([]string{}, mergedPaths(oldCfg)...)
	target := mergedPaths(newCfg)
	commit := func() error { return r.s.ReplaceWatchPaths(target) }
	rollback := func() { _ = r.s.ReplaceWatchPaths(prev) }
	return commit, rollback, nil
}

func mergedPaths(c *config.Config) []string {
	out := make([]string, 0, len(c.Watch.Movies)+len(c.Watch.TV))
	out = append(out, c.Watch.Movies...)
	out = append(out, c.Watch.TV...)
	return out
}
```

- [ ] **Step 4: Add `ReplaceWatchPaths` to scanner**

Edit `internal/scanner/scanner.go` (or wherever the watcher lives — locate via `grep -n "fsnotify\|watcher" internal/scanner/`). Add a method that swaps the watched path set under a mutex.

```go
// internal/scanner/scanner.go (additions)
func (s *Scanner) ReplaceWatchPaths(paths []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.unwatchAllLocked(); err != nil {
		return err
	}
	for _, p := range paths {
		if err := s.watcher.Add(p); err != nil {
			return err
		}
	}
	s.watched = append([]string{}, paths...)
	return nil
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/daemon/reload/... ./internal/scanner/...`
Expected: PASS.

- [ ] **Step 6: Wire into daemon main**

In `cmd/jellywatchd/main.go`:

```go
reload.Register(reload.NewScannerReloadable(scanner))
```

- [ ] **Step 7: Commit**

```bash
git add internal/daemon/reload/scanner_reloadable.go internal/daemon/reload/scanner_reloadable_test.go internal/scanner/scanner.go cmd/jellywatchd/main.go
git commit -m "feat(reload): scanner reloadable replaces watch path set atomically"
```

---

### Task 1.9: AI Reloadable (matcher endpoint + models)

**Files:** Create: `internal/daemon/reload/ai_reloadable.go`, `internal/daemon/reload/ai_reloadable_test.go`.

- [ ] **Step 1: Failing test**

```go
// internal/daemon/reload/ai_reloadable_test.go
package reload

import (
	"context"
	"errors"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/config"
)

type fakeAI struct {
	endpoint string
	model    string
}

func (f *fakeAI) Reconfigure(endpoint, model string) error {
	if endpoint == "" {
		return errors.New("empty endpoint")
	}
	f.endpoint = endpoint
	f.model = model
	return nil
}

func TestAIReloadableSwapsEndpoint(t *testing.T) {
	ai := &fakeAI{endpoint: "http://old", model: "m1"}
	r := NewAIReloadable(ai)
	old := &config.Config{AI: config.AIConfig{OllamaEndpoint: "http://old", Model: "m1"}}
	new := &config.Config{AI: config.AIConfig{OllamaEndpoint: "http://new", Model: "m2"}}
	commit, _, err := r.Prepare(context.Background(), old, new)
	if err != nil {
		t.Fatal(err)
	}
	if ai.endpoint != "http://old" {
		t.Errorf("endpoint changed before commit")
	}
	if err := commit(); err != nil {
		t.Fatal(err)
	}
	if ai.endpoint != "http://new" || ai.model != "m2" {
		t.Errorf("after commit: %+v", ai)
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/daemon/reload/... -run TestAIReloadable`
Expected: FAIL.

- [ ] **Step 3: Implement**

```go
// internal/daemon/reload/ai_reloadable.go
package reload

import (
	"context"

	"github.com/Nomadcxx/jellywatch/internal/config"
)

type AILike interface {
	Reconfigure(endpoint, model string) error
}

type aiReloadable struct {
	ai AILike
}

func NewAIReloadable(ai AILike) Reloadable {
	return &aiReloadable{ai: ai}
}

func (r *aiReloadable) Name() string { return "ai" }

func (r *aiReloadable) Prepare(ctx context.Context, oldCfg, newCfg *config.Config) (Commit, Rollback, error) {
	prevEnd := oldCfg.AI.OllamaEndpoint
	prevModel := oldCfg.AI.Model
	target := newCfg.AI
	commit := func() error { return r.ai.Reconfigure(target.OllamaEndpoint, target.Model) }
	rollback := func() { _ = r.ai.Reconfigure(prevEnd, prevModel) }
	return commit, rollback, nil
}
```

- [ ] **Step 4: Expose `Reconfigure` on the AI matcher**

In `internal/ai/matcher.go`, add:

```go
func (m *Matcher) Reconfigure(endpoint, model string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.endpoint = endpoint
	m.model = model
	return nil
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/daemon/reload/... ./internal/ai/...`
Expected: PASS.

- [ ] **Step 6: Wire into daemon main**

```go
reload.Register(reload.NewAIReloadable(aiMatcher))
```

- [ ] **Step 7: Commit**

```bash
git add internal/daemon/reload/ai_reloadable.go internal/daemon/reload/ai_reloadable_test.go internal/ai/matcher.go cmd/jellywatchd/main.go
git commit -m "feat(reload): ai matcher reloadable for endpoint+model swap"
```

---

## Phase 2 — Atomic config writer + flock

### Task 2.1: Atomic write helper

**Files:** Create: `internal/config/save.go`, `internal/config/save_test.go`.

- [ ] **Step 1: Failing test**

```go
// internal/config/save_test.go
package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestAtomicSaveNoPartialFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
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

func TestAtomicSaveSerializesParallelWriters(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			content := []byte("writer" + string(rune('0'+i)))
			_ = AtomicWriteWithLock(path, content, 0600)
		}()
	}
	wg.Wait()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !looksLikeOneOfWriters(string(got)) {
		t.Errorf("got partial/garbled content %q", string(got))
	}
}

func looksLikeOneOfWriters(s string) bool {
	for i := 0; i < 16; i++ {
		if s == "writer"+string(rune('0'+i)) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/config/... -run TestAtomicSave`
Expected: FAIL — function undefined.

- [ ] **Step 3: Implement**

```go
// internal/config/save.go
package config

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// AtomicWriteWithLock writes content to path atomically, holding a
// flock(2) advisory lock for the duration of the write. Use this for any
// config.toml mutation to make CLI/installer/jellyweb writes mutually
// exclusive.
func AtomicWriteWithLock(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	lockPath := path + ".lock"
	lf, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lf.Close()

	if err := acquireLock(lf, 2*time.Second); err != nil {
		return err
	}
	defer syscall.Flock(int(lf.Fd()), syscall.LOCK_UN)

	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

func acquireLock(f *os.File, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("config lock contention timeout")
		}
		time.Sleep(25 * time.Millisecond)
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/config/... -run TestAtomicSave -race`
Expected: PASS.

- [ ] **Step 5: Replace `Save()` body to use it**

Edit `internal/config/config.go` `(c *Config) Save()`:

```go
func (c *Config) Save() error {
	configFile, err := ConfigPath()
	if err != nil {
		return err
	}
	return AtomicWriteWithLock(configFile, []byte(c.ToTOML()), 0600)
}
```

- [ ] **Step 6: Existing tests still pass**

Run: `go test ./internal/config/... ./cmd/jellywatch/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/config/save.go internal/config/save_test.go internal/config/config.go
git commit -m "feat(config): atomic write with flock for safe concurrent mutation"
```

---

### Task 2.2: Apply same lock to installer write path

**Files:** Modify: `cmd/installer/tasks.go`.

- [ ] **Step 1: Failing test**

Add to `cmd/installer/tasks_test.go` (create if missing):

```go
// cmd/installer/tasks_test.go
package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/config"
)

func TestInstallerWritesViaAtomicLock(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	target := filepath.Join(dir, "jellywatch", "config.toml")

	if err := config.AtomicWriteWithLock(target, []byte("[watch]\nmovies = []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run, verify pass**

(This test exists to assert the helper is publicly usable; should pass already.)
Run: `go test ./cmd/installer/... -run TestInstallerWritesViaAtomicLock`
Expected: PASS.

- [ ] **Step 3: Refactor installer to use the helper**

In `cmd/installer/tasks.go`, replace the `os.WriteFile(configPath, ...)` call in `writeConfig` with:

```go
if err := config.AtomicWriteWithLock(configPath, []byte(configStr), 0600); err != nil {
	return err
}
```

Add the import: `"github.com/Nomadcxx/jellywatch/internal/config"`.

- [ ] **Step 4: Build & test**

Run: `go test ./cmd/installer/... && go build ./...`
Expected: PASS, clean build.

- [ ] **Step 5: Commit**

```bash
git add cmd/installer/tasks.go cmd/installer/tasks_test.go
git commit -m "refactor(installer): write config.toml via AtomicWriteWithLock"
```

---

## Phase 3 — Section registry + handlers

### Task 3.1: Section registry

**Files:** Create: `internal/config/sections.go`, `internal/config/sections_test.go`.

- [ ] **Step 1: Failing test**

```go
// internal/config/sections_test.go
package config

import (
	"encoding/json"
	"testing"
)

func TestSectionsListsAllKnown(t *testing.T) {
	want := []string{
		"paths", "libraries", "sonarr", "radarr", "jellyfin",
		"ai", "daemon", "logging", "options", "permissions",
	}
	got := SectionNames()
	gotMap := make(map[string]bool)
	for _, s := range got {
		gotMap[s] = true
	}
	for _, w := range want {
		if !gotMap[w] {
			t.Errorf("missing section %q", w)
		}
	}
}

func TestGetSectionAI(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AI.Model = "qwen3:32b"
	b, err := GetSection(cfg, "ai")
	if err != nil {
		t.Fatal(err)
	}
	var got AIConfig
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Model != "qwen3:32b" {
		t.Errorf("got %s", got.Model)
	}
}

func TestSetSectionAI(t *testing.T) {
	cfg := DefaultConfig()
	patch := AIConfig{Model: "x", FallbackModel: "y", Enabled: true}
	b, _ := json.Marshal(patch)
	if err := SetSection(cfg, "ai", b); err != nil {
		t.Fatal(err)
	}
	if cfg.AI.Model != "x" || cfg.AI.FallbackModel != "y" {
		t.Errorf("config not updated: %+v", cfg.AI)
	}
}

func TestSetSectionUnknown(t *testing.T) {
	if err := SetSection(DefaultConfig(), "bogus", []byte("{}")); err == nil {
		t.Error("expected error")
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/config/... -run TestSection`
Expected: FAIL.

- [ ] **Step 3: Implement registry**

```go
// internal/config/sections.go
package config

import (
	"encoding/json"
	"fmt"
)

type sectionAccessor struct {
	get func(c *Config) any
	set func(c *Config, raw json.RawMessage) error
}

var sections = map[string]sectionAccessor{
	"paths":       {get: func(c *Config) any { return c.Watch }, set: setWatch},
	"libraries":   {get: func(c *Config) any { return c.Libraries }, set: setLibraries},
	"sonarr":      {get: func(c *Config) any { return c.Sonarr }, set: setSonarr},
	"radarr":      {get: func(c *Config) any { return c.Radarr }, set: setRadarr},
	"jellyfin":    {get: func(c *Config) any { return c.Jellyfin }, set: setJellyfin},
	"ai":          {get: func(c *Config) any { return c.AI }, set: setAI},
	"daemon":      {get: func(c *Config) any { return c.Daemon }, set: setDaemon},
	"logging":     {get: func(c *Config) any { return c.Logging }, set: setLogging},
	"options":     {get: func(c *Config) any { return c.Options }, set: setOptions},
	"permissions": {get: func(c *Config) any { return c.Permissions }, set: setPermissions},
}

func SectionNames() []string {
	out := make([]string, 0, len(sections))
	for k := range sections {
		out = append(out, k)
	}
	return out
}

func GetSection(c *Config, name string) (json.RawMessage, error) {
	a, ok := sections[name]
	if !ok {
		return nil, fmt.Errorf("unknown section %q", name)
	}
	return json.Marshal(a.get(c))
}

func SetSection(c *Config, name string, raw json.RawMessage) error {
	a, ok := sections[name]
	if !ok {
		return fmt.Errorf("unknown section %q", name)
	}
	return a.set(c, raw)
}

func setWatch(c *Config, raw json.RawMessage) error {
	var v WatchConfig
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	c.Watch = v
	return nil
}
func setLibraries(c *Config, raw json.RawMessage) error {
	var v LibrariesConfig
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	c.Libraries = v
	return nil
}
func setSonarr(c *Config, raw json.RawMessage) error {
	var v SonarrConfig
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	c.Sonarr = v
	return nil
}
func setRadarr(c *Config, raw json.RawMessage) error {
	var v RadarrConfig
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	c.Radarr = v
	return nil
}
func setJellyfin(c *Config, raw json.RawMessage) error {
	var v JellyfinConfig
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	c.Jellyfin = v
	return nil
}
func setAI(c *Config, raw json.RawMessage) error {
	var v AIConfig
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	c.AI = v
	return nil
}
func setDaemon(c *Config, raw json.RawMessage) error {
	var v DaemonConfig
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	c.Daemon = v
	return nil
}
func setLogging(c *Config, raw json.RawMessage) error {
	var v LoggingConfig
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	c.Logging = v
	return nil
}
func setOptions(c *Config, raw json.RawMessage) error {
	var v OptionsConfig
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	c.Options = v
	return nil
}
func setPermissions(c *Config, raw json.RawMessage) error {
	var v PermissionsConfig
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	c.Permissions = v
	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/config/... -run TestSection`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/sections.go internal/config/sections_test.go
git commit -m "feat(config): section registry for typed get/set by name"
```

---

### Task 3.2: Mark secret fields

**Files:** Modify: `internal/config/config.go`.

- [ ] **Step 1: Failing test**

```go
// internal/config/secret_test.go
package config

import (
	"reflect"
	"testing"
)

func TestSecretFieldsTagged(t *testing.T) {
	want := map[string][]string{
		"SonarrConfig":   {"APIKey"},
		"RadarrConfig":   {"APIKey"},
		"JellyfinConfig": {"APIKey", "WebhookSecret", "PluginSharedSecret"},
		"Config":         {"Password"},
	}
	for typeName, fields := range want {
		typ := lookupType(typeName)
		if typ == nil {
			t.Fatalf("type %s not found", typeName)
		}
		for _, f := range fields {
			sf, ok := typ.FieldByName(f)
			if !ok {
				t.Errorf("%s.%s missing", typeName, f)
				continue
			}
			if sf.Tag.Get("secret") != "true" {
				t.Errorf("%s.%s missing secret:\"true\" tag", typeName, f)
			}
		}
	}
}

func lookupType(name string) reflect.Type {
	switch name {
	case "SonarrConfig":
		return reflect.TypeOf(SonarrConfig{})
	case "RadarrConfig":
		return reflect.TypeOf(RadarrConfig{})
	case "JellyfinConfig":
		return reflect.TypeOf(JellyfinConfig{})
	case "Config":
		return reflect.TypeOf(Config{})
	}
	return nil
}
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/config/... -run TestSecretFieldsTagged`
Expected: FAIL.

- [ ] **Step 3: Add `secret:"true"` tags**

In `internal/config/config.go`, on the relevant fields:

```go
type SonarrConfig struct {
	Enabled         bool   `mapstructure:"enabled"`
	URL             string `mapstructure:"url"`
	APIKey          string `mapstructure:"api_key" secret:"true"`
	NotifyOnImport  bool   `mapstructure:"notify_on_import"`
}

type RadarrConfig struct {
	Enabled         bool   `mapstructure:"enabled"`
	URL             string `mapstructure:"url"`
	APIKey          string `mapstructure:"api_key" secret:"true"`
	NotifyOnImport  bool   `mapstructure:"notify_on_import"`
}

type JellyfinConfig struct {
	URL    string `mapstructure:"url"`
	APIKey string `mapstructure:"api_key" secret:"true"`
	WebhookSecret string `mapstructure:"webhook_secret" secret:"true"`
	PluginSharedSecret string `mapstructure:"plugin_shared_secret" secret:"true"`
	// ... rest of fields ...
}

type Config struct {
	// ...
	Password string `mapstructure:"password" secret:"true"`
	// ...
}
```

(Inspect each struct definition; only edit the API-key/password lines.)

- [ ] **Step 4: Run tests**

Run: `go test ./internal/config/...`
Expected: PASS.

- [ ] **Step 5: Add masking helper**

```go
// internal/config/mask.go
package config

import (
	"reflect"
)

// MaskSecrets mutates v in place, replacing every field tagged
// secret:"true" with "****" + last 4 chars of the original value.
// Pass a pointer-to-struct.
func MaskSecrets(v any) {
	rv := reflect.ValueOf(v).Elem()
	maskValue(rv)
}

func maskValue(rv reflect.Value) {
	if rv.Kind() != reflect.Struct {
		return
	}
	rt := rv.Type()
	for i := 0; i < rv.NumField(); i++ {
		field := rv.Field(i)
		ft := rt.Field(i)
		if ft.Tag.Get("secret") == "true" && field.Kind() == reflect.String {
			s := field.String()
			if len(s) > 4 {
				field.SetString("****" + s[len(s)-4:])
			} else if s != "" {
				field.SetString("****")
			}
		}
		if field.Kind() == reflect.Struct {
			maskValue(field)
		}
	}
}
```

Add a test:

```go
func TestMaskSecretsMasksAPIKey(t *testing.T) {
	c := SonarrConfig{APIKey: "abcdef1234567890"}
	MaskSecrets(&c)
	if c.APIKey != "****7890" {
		t.Errorf("got %q", c.APIKey)
	}
}
```

Run: `go test ./internal/config/... -run TestMaskSecrets` → PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/mask.go internal/config/secret_test.go
git commit -m "feat(config): tag secret fields and add reflect-based masker"
```

---

### Task 3.2b: Canonical save pipeline with reload restore

**Files:** Create: `internal/api/config_save_pipeline.go`, `internal/api/config_save_pipeline_test.go`.

- [ ] **Step 1: Failing test**

```go
// internal/api/config_save_pipeline_test.go
package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
)

type failingReloadIPC struct{}

func (f failingReloadIPC) Call(ctx context.Context, cmd ipc.Command, args any) (json.RawMessage, error) {
	return json.RawMessage(`{"ok":false,"failed":[{"name":"ai","error":"bad config"}]}`), nil
}

func TestSavePipelineRestoresPreviousConfigOnReloadFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	oldCfg := config.DefaultConfig()
	oldCfg.Logging.Level = "info"
	if err := config.AtomicWriteWithLock(path, []byte(oldCfg.ToTOML()), 0600); err != nil {
		t.Fatal(err)
	}

	newCfg := config.DefaultConfig()
	newCfg.Logging.Level = "debug"
	resp, err := SaveConfigAndReload(context.Background(), path, newCfg, failingReloadIPC{})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.RestoredPreviousConfig {
		t.Fatalf("expected restore response: %+v", resp)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte(`level = "info"`)) {
		t.Fatalf("previous config was not restored:\n%s", string(got))
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/api/... -run TestSavePipelineRestoresPreviousConfig`
Expected: FAIL — `SaveConfigAndReload` undefined.

- [ ] **Step 3: Implement**

```go
// internal/api/config_save_pipeline.go
package api

import (
	"context"
	"encoding/json"
	"os"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
)

type IPCCaller interface {
	Call(ctx context.Context, cmd ipc.Command, args any) (json.RawMessage, error)
}

type ReloadResult struct {
	OK       bool                    `json:"ok"`
	Reloaded []string                `json:"reloaded"`
	Failed   []ReloadFailedSubsystem `json:"failed,omitempty"`
}

type ReloadFailedSubsystem struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

type PutSectionResponse struct {
	Saved                  bool         `json:"saved"`
	Validation             any          `json:"validation,omitempty"`
	Reload                 ReloadResult `json:"reload"`
	RestoredPreviousConfig bool         `json:"restored_previous_config"`
}

func SaveConfigAndReload(ctx context.Context, path string, candidate *config.Config, ipcClient IPCCaller) (PutSectionResponse, error) {
	prev, _ := os.ReadFile(path)
	if err := config.AtomicWriteWithLock(path, []byte(candidate.ToTOML()), 0600); err != nil {
		return PutSectionResponse{}, err
	}

	reload := ReloadResult{}
	data, err := ipcClient.Call(ctx, ipc.CmdReload, nil)
	if err != nil {
		reload = ReloadResult{OK: false, Failed: []ReloadFailedSubsystem{{Name: "ipc", Error: err.Error()}}}
	} else if err := json.Unmarshal(data, &reload); err != nil {
		reload = ReloadResult{OK: false, Failed: []ReloadFailedSubsystem{{Name: "ipc", Error: err.Error()}}}
	}

	resp := PutSectionResponse{Saved: true, Reload: reload}
	if !reload.OK {
		if len(prev) > 0 {
			if err := config.AtomicWriteWithLock(path, prev, 0600); err != nil {
				return resp, err
			}
		}
		resp.RestoredPreviousConfig = true
	}
	return resp, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/api/... -run TestSavePipelineRestoresPreviousConfig`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/config_save_pipeline.go internal/api/config_save_pipeline_test.go
git commit -m "feat(api): restore previous config when daemon reload fails"
```

---

### Task 3.2c: Reveal-token endpoint and masked-secret preservation

**Files:** Extend `internal/api/settings_handlers.go`, `internal/api/settings_handlers_test.go`.

- [ ] Add `POST /settings/reveal-token` that issues a short-lived nonce stored in memory on the API server.
- [ ] Allow `GET /settings/{section}?reveal=1` only when `X-Reveal-Token` matches an unexpired nonce; otherwise return masked values.
- [ ] Record reveal events through the existing activity/audit logging path if available; if not available, log a structured warning with section name and timestamp.
- [ ] Preserve existing secret values on write when the submitted field value starts with `"****"` (masked placeholder). This prevents a user from overwriting `api_key`, `webhook_secret`, or `password` with the display mask.
- [ ] Tests:
  - default `GET /settings/sonarr` masks `api_key`
  - `GET /settings/sonarr?reveal=1` without token still masks
  - reveal token permits one unmasked read before expiry
  - `PUT /settings/sonarr` with `api_key: "****7890"` keeps the previous stored key

Run: `go test ./internal/api/... -run 'Test.*(Reveal|Masked|Secret)'`
Expected: PASS.

---

### Task 3.3: Generic settings handlers (read/write per section)

**Files:** Create: `internal/api/settings_handlers.go`, `internal/api/settings_handlers_test.go`.

- [ ] **Step 1: Failing test**

```go
// internal/api/settings_handlers_test.go
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
	"github.com/go-chi/chi/v5"
)

type stubIPC struct{ called bool }

func (s *stubIPC) Call(ctx context.Context, cmd ipc.Command, args any) (json.RawMessage, error) {
	s.called = true
	return json.RawMessage(`{"ok":true,"reloaded":["ai"]}`), nil
}

func newTestSettingsRouter(t *testing.T, cfg *config.Config) (*chi.Mux, *stubIPC) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	ipcStub := &stubIPC{}
	h := &SettingsHandlers{Cfg: cfg, IPC: ipcStub}
	r.Get("/settings/{section}", h.Get)
	r.Put("/settings/{section}", h.Put)
	return r, ipcStub
}

func TestGetSectionMasksSecrets(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Sonarr.APIKey = "abcdef1234567890"
	r, _ := newTestSettingsRouter(t, cfg)
	req := httptest.NewRequest("GET", "/settings/sonarr", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	var got config.SonarrConfig
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.APIKey != "****7890" {
		t.Errorf("expected masked, got %q", got.APIKey)
	}
}

func TestPutSectionTriggersReload(t *testing.T) {
	cfg := config.DefaultConfig()
	r, ipcStub := newTestSettingsRouter(t, cfg)
	body, _ := json.Marshal(config.AIConfig{
		Enabled: true, OllamaEndpoint: "http://x", Model: "m",
	})
	req := httptest.NewRequest("PUT", "/settings/ai", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if !ipcStub.called {
		t.Error("expected IPC reload to be called")
	}
	var resp PutSectionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Saved {
		t.Errorf("expected saved=true: %+v", resp)
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/api/... -run TestGetSection`
Expected: FAIL.

- [ ] **Step 3: Implement**

```go
// internal/api/settings_handlers.go
package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/go-chi/chi/v5"
)

type SettingsHandlers struct {
	mu  sync.Mutex
	Cfg *config.Config
	IPC IPCCaller
}

func (h *SettingsHandlers) Get(w http.ResponseWriter, r *http.Request) {
	section := chi.URLParam(r, "section")

	h.mu.Lock()
	defer h.mu.Unlock()

	raw, err := config.GetSection(h.Cfg, section)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if r.URL.Query().Get("reveal") != "1" || !h.ValidRevealToken(r.Header.Get("X-Reveal-Token")) {
		var v any
		if err := json.Unmarshal(raw, &v); err == nil {
			masked := maskSectionJSON(section, v)
			raw, _ = json.Marshal(masked)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(raw)
}

func (h *SettingsHandlers) Put(w http.ResponseWriter, r *http.Request) {
	section := chi.URLParam(r, "section")
	body, err := readBody(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	current, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	body, err = preserveMaskedSectionSecrets(current, section, body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := config.SetSection(current, section, body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	configPath, err := config.ConfigPath()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp, err := SaveConfigAndReload(r.Context(), configPath, current, h.IPC)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if resp.Reload.OK {
		h.Cfg = current
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func readBody(r *http.Request) (json.RawMessage, error) {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// maskSectionJSON re-marshals via the typed struct so the secret tags apply.
func maskSectionJSON(section string, v any) any {
	b, _ := json.Marshal(v)
	switch section {
	case "sonarr":
		var c config.SonarrConfig
		_ = json.Unmarshal(b, &c)
		config.MaskSecrets(&c)
		return c
	case "radarr":
		var c config.RadarrConfig
		_ = json.Unmarshal(b, &c)
		config.MaskSecrets(&c)
		return c
	case "jellyfin":
		var c config.JellyfinConfig
		_ = json.Unmarshal(b, &c)
		config.MaskSecrets(&c)
		return c
	}
	return v
}

func preserveMaskedSectionSecrets(current *config.Config, section string, raw json.RawMessage) (json.RawMessage, error) {
	switch section {
	case "sonarr":
		var v config.SonarrConfig
		if err := json.Unmarshal(raw, &v); err != nil { return nil, err }
		if isMaskedSecret(v.APIKey) { v.APIKey = current.Sonarr.APIKey }
		return json.Marshal(v)
	case "radarr":
		var v config.RadarrConfig
		if err := json.Unmarshal(raw, &v); err != nil { return nil, err }
		if isMaskedSecret(v.APIKey) { v.APIKey = current.Radarr.APIKey }
		return json.Marshal(v)
	case "jellyfin":
		var v config.JellyfinConfig
		if err := json.Unmarshal(raw, &v); err != nil { return nil, err }
		if isMaskedSecret(v.APIKey) { v.APIKey = current.Jellyfin.APIKey }
		if isMaskedSecret(v.WebhookSecret) { v.WebhookSecret = current.Jellyfin.WebhookSecret }
		if isMaskedSecret(v.PluginSharedSecret) { v.PluginSharedSecret = current.Jellyfin.PluginSharedSecret }
		return json.Marshal(v)
	}
	return raw, nil
}

func isMaskedSecret(v string) bool {
	return strings.HasPrefix(v, "****")
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/api/...`
Expected: PASS.

- [ ] **Step 5: Mount routes in api/server.go**

In `internal/api/server.go`, add an `ipc IPCCaller` field to `Server`, pass it from `cmd/jellyweb/main.go`, then mount routes in `apiRouter`:

```go
settingsH := &SettingsHandlers{Cfg: s.cfg, IPC: s.ipc}
r.Route("/settings", func(r chi.Router) {
	r.Get("/{section}", settingsH.Get)
	r.Put("/{section}", settingsH.Put)
})
```

(The production value is a `*ipc.Client` constructed in jellyweb startup using `paths.JellyWatchDir()/control.sock`; tests can pass a stub.)

- [ ] **Step 6: Commit**

```bash
git add internal/api/settings_handlers.go internal/api/settings_handlers_test.go internal/api/server.go
git commit -m "feat(api): generic settings GET/PUT with masking and reload trigger"
```

---

### Task 3.4: Path / library array CRUD handlers

**Files:** Create: `internal/api/paths_handlers.go`, `internal/api/paths_handlers_test.go`.

- [ ] **Step 1: Failing test**

```go
// internal/api/paths_handlers_test.go
package api

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/go-chi/chi/v5"
)

func newPathsRouter(t *testing.T, cfg *config.Config) *chi.Mux {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	ipcStub := &stubIPC{}
	h := &PathsHandlers{Cfg: cfg, IPC: ipcStub}
	r.Get("/settings/paths/{kind}", h.Get)
	r.Post("/settings/paths/{kind}", h.Add)
	r.Delete("/settings/paths/{kind}/{index}", h.Remove)
	r.Put("/settings/paths/{kind}", h.Replace)
	return r
}

func TestAddTVWatchPath(t *testing.T) {
	cfg := config.DefaultConfig()
	r := newPathsRouter(t, cfg)
	body := []byte(`{"path":"/storage/tv"}`)
	req := httptest.NewRequest("POST", "/settings/paths/tv", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if len(cfg.Watch.TV) != 1 || cfg.Watch.TV[0] != "/storage/tv" {
		t.Errorf("got %v", cfg.Watch.TV)
	}
}

func TestRemoveTVWatchPath(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Watch.TV = []string{"/a", "/b", "/c"}
	r := newPathsRouter(t, cfg)
	req := httptest.NewRequest("DELETE", "/settings/paths/tv/1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 204 {
		t.Fatalf("status %d", w.Code)
	}
	want := []string{"/a", "/c"}
	for i, p := range cfg.Watch.TV {
		if p != want[i] {
			t.Errorf("got %v", cfg.Watch.TV)
			break
		}
	}
}

func TestReplaceMoviesWatchPaths(t *testing.T) {
	cfg := config.DefaultConfig()
	r := newPathsRouter(t, cfg)
	body, _ := json.Marshal(map[string][]string{"paths": []string{"/x", "/y"}})
	req := httptest.NewRequest("PUT", "/settings/paths/movies", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	if len(cfg.Watch.Movies) != 2 {
		t.Errorf("got %v", cfg.Watch.Movies)
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/api/... -run TestAdd`
Expected: FAIL.

- [ ] **Step 3: Implement**

```go
// internal/api/paths_handlers.go
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/go-chi/chi/v5"
)

type PathsHandlers struct {
	mu  sync.Mutex
	Cfg *config.Config
	IPC IPCCaller
}

type addPathBody struct {
	Path string `json:"path"`
}
type replacePathsBody struct {
	Paths []string `json:"paths"`
}

func (h *PathsHandlers) Get(w http.ResponseWriter, r *http.Request) {
	kind := chi.URLParam(r, "kind")
	h.mu.Lock()
	defer h.mu.Unlock()
	paths, err := h.read(kind)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(map[string][]string{"paths": paths})
}

func (h *PathsHandlers) Add(w http.ResponseWriter, r *http.Request) {
	kind := chi.URLParam(r, "kind")
	var body addPathBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Path == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	cur, err := h.read(kind)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	cur = append(cur, body.Path)
	if err := h.write(kind, cur); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
		if err := h.persist(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string][]string{"paths": cur})
}

func (h *PathsHandlers) Remove(w http.ResponseWriter, r *http.Request) {
	kind := chi.URLParam(r, "kind")
	idxStr := chi.URLParam(r, "index")
	idx, err := strconv.Atoi(idxStr)
	if err != nil || idx < 0 {
		http.Error(w, "bad index", http.StatusBadRequest)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	cur, err := h.read(kind)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if idx >= len(cur) {
		http.Error(w, "index out of range", http.StatusNotFound)
		return
	}
	cur = append(cur[:idx], cur[idx+1:]...)
	if err := h.write(kind, cur); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
		if err := h.persist(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *PathsHandlers) Replace(w http.ResponseWriter, r *http.Request) {
	kind := chi.URLParam(r, "kind")
	var body replacePathsBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.write(kind, body.Paths); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
		if err := h.persist(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string][]string{"paths": body.Paths})
}

func (h *PathsHandlers) read(kind string) ([]string, error) {
	switch kind {
	case "tv":
		return append([]string{}, h.Cfg.Watch.TV...), nil
	case "movies":
		return append([]string{}, h.Cfg.Watch.Movies...), nil
	}
	return nil, errKindUnknown
}

func (h *PathsHandlers) write(kind string, v []string) error {
	switch kind {
	case "tv":
		h.Cfg.Watch.TV = v
		return nil
	case "movies":
		h.Cfg.Watch.Movies = v
		return nil
	}
	return errKindUnknown
}

func (h *PathsHandlers) persist(ctx context.Context) error {
	configPath, err := config.ConfigPath()
	if err != nil {
		return err
	}
	resp, err := SaveConfigAndReload(ctx, configPath, h.Cfg, h.IPC)
	if err != nil {
		return err
	}
	if !resp.Reload.OK {
		if restored, loadErr := config.Load(); loadErr == nil {
			h.Cfg = restored
		}
		return errors.New("daemon reload failed; previous config restored")
	}
	return nil
}

var errKindUnknown = errKindUnknownErr{}

type errKindUnknownErr struct{}

func (errKindUnknownErr) Error() string { return "unknown kind (must be tv or movies)" }
```

Add a parallel `LibrariesHandlers` in the same file with identical structure but operating on `cfg.Libraries.{TV,Movies}` — it's a copy/paste of the body with the field swapped:

```go
type LibrariesHandlers struct {
	mu  sync.Mutex
	Cfg *config.Config
	IPC IPCCaller
}

func (h *LibrariesHandlers) Get(w http.ResponseWriter, r *http.Request)    { /* same as Paths.Get with read/write below */ }
func (h *LibrariesHandlers) Add(w http.ResponseWriter, r *http.Request)    { /* same as Paths.Add */ }
func (h *LibrariesHandlers) Remove(w http.ResponseWriter, r *http.Request) { /* same as Paths.Remove */ }
func (h *LibrariesHandlers) Replace(w http.ResponseWriter, r *http.Request){ /* same as Paths.Replace */ }

func (h *LibrariesHandlers) read(kind string) ([]string, error) {
	switch kind {
	case "tv":
		return append([]string{}, h.Cfg.Libraries.TV...), nil
	case "movies":
		return append([]string{}, h.Cfg.Libraries.Movies...), nil
	}
	return nil, errKindUnknown
}
func (h *LibrariesHandlers) write(kind string, v []string) error {
	switch kind {
	case "tv":
		h.Cfg.Libraries.TV = v
		return nil
	case "movies":
		h.Cfg.Libraries.Movies = v
		return nil
	}
	return errKindUnknown
}
func (h *LibrariesHandlers) persist(ctx context.Context) error {
	configPath, err := config.ConfigPath()
	if err != nil {
		return err
	}
	resp, err := SaveConfigAndReload(ctx, configPath, h.Cfg, h.IPC)
	if err != nil {
		return err
	}
	if !resp.Reload.OK {
		if restored, loadErr := config.Load(); loadErr == nil {
			h.Cfg = restored
		}
		return errors.New("daemon reload failed; previous config restored")
	}
	return nil
}
```

- [ ] **Step 4: Add a libraries test mirroring the paths tests**

Copy `TestAddTVWatchPath` → `TestAddTVLibrary`, swap routes/struct accordingly. Run all paths tests.

Run: `go test ./internal/api/... -run TestAdd`
Expected: PASS.

- [ ] **Step 5: Mount in server.go**

```go
pathsH := &PathsHandlers{Cfg: s.cfg, IPC: s.ipc}
libsH  := &LibrariesHandlers{Cfg: s.cfg, IPC: s.ipc}
r.Route("/settings/paths", func(r chi.Router) {
	r.Get("/{kind}", pathsH.Get)
	r.Post("/{kind}", pathsH.Add)
	r.Delete("/{kind}/{index}", pathsH.Remove)
	r.Put("/{kind}", pathsH.Replace)
})
r.Route("/settings/libraries", func(r chi.Router) {
	r.Get("/{kind}", libsH.Get)
	r.Post("/{kind}", libsH.Add)
	r.Delete("/{kind}/{index}", libsH.Remove)
	r.Put("/{kind}", libsH.Replace)
})
```

- [ ] **Step 6: Commit**

```bash
git add internal/api/paths_handlers.go internal/api/paths_handlers_test.go internal/api/server.go
git commit -m "feat(api): array CRUD for watch folders and library locations"
```

---

### Task 3.5: Connection-test handlers (sonarr, radarr, jellyfin)

**Files:** Create: `internal/api/test_handlers.go`, `internal/api/test_handlers_test.go`.

- [ ] **Step 1: Failing test**

```go
// internal/api/test_handlers_test.go
package api

import (
	"bytes"
	"net/http/httptest"
	"testing"
)

func TestTestSonarrFailsWithBadURL(t *testing.T) {
	h := &TestHandlers{}
	body := []byte(`{"url":"http://127.0.0.1:1","api_key":"x","enabled":true}`)
	req := httptest.NewRequest("POST", "/settings/sonarr/test", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Sonarr(w, req)
	// We expect a 200 with ok=false (this endpoint should not 5xx).
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"ok":false`)) {
		t.Errorf("expected ok=false; body=%s", w.Body.String())
	}
}

func TestTestSonarrSucceedsAgainstMock(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"appName":"Sonarr","version":"4.0"}`))
	}))
	defer mock.Close()
	h := &TestHandlers{}
	body := []byte(`{"url":"` + mock.URL + `","api_key":"x","enabled":true}`)
	req := httptest.NewRequest("POST", "/settings/sonarr/test", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Sonarr(w, req)
	if !bytes.Contains(w.Body.Bytes(), []byte(`"ok":true`)) {
		t.Errorf("body=%s", w.Body.String())
	}
}
```

(Add `import "net/http"` to the test file.)

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/api/... -run TestTestSonarr`
Expected: FAIL.

- [ ] **Step 3: Implement**

```go
// internal/api/test_handlers.go
package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/jellyfin"
	"github.com/Nomadcxx/jellywatch/internal/radarr"
	"github.com/Nomadcxx/jellywatch/internal/sonarr"
)

type TestHandlers struct{}

type testResult struct {
	OK      bool   `json:"ok"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (TestHandlers) Sonarr(w http.ResponseWriter, r *http.Request) {
	var c config.SonarrConfig
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			writeJSON(w, http.StatusOK, testResult{OK: false, Error: err.Error()})
		return
	}
	cli := sonarr.NewClient(sonarr.Config{URL: c.URL, APIKey: c.APIKey, Timeout: 5 * time.Second})
	st, err := cli.GetSystemStatus()
	if err != nil {
			writeJSON(w, http.StatusOK, testResult{OK: false, Error: err.Error()})
		return
	}
		writeJSON(w, http.StatusOK, testResult{OK: true, Version: st.Version})
}

func (TestHandlers) Radarr(w http.ResponseWriter, r *http.Request) {
	var c config.RadarrConfig
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			writeJSON(w, http.StatusOK, testResult{OK: false, Error: err.Error()})
		return
	}
	cli := radarr.NewClient(radarr.Config{URL: c.URL, APIKey: c.APIKey, Timeout: 5 * time.Second})
	st, err := cli.GetSystemStatus()
	if err != nil {
			writeJSON(w, http.StatusOK, testResult{OK: false, Error: err.Error()})
		return
	}
		writeJSON(w, http.StatusOK, testResult{OK: true, Version: st.Version})
}

func (TestHandlers) Jellyfin(w http.ResponseWriter, r *http.Request) {
	var c config.JellyfinConfig
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			writeJSON(w, http.StatusOK, testResult{OK: false, Error: err.Error()})
		return
	}
		cli := jellyfin.NewClient(jellyfin.Config{URL: c.URL, APIKey: c.APIKey, Timeout: 5 * time.Second})
		v, err := cli.GetSystemInfo()
	if err != nil {
			writeJSON(w, http.StatusOK, testResult{OK: false, Error: err.Error()})
		return
	}
		writeJSON(w, http.StatusOK, testResult{OK: true, Version: v.Version})
	}
```

(Jellyfin already exposes `GetSystemInfo()`.)

- [ ] **Step 4: Mount routes**

In `internal/api/server.go`:

```go
testH := TestHandlers{}
r.Post("/settings/sonarr/test", testH.Sonarr)
r.Post("/settings/radarr/test", testH.Radarr)
r.Post("/settings/jellyfin/test", testH.Jellyfin)
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/api/... -run TestTestSonarr`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/api/test_handlers.go internal/api/test_handlers_test.go internal/api/server.go
git commit -m "feat(api): connection-test endpoints for sonarr/radarr/jellyfin"
```

---

### Task 3.6: `/paths/preflight` handler

**Files:** Create: `internal/api/preflight_handlers.go`, `internal/api/preflight_handlers_test.go`.

- [ ] **Step 1: Failing test**

```go
// internal/api/preflight_handlers_test.go
package api

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPreflightReportsExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub")
	os.MkdirAll(path, 0755)
	body, _ := json.Marshal(map[string]string{"path": path, "kind": "watch"})
	req := httptest.NewRequest("POST", "/paths/preflight", bytes.NewReader(body))
	w := httptest.NewRecorder()
	(PreflightHandler{}).ServeHTTP(w, req)
	var got preflightResult
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Readable {
		t.Errorf("expected readable, got %+v", got)
	}
}

func TestPreflightReportsMissing(t *testing.T) {
	body, _ := json.Marshal(map[string]string{"path": "/nonexistent/zzz", "kind": "watch"})
	req := httptest.NewRequest("POST", "/paths/preflight", bytes.NewReader(body))
	w := httptest.NewRecorder()
	(PreflightHandler{}).ServeHTTP(w, req)
	var got preflightResult
	json.Unmarshal(w.Body.Bytes(), &got)
	if got.Readable {
		t.Errorf("expected not readable")
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/api/... -run TestPreflight`
Expected: FAIL.

- [ ] **Step 3: Implement**

```go
// internal/api/preflight_handlers.go
package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

type PreflightHandler struct{}

type preflightBody struct {
	Path string `json:"path"`
	Kind string `json:"kind"` // "watch" | "library"
}

type preflightResult struct {
	Path           string `json:"path"`
	Exists         bool   `json:"exists"`
	IsDir          bool   `json:"is_dir"`
	Readable       bool   `json:"readable"`
	Writable       bool   `json:"writable"`
	OwnerUID       int    `json:"owner_uid"`
	DaemonUIDOK    bool   `json:"daemon_uid_can_access"`
	FreeSpaceBytes int64  `json:"free_space_bytes"`
	Warnings       []string `json:"warnings,omitempty"`
}

func (PreflightHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body preflightBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res := preflightResult{Path: body.Path}
	info, err := os.Stat(body.Path)
	if err == nil {
		res.Exists = true
		res.IsDir = info.IsDir()
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			res.OwnerUID = int(stat.Uid)
			res.DaemonUIDOK = res.OwnerUID == os.Getuid()
		}
		if _, rerr := os.ReadDir(body.Path); rerr == nil {
			res.Readable = true
		} else {
			res.Warnings = append(res.Warnings, "not readable: "+rerr.Error())
		}
		if body.Kind == "library" {
			testFile := filepath.Join(body.Path, ".jellywatch_write_test_"+time.Now().Format("150405.000000"))
			if f, werr := os.Create(testFile); werr == nil {
				f.Close()
				os.Remove(testFile)
				res.Writable = true
			} else {
				res.Warnings = append(res.Warnings, "not writable: "+werr.Error())
			}
		}
		var stat syscall.Statfs_t
		if statErr := syscall.Statfs(body.Path, &stat); statErr == nil {
			res.FreeSpaceBytes = int64(stat.Bavail) * stat.Bsize
		}
	} else {
		res.Warnings = append(res.Warnings, err.Error())
	}
		writeJSON(w, http.StatusOK, res)
	}
```

- [ ] **Step 4: Mount route**

In `internal/api/server.go`:

```go
r.Post("/paths/preflight", PreflightHandler{}.ServeHTTP)
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/api/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/api/preflight_handlers.go internal/api/preflight_handlers_test.go internal/api/server.go
git commit -m "feat(api): /paths/preflight reports access + free space + ownership"
```

---

### Task 3.7: Update OpenAPI spec

**Files:** Modify: `api/openapi.yaml`.

- [ ] **Step 1: Add path entries**

Append the following under `paths:` in `api/openapi.yaml` (preserve existing format/style):

```yaml
  /settings/{section}:
    get:
      summary: Read a config section
      parameters:
        - name: section
          in: path
          required: true
          schema: { type: string }
        - name: reveal
          in: query
          required: false
          schema: { type: integer, enum: [1] }
      responses:
        '200': { description: OK, content: { application/json: { schema: { type: object } } } }
        '404': { description: Unknown section }
    put:
      summary: Replace a config section
      parameters:
        - name: section
          in: path
          required: true
          schema: { type: string }
      requestBody:
        required: true
        content: { application/json: { schema: { type: object } } }
      responses:
        '200':
          description: Saved
          content:
            application/json:
              schema:
                type: object
                properties:
                  saved: { type: boolean }
	                  reload:
	                    type: object
	                    properties:
	                      ok: { type: boolean }
	                      reloaded: { type: array, items: { type: string } }
	                      failed: { type: array, items: { type: object } }
	                  restored_previous_config: { type: boolean }

  /settings/paths/{kind}:
    parameters:
      - { name: kind, in: path, required: true, schema: { type: string, enum: [movies, tv] } }
    get:    { responses: { '200': { description: OK } } }
    post:   { requestBody: { required: true }, responses: { '201': { description: Added } } }
    put:    { requestBody: { required: true }, responses: { '200': { description: Replaced } } }
  /settings/paths/{kind}/{index}:
    delete:
      parameters:
        - { name: kind, in: path, required: true, schema: { type: string } }
        - { name: index, in: path, required: true, schema: { type: integer } }
      responses: { '204': { description: Removed } }

  /settings/libraries/{kind}:    # mirror of /settings/paths/{kind}
    parameters:
      - { name: kind, in: path, required: true, schema: { type: string, enum: [movies, tv] } }
    get:  { responses: { '200': { description: OK } } }
    post: { requestBody: { required: true }, responses: { '201': { description: Added } } }
    put:  { requestBody: { required: true }, responses: { '200': { description: Replaced } } }
  /settings/libraries/{kind}/{index}:
    delete:
      parameters:
        - { name: kind, in: path, required: true, schema: { type: string } }
        - { name: index, in: path, required: true, schema: { type: integer } }
      responses: { '204': { description: Removed } }

  /settings/sonarr/test:
    post:
      requestBody: { required: true }
      responses: { '200': { description: Result } }
  /settings/radarr/test:
    post:
      requestBody: { required: true }
      responses: { '200': { description: Result } }
  /settings/jellyfin/test:
    post:
      requestBody: { required: true }
      responses: { '200': { description: Result } }

  /paths/preflight:
    post:
      requestBody: { required: true }
      responses: { '200': { description: Probe result } }
```

- [ ] **Step 2: Regenerate types if codegen exists**

Check for `make openapi-codegen` or similar:

Run: `grep -n "openapi" Makefile`
If a target exists, run: `make openapi-codegen`. Otherwise skip.

- [ ] **Step 3: Build**

Run: `go build ./... && (cd web && npm run build)`
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add api/openapi.yaml
git commit -m "docs(openapi): declare new settings/paths/libraries/test/preflight routes"
```

---

## Phase 4 — Frontend foundation

### Task 4.1: Settings layout (left rail)

**Files:** Create: `web/src/app/settings/layout.tsx`. Modify: `web/src/app/settings/page.tsx`.

- [ ] **Step 1: Failing test**

```tsx
// web/src/app/settings/layout.test.tsx
import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import SettingsLayout from './layout';

describe('SettingsLayout', () => {
  it('renders nav items for every section', () => {
    render(<SettingsLayout>{<div>child</div>}</SettingsLayout>);
    for (const label of ['Paths', 'Libraries', 'Sonarr', 'Radarr', 'Jellyfin', 'AI', 'Daemon', 'Database', 'Logging', 'Options', 'Permissions']) {
      expect(screen.getByRole('link', { name: label })).toBeInTheDocument();
    }
  });
});
```

- [ ] **Step 2: Run, verify fail**

Run: `(cd web && npx vitest run src/app/settings/layout.test.tsx)`
Expected: FAIL.

- [ ] **Step 3: Implement**

```tsx
// web/src/app/settings/layout.tsx
'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { ReactNode } from 'react';

const SECTIONS = [
  { href: '/settings/paths', label: 'Paths' },
  { href: '/settings/libraries', label: 'Libraries' },
  { href: '/settings/sonarr', label: 'Sonarr' },
  { href: '/settings/radarr', label: 'Radarr' },
  { href: '/settings/jellyfin', label: 'Jellyfin' },
  { href: '/settings/ai', label: 'AI' },
  { href: '/settings/daemon', label: 'Daemon' },
  { href: '/settings/database', label: 'Database' },
  { href: '/settings/logging', label: 'Logging' },
  { href: '/settings/options', label: 'Options' },
  { href: '/settings/permissions', label: 'Permissions' },
];

export default function SettingsLayout({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  return (
    <div className="flex min-h-screen">
      <aside className="w-56 border-r border-border bg-card p-4">
        <h2 className="mb-4 text-sm font-semibold uppercase text-muted-foreground">Settings</h2>
        <nav className="space-y-1">
          {SECTIONS.map((s) => {
            const active = pathname === s.href;
            return (
              <Link
                key={s.href}
                href={s.href}
                aria-current={active ? 'page' : undefined}
                className={
                  'block rounded px-3 py-2 text-sm transition-colors ' +
                  (active ? 'bg-accent font-medium' : 'hover:bg-accent/50')
                }
              >
                {s.label}
              </Link>
            );
          })}
        </nav>
      </aside>
      <main className="flex-1 p-8">{children}</main>
    </div>
  );
}
```

- [ ] **Step 4: Run test**

Run: `(cd web && npx vitest run src/app/settings/layout.test.tsx)`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/app/settings/layout.tsx web/src/app/settings/layout.test.tsx
git commit -m "feat(web): settings layout with left-rail navigation"
```

---

### Task 4.2: Generic SettingsForm wrapper

**Files:** Create: `web/src/components/settings/SettingsForm.tsx`, `web/src/components/settings/SettingsForm.test.tsx`.

- [ ] **Step 1: Failing test**

```tsx
// web/src/components/settings/SettingsForm.test.tsx
import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { SettingsForm, ReloadResult } from './SettingsForm';

describe('SettingsForm', () => {
  it('shows green toast on success', async () => {
    const onSave = vi.fn(async () => ({
      saved: true,
      reload: { ok: true, reloaded: ['ai'], failed: [] } as ReloadResult,
    }));
    render(
      <SettingsForm onSave={onSave}>
        <input data-testid="x" defaultValue="hello" />
      </SettingsForm>,
    );
    fireEvent.click(screen.getByRole('button', { name: /save/i }));
    await waitFor(() => expect(onSave).toHaveBeenCalled());
    expect(await screen.findByText(/Saved\. Daemon reconfigured/)).toBeInTheDocument();
  });

  it('shows red banner when reload reports failures', async () => {
    const onSave = vi.fn(async () => ({
      saved: true,
      reload: { ok: false, reloaded: [], failed: [{ name: 'ai', error: 'no model' }] } as ReloadResult,
    }));
    render(
      <SettingsForm onSave={onSave}>
        <input data-testid="x" defaultValue="hello" />
      </SettingsForm>,
    );
    fireEvent.click(screen.getByRole('button', { name: /save/i }));
    expect(await screen.findByText(/couldn.t apply/)).toBeInTheDocument();
    expect(screen.getByText(/ai/)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run, verify fail**

Run: `(cd web && npx vitest run src/components/settings/SettingsForm.test.tsx)`
Expected: FAIL.

- [ ] **Step 3: Implement**

```tsx
// web/src/components/settings/SettingsForm.tsx
'use client';

import { ReactNode, useState } from 'react';
import { Button } from '@/components/ui/button';
import { Loader2 } from 'lucide-react';

export type ReloadResult = {
  ok: boolean;
  reloaded: string[];
  failed: { name: string; error: string }[];
};

export type SaveResult = {
  saved: boolean;
  validation?: { warnings?: string[] };
  reload: ReloadResult;
  restored_previous_config?: boolean;
};

type Props = {
  children: ReactNode;
  onSave: () => Promise<SaveResult>;
  saveLabel?: string;
};

export function SettingsForm({ children, onSave, saveLabel = 'Save' }: Props) {
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<SaveResult | null>(null);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setResult(null);
    try {
      const res = await onSave();
      setResult(res);
    } finally {
      setBusy(false);
    }
  }

  const banner = (() => {
    if (!result) return null;
    if (!result.reload.ok) {
      return (
        <div className="mb-4 rounded border border-destructive/50 bg-destructive/10 p-4">
          <p className="font-semibold text-destructive">Daemon couldn&apos;t apply changes. Previous config was restored:</p>
          <ul className="mt-2 list-disc pl-6 text-sm">
            {result.reload.failed.map((f) => (
              <li key={f.name}>
                <span className="font-mono">{f.name}</span>: {f.error}
              </li>
            ))}
          </ul>
        </div>
      );
    }
    if (result.validation?.warnings?.length) {
      return (
        <div className="mb-4 rounded border border-yellow-500/50 bg-yellow-500/10 p-4 text-yellow-900 dark:text-yellow-100">
          Saved with warnings: {result.validation.warnings.join('; ')}
        </div>
      );
    }
    return (
      <div className="mb-4 rounded border border-green-500/50 bg-green-500/10 p-4 text-green-900 dark:text-green-100">
        Saved. Daemon reconfigured. Subsystems: {result.reload.reloaded.join(', ')}.
      </div>
    );
  })();

  return (
    <form onSubmit={handleSubmit} className="max-w-2xl space-y-4">
      {banner}
      {children}
      <Button type="submit" disabled={busy}>
        {busy && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
        {saveLabel}
      </Button>
    </form>
  );
}
```

- [ ] **Step 4: Run test**

Run: `(cd web && npx vitest run src/components/settings/SettingsForm.test.tsx)`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/settings/SettingsForm.tsx web/src/components/settings/SettingsForm.test.tsx
git commit -m "feat(web): SettingsForm wrapper with three-state result banner"
```

---

### Task 4.3: SecretField component

**Files:** Create: `web/src/components/settings/SecretField.tsx`, `web/src/components/settings/SecretField.test.tsx`.

- [ ] **Step 1: Failing test**

```tsx
// web/src/components/settings/SecretField.test.tsx
import { describe, expect, it } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { SecretField } from './SecretField';

describe('SecretField', () => {
  it('starts masked and reveals on toggle', () => {
    render(<SecretField label="API Key" name="apiKey" defaultValue="secret123" />);
    const input = screen.getByLabelText('API Key') as HTMLInputElement;
    expect(input.type).toBe('password');
    fireEvent.click(screen.getByRole('button', { name: /show/i }));
    expect(input.type).toBe('text');
    expect(input.value).toBe('secret123');
  });

  it('treats masked placeholder as untouched (does not submit unless edited)', () => {
    render(<SecretField label="API Key" name="apiKey" defaultValue="****1234" />);
    const input = screen.getByLabelText('API Key') as HTMLInputElement;
    expect(input.dataset.masked).toBe('true');
  });
});
```

- [ ] **Step 2: Run, verify fail**

Run: `(cd web && npx vitest run src/components/settings/SecretField.test.tsx)`
Expected: FAIL.

- [ ] **Step 3: Implement**

```tsx
// web/src/components/settings/SecretField.tsx
'use client';

import { useState } from 'react';
import { Eye, EyeOff } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';

type Props = {
  label: string;
  name: string;
  defaultValue?: string;
  onChange?: (v: string) => void;
};

export function SecretField({ label, name, defaultValue = '', onChange }: Props) {
  const [reveal, setReveal] = useState(false);
  const [value, setValue] = useState(defaultValue);
  const isMasked = /^\*+/.test(defaultValue) && value === defaultValue;
  return (
    <div className="space-y-1">
      <Label htmlFor={name}>{label}</Label>
      <div className="flex gap-2">
        <Input
          id={name}
          name={name}
          type={reveal ? 'text' : 'password'}
          value={value}
          data-masked={isMasked ? 'true' : 'false'}
          onChange={(e) => {
            setValue(e.target.value);
            onChange?.(e.target.value);
          }}
        />
        <Button type="button" variant="outline" onClick={() => setReveal((r) => !r)} aria-label={reveal ? 'Hide' : 'Show'}>
          {reveal ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
        </Button>
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Run test**

Run: `(cd web && npx vitest run src/components/settings/SecretField.test.tsx)`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/settings/SecretField.tsx web/src/components/settings/SecretField.test.tsx
git commit -m "feat(web): SecretField component with reveal toggle and masked-state hint"
```

---

### Task 4.4: TestConnectionButton component

**Files:** Create: `web/src/components/settings/TestConnectionButton.tsx`, `web/src/components/settings/TestConnectionButton.test.tsx`.

- [ ] **Step 1: Failing test**

```tsx
// web/src/components/settings/TestConnectionButton.test.tsx
import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { TestConnectionButton } from './TestConnectionButton';

describe('TestConnectionButton', () => {
  it('renders OK when test resolves ok=true', async () => {
    const fn = vi.fn(async () => ({ ok: true, version: '4.0' }));
    render(<TestConnectionButton onTest={fn} />);
    fireEvent.click(screen.getByRole('button', { name: /test/i }));
    expect(await screen.findByText(/Connected.*4\.0/i)).toBeInTheDocument();
  });

  it('renders error when test resolves ok=false', async () => {
    const fn = vi.fn(async () => ({ ok: false, error: 'denied' }));
    render(<TestConnectionButton onTest={fn} />);
    fireEvent.click(screen.getByRole('button', { name: /test/i }));
    expect(await screen.findByText(/denied/)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run, verify fail**

Run: `(cd web && npx vitest run src/components/settings/TestConnectionButton.test.tsx)`
Expected: FAIL.

- [ ] **Step 3: Implement**

```tsx
// web/src/components/settings/TestConnectionButton.tsx
'use client';

import { useState } from 'react';
import { Button } from '@/components/ui/button';

type TestResult = { ok: boolean; version?: string; error?: string };

export function TestConnectionButton({ onTest }: { onTest: () => Promise<TestResult> }) {
  const [result, setResult] = useState<TestResult | null>(null);
  const [busy, setBusy] = useState(false);

  async function run() {
    setBusy(true);
    setResult(null);
    try {
      setResult(await onTest());
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex items-center gap-3">
      <Button type="button" onClick={run} disabled={busy} variant="outline">
        Test connection
      </Button>
      {result && (
        <span className={result.ok ? 'text-green-600' : 'text-destructive'}>
          {result.ok ? `Connected (${result.version ?? 'OK'})` : result.error || 'Failed'}
        </span>
      )}
    </div>
  );
}
```

- [ ] **Step 4: Run test**

Run: `(cd web && npx vitest run src/components/settings/TestConnectionButton.test.tsx)`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/settings/TestConnectionButton.tsx web/src/components/settings/TestConnectionButton.test.tsx
git commit -m "feat(web): TestConnectionButton with inline result display"
```

---

### Task 4.5: PathListEditor component

**Files:** Create: `web/src/components/settings/PathListEditor.tsx`, `web/src/components/settings/PathListEditor.test.tsx`.

- [ ] **Step 1: Failing test**

```tsx
// web/src/components/settings/PathListEditor.test.tsx
import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { PathListEditor } from './PathListEditor';

describe('PathListEditor', () => {
  it('lists existing paths', () => {
    render(
      <PathListEditor
        title="TV Watch Folders"
        paths={['/a', '/b']}
        onAdd={vi.fn()}
        onRemove={vi.fn()}
        preflight={async () => ({ readable: true, writable: true, free_space_bytes: 1024, warnings: [] })}
      />,
    );
    expect(screen.getByText('/a')).toBeInTheDocument();
    expect(screen.getByText('/b')).toBeInTheDocument();
  });

  it('calls onRemove with the right index', () => {
    const onRemove = vi.fn();
    render(
      <PathListEditor
        title="x"
        paths={['/a', '/b']}
        onAdd={vi.fn()}
        onRemove={onRemove}
        preflight={async () => ({ readable: true, writable: true, free_space_bytes: 0, warnings: [] })}
      />,
    );
    fireEvent.click(screen.getAllByRole('button', { name: /remove/i })[1]);
    fireEvent.change(screen.getByPlaceholderText(/type the path/i), { target: { value: '/b' } });
    fireEvent.click(screen.getByRole('button', { name: /confirm/i }));
    expect(onRemove).toHaveBeenCalledWith(1);
  });
});
```

- [ ] **Step 2: Run, verify fail**

Run: `(cd web && npx vitest run src/components/settings/PathListEditor.test.tsx)`
Expected: FAIL.

- [ ] **Step 3: Implement**

```tsx
// web/src/components/settings/PathListEditor.tsx
'use client';

import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Trash2 } from 'lucide-react';

type PreflightResult = {
  readable: boolean;
  writable: boolean;
  free_space_bytes: number;
  warnings: string[];
};

type Props = {
  title: string;
  paths: string[];
  onAdd: (path: string) => Promise<void> | void;
  onRemove: (index: number) => Promise<void> | void;
  preflight: (path: string) => Promise<PreflightResult>;
};

export function PathListEditor({ title, paths, onAdd, onRemove, preflight }: Props) {
  const [adding, setAdding] = useState(false);
  const [draft, setDraft] = useState('');
  const [draftPreflight, setDraftPreflight] = useState<PreflightResult | null>(null);
  const [removeIdx, setRemoveIdx] = useState<number | null>(null);
  const [confirmText, setConfirmText] = useState('');

  async function checkDraft(v: string) {
    setDraft(v);
    if (!v) { setDraftPreflight(null); return; }
    setDraftPreflight(await preflight(v));
  }

  return (
    <div className="space-y-3 rounded border p-4">
      <div className="flex items-center justify-between">
        <h3 className="font-semibold">{title}</h3>
        <Button size="sm" onClick={() => setAdding(true)}>+ Add path</Button>
      </div>

      {paths.length === 0 && (
        <p className="text-sm text-muted-foreground">No paths configured.</p>
      )}

      <ul className="divide-y">
        {paths.map((p, i) => (
          <li key={p + i} className="flex items-center justify-between py-2">
            <span className="font-mono text-sm">{p}</span>
            <Button size="sm" variant="ghost" onClick={() => setRemoveIdx(i)} aria-label="Remove">
              <Trash2 className="h-4 w-4" />
            </Button>
          </li>
        ))}
      </ul>

      {adding && (
        <div className="space-y-2 rounded border p-3 bg-muted/30">
          <Input
            placeholder="/path/to/folder"
            value={draft}
            onChange={(e) => checkDraft(e.target.value)}
          />
          {draftPreflight && (
            <p className={draftPreflight.readable ? 'text-sm text-green-600' : 'text-sm text-destructive'}>
              {draftPreflight.readable ? `OK — ${(draftPreflight.free_space_bytes / 1e9).toFixed(1)} GB free` : draftPreflight.warnings.join('; ')}
            </p>
          )}
          <div className="flex gap-2">
            <Button
              size="sm"
              disabled={!draftPreflight?.readable}
              onClick={async () => { await onAdd(draft); setAdding(false); setDraft(''); setDraftPreflight(null); }}
            >
              Add
            </Button>
            <Button size="sm" variant="ghost" onClick={() => { setAdding(false); setDraft(''); }}>Cancel</Button>
          </div>
        </div>
      )}

      {removeIdx !== null && (
        <div className="space-y-2 rounded border border-destructive/50 p-3 bg-destructive/5">
          <p className="text-sm">
            Type <span className="font-mono">{paths[removeIdx]}</span> to confirm removal.
          </p>
          <Input
            placeholder="type the path to confirm"
            value={confirmText}
            onChange={(e) => setConfirmText(e.target.value)}
          />
          <div className="flex gap-2">
            <Button
              size="sm"
              variant="destructive"
              disabled={confirmText !== paths[removeIdx]}
              onClick={async () => { await onRemove(removeIdx); setRemoveIdx(null); setConfirmText(''); }}
            >
              Confirm remove
            </Button>
            <Button size="sm" variant="ghost" onClick={() => { setRemoveIdx(null); setConfirmText(''); }}>Cancel</Button>
          </div>
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 4: Run test**

Run: `(cd web && npx vitest run src/components/settings/PathListEditor.test.tsx)`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/settings/PathListEditor.tsx web/src/components/settings/PathListEditor.test.tsx
git commit -m "feat(web): PathListEditor with preflight on add and typed confirm on remove"
```

---

### Task 4.6: usePathPreflight hook (debounced)

**Files:** Create: `web/src/hooks/usePathPreflight.ts`, `web/src/hooks/usePathPreflight.test.ts`.

- [ ] **Step 1: Failing test**

```ts
// web/src/hooks/usePathPreflight.test.ts
import { describe, expect, it, vi } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';
import { usePathPreflight } from './usePathPreflight';

describe('usePathPreflight', () => {
  it('debounces requests', async () => {
    const fn = vi.fn(async () => ({ readable: true, writable: true, free_space_bytes: 0, warnings: [] }));
    const { result } = renderHook(() => usePathPreflight(fn, 50));
    act(() => { result.current.check('/a'); result.current.check('/b'); result.current.check('/c'); });
    await waitFor(() => expect(fn).toHaveBeenCalledTimes(1));
    expect(fn).toHaveBeenCalledWith('/c');
  });
});
```

- [ ] **Step 2: Run, verify fail**

Run: `(cd web && npx vitest run src/hooks/usePathPreflight.test.ts)`
Expected: FAIL.

- [ ] **Step 3: Implement**

```ts
// web/src/hooks/usePathPreflight.ts
'use client';

import { useCallback, useRef, useState } from 'react';

export type PreflightResult = {
  readable: boolean;
  writable: boolean;
  free_space_bytes: number;
  warnings: string[];
};

type ProbeFn = (path: string) => Promise<PreflightResult>;

export function usePathPreflight(probe: ProbeFn, debounceMs = 300) {
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [result, setResult] = useState<PreflightResult | null>(null);

  const check = useCallback(
    (path: string) => {
      if (timer.current) clearTimeout(timer.current);
      if (!path) { setResult(null); return; }
      timer.current = setTimeout(async () => {
        const r = await probe(path);
        setResult(r);
      }, debounceMs);
    },
    [probe, debounceMs],
  );

  return { check, result };
}
```

- [ ] **Step 4: Run test**

Run: `(cd web && npx vitest run src/hooks/usePathPreflight.test.ts)`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/hooks/usePathPreflight.ts web/src/hooks/usePathPreflight.test.ts
git commit -m "feat(web): usePathPreflight debounced hook"
```

---

## Phase 5 — Section pages

> Each section page is small (<150 LOC). The pattern is identical: load section JSON, render the form, wire `SettingsForm.onSave` to a `PUT /settings/{name}` call. We'll build one canonical example (Sonarr) and three array-based pages, then mirror for the rest.

### Task 5.1: Sonarr settings page

**Files:** Create: `web/src/app/settings/sonarr/page.tsx`. Modify: `web/src/lib/api/client.ts`.

- [ ] **Step 1: Add typed client methods**

In `web/src/lib/api/client.ts`:

```ts
// extend the existing `api` helper with PUT:

export const api = {
  get: <T>(endpoint: string) => apiRequest<T>(endpoint, { method: 'GET' }),
  post: <T>(endpoint: string, body?: unknown) =>
    apiRequest<T>(endpoint, {
      method: 'POST',
      body: body ? JSON.stringify(body) : undefined,
    }),
  put: <T>(endpoint: string, body?: unknown) =>
    apiRequest<T>(endpoint, {
      method: 'PUT',
      body: body ? JSON.stringify(body) : undefined,
    }),
  delete: <T>(endpoint: string) => apiRequest<T>(endpoint, { method: 'DELETE' }),
};

// at the bottom of the file, alongside existing api functions:

export type SaveResult = {
  saved: boolean;
  validation?: { warnings?: string[] };
  reload: { ok: boolean; reloaded: string[]; failed: { name: string; error: string }[] };
  restored_previous_config?: boolean;
};

export async function getSettingsSection<T = unknown>(name: string): Promise<T> {
  return api.get<T>(`/settings/${name}`);
}

export async function putSettingsSection(name: string, body: unknown): Promise<SaveResult> {
  return api.put<SaveResult>(`/settings/${name}`, body);
}

export async function testConnection(section: 'sonarr' | 'radarr' | 'jellyfin', body: unknown) {
  return api.post<{ ok: boolean; version?: string; error?: string }>(`/settings/${section}/test`, body);
}

export async function preflightPath(path: string, kind: 'watch' | 'library') {
  return api.post<PreflightResult>('/paths/preflight', { path, kind });
}

export type PreflightResult = {
  readable: boolean;
  writable: boolean;
  free_space_bytes: number;
  warnings: string[];
};
```

- [ ] **Step 2: Implement page**

```tsx
// web/src/app/settings/sonarr/page.tsx
'use client';

import { useEffect, useState } from 'react';
import { SettingsForm } from '@/components/settings/SettingsForm';
import { SecretField } from '@/components/settings/SecretField';
import { TestConnectionButton } from '@/components/settings/TestConnectionButton';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { getSettingsSection, putSettingsSection, testConnection } from '@/lib/api/client';

type SonarrConfig = {
  enabled: boolean;
  url: string;
  api_key: string;
  notify_on_import: boolean;
};

export default function SonarrSettingsPage() {
  const [cfg, setCfg] = useState<SonarrConfig | null>(null);

  useEffect(() => {
    getSettingsSection<SonarrConfig>('sonarr').then(setCfg);
  }, []);

  if (!cfg) return <p>Loading…</p>;

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">Sonarr</h1>
      <SettingsForm onSave={() => putSettingsSection('sonarr', cfg)}>
        <div className="flex items-center gap-3">
          <Switch checked={cfg.enabled} onCheckedChange={(v) => setCfg({ ...cfg, enabled: v })} />
          <Label>Enabled</Label>
        </div>
        <div className="space-y-1">
          <Label htmlFor="url">URL</Label>
          <Input id="url" value={cfg.url} onChange={(e) => setCfg({ ...cfg, url: e.target.value })} />
        </div>
        <SecretField
          label="API Key"
          name="api_key"
          defaultValue={cfg.api_key}
          onChange={(v) => setCfg({ ...cfg, api_key: v })}
        />
        <div className="flex items-center gap-3">
          <Switch
            checked={cfg.notify_on_import}
            onCheckedChange={(v) => setCfg({ ...cfg, notify_on_import: v })}
          />
          <Label>Notify on import</Label>
        </div>
        <TestConnectionButton onTest={() => testConnection('sonarr', cfg)} />
      </SettingsForm>
    </div>
  );
}
```

- [ ] **Step 3: Smoke build**

Run: `(cd web && npm run build)`
Expected: clean build.

- [ ] **Step 4: Commit**

```bash
git add web/src/app/settings/sonarr/page.tsx web/src/lib/api/client.ts
git commit -m "feat(web): sonarr settings page (canonical SettingsForm pattern)"
```

---

### Task 5.2: Radarr settings page (mirror of sonarr)

**Files:** Create: `web/src/app/settings/radarr/page.tsx`.

- [ ] **Step 1: Implement**

```tsx
// web/src/app/settings/radarr/page.tsx
'use client';

import { useEffect, useState } from 'react';
import { SettingsForm } from '@/components/settings/SettingsForm';
import { SecretField } from '@/components/settings/SecretField';
import { TestConnectionButton } from '@/components/settings/TestConnectionButton';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { getSettingsSection, putSettingsSection, testConnection } from '@/lib/api/client';

type RadarrConfig = {
  enabled: boolean;
  url: string;
  api_key: string;
  notify_on_import: boolean;
};

export default function RadarrSettingsPage() {
  const [cfg, setCfg] = useState<RadarrConfig | null>(null);
  useEffect(() => {
    getSettingsSection<RadarrConfig>('radarr').then(setCfg);
  }, []);
  if (!cfg) return <p>Loading…</p>;
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">Radarr</h1>
      <SettingsForm onSave={() => putSettingsSection('radarr', cfg)}>
        <div className="flex items-center gap-3">
          <Switch checked={cfg.enabled} onCheckedChange={(v) => setCfg({ ...cfg, enabled: v })} />
          <Label>Enabled</Label>
        </div>
        <div className="space-y-1">
          <Label htmlFor="url">URL</Label>
          <Input id="url" value={cfg.url} onChange={(e) => setCfg({ ...cfg, url: e.target.value })} />
        </div>
        <SecretField
          label="API Key"
          name="api_key"
          defaultValue={cfg.api_key}
          onChange={(v) => setCfg({ ...cfg, api_key: v })}
        />
        <div className="flex items-center gap-3">
          <Switch
            checked={cfg.notify_on_import}
            onCheckedChange={(v) => setCfg({ ...cfg, notify_on_import: v })}
          />
          <Label>Notify on import</Label>
        </div>
        <TestConnectionButton onTest={() => testConnection('radarr', cfg)} />
      </SettingsForm>
    </div>
  );
}
```

- [ ] **Step 2: Smoke build**

Run: `(cd web && npm run build)`
Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add web/src/app/settings/radarr/page.tsx
git commit -m "feat(web): radarr settings page"
```

---

### Task 5.3: Jellyfin settings page

**Files:** Create: `web/src/app/settings/jellyfin/page.tsx`.

- [ ] **Step 1: Implement**

```tsx
// web/src/app/settings/jellyfin/page.tsx
'use client';

import { useEffect, useState } from 'react';
import { SettingsForm } from '@/components/settings/SettingsForm';
import { SecretField } from '@/components/settings/SecretField';
import { TestConnectionButton } from '@/components/settings/TestConnectionButton';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { getSettingsSection, putSettingsSection, testConnection } from '@/lib/api/client';

type JellyfinConfig = {
  url: string;
  api_key: string;
};

export default function JellyfinSettingsPage() {
  const [cfg, setCfg] = useState<JellyfinConfig | null>(null);
  useEffect(() => {
    getSettingsSection<JellyfinConfig>('jellyfin').then(setCfg);
  }, []);
  if (!cfg) return <p>Loading…</p>;
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">Jellyfin</h1>
      <SettingsForm onSave={() => putSettingsSection('jellyfin', cfg)}>
        <div className="space-y-1">
          <Label htmlFor="url">Server URL</Label>
          <Input id="url" value={cfg.url} onChange={(e) => setCfg({ ...cfg, url: e.target.value })} />
        </div>
        <SecretField
          label="API Key"
          name="api_key"
          defaultValue={cfg.api_key}
          onChange={(v) => setCfg({ ...cfg, api_key: v })}
        />
        <TestConnectionButton onTest={() => testConnection('jellyfin', cfg)} />
      </SettingsForm>
    </div>
  );
}
```

- [ ] **Step 2: Build & commit**

Run: `(cd web && npm run build)` → clean.

```bash
git add web/src/app/settings/jellyfin/page.tsx
git commit -m "feat(web): jellyfin settings page"
```

---

### Task 5.4: Paths page (watch folders)

**Files:** Create: `web/src/app/settings/paths/page.tsx`.

- [ ] **Step 1: Implement**

```tsx
// web/src/app/settings/paths/page.tsx
'use client';

import { useEffect, useState } from 'react';
import { PathListEditor } from '@/components/settings/PathListEditor';
import { preflightPath } from '@/lib/api/client';

type Kind = 'tv' | 'movies';

async function listPaths(kind: Kind): Promise<string[]> {
  const r = await fetch(`/api/v1/settings/paths/${kind}`);
  return (await r.json()).paths ?? [];
}

async function addPath(kind: Kind, path: string) {
  await fetch(`/api/v1/settings/paths/${kind}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path }),
  });
}

async function removePath(kind: Kind, index: number) {
  await fetch(`/api/v1/settings/paths/${kind}/${index}`, { method: 'DELETE' });
}

export default function PathsSettingsPage() {
  const [tv, setTv] = useState<string[]>([]);
  const [movies, setMovies] = useState<string[]>([]);

  useEffect(() => {
    listPaths('tv').then(setTv);
    listPaths('movies').then(setMovies);
  }, []);

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">Watch Folders</h1>
      <PathListEditor
        title="TV"
        paths={tv}
        onAdd={async (p) => { await addPath('tv', p); setTv(await listPaths('tv')); }}
        onRemove={async (i) => { await removePath('tv', i); setTv(await listPaths('tv')); }}
        preflight={(p) => preflightPath(p, 'watch')}
      />
      <PathListEditor
        title="Movies"
        paths={movies}
        onAdd={async (p) => { await addPath('movies', p); setMovies(await listPaths('movies')); }}
        onRemove={async (i) => { await removePath('movies', i); setMovies(await listPaths('movies')); }}
        preflight={(p) => preflightPath(p, 'watch')}
      />
    </div>
  );
}
```

- [ ] **Step 2: Build & commit**

Run: `(cd web && npm run build)` → clean.

```bash
git add web/src/app/settings/paths/page.tsx
git commit -m "feat(web): watch folders page (TV + movies)"
```

---

### Task 5.5: Libraries page

**Files:** Create: `web/src/app/settings/libraries/page.tsx`.

- [ ] **Step 1: Implement**

```tsx
// web/src/app/settings/libraries/page.tsx
'use client';

import { useEffect, useState } from 'react';
import { PathListEditor } from '@/components/settings/PathListEditor';
import { preflightPath } from '@/lib/api/client';

type Kind = 'tv' | 'movies';

async function list(kind: Kind): Promise<string[]> {
  const r = await fetch(`/api/v1/settings/libraries/${kind}`);
  return (await r.json()).paths ?? [];
}
async function add(kind: Kind, path: string) {
  await fetch(`/api/v1/settings/libraries/${kind}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path }),
  });
}
async function remove(kind: Kind, index: number) {
  await fetch(`/api/v1/settings/libraries/${kind}/${index}`, { method: 'DELETE' });
}

export default function LibrariesSettingsPage() {
  const [tv, setTv] = useState<string[]>([]);
  const [movies, setMovies] = useState<string[]>([]);
  useEffect(() => { list('tv').then(setTv); list('movies').then(setMovies); }, []);
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">Library Locations</h1>
      <PathListEditor
        title="TV"
        paths={tv}
        onAdd={async (p) => { await add('tv', p); setTv(await list('tv')); }}
        onRemove={async (i) => { await remove('tv', i); setTv(await list('tv')); }}
        preflight={(p) => preflightPath(p, 'library')}
      />
      <PathListEditor
        title="Movies"
        paths={movies}
        onAdd={async (p) => { await add('movies', p); setMovies(await list('movies')); }}
        onRemove={async (i) => { await remove('movies', i); setMovies(await list('movies')); }}
        preflight={(p) => preflightPath(p, 'library')}
      />
    </div>
  );
}
```

- [ ] **Step 2: Build & commit**

Run: `(cd web && npm run build)` → clean.

```bash
git add web/src/app/settings/libraries/page.tsx
git commit -m "feat(web): library locations page (TV + movies)"
```

---

### Task 5.6: Logging settings page

**Files:** Create: `web/src/app/settings/logging/page.tsx`.

- [ ] **Step 1: Implement**

```tsx
// web/src/app/settings/logging/page.tsx
'use client';

import { useEffect, useState } from 'react';
import { SettingsForm } from '@/components/settings/SettingsForm';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { getSettingsSection, putSettingsSection } from '@/lib/api/client';

type LoggingConfig = {
  level: 'trace' | 'debug' | 'info' | 'warn' | 'error';
  format: string;
};

export default function LoggingSettingsPage() {
  const [cfg, setCfg] = useState<LoggingConfig | null>(null);
  useEffect(() => { getSettingsSection<LoggingConfig>('logging').then(setCfg); }, []);
  if (!cfg) return <p>Loading…</p>;
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">Logging</h1>
      <SettingsForm onSave={() => putSettingsSection('logging', cfg)}>
        <div className="space-y-1">
          <Label>Level</Label>
          <Select value={cfg.level} onValueChange={(v) => setCfg({ ...cfg, level: v as LoggingConfig['level'] })}>
            <SelectTrigger className="w-48"><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="trace">trace</SelectItem>
              <SelectItem value="debug">debug</SelectItem>
              <SelectItem value="info">info</SelectItem>
              <SelectItem value="warn">warn</SelectItem>
              <SelectItem value="error">error</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </SettingsForm>
    </div>
  );
}
```

- [ ] **Step 2: Build & commit**

```bash
(cd web && npm run build)
git add web/src/app/settings/logging/page.tsx
git commit -m "feat(web): logging settings page (level select)"
```

---

### Task 5.7: Options settings page

**Files:** Create: `web/src/app/settings/options/page.tsx`.

- [ ] **Step 1: Implement**

```tsx
// web/src/app/settings/options/page.tsx
'use client';

import { useEffect, useState } from 'react';
import { SettingsForm } from '@/components/settings/SettingsForm';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { getSettingsSection, putSettingsSection } from '@/lib/api/client';

type OptionsConfig = {
  dry_run: boolean;
  delete_source: boolean;
  verify_checksums: boolean;
};

export default function OptionsSettingsPage() {
  const [cfg, setCfg] = useState<OptionsConfig | null>(null);
  useEffect(() => { getSettingsSection<OptionsConfig>('options').then(setCfg); }, []);
  if (!cfg) return <p>Loading…</p>;
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">Options</h1>
      <SettingsForm onSave={() => putSettingsSection('options', cfg)}>
        <Row label="Dry run" checked={cfg.dry_run} onChange={(v) => setCfg({ ...cfg, dry_run: v })} />
        <Row label="Delete source after import" checked={cfg.delete_source} onChange={(v) => setCfg({ ...cfg, delete_source: v })} />
        <Row label="Verify checksums" checked={cfg.verify_checksums} onChange={(v) => setCfg({ ...cfg, verify_checksums: v })} />
      </SettingsForm>
    </div>
  );
}

function Row({ label, checked, onChange }: { label: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <div className="flex items-center gap-3">
      <Switch checked={checked} onCheckedChange={onChange} />
      <Label>{label}</Label>
    </div>
  );
}
```

- [ ] **Step 2: Build & commit**

```bash
(cd web && npm run build)
git add web/src/app/settings/options/page.tsx
git commit -m "feat(web): options settings page (dry_run, delete_source, verify)"
```

---

### Task 5.7b: Permissions settings page

**Files:** Create: `web/src/app/settings/permissions/page.tsx`.

- [ ] **Step 1: Implement**

```tsx
// web/src/app/settings/permissions/page.tsx
'use client';

import { useEffect, useState } from 'react';
import { SettingsForm } from '@/components/settings/SettingsForm';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { getSettingsSection, putSettingsSection } from '@/lib/api/client';

type PermissionsConfig = {
  user: string;
  group: string;
  file_mode: string;
  dir_mode: string;
};

export default function PermissionsSettingsPage() {
  const [cfg, setCfg] = useState<PermissionsConfig | null>(null);
  useEffect(() => { getSettingsSection<PermissionsConfig>('permissions').then(setCfg); }, []);
  if (!cfg) return <p>Loading…</p>;
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">Permissions</h1>
      <SettingsForm onSave={() => putSettingsSection('permissions', cfg)}>
        <Field label="User" value={cfg.user ?? ''} onChange={(v) => setCfg({ ...cfg, user: v })} />
        <Field label="Group" value={cfg.group ?? ''} onChange={(v) => setCfg({ ...cfg, group: v })} />
        <Field label="File mode" value={cfg.file_mode ?? ''} onChange={(v) => setCfg({ ...cfg, file_mode: v })} />
        <Field label="Directory mode" value={cfg.dir_mode ?? ''} onChange={(v) => setCfg({ ...cfg, dir_mode: v })} />
      </SettingsForm>
    </div>
  );
}

function Field({ label, value, onChange }: { label: string; value: string; onChange: (v: string) => void }) {
  return (
    <div className="space-y-1">
      <Label>{label}</Label>
      <Input value={value} onChange={(e) => onChange(e.target.value)} />
    </div>
  );
}
```

- [ ] **Step 2: Build & commit**

```bash
(cd web && npm run build)
git add web/src/app/settings/permissions/page.tsx
git commit -m "feat(web): permissions settings page"
```

---

### Task 5.8: Update AI settings page to use SettingsForm

**Files:** Modify: `web/src/app/settings/ai/page.tsx`.

- [ ] **Step 1: Refactor**

Replace the existing AI page implementation with:

```tsx
// web/src/app/settings/ai/page.tsx
'use client';

import { useEffect, useState } from 'react';
import { SettingsForm } from '@/components/settings/SettingsForm';
import { TestConnectionButton } from '@/components/settings/TestConnectionButton';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { getSettingsSection, putSettingsSection, testConnection } from '@/lib/api/client';

type AIConfig = {
  enabled: boolean;
  ollama_endpoint: string;
  model: string;
  fallback_model: string;
  hourly_limit: number;
  daily_limit: number;
};

export default function AISettingsPage() {
  const [cfg, setCfg] = useState<AIConfig | null>(null);
  useEffect(() => { getSettingsSection<AIConfig>('ai').then(setCfg); }, []);
  if (!cfg) return <p>Loading…</p>;
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">AI</h1>
      <SettingsForm onSave={() => putSettingsSection('ai', cfg)}>
        <div className="flex items-center gap-3">
          <Switch checked={cfg.enabled} onCheckedChange={(v) => setCfg({ ...cfg, enabled: v })} />
          <Label>Enabled</Label>
        </div>
        <Field label="Ollama endpoint" value={cfg.ollama_endpoint} onChange={(v) => setCfg({ ...cfg, ollama_endpoint: v })} />
        <Field label="Primary model" value={cfg.model} onChange={(v) => setCfg({ ...cfg, model: v })} />
        <Field label="Fallback model" value={cfg.fallback_model} onChange={(v) => setCfg({ ...cfg, fallback_model: v })} />
        <Field label="Hourly limit" value={String(cfg.hourly_limit)} onChange={(v) => setCfg({ ...cfg, hourly_limit: Number(v) || 0 })} />
        <Field label="Daily limit" value={String(cfg.daily_limit)} onChange={(v) => setCfg({ ...cfg, daily_limit: Number(v) || 0 })} />
        <TestConnectionButton onTest={async () => {
          const r = await fetch('/api/v1/ai/test-connection', { method: 'POST' });
          const data = await r.json();
          return { ok: Boolean(data.success), version: data.modelCount ? `${data.modelCount} models` : undefined, error: data.message };
        }} />
      </SettingsForm>
    </div>
  );
}

function Field({ label, value, onChange }: { label: string; value: string; onChange: (v: string) => void }) {
  return (
    <div className="space-y-1">
      <Label>{label}</Label>
      <Input value={value} onChange={(e) => onChange(e.target.value)} />
    </div>
  );
}
```

- [ ] **Step 2: Build**

Run: `(cd web && npm run build)` → clean.

- [ ] **Step 3: Commit**

```bash
git add web/src/app/settings/ai/page.tsx
git commit -m "refactor(web): AI settings page adopts shared SettingsForm pipeline"
```

---

### Task 5.9: Settings overview page

**Files:** Modify: `web/src/app/settings/page.tsx`.

- [ ] **Step 1: Replace with overview cards**

```tsx
// web/src/app/settings/page.tsx
'use client';

import Link from 'next/link';

const SECTIONS = [
  { href: '/settings/paths',     title: 'Watch Folders',    desc: 'Where the daemon scans for new media' },
  { href: '/settings/libraries', title: 'Library Locations', desc: 'Where organized media lives'         },
  { href: '/settings/sonarr',    title: 'Sonarr',            desc: 'TV manager integration'              },
  { href: '/settings/radarr',    title: 'Radarr',            desc: 'Movie manager integration'           },
  { href: '/settings/jellyfin',  title: 'Jellyfin',          desc: 'Library notifications'               },
  { href: '/settings/ai',        title: 'AI',                desc: 'Title-matching model and limits'     },
  { href: '/settings/options',   title: 'Options',           desc: 'Dry-run, delete source, checksums'   },
  { href: '/settings/logging',   title: 'Logging',           desc: 'Log level and rotation'              },
  { href: '/settings/permissions', title: 'Permissions',      desc: 'Ownership and file modes'           },
];

export default function SettingsOverview() {
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">Settings</h1>
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        {SECTIONS.map((s) => (
          <Link key={s.href} href={s.href} className="block rounded border p-4 transition-colors hover:bg-accent/50">
            <h2 className="font-semibold">{s.title}</h2>
            <p className="text-sm text-muted-foreground">{s.desc}</p>
          </Link>
        ))}
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Build & commit**

```bash
(cd web && npm run build)
git add web/src/app/settings/page.tsx
git commit -m "refactor(web): settings overview as section cards"
```

---

## Phase 6 — Integration & verification

### Task 6.1: End-to-end smoke (jellyweb + jellywatchd)

**Files:** Create: `internal/api/integration_test.go`.

- [ ] **Step 1: Write integration test**

```go
// internal/api/integration_test.go
//go:build integration

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
	"github.com/Nomadcxx/jellywatch/internal/daemon/reload"
)

func TestSettingsRoundtripWithReload(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	// 1. Start IPC server (the "daemon" side).
	sock := filepath.Join(dir, "jellywatch", "control.sock")
	if err := config.AtomicWriteWithLock(filepath.Join(dir, "jellywatch", "config.toml"), []byte(""), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, _ := config.Load()

	level := loaded.Logging.Level
	reload.Default = reload.NewSupervisor()
	reload.Register(reload.NewLoggingReloadable(&level))

	srv := ipc.NewServer(sock)
	srv.Register(ipc.CmdReload, func(ctx context.Context, req ipc.Request, w ipc.FrameWriter) {
		fresh, _ := config.Load()
		res := reload.Default.Reload(ctx, loaded, fresh)
		loaded = fresh
		b, _ := json.Marshal(res)
		w.Result(req.ID, b)
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	// 2. Wire jellyweb side: settings handler with a real IPC client.
	cli := ipc.NewClient(sock)
	h := &SettingsHandlers{Cfg: loaded, IPC: ipcCallerAdapter{c: cli}}

	// 3. PUT /settings/logging.
	body, _ := json.Marshal(config.LoggingConfig{Level: "debug"})
	req := httptest.NewRequest("PUT", "/settings/logging", bytes.NewReader(body))
	req = withChiRouteParam(req, "section", "logging")
	w := httptest.NewRecorder()
	h.Put(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}

	// 4. Verify the on-disk config and the live level both flipped to debug.
	time.Sleep(50 * time.Millisecond)
	disk, _ := config.Load()
	if disk.Logging.Level != "debug" {
		t.Errorf("disk level %q", disk.Logging.Level)
	}
	if level != "debug" {
		t.Errorf("live level %q", level)
	}
}
```

The helpers `withChiRouteParam` and `ipcCallerAdapter` are tiny test glue — add them to a `testhelpers_test.go` in the same package:

```go
// internal/api/testhelpers_test.go
package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
	"github.com/go-chi/chi/v5"
)

func withChiRouteParam(r *http.Request, key, val string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, val)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

type ipcCallerAdapter struct{ c *ipc.Client }

func (a ipcCallerAdapter) Call(ctx context.Context, cmd ipc.Command, args any) (json.RawMessage, error) {
	return a.c.Call(ctx, cmd, args)
}
```

- [ ] **Step 2: Run integration tests**

Run: `go test -tags=integration ./internal/api/...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/api/integration_test.go internal/api/testhelpers_test.go
git commit -m "test(api): integration smoke for settings PUT → daemon RELOAD round-trip"
```

---

### Task 6.2: Final build & lint sweep

- [ ] **Step 1: Run full check**

```bash
go vet ./...
go test ./...
(cd web && npm run lint && npm run build)
```

Expected: clean across the board.

- [ ] **Step 2: Update CHANGELOG / README if either exists with installation notes**

Run: `grep -l "config" README.md docs/*.md 2>/dev/null`
If a user-facing doc mentions editing `config.toml` directly, add a sentence pointing them to `/settings/*` in the webui.

- [ ] **Step 3: Final commit**

```bash
git add -A docs/ README.md
git commit -m "docs: point config.toml hand-editing notes at the webui settings UI" --allow-empty
```

---

## Self-Review (run before merging)

- [ ] **Spec coverage:** Every section in spec §3, §4 (subset: STATUS+RELOAD), §5.1–§5.4, §6 has at least one task. Validation table §7 entries: schema (Task 3.3), filesystem (Task 3.6), connection (Task 3.5), atomic write (Task 2.1), flock (Task 2.1), IPC RELOAD (Task 1.5), SO_PEERCRED (Task 1.2). Op log + crash recovery + STOP/RESCAN/RESET_DB/ATTACH/CANCEL **deferred to Plan 2** by design.
- [ ] **Placeholder scan:** Search the plan for `TBD`, `TODO`, `etc.` — fix any.
- [ ] **Type consistency:** `IPCCaller`, `Reloadable`, `Commit`, `Rollback`, `SettingsForm`, `SaveResult`, `ReloadResult`, `PreflightResult` are referenced consistently throughout.
- [ ] All commit messages are conventional and contain no AI attribution.

---

**Plan complete.** Two execution options:

1. **Subagent-Driven (recommended)** — fresh subagent per task, review between tasks.
2. **Inline Execution** — execute tasks in this session with batch checkpoints.

Plan 2 (Daemon & Database lifecycle) and Plan 3 (Observability) are tracked separately. Plan 3 requires Design 2 to be brainstormed first.
