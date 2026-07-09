package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/daemon/ipc"
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

// GetTask returns the full payload + metadata for a single task.
func (h *HousekeepingHandlers) GetTask(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	body, err := h.IPC.Call(r.Context(), ipc.CmdTaskGet, map[string]int64{"id": id})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

// BulkAction performs retry/cancel against a list of task ids in one call.
func (h *HousekeepingHandlers) BulkAction(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		IDs    []int64 `json:"ids"`
		Action string  `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	body, err := h.IPC.Call(r.Context(), ipc.CmdTasksBulk, payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

// PurgeTasks deletes terminal-state tasks. ?statuses=done,skipped,canceled
// (default if omitted: done+skipped+canceled).
func (h *HousekeepingHandlers) PurgeTasks(w http.ResponseWriter, r *http.Request) {
	var statuses []string
	if s := r.URL.Query().Get("statuses"); s != "" {
		for _, p := range strings.Split(s, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				statuses = append(statuses, p)
			}
		}
	}
	body, err := h.IPC.Call(r.Context(), ipc.CmdTasksPurge, map[string]any{"statuses": statuses})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

// VerifyTask re-runs the verifier on a single task.
func (h *HousekeepingHandlers) VerifyTask(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	body, err := h.IPC.Call(ctx, ipc.CmdTaskVerify, map[string]int64{"id": id})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

// GetTaskGroup returns the duplicate group attached to a task — files
// with size/resolution/quality_score plus the would_keep flag — used
// by the scheduler UI to render the inspector panel before approval.
func (h *HousekeepingHandlers) GetTaskGroup(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	body, err := h.IPC.Call(r.Context(), ipc.CmdTaskGroup, map[string]int64{"id": id})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

// ApproveTask converts a flagged duplicate task (cross_volume_duplicate
// or year_mismatch) into a pending consolidate_duplicate task so the
// housekeeping engine will resolve it on the next tick.
func (h *HousekeepingHandlers) ApproveTask(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	body, err := h.IPC.Call(r.Context(), ipc.CmdTaskApprove, map[string]int64{"id": id})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}
