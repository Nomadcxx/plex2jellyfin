package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/Nomadcxx/plex2jellyfin/internal/daemon/ipc"
	"github.com/Nomadcxx/plex2jellyfin/internal/paths"
	"github.com/google/uuid"
)

type DatabaseHandlers struct {
	IPC IPCCaller
}

type rescanReq struct {
	Paths  []string `json:"paths"`
	DryRun bool     `json:"dry_run"`
}

func (h *DatabaseHandlers) Rescan(w http.ResponseWriter, r *http.Request) {
	var body rescanReq
	json.NewDecoder(r.Body).Decode(&body)
	opID := "op-" + uuid.NewString()
	go h.IPC.StreamWithID(context.Background(), ipc.CmdRescan, body, opID)
	respondAccepted(w, opID)
}

type resetReq struct {
	Confirm  string   `json:"confirm"`
	Preserve []string `json:"preserve"`
}

func (h *DatabaseHandlers) Reset(w http.ResponseWriter, r *http.Request) {
	var body resetReq
	json.NewDecoder(r.Body).Decode(&body)
	if body.Confirm != "media.db" {
		http.Error(w, "confirm must equal media.db", http.StatusBadRequest)
		return
	}
	opID := "op-" + uuid.NewString()
	go h.IPC.StreamWithID(context.Background(), ipc.CmdResetDB, body, opID)
	respondAccepted(w, opID)
}

// LastRescans returns the most recent rescan executions read from
// op_log.jsonl. Used by the Indexing tab to display "last scan" info.
func (h *DatabaseHandlers) LastRescans(w http.ResponseWriter, r *http.Request) {
	logPath, err := paths.OpLogPath()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	entries, err := ipc.RecentByCmd(logPath, ipc.CmdRescan, 10)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"entries": entries})
}

func respondAccepted(w http.ResponseWriter, opID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"op_id": opID})
}
