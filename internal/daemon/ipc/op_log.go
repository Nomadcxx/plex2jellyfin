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
	State     string         `json:"state"`
	Msg       string         `json:"msg,omitempty"`
	StartedAt time.Time      `json:"started_at"`
	EndedAt   *time.Time     `json:"ended_at,omitempty"`
}

// OpLog is an append-only JSONL record of destructive ops.
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

func (l *OpLog) Begin(id string, cmd Command, args map[string]any) error {
	return l.append(OpLogEntry{
		ID: id, Cmd: cmd, Args: args,
		State: "in_progress", StartedAt: time.Now(),
	})
}

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

func (l *OpLog) MarkDiscarded(id string) error {
	return l.End(id, "cancelled", "discarded by recovery")
}

// RecentByCmd returns the most recent finished entries (most-recent first)
// for the given command, merged with their Begin metadata so callers can
// compute durations. n=0 returns all matching entries.
func RecentByCmd(path string, cmd Command, n int) ([]OpLogEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	begins := map[string]OpLogEntry{}
	type pair struct {
		begin, end OpLogEntry
	}
	var done []pair
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var e OpLogEntry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue
		}
		if e.State == "in_progress" {
			if e.Cmd == cmd {
				begins[e.ID] = e
			}
			continue
		}
		b, ok := begins[e.ID]
		if !ok {
			continue
		}
		delete(begins, e.ID)
		done = append(done, pair{begin: b, end: e})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	out := make([]OpLogEntry, 0, len(done))
	for i := len(done) - 1; i >= 0; i-- {
		merged := done[i].begin
		merged.State = done[i].end.State
		merged.Msg = done[i].end.Msg
		merged.EndedAt = done[i].end.EndedAt
		out = append(out, merged)
		if n > 0 && len(out) >= n {
			break
		}
	}
	return out, nil
}

var ErrInterruptedOp = errors.New("interrupted op exists; recovery required")
