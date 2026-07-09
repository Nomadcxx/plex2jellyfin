package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/Nomadcxx/plex2jellyfin/internal/daemon/ipc"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// StreamingOpHandlers exposes long-running daemon ops via the REST surface.
// Each handler accepts a body, hands it to the daemon over IPC, and returns
// {op_id} so the WebUI can subscribe to /api/v1/events/op/{op_id} via SSE.
type StreamingOpHandlers struct {
	IPC IPCCaller
}

func (h *StreamingOpHandlers) startStream(w http.ResponseWriter, r *http.Request, cmd ipc.Command, body any) {
	opID := "op-" + uuid.NewString()
	go h.IPC.StreamWithID(context.Background(), cmd, body, opID)
	respondAccepted(w, opID)
}

func (h *StreamingOpHandlers) Consolidate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DryRun bool `json:"dry_run"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	h.startStream(w, r, ipc.CmdConsolidate, body)
}

func (h *StreamingOpHandlers) DupScan(w http.ResponseWriter, r *http.Request) {
	h.startStream(w, r, ipc.CmdDupScan, struct{}{})
}

func (h *StreamingOpHandlers) AIBatch(w http.ResponseWriter, r *http.Request) {
	h.startStream(w, r, ipc.CmdAIBatch, struct{}{})
}

func (h *StreamingOpHandlers) MetadataRefresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ItemIDs []string `json:"item_ids"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	h.startStream(w, r, ipc.CmdMetadataRefresh, body)
}

func (h *StreamingOpHandlers) MetadataReconcile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Limit int `json:"limit"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	h.startStream(w, r, ipc.CmdMetadataReconcile, body)
}

func (h *StreamingOpHandlers) MetadataRepair(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Limit       int     `json:"limit"`
		DecisionIDs []int64 `json:"decision_ids"`
		DryRun      bool    `json:"dry_run"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	h.startStream(w, r, ipc.CmdMetadataRepair, body)
}

func (h *StreamingOpHandlers) MetadataRepairItem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "bad_id", "id must be a positive integer")
		return
	}
	body := struct {
		DecisionID int64 `json:"decision_id"`
		Limit      int   `json:"limit"`
	}{
		DecisionID: id,
		Limit:      1,
	}
	h.startStream(w, r, ipc.CmdMetadataRepair, body)
}

func (h *StreamingOpHandlers) Sweep(w http.ResponseWriter, r *http.Request) {
	var body struct {
		LookbackHours int `json:"lookback_hours"`
		TTLHours      int `json:"ttl_hours"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	h.startStream(w, r, ipc.CmdSweep, body)
}

func (h *StreamingOpHandlers) ParsesAudit(w http.ResponseWriter, r *http.Request) {
	h.startStream(w, r, ipc.CmdParsesAudit, struct{}{})
}
