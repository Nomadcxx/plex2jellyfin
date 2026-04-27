package main

import (
	"context"
	"encoding/json"
	"path/filepath"
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
	}, nil)

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

func TestStopHandlerCallsShutdown(t *testing.T) {
called := false
stop := func() { called = true }
srv := ipc.NewServer(filepath.Join(t.TempDir(), "ctl.sock"))
srv.Register(ipc.CmdStop, stopHandler(stop))
ctx, cancel := context.WithCancel(context.Background())
defer cancel()
if err := srv.Start(ctx); err != nil {
t.Fatal(err)
}
defer srv.Stop()
cli := ipc.NewClient(srv.Path())
_, _ = cli.Call(ctx, ipc.CmdStop, nil)
time.Sleep(50 * time.Millisecond)
if !called {
t.Error("stop func not invoked")
}
}

func TestRecoverDiscardClearsPending(t *testing.T) {
dir := t.TempDir()
sock := filepath.Join(dir, "ctl.sock")
logFile, _ := ipc.OpenOpLog(filepath.Join(dir, "op_log.jsonl"))
defer logFile.Close()
_ = logFile.Begin("op-x", ipc.CmdResetDB, nil)

pending, _ := logFile.Pending()
if len(pending) != 1 {
t.Fatal("setup: expected 1 pending")
}

current := pending
getPending := func() []ipc.OpLogEntry { return current }
clearPending := func() { current = nil }

srv := ipc.NewServer(sock)
srv.Register(ipc.CmdRecover, recoverHandler(logFile, getPending, clearPending))
ctx, cancel := context.WithCancel(context.Background())
defer cancel()
if err := srv.Start(ctx); err != nil {
t.Fatal(err)
}
defer srv.Stop()

cli := ipc.NewClient(sock)
if _, err := cli.Call(ctx, ipc.CmdRecover, map[string]string{"action": "discard"}); err != nil {
t.Fatal(err)
}
if got, _ := logFile.Pending(); len(got) != 0 {
t.Errorf("expected pending cleared, got %d", len(got))
}
if len(getPending()) != 0 {
t.Error("in-memory pending not cleared")
}
}
