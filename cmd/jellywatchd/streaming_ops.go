package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/ai"
	"github.com/Nomadcxx/jellywatch/internal/consolidate"
	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/jellyfin"
	"github.com/Nomadcxx/jellywatch/internal/service"
)

// streamRunner wires a phased internal operation to the IPC progress channel.
// It begins/ends an op-log entry, spawns relayProgress, and awaits run().
// run() must close the progress channel when complete; relayProgress closes
// doneCh once the channel drains, so the function returns deterministically.
func streamRunner(
	ctx context.Context,
	cmd ipc.Command,
	args map[string]any,
	op *ipc.Op,
	w ipc.FrameWriter,
	log *ipc.OpLog,
	run func(progress chan<- database.ProgressEvent) error,
) {
	_ = log.Begin(op.ID, cmd, args)
	progress := make(chan database.ProgressEvent, 64)
	doneCh := make(chan struct{})
	go relayProgress(progress, w, op.ID, doneCh)

	runErr := run(progress)
	close(progress)
	<-doneCh

	if runErr != nil {
		w.Error(op.ID, ipc.ErrInternal, runErr.Error())
		_ = log.End(op.ID, "error", runErr.Error())
		return
	}
	w.Done(op.ID, json.RawMessage(`{"ok":true}`))
	_ = log.End(op.ID, "done", "")
}

// ----- B1: CONSOLIDATE -----

type consolidateArgs struct {
	DryRun bool `json:"dry_run"`
}

func consolidateHandler(db *database.MediaDB, log *ipc.OpLog) ipc.StreamingHandler {
	return func(ctx context.Context, raw json.RawMessage, w ipc.FrameWriter, op *ipc.Op) {
		var args consolidateArgs
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &args)
		}
		streamRunner(ctx, ipc.CmdConsolidate,
			map[string]any{"dry_run": args.DryRun}, op, w, log,
			func(progress chan<- database.ProgressEvent) error {
				progress <- database.ProgressEvent{Phase: "planning", Msg: "fetching pending plans"}
				exec := consolidate.NewExecutor(db, args.DryRun, nil)
				planner := consolidate.NewPlanner(db)

				plans, err := planner.GetPendingPlans()
				if err != nil {
					return fmt.Errorf("get pending plans: %w", err)
				}
				total := len(plans)
				if total == 0 {
					progress <- database.ProgressEvent{Phase: "complete", Msg: "no pending plans"}
					return nil
				}
				progress <- database.ProgressEvent{Phase: "moving", Msg: "executing", Current: 0, Total: total}

				for i, plan := range plans {
					select {
					case <-ctx.Done():
						return ctx.Err()
					default:
					}
					if err := exec.ExecutePlan(ctx, plan.ID); err != nil {
						progress <- database.ProgressEvent{
							Phase: "moving", Msg: fmt.Sprintf("plan %d failed: %v", plan.ID, err),
							Current: i + 1, Total: total,
						}
						continue
					}
					progress <- database.ProgressEvent{
						Phase: "moving", Msg: fmt.Sprintf("plan %d ok", plan.ID),
						Current: i + 1, Total: total,
					}
				}
				progress <- database.ProgressEvent{Phase: "verifying", Msg: "finalizing"}
				progress <- database.ProgressEvent{Phase: "complete", Current: total, Total: total}
				return nil
			})
	}
}

// ----- B2: DUP_SCAN -----

func dupScanHandler(svc *service.CleanupService, log *ipc.OpLog) ipc.StreamingHandler {
	return func(ctx context.Context, raw json.RawMessage, w ipc.FrameWriter, op *ipc.Op) {
		streamRunner(ctx, ipc.CmdDupScan, nil, op, w, log,
			func(progress chan<- database.ProgressEvent) error {
				progress <- database.ProgressEvent{Phase: "scanning", Msg: "querying duplicates"}
				analysis, err := svc.AnalyzeDuplicates()
				if err != nil {
					return err
				}
				progress <- database.ProgressEvent{
					Phase: "grouping",
					Msg:   fmt.Sprintf("%d groups, %d files", len(analysis.Groups), analysis.TotalFiles),
					Total: len(analysis.Groups),
				}
				progress <- database.ProgressEvent{
					Phase: "complete",
					Msg:   fmt.Sprintf("%d groups", len(analysis.Groups)),
					Total: len(analysis.Groups),
				}
				return nil
			})
	}
}

// ----- B3: AI_BATCH -----

type aiBatchArgs struct{}

// aiBatcher is the subset of MediaHandler used by aiBatchHandler.
type aiBatcher interface {
	ProcessPendingAIWithProgress(ctx context.Context, progress chan<- database.ProgressEvent)
}

func aiBatchHandler(h aiBatcher, matcher *ai.Matcher, log *ipc.OpLog) ipc.StreamingHandler {
	return func(ctx context.Context, raw json.RawMessage, w ipc.FrameWriter, op *ipc.Op) {
		if matcher == nil || h == nil {
			w.Error(op.ID, ipc.ErrBadRequest, "AI matcher not configured")
			return
		}
		streamRunner(ctx, ipc.CmdAIBatch, nil, op, w, log,
			func(progress chan<- database.ProgressEvent) error {
				h.ProcessPendingAIWithProgress(ctx, progress)
				return nil
			})
	}
}

// ----- B4: METADATA_REFRESH -----

type metadataRefreshArgs struct {
	ItemIDs []string `json:"item_ids,omitempty"`
}

func metadataRefreshHandler(client *jellyfin.Client, log *ipc.OpLog) ipc.StreamingHandler {
	return func(ctx context.Context, raw json.RawMessage, w ipc.FrameWriter, op *ipc.Op) {
		var args metadataRefreshArgs
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &args)
		}
		if client == nil {
			w.Error(op.ID, ipc.ErrBadRequest, "Jellyfin client not configured")
			return
		}
		streamRunner(ctx, ipc.CmdMetadataRefresh,
			map[string]any{"items": len(args.ItemIDs)}, op, w, log,
			func(progress chan<- database.ProgressEvent) error {
				if len(args.ItemIDs) == 0 {
					progress <- database.ProgressEvent{Phase: "fetching-jellyfin", Msg: "triggering full library refresh"}
					if err := client.RefreshLibrary(); err != nil {
						return err
					}
					progress <- database.ProgressEvent{Phase: "complete", Msg: "library refresh queued in Jellyfin"}
					return nil
				}
				total := len(args.ItemIDs)
				for i, id := range args.ItemIDs {
					select {
					case <-ctx.Done():
						return ctx.Err()
					default:
					}
					progress <- database.ProgressEvent{
						Phase: "updating-jellyfin", Msg: id,
						Current: i, Total: total,
					}
					if err := client.RefreshItem(id); err != nil {
						progress <- database.ProgressEvent{
							Phase: "updating-jellyfin", Msg: fmt.Sprintf("%s: %v", id, err),
							Current: i + 1, Total: total,
						}
						continue
					}
				}
				progress <- database.ProgressEvent{Phase: "complete", Current: total, Total: total}
				return nil
			})
	}
}

// ----- B5: SWEEP (manual sweeper run) -----

type sweepArgs struct {
	LookbackHours int `json:"lookback_hours,omitempty"`
	TTLHours      int `json:"ttl_hours,omitempty"`
}

func sweepHandler(sweeper *jellyfin.Sweeper, log *ipc.OpLog) ipc.StreamingHandler {
	return func(ctx context.Context, raw json.RawMessage, w ipc.FrameWriter, op *ipc.Op) {
		if sweeper == nil {
			w.Error(op.ID, ipc.ErrBadRequest, "sweeper not configured")
			return
		}
		var args sweepArgs
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &args)
		}
		lookback := 24 * time.Hour
		if args.LookbackHours > 0 {
			lookback = time.Duration(args.LookbackHours) * time.Hour
		}
		ttl := 7 * 24 * time.Hour
		if args.TTLHours > 0 {
			ttl = time.Duration(args.TTLHours) * time.Hour
		}
		streamRunner(ctx, ipc.CmdSweep,
			map[string]any{"lookback_hours": args.LookbackHours, "ttl_hours": args.TTLHours},
			op, w, log,
			func(progress chan<- database.ProgressEvent) error {
				progress <- database.ProgressEvent{Phase: "scanning", Msg: "walking Jellyfin library"}
				if err := sweeper.RunOnce(ctx, lookback, ttl); err != nil {
					return err
				}
				progress <- database.ProgressEvent{Phase: "complete"}
				return nil
			})
	}
}

// ----- B6: PARSES_AUDIT -----

func parsesAuditHandler(db *database.MediaDB, log *ipc.OpLog) ipc.StreamingHandler {
	return func(ctx context.Context, raw json.RawMessage, w ipc.FrameWriter, op *ipc.Op) {
		if db == nil {
			w.Error(op.ID, ipc.ErrBadRequest, "database not available")
			return
		}
		streamRunner(ctx, ipc.CmdParsesAudit, nil, op, w, log,
			func(progress chan<- database.ProgressEvent) error {
				progress <- database.ProgressEvent{Phase: "counting", Msg: "totals"}
				total, err := db.CountParseDecisions()
				if err != nil {
					return fmt.Errorf("count: %w", err)
				}
				if total == 0 {
					progress <- database.ProgressEvent{Phase: "complete", Msg: "no parse decisions"}
					return nil
				}
				progress <- database.ProgressEvent{
					Phase: "auditing", Msg: "scanning corpus",
					Current: 0, Total: total,
				}
				const pageSize = 500
				processed := 0
				for offset := 0; offset < total; offset += pageSize {
					select {
					case <-ctx.Done():
						return ctx.Err()
					default:
					}
					n, err := db.AuditParseDecisionsPage(offset, pageSize)
					if err != nil {
						return fmt.Errorf("page %d: %w", offset, err)
					}
					processed += n
					progress <- database.ProgressEvent{
						Phase: "auditing", Current: processed, Total: total,
					}
					if n == 0 {
						break
					}
				}
				progress <- database.ProgressEvent{Phase: "complete", Current: processed, Total: total}
				return nil
			})
	}
}

// notImplemented returns a 501 Done frame; used to register placeholder
// streaming handlers when an underlying capability is missing at runtime.
func notImplemented(reason string, log *ipc.OpLog, cmd ipc.Command) ipc.StreamingHandler {
	return func(ctx context.Context, raw json.RawMessage, w ipc.FrameWriter, op *ipc.Op) {
		_ = log.Begin(op.ID, cmd, nil)
		w.Error(op.ID, ipc.ErrNotImplemented, reason)
		_ = log.End(op.ID, "error", reason)
	}
}

var _ = errors.New
