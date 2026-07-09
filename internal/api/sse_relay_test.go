package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/daemon/ipc"
	"github.com/go-chi/chi/v5"
)

type fakeAttacher struct {
	frames <-chan ipc.Frame
	err    error
}

func (f *fakeAttacher) Attach(ctx context.Context, opID string) (<-chan ipc.Frame, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.frames, nil
}

func withChiRouteParam(r *http.Request, key, val string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, val)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestSSERelayForwardsFrames(t *testing.T) {
	ch := make(chan ipc.Frame, 4)
	ch <- ipc.Frame{ID: "x", Type: ipc.FrameProgress, Phase: "p", Msg: "hi"}
	ch <- ipc.Frame{ID: "x", Type: ipc.FrameDone}
	close(ch)
	h := &SSERelay{IPC: &fakeAttacher{frames: ch}}

	req := httptest.NewRequest("GET", "/events/op/x", nil)
	req = withChiRouteParam(req, "op_id", "x")
	ctx, cancel := context.WithTimeout(req.Context(), 500*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.Stream(w, req)

	if got := w.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q", got)
	}
	out := w.Body.String()
	if !strings.Contains(out, `"phase":"p"`) {
		t.Errorf("missing progress frame in SSE: %q", out)
	}
	if !strings.Contains(out, `"type":"done"`) {
		t.Errorf("missing terminal frame in SSE: %q", out)
	}
}

func TestSSERelayAttachError(t *testing.T) {
	h := &SSERelay{IPC: &fakeAttacher{err: errors.New("no such op")}}
	req := httptest.NewRequest("GET", "/events/op/x", nil)
	req = withChiRouteParam(req, "op_id", "x")
	w := httptest.NewRecorder()

	h.Stream(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}
