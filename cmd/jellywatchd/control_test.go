package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
	"github.com/Nomadcxx/jellywatch/internal/daemon/reload"
)

type captureFrameWriter struct {
	result json.RawMessage
	code   ipc.ErrorCode
	msg    string
}

func (w *captureFrameWriter) Result(id string, data json.RawMessage) { w.result = data }
func (w *captureFrameWriter) Progress(id string, phase, msg string, current, total int) {
}
func (w *captureFrameWriter) Done(id string, data json.RawMessage) {}
func (w *captureFrameWriter) Error(id string, code ipc.ErrorCode, msg string) {
	w.code = code
	w.msg = msg
}

func TestStatusHandlerReportsDaemonStatus(t *testing.T) {
	started := time.Now().Add(-time.Minute)
	w := &captureFrameWriter{}
	h := statusHandler(started, func() *config.Config {
		return &config.Config{}
	})

	h(context.Background(), ipc.Request{ID: "status", Cmd: ipc.CmdStatus}, w)

	var got daemonStatus
	if err := json.Unmarshal(w.result, &got); err != nil {
		t.Fatal(err)
	}
	if got.PID == 0 {
		t.Fatal("pid was not set")
	}
	if got.UptimeSeconds <= 0 {
		t.Fatalf("uptime_seconds = %d, want > 0", got.UptimeSeconds)
	}
	if !got.ConfigLoaded {
		t.Fatal("config_loaded = false, want true")
	}
}

func TestReloadHandlerLoadsConfigAndUpdatesCurrentConfig(t *testing.T) {
	oldCfg := &config.Config{Logging: config.LoggingConfig{Level: "info"}}
	newCfg := &config.Config{Logging: config.LoggingConfig{Level: "debug"}}
	supervisor := reload.NewSupervisor()
	w := &captureFrameWriter{}

	h := reloadHandler(
		func() *config.Config { return oldCfg },
		func(next *config.Config) { oldCfg = next },
		func() (*config.Config, error) { return newCfg, nil },
		supervisor,
	)

	h(context.Background(), ipc.Request{ID: "reload", Cmd: ipc.CmdReload}, w)

	var got reload.Result
	if err := json.Unmarshal(w.result, &got); err != nil {
		t.Fatal(err)
	}
	if !got.OK {
		t.Fatalf("reload failed: %+v", got)
	}
	if oldCfg != newCfg {
		t.Fatal("current config was not updated")
	}
}
