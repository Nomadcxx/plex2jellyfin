package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
)

type stubDaemonIPC struct {
	statusBody json.RawMessage
	streamErr  error
}

func (s stubDaemonIPC) Call(ctx context.Context, cmd ipc.Command, args any) (json.RawMessage, error) {
	return s.statusBody, nil
}

func (s stubDaemonIPC) StreamWithID(ctx context.Context, cmd ipc.Command, args any, opID string) error {
	return s.streamErr
}

func TestDaemonStatusReturnsRunning(t *testing.T) {
	h := &DaemonHandlers{IPC: stubDaemonIPC{statusBody: json.RawMessage(`{"state":"running","version":"x"}`)}}
	req := httptest.NewRequest("GET", "/daemon/status", nil)
	w := httptest.NewRecorder()
	h.Status(w, req)
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	var got map[string]any
	json.Unmarshal(w.Body.Bytes(), &got)
	if got["state"] != "running" {
		t.Errorf("state = %v", got["state"])
	}
}
