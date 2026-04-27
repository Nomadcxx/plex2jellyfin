package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
	"github.com/Nomadcxx/jellywatch/internal/daemon/reload"
	"github.com/Nomadcxx/jellywatch/internal/database"
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

type recoverArgs struct {
Action string `json:"action"`
}

func recoverHandler(log *ipc.OpLog, getPending func() []ipc.OpLogEntry, clearPending func()) ipc.Handler {
return func(ctx context.Context, req ipc.Request, w ipc.FrameWriter) {
var a recoverArgs
if err := json.Unmarshal(req.Args, &a); err != nil {
w.Error(req.ID, ipc.ErrBadRequest, err.Error())
return
}
switch a.Action {
case "discard":
for _, p := range getPending() {
if err := log.MarkDiscarded(p.ID); err != nil {
w.Error(req.ID, ipc.ErrInternal, err.Error())
return
}
}
clearPending()
w.Result(req.ID, json.RawMessage(`{"discarded":true}`))
case "resume":
w.Error(req.ID, ipc.ErrNotImplemented, "resume not supported in v1")
default:
w.Error(req.ID, ipc.ErrBadRequest, "action must be discard or resume")
}
}
}

type rescanArgs struct {
Paths  []string `json:"paths,omitempty"`
DryRun bool     `json:"dry_run"`
}

type rescanScanner interface {
FullRescan(ctx context.Context, paths []string, dryRun bool, p chan<- database.ProgressEvent) error
}

func rescanHandler(scanner rescanScanner, log *ipc.OpLog) ipc.StreamingHandler {
return func(ctx context.Context, raw json.RawMessage, w ipc.FrameWriter, op *ipc.Op) {
var args rescanArgs
if len(raw) > 0 {
_ = json.Unmarshal(raw, &args)
}
_ = log.Begin(op.ID, ipc.CmdRescan, map[string]any{"paths": args.Paths, "dry_run": args.DryRun})
progress := make(chan database.ProgressEvent, 64)
doneCh := make(chan struct{})
go func() {
for ev := range progress {
w.Progress(op.ID, ev.Phase, ev.Msg, ev.Current, ev.Total)
}
close(doneCh)
}()
err := scanner.FullRescan(ctx, args.Paths, args.DryRun, progress)
close(progress)
<-doneCh
if err != nil {
w.Error(op.ID, ipc.ErrInternal, err.Error())
_ = log.End(op.ID, "error", err.Error())
return
}
w.Done(op.ID, json.RawMessage(`{"ok":true}`))
_ = log.End(op.ID, "done", "")
}
}

type resetArgs struct {
Confirm  string   `json:"confirm"`
Preserve []string `json:"preserve,omitempty"`
}

func resetDBHandler(db *sql.DB, log *ipc.OpLog) ipc.StreamingHandler {
return func(ctx context.Context, raw json.RawMessage, w ipc.FrameWriter, op *ipc.Op) {
var args resetArgs
if err := json.Unmarshal(raw, &args); err != nil {
w.Error(op.ID, ipc.ErrBadRequest, err.Error())
return
}
if args.Confirm != "media.db" {
w.Error(op.ID, ipc.ErrBadRequest, `confirm must equal "media.db"`)
return
}
_ = log.Begin(op.ID, ipc.CmdResetDB, map[string]any{"preserve": args.Preserve})
progress := make(chan database.ProgressEvent, 64)
doneCh := make(chan struct{})
go func() {
for ev := range progress {
w.Progress(op.ID, ev.Phase, ev.Msg, ev.Current, ev.Total)
}
close(doneCh)
}()
err := database.ResetDatabase(ctx, db, args.Preserve, progress)
close(progress)
<-doneCh
if err != nil {
w.Error(op.ID, ipc.ErrInternal, err.Error())
_ = log.End(op.ID, "error", err.Error())
return
}
w.Done(op.ID, json.RawMessage(`{"ok":true}`))
_ = log.End(op.ID, "done", "")
}
}

// guardMutator rejects mutator ops while interrupted ops are still pending
// (operator hasn't called RECOVER yet). Wrap streaming handlers for RESCAN
// and RESET_DB at registration time in main.
func guardMutator(getPending func() []ipc.OpLogEntry, h ipc.StreamingHandler) ipc.StreamingHandler {
return func(ctx context.Context, raw json.RawMessage, w ipc.FrameWriter, op *ipc.Op) {
if getPending != nil && len(getPending()) > 0 {
w.Error(op.ID, ipc.ErrConflict, "interrupted ops pending; call RECOVER first")
return
}
h(ctx, raw, w, op)
}
}
