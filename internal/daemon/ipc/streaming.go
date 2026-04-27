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
func (w *ringWriter) Progress(id string, phase, msg string, current, total int) {
	w.ring.Append(Frame{ID: id, Type: FrameProgress, Phase: phase, Msg: msg, Current: current, Total: total})
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

// AttachHandler replays the op's frame ring then keeps the connection
// open until the op finalizes (or the client disconnects).
func AttachHandler(s *Server) Handler {
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
		fw, _ := w.(*frameWriter)
		for _, f := range op.Frames.Snapshot() {
			if fw != nil {
				fw.write(f)
			}
		}
		op.mu.Lock()
		final := op.Final
		op.mu.Unlock()
		if final != nil {
			return
		}
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
					if fw != nil {
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

// CancelHandler cancels an in-flight op via the registry.
func CancelHandler(s *Server) Handler {
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

// ListOpsHandler returns a JSON snapshot of every tracked op.
func ListOpsHandler(s *Server) Handler {
	return func(ctx context.Context, req Request, w FrameWriter) {
		ops := s.registry.List()
		data, err := json.Marshal(map[string]any{"ops": ops})
		if err != nil {
			w.Error(req.ID, ErrInternal, err.Error())
			return
		}
		w.Result(req.ID, data)
	}
}
