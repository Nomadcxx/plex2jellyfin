//go:build integration

package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
	"github.com/go-chi/chi/v5"
)

// ipcCallerAdapter satisfies both IPCCaller and IPCAttacher around the
// real ipc.Client.
type ipcCallerAdapter struct{ c *ipc.Client }

func (a ipcCallerAdapter) Call(ctx context.Context, cmd ipc.Command, args any) (json.RawMessage, error) {
	return a.c.Call(ctx, cmd, args)
}
func (a ipcCallerAdapter) StreamWithID(ctx context.Context, cmd ipc.Command, args any, opID string) error {
	return a.c.StreamWithID(ctx, cmd, args, opID)
}
func (a ipcCallerAdapter) Attach(ctx context.Context, opID string) (<-chan ipc.Frame, error) {
	return a.c.Attach(ctx, opID)
}

func TestRescanEndToEnd(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "ctl.sock")

	srv := ipc.NewServer(sock)
	srv.RegisterStreaming(ipc.CmdRescan, func(ctx context.Context, _ json.RawMessage, w ipc.FrameWriter, op *ipc.Op) {
		w.Progress(op.ID, "walking", "/x", 1, 2)
		w.Progress(op.ID, "indexing", "/x", 2, 2)
		w.Done(op.ID, json.RawMessage(`{"ok":true}`))
	})
	srv.Register(ipc.CmdAttach, ipc.AttachHandler(srv))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("ipc server start: %v", err)
	}
	defer srv.Stop()

	cli := ipc.NewClient(sock)
	adapter := ipcCallerAdapter{c: cli}
	dbH := &DatabaseHandlers{IPC: adapter}
	sse := &SSERelay{IPC: adapter}
	mux := chi.NewRouter()
	mux.Post("/database/rescan", dbH.Rescan)
	mux.Get("/events/op/{op_id}", sse.Stream)

	req := httptest.NewRequest("POST", "/database/rescan", strings.NewReader(`{"dry_run":false}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != 202 {
		t.Fatalf("rescan kickoff status %d", w.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode kickoff body: %v", err)
	}
	opID := body["op_id"]
	if opID == "" {
		t.Fatal("missing op_id")
	}

	// Give the streaming handler a moment to register the op so Attach
	// finds it on first poll.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := srv.Registry().Get(opID); ok {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	req2 := httptest.NewRequest("GET", "/events/op/"+opID, nil)
	w2 := httptest.NewRecorder()
	ctx2, cancel2 := context.WithTimeout(req2.Context(), 3*time.Second)
	defer cancel2()
	mux.ServeHTTP(w2, req2.WithContext(ctx2))

	out := w2.Body.String()
	if !strings.Contains(out, `"phase":"walking"`) {
		t.Errorf("missing walking frame in SSE: %s", out)
	}
	if !strings.Contains(out, `"type":"done"`) {
		t.Errorf("missing done frame in SSE: %s", out)
	}
}
