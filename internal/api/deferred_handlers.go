package api

import (
	"context"
	"net/http"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/daemon/ipc"
)

const deferredIPCTimeout = 5 * time.Second

type DeferredHandlers struct {
	IPC IPCCaller
}

func (h *DeferredHandlers) List(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), deferredIPCTimeout)
	defer cancel()
	raw, err := h.IPC.Call(ctx, ipc.CmdDeferred, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
}
