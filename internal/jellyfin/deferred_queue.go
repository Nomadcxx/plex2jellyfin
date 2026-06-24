package jellyfin

import (
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/database"
)

// DeferredOp represents an action deferred due to active playback.
type DeferredOp struct {
	Type       string
	SourcePath string
	TargetPath string
	Reason     string
	DeferredAt time.Time
}

// DeferredQueue stores deferred operations per locked file path.
type DeferredQueue struct {
	mu  sync.RWMutex
	ops map[string][]DeferredOp
	db  *database.MediaDB
}

func NewDeferredQueue() *DeferredQueue {
	return &DeferredQueue{ops: make(map[string][]DeferredOp)}
}

func NewDeferredQueueWithDB(db *database.MediaDB) *DeferredQueue {
	return &DeferredQueue{ops: make(map[string][]DeferredOp), db: db}
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

	if q.db != nil {
		if err := q.db.SaveDeferredOp(database.DeferredOp{
			Path:       key,
			Type:       op.Type,
			SourcePath: op.SourcePath,
			TargetPath: op.TargetPath,
			Reason:     op.Reason,
			DeferredAt: op.DeferredAt,
		}); err != nil {
			log.Printf("[deferred-queue] failed to persist deferred op: %v", err)
		}
	}
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

	if q.db != nil {
		if err := q.db.DeleteDeferredOpsForPath(key); err != nil {
			log.Printf("[deferred-queue] failed to delete persisted ops: %v", err)
		}
	}

	return out
}

func (q *DeferredQueue) LoadFromDB() {
	if q.db == nil {
		return
	}
	ops, err := q.db.LoadDeferredOps()
	if err != nil {
		log.Printf("[deferred-queue] failed to load deferred ops from DB: %v", err)
		return
	}
	q.mu.Lock()
	for _, op := range ops {
		q.ops[op.Path] = append(q.ops[op.Path], DeferredOp{
			Type:       op.Type,
			SourcePath: op.SourcePath,
			TargetPath: op.TargetPath,
			Reason:     op.Reason,
			DeferredAt: op.DeferredAt,
		})
	}
	q.mu.Unlock()
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
