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
	State string
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

var mutators = map[Command]bool{
	CmdRescan:  true,
	CmdResetDB: true,
}

func NewOpRegistry() *OpRegistry {
	return &OpRegistry{ops: map[string]*Op{}, ttl: 10 * time.Minute}
}

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
