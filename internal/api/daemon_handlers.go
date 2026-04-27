package api

import (
	"encoding/json"
	"net/http"

	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
	"github.com/Nomadcxx/jellywatch/internal/jellyweb/daemonctl"
)

type DaemonHandlers struct {
	IPC      IPCCaller
	Launcher *daemonctl.Launcher
}

func (h *DaemonHandlers) Status(w http.ResponseWriter, r *http.Request) {
	body, err := h.IPC.Call(r.Context(), ipc.CmdStatus, nil)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"state":"stopped"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

func (h *DaemonHandlers) Stop(w http.ResponseWriter, r *http.Request) {
	if _, err := h.IPC.Call(r.Context(), ipc.CmdStop, nil); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *DaemonHandlers) Reload(w http.ResponseWriter, r *http.Request) {
	body, err := h.IPC.Call(r.Context(), ipc.CmdReload, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

func (h *DaemonHandlers) Start(w http.ResponseWriter, r *http.Request) {
	if err := h.Launcher.Start(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *DaemonHandlers) Restart(w http.ResponseWriter, r *http.Request) {
	_, _ = h.IPC.Call(r.Context(), ipc.CmdStop, nil)
	if err := h.Launcher.Start(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

type recoverArgs struct {
	Action string `json:"action"`
}

func (h *DaemonHandlers) Recover(w http.ResponseWriter, r *http.Request) {
	var a recoverArgs
	json.NewDecoder(r.Body).Decode(&a)
	body, err := h.IPC.Call(r.Context(), ipc.CmdRecover, a)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}
