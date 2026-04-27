package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
	"github.com/go-chi/chi/v5"
)

// IPCAttacher is the subset of the IPC client surface used by the SSE relay.
// Production wiring is satisfied by *ipc.Client.
type IPCAttacher interface {
	Attach(ctx context.Context, opID string) (<-chan ipc.Frame, error)
}

// SSERelay forwards IPC progress frames as Server-Sent Events.
type SSERelay struct {
	IPC IPCAttacher
}

// Stream attaches to the op identified by the {op_id} route param and relays
// every frame to the client until the stream ends, the request context is
// cancelled, or a terminal frame (done/error) is observed.
func (s *SSERelay) Stream(w http.ResponseWriter, r *http.Request) {
	opID := chi.URLParam(r, "op_id")
	frames, err := s.IPC.Attach(r.Context(), opID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	for {
		select {
		case <-r.Context().Done():
			return
		case f, ok := <-frames:
			if !ok {
				return
			}
			b, _ := json.Marshal(f)
			fmt.Fprintf(w, "data: %s\n\n", b)
			if flusher != nil {
				flusher.Flush()
			}
			if f.Type == ipc.FrameDone || f.Type == ipc.FrameError {
				return
			}
		}
	}
}
