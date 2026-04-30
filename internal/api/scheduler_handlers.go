package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
	"github.com/go-chi/chi/v5"
)

// SchedulerHandlers exposes the daemon's scheduled-jobs registry and
// housekeeping task queue to the WebUI.
type SchedulerHandlers struct {
	IPC IPCCaller
}

func (h *SchedulerHandlers) ListJobs(w http.ResponseWriter, r *http.Request) {
	body, err := h.IPC.Call(r.Context(), ipc.CmdJobsList, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

func (h *SchedulerHandlers) RunJob(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	body, err := h.IPC.Call(r.Context(), ipc.CmdJobRun, map[string]string{"name": name})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

func (h *SchedulerHandlers) StopJob(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	body, err := h.IPC.Call(r.Context(), ipc.CmdJobStop, map[string]string{"name": name})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

func (h *SchedulerHandlers) UpdateJob(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var payload struct {
		Schedule string `json:"schedule"`
		Enabled  bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	body, err := h.IPC.Call(r.Context(), ipc.CmdJobUpdate, map[string]any{
		"name":     name,
		"schedule": payload.Schedule,
		"enabled":  payload.Enabled,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

// HousekeepingHandlers exposes the queued task table.
type HousekeepingHandlers struct {
	IPC IPCCaller
}

func (h *HousekeepingHandlers) ListTasks(w http.ResponseWriter, r *http.Request) {
	args := map[string]any{}
	if s := r.URL.Query().Get("status"); s != "" {
		args["status"] = s
	}
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			args["limit"] = n
		}
	}
	body, err := h.IPC.Call(r.Context(), ipc.CmdTasksList, args)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

func (h *HousekeepingHandlers) RetryTask(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	body, err := h.IPC.Call(r.Context(), ipc.CmdTaskRetry, map[string]int64{"id": id})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

func (h *HousekeepingHandlers) CancelTask(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	body, err := h.IPC.Call(r.Context(), ipc.CmdTaskCancel, map[string]int64{"id": id})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

// VerifyFlagged re-runs the TMDB verifier across every flagged
// year_mismatch task and reclassifies them as distinct/duplicate/unknown.
// May take 30s+ for large flag backlogs (~1s per remote lookup).
func (h *HousekeepingHandlers) VerifyFlagged(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	body, err := h.IPC.Call(ctx, ipc.CmdVerifyFlagged, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}
