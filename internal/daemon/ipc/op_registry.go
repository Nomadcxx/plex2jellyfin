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

// OpSummary is a serialisable snapshot of an op for status/list APIs.
type OpSummary struct {
	ID         string  `json:"id"`
	Cmd        string  `json:"cmd"`
	StartedAt  int64   `json:"started_at"`
	State      string  `json:"state"`
	Phase      string  `json:"phase,omitempty"`
	Msg        string  `json:"msg,omitempty"`
	Current    int     `json:"current,omitempty"`
	Total      int     `json:"total,omitempty"`
	FinalCode  string  `json:"final_code,omitempty"`
	FinalMsg   string  `json:"final_msg,omitempty"`
	FinishedAt int64   `json:"finished_at,omitempty"`
	Pct        float64 `json:"pct,omitempty"`
}

// List returns a snapshot of every tracked op (running and recently
// finished, until evicted by TTL). Sorted by StartedAt descending.
func (r *OpRegistry) List() []OpSummary {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]OpSummary, 0, len(r.ops))
	for _, op := range r.ops {
		op.mu.Lock()
		s := OpSummary{
			ID:        op.ID,
			Cmd:       string(op.Cmd),
			StartedAt: op.StartedAt.Unix(),
			State:     "running",
		}
		// Pull last progress frame for live phase/msg/current/total.
		frames := op.Frames.Snapshot()
		for i := len(frames) - 1; i >= 0; i-- {
			f := frames[i]
			if f.Type == FrameProgress {
				s.Phase = f.Phase
				s.Msg = f.Msg
				s.Current = f.Current
				s.Total = f.Total
				if f.Total > 0 {
					s.Pct = float64(f.Current) / float64(f.Total) * 100
				}
				break
			}
		}
		if op.Final != nil {
			s.State = op.Final.State
			s.FinalCode = string(op.Final.Code)
			s.FinalMsg = op.Final.Msg
			s.FinishedAt = op.Final.At.Unix()
		}
		op.mu.Unlock()
		out = append(out, s)
	}
	// newest first
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].StartedAt > out[i].StartedAt {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
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
