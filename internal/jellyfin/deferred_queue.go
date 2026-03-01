package jellyfin

import (
	"strings"
	"sync"
	"time"
)

// DeferredOp represents an action deferred due to active playback.
type DeferredOp struct {
	Type       string
	SourcePath string
	TargetPath string
	Reason     string
	DeferredAt time.Time
	RetryCount int
}

// DeferredQueue stores deferred operations per locked file path.
type DeferredQueue struct {
	mu  sync.RWMutex
	ops map[string][]DeferredOp
}

func NewDeferredQueue() *DeferredQueue {
	return &DeferredQueue{ops: make(map[string][]DeferredOp)}
}

func (q *DeferredQueue) Add(path string, op DeferredOp) {
	key := strings.TrimSpace(path)
	if key == "" {
		return
	}
	if op.DeferredAt.IsZero() {
		op.DeferredAt = time.Now()
	}

	q.mu.Lock()
	q.ops[key] = append(q.ops[key], op)
	q.mu.Unlock()
}

func (q *DeferredQueue) GetForPath(path string) []DeferredOp {
	key := strings.TrimSpace(path)
	if key == "" {
		return nil
	}

	q.mu.RLock()
	defer q.mu.RUnlock()

	ops := q.ops[key]
	if len(ops) == 0 {
		return nil
	}

	out := make([]DeferredOp, len(ops))
	copy(out, ops)
	return out
}

func (q *DeferredQueue) RemoveForPath(path string) []DeferredOp {
	key := strings.TrimSpace(path)
	if key == "" {
		return nil
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	ops := q.ops[key]
	if len(ops) == 0 {
		return nil
	}

	out := make([]DeferredOp, len(ops))
	copy(out, ops)
	delete(q.ops, key)
	return out
}

func (q *DeferredQueue) GetAll() map[string][]DeferredOp {
	q.mu.RLock()
	defer q.mu.RUnlock()

	out := make(map[string][]DeferredOp, len(q.ops))
	for key, ops := range q.ops {
		copied := make([]DeferredOp, len(ops))
		copy(copied, ops)
		out[key] = copied
	}
	return out
}

func (q *DeferredQueue) Count() int {
	q.mu.RLock()
	defer q.mu.RUnlock()

	total := 0
	for _, ops := range q.ops {
		total += len(ops)
	}
	return total
}
