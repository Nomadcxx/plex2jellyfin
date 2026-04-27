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

func (l *OpLog) MarkDiscarded(id string) error { return l.End(id, "cancelled", "discarded by recovery") }

var ErrInterruptedOp = errors.New("interrupted op exists; recovery required")
