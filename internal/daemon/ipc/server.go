package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"
)

const (
	maxFrameBytes        = 64 * 1024
	idleTimeout          = 5 * time.Minute
	maxConcurrentPerConn = 50
)

var requestTimeout = 30 * time.Second

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
	socketOwnerUID  int
	socketOwnerGID  int
	socketOwnerSet  bool
	listener        *net.UnixListener
	wg              sync.WaitGroup
	mu              sync.Mutex
	stopped         bool
	registry        *OpRegistry
}

func NewServer(path string) *Server {
	return &Server{
		path:            path,
		handlers:        make(map[Command]Handler),
		allowedPeerUIDs: map[int]struct{}{os.Getuid(): {}},
		registry:        NewOpRegistry(),
	}
}

// Path returns the unix socket path; tests use it to dial the running server.
func (s *Server) Path() string { return s.path }

// SetRegistry replaces the default registry. Streaming handlers panic at
// request time if the registry is nil — NewServer already provides one.
func (s *Server) SetRegistry(r *OpRegistry) {
	if r == nil {
		panic("ipc: SetRegistry(nil)")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.registry = r
}

// Registry exposes the op registry so handlers in main can access it.
func (s *Server) Registry() *OpRegistry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.registry
}

// StreamingHandler is invoked once per streaming request. The server has
// already allocated an op_id, started the registry entry, and started a
// heartbeat goroutine before calling.
type StreamingHandler func(ctx context.Context, args json.RawMessage, w FrameWriter, op *Op)

// RegisterStreaming wraps a streaming handler with op_id allocation,
// registry tracking, and frame ring mirroring.
func (s *Server) RegisterStreaming(cmd Command, h StreamingHandler) {
	if s.registry == nil {
		panic("ipc: RegisterStreaming requires a registry")
	}
	s.Register(cmd, func(ctx context.Context, req Request, w FrameWriter) {
		// ponytail: detach from the 30s unary requestTimeout — streaming ops
		// (rescan, dup scan, reset) run for minutes; cancel comes via registry.
		opCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
		op, err := s.registry.Start(req.ID, cmd, cancel)
		if err != nil {
			w.Error(req.ID, ErrBusy, err.Error())
			return
		}
		defer s.registry.Finish(op.ID, "done", nil)
		ringW := &ringWriter{inner: w, ring: op.Frames}
		hbDone := make(chan struct{})
		go heartbeatLoop(opCtx, req.ID, ringW, hbDone)
		defer func() {
			if r := recover(); r != nil {
				op.Final = &FinalState{State: "error", Msg: fmt.Sprintf("handler panic: %v", r), At: time.Now()}
			}
			close(hbDone)
		}()
		h(opCtx, req.Args, ringW, op)
	})
}

func (s *Server) Register(cmd Command, h Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[cmd] = h
}

func (s *Server) AddAllowedPeerUID(uid int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.allowedPeerUIDs[uid] = struct{}{}
}

func (s *Server) SetSocketOwner(uid, gid int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.socketOwnerUID = uid
	s.socketOwnerGID = gid
	s.socketOwnerSet = true
}

func (s *Server) Start(ctx context.Context) error {
	if _, err := os.Stat(s.path); err == nil {
		if c, err := net.DialTimeout("unix", s.path, 200*time.Millisecond); err == nil {
			_ = c.Close()
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
	if s.socketOwnerSet {
		if err := os.Chown(s.path, s.socketOwnerUID, s.socketOwnerGID); err != nil {
			_ = l.Close()
			return err
		}
	}
	if err := os.Chmod(s.path, 0600); err != nil {
		_ = l.Close()
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
	l := s.listener
	s.mu.Unlock()

	if l != nil {
		_ = l.Close()
	}
	s.wg.Wait()
	_ = os.Remove(s.path)
}

func (s *Server) acceptLoop(ctx context.Context) {
	defer s.wg.Done()
	for {
		_ = s.listener.SetDeadline(time.Now().Add(time.Second))
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
	sem := make(chan struct{}, maxConcurrentPerConn)
	for {
		_ = c.SetReadDeadline(time.Now().Add(idleTimeout))
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

		s.mu.Lock()
		h := s.handlers[req.Cmd]
		s.mu.Unlock()
		if h == nil {
			w.Error(req.ID, ErrNotImplemented, string(req.Cmd))
			continue
		}
		select {
		case sem <- struct{}{}:
			reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
			go func() {
				defer func() {
					cancel()
					<-sem
				}()
				h(reqCtx, req, w)
			}()
		default:
			w.Error(req.ID, ErrBusy, "too many concurrent requests on this connection")
		}
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

func newFrameWriter(c net.Conn) *frameWriter {
	return &frameWriter{c: c}
}

func (w *frameWriter) write(f Frame) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = json.NewEncoder(w.c).Encode(f)
}

func (w *frameWriter) Result(id string, data json.RawMessage) {
	w.write(Frame{ID: id, Type: FrameResult, Data: data})
}

func (w *frameWriter) Progress(id string, phase, msg string, current, total int) {
	w.write(Frame{ID: id, Type: FrameProgress, Phase: phase, Msg: msg, Current: current, Total: total})
}

func (w *frameWriter) Done(id string, data json.RawMessage) {
	w.write(Frame{ID: id, Type: FrameDone, Data: data})
}

func (w *frameWriter) Error(id string, code ErrorCode, msg string) {
	w.write(Frame{ID: id, Type: FrameError, Code: code, Msg: msg})
}
