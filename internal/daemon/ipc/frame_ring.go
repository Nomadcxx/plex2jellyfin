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
