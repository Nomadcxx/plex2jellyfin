package main

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
	"github.com/Nomadcxx/jellywatch/internal/daemon/reload"
)

type daemonStatus struct {
	PID            int              `json:"pid"`
	UptimeSeconds  int64            `json:"uptime_seconds"`
	ConfigLoaded   bool             `json:"config_loaded"`
	State          string           `json:"state"`
	InterruptedOps []ipc.OpLogEntry `json:"interrupted_ops,omitempty"`
}

func statusHandler(startedAt time.Time, getConfig func() *config.Config, getPending func() []ipc.OpLogEntry) ipc.Handler {
	return func(ctx context.Context, req ipc.Request, w ipc.FrameWriter) {
		var pending []ipc.OpLogEntry
		if getPending != nil {
			pending = getPending()
		}
		status := daemonStatus{
			PID:            os.Getpid(),
			UptimeSeconds:  int64(time.Since(startedAt).Seconds()),
			ConfigLoaded:   getConfig() != nil,
			State:          "running",
			InterruptedOps: pending,
		}
		if len(pending) > 0 {
			status.State = "interrupted"
		}
		data, err := json.Marshal(status)
		if err != nil {
			w.Error(req.ID, ipc.ErrInternal, err.Error())
			return
		}
		w.Result(req.ID, data)
	}
}

func stopHandler(stop func()) ipc.Handler {
	return func(ctx context.Context, req ipc.Request, w ipc.FrameWriter) {
		w.Result(req.ID, json.RawMessage(`{"stopping":true}`))
		go stop()
	}
}

func reloadHandler(
	getConfig func() *config.Config,
	setConfig func(*config.Config),
	loadConfig func() (*config.Config, error),
	supervisor *reload.Supervisor,
) ipc.Handler {
	return func(ctx context.Context, req ipc.Request, w ipc.FrameWriter) {
		oldCfg := getConfig()
		newCfg, err := loadConfig()
		if err != nil {
			w.Error(req.ID, ipc.ErrInternal, err.Error())
			return
		}
		result := supervisor.Reload(ctx, oldCfg, newCfg)
		if result.OK {
			setConfig(newCfg)
		}
		data, err := json.Marshal(result)
		if err != nil {
			w.Error(req.ID, ipc.ErrInternal, err.Error())
			return
		}
		w.Result(req.ID, data)
	}
}
