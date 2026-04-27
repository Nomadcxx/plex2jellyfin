package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
	"github.com/go-chi/chi/v5"
)

// OpsHandlers exposes the daemon's operation registry to the web UI.
// Backs the global "Active Jobs" tray and per-op cancel actions.
type OpsHandlers struct {
	IPC IPCCaller
}

// List returns every op currently tracked by the daemon's registry
// (running plus recently-finished, until evicted by TTL).
func (h *OpsHandlers) List(w http.ResponseWriter, r *http.Request) {
	if h.IPC == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ops": []any{}})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	raw, err := h.IPC.Call(ctx, ipc.CmdListOps, nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, "ipc_error", err.Error())
		return
	}
	var payload struct {
		Ops []ipc.OpSummary `json:"ops"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		writeError(w, http.StatusInternalServerError, "decode_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// Cancel sends CmdCancel to the daemon for the given op id.
func (h *OpsHandlers) Cancel(w http.ResponseWriter, r *http.Request) {
	if h.IPC == nil {
		writeError(w, http.StatusServiceUnavailable, "ipc_unavailable", "daemon IPC not connected")
		return
	}
	id := chi.URLParam(r, "op_id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_op_id", "op_id is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if _, err := h.IPC.Call(ctx, ipc.CmdCancel, map[string]string{"op_id": id}); err != nil {
		writeError(w, http.StatusBadGateway, "ipc_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cancelled": true, "op_id": id})
}
