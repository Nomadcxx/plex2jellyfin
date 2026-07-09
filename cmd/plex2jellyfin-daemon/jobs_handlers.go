package main

import (
	"context"
	"encoding/json"

	"github.com/Nomadcxx/plex2jellyfin/internal/daemon/ipc"
	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/housekeeping"
	"github.com/Nomadcxx/plex2jellyfin/internal/scheduler"
	"github.com/Nomadcxx/plex2jellyfin/internal/service"
)

// jobsListHandler returns all registered scheduled jobs with their last-run
// metadata. Used by the WebUI Scheduled Jobs page.
func jobsListHandler(db *database.MediaDB) ipc.Handler {
	return func(ctx context.Context, req ipc.Request, w ipc.FrameWriter) {
		jobs, err := db.ListScheduledJobs()
		if err != nil {
			w.Error(req.ID, ipc.ErrInternal, err.Error())
			return
		}
		out := make([]map[string]any, 0, len(jobs))
		for _, j := range jobs {
			row := map[string]any{
				"name":     j.Name,
				"schedule": j.Schedule,
				"enabled":  j.Enabled,
				"running":  j.Running,
			}
			if j.LastRunAt.Valid {
				row["last_run_at"] = j.LastRunAt.Time
			}
			if j.LastDurationMS.Valid {
				row["last_duration_ms"] = j.LastDurationMS.Int64
			}
			if j.LastResult.Valid {
				row["last_result"] = j.LastResult.String
			}
			if j.LastError.Valid {
				row["last_error"] = j.LastError.String
			}
			if j.NextRunAt.Valid {
				row["next_run_at"] = j.NextRunAt.Time
			}
			out = append(out, row)
		}
		data, err := json.Marshal(map[string]any{"jobs": out})
		if err != nil {
			w.Error(req.ID, ipc.ErrInternal, err.Error())
			return
		}
		w.Result(req.ID, data)
	}
}

type jobNameArgs struct {
	Name string `json:"name"`
}

type jobUpdateArgs struct {
	Name     string `json:"name"`
	Schedule string `json:"schedule"`
	Enabled  bool   `json:"enabled"`
}

func jobRunHandler(sched *scheduler.Scheduler, daemonCtx context.Context) ipc.Handler {
	return func(ctx context.Context, req ipc.Request, w ipc.FrameWriter) {
		var args jobNameArgs
		if len(req.Args) > 0 {
			_ = json.Unmarshal(req.Args, &args)
		}
		if args.Name == "" {
			w.Error(req.ID, ipc.ErrBadRequest, "missing job name")
			return
		}
		if err := sched.RunNow(daemonCtx, args.Name); err != nil {
			w.Error(req.ID, ipc.ErrInternal, err.Error())
			return
		}
		w.Result(req.ID, json.RawMessage(`{"started":true}`))
	}
}

func jobStopHandler(sched *scheduler.Scheduler) ipc.Handler {
	return func(ctx context.Context, req ipc.Request, w ipc.FrameWriter) {
		var args jobNameArgs
		if len(req.Args) > 0 {
			_ = json.Unmarshal(req.Args, &args)
		}
		if args.Name == "" {
			w.Error(req.ID, ipc.ErrBadRequest, "missing job name")
			return
		}
		if err := sched.Stop(args.Name); err != nil {
			w.Error(req.ID, ipc.ErrInternal, err.Error())
			return
		}
		w.Result(req.ID, json.RawMessage(`{"stopped":true}`))
	}
}

func jobUpdateHandler(db *database.MediaDB) ipc.Handler {
	return func(ctx context.Context, req ipc.Request, w ipc.FrameWriter) {
		var args jobUpdateArgs
		if err := json.Unmarshal(req.Args, &args); err != nil {
			w.Error(req.ID, ipc.ErrBadRequest, err.Error())
			return
		}
		if args.Name == "" || args.Schedule == "" {
			w.Error(req.ID, ipc.ErrBadRequest, "name and schedule required")
			return
		}
		if err := db.UpdateScheduledJob(args.Name, args.Schedule, args.Enabled); err != nil {
			w.Error(req.ID, ipc.ErrInternal, err.Error())
			return
		}
		w.Result(req.ID, json.RawMessage(`{"updated":true}`))
	}
}

type tasksListArgs struct {
	Status string `json:"status"`
	Limit  int    `json:"limit"`
}

func tasksListHandler(db *database.MediaDB) ipc.Handler {
	return func(ctx context.Context, req ipc.Request, w ipc.FrameWriter) {
		var args tasksListArgs
		if len(req.Args) > 0 {
			_ = json.Unmarshal(req.Args, &args)
		}
		tasks, err := db.ListHousekeepingTasks(args.Status, args.Limit)
		if err != nil {
			w.Error(req.ID, ipc.ErrInternal, err.Error())
			return
		}
		counts, _ := db.CountHousekeepingTasks()
		out := make([]map[string]any, 0, len(tasks))
		for _, t := range tasks {
			row := map[string]any{
				"id":       t.ID,
				"job_name": t.JobName,
				"kind":     t.Kind,
				"payload":  t.Payload,
				"status":   t.Status,
				"attempts": t.Attempts,
				"priority": t.Priority,
				"created_at": t.CreatedAt,
			}
			if t.LastError.Valid {
				row["last_error"] = t.LastError.String
			}
			if t.StartedAt.Valid {
				row["started_at"] = t.StartedAt.Time
			}
			if t.FinishedAt.Valid {
				row["finished_at"] = t.FinishedAt.Time
			}
			if t.NextAttemptAt.Valid {
				row["next_attempt_at"] = t.NextAttemptAt.Time
			}
			out = append(out, row)
		}
		data, err := json.Marshal(map[string]any{"tasks": out, "counts": counts})
		if err != nil {
			w.Error(req.ID, ipc.ErrInternal, err.Error())
			return
		}
		w.Result(req.ID, data)
	}
}

type taskIDArgs struct {
	ID int64 `json:"id"`
}

func taskRetryHandler(db *database.MediaDB) ipc.Handler {
	return func(ctx context.Context, req ipc.Request, w ipc.FrameWriter) {
		var args taskIDArgs
		if err := json.Unmarshal(req.Args, &args); err != nil || args.ID == 0 {
			w.Error(req.ID, ipc.ErrBadRequest, "id required")
			return
		}
		if err := db.RetryHousekeepingTask(args.ID); err != nil {
			w.Error(req.ID, ipc.ErrInternal, err.Error())
			return
		}
		w.Result(req.ID, json.RawMessage(`{"retried":true}`))
	}
}

func taskCancelHandler(db *database.MediaDB) ipc.Handler {
	return func(ctx context.Context, req ipc.Request, w ipc.FrameWriter) {
		var args taskIDArgs
		if err := json.Unmarshal(req.Args, &args); err != nil || args.ID == 0 {
			w.Error(req.ID, ipc.ErrBadRequest, "id required")
			return
		}
		if err := db.CancelHousekeepingTask(args.ID); err != nil {
			w.Error(req.ID, ipc.ErrInternal, err.Error())
			return
		}
		w.Result(req.ID, json.RawMessage(`{"canceled":true}`))
	}
}

func verifyFlaggedHandler(eng *housekeeping.Engine) ipc.Handler {
	return func(ctx context.Context, req ipc.Request, w ipc.FrameWriter) {
		res, err := eng.VerifyFlagged(ctx)
		if err != nil {
			w.Error(req.ID, ipc.ErrInternal, err.Error())
			return
		}
		data, _ := json.Marshal(res)
		w.Result(req.ID, data)
	}
}

func taskGetHandler(db *database.MediaDB) ipc.Handler {
	return func(ctx context.Context, req ipc.Request, w ipc.FrameWriter) {
		var args taskIDArgs
		if err := json.Unmarshal(req.Args, &args); err != nil || args.ID == 0 {
			w.Error(req.ID, ipc.ErrBadRequest, "id required")
			return
		}
		t, err := db.GetHousekeepingTask(args.ID)
		if err != nil {
			w.Error(req.ID, ipc.ErrInternal, err.Error())
			return
		}
		if t == nil {
			w.Error(req.ID, ipc.ErrNotFound, "task not found")
			return
		}
		row := map[string]any{
			"id":         t.ID,
			"job_name":   t.JobName,
			"kind":       t.Kind,
			"payload":    t.Payload,
			"status":     t.Status,
			"attempts":   t.Attempts,
			"priority":   t.Priority,
			"created_at": t.CreatedAt,
			"dedup_key":  t.DedupKey,
		}
		if t.LastError.Valid {
			row["last_error"] = t.LastError.String
		}
		if t.StartedAt.Valid {
			row["started_at"] = t.StartedAt.Time
		}
		if t.FinishedAt.Valid {
			row["finished_at"] = t.FinishedAt.Time
		}
		if t.NextAttemptAt.Valid {
			row["next_attempt_at"] = t.NextAttemptAt.Time
		}
		data, _ := json.Marshal(row)
		w.Result(req.ID, data)
	}
}

type tasksBulkArgs struct {
	IDs    []int64 `json:"ids"`
	Action string  `json:"action"` // "retry" | "cancel"
}

func tasksBulkHandler(db *database.MediaDB) ipc.Handler {
	return func(ctx context.Context, req ipc.Request, w ipc.FrameWriter) {
		var args tasksBulkArgs
		if err := json.Unmarshal(req.Args, &args); err != nil {
			w.Error(req.ID, ipc.ErrBadRequest, err.Error())
			return
		}
		if len(args.IDs) == 0 {
			w.Error(req.ID, ipc.ErrBadRequest, "ids required")
			return
		}
		var n int64
		var err error
		switch args.Action {
		case "retry":
			n, err = db.BulkRetryHousekeepingTasks(args.IDs)
		case "cancel":
			n, err = db.BulkCancelHousekeepingTasks(args.IDs)
		case "approve":
			n, err = db.BulkApproveHousekeepingTasks(args.IDs)
		default:
			w.Error(req.ID, ipc.ErrBadRequest, "action must be retry|cancel|approve")
			return
		}
		if err != nil {
			w.Error(req.ID, ipc.ErrInternal, err.Error())
			return
		}
		data, _ := json.Marshal(map[string]any{"affected": n})
		w.Result(req.ID, data)
	}
}

type tasksPurgeArgs struct {
	Statuses []string `json:"statuses"`
}

func tasksPurgeHandler(db *database.MediaDB) ipc.Handler {
	return func(ctx context.Context, req ipc.Request, w ipc.FrameWriter) {
		var args tasksPurgeArgs
		if len(req.Args) > 0 {
			_ = json.Unmarshal(req.Args, &args)
		}
		n, err := db.PurgeHousekeepingTasks(args.Statuses)
		if err != nil {
			w.Error(req.ID, ipc.ErrInternal, err.Error())
			return
		}
		data, _ := json.Marshal(map[string]any{"deleted": n})
		w.Result(req.ID, data)
	}
}

func taskVerifyHandler(eng *housekeeping.Engine) ipc.Handler {
	return func(ctx context.Context, req ipc.Request, w ipc.FrameWriter) {
		var args taskIDArgs
		if err := json.Unmarshal(req.Args, &args); err != nil || args.ID == 0 {
			w.Error(req.ID, ipc.ErrBadRequest, "id required")
			return
		}
		vr, err := eng.VerifyTask(ctx, args.ID)
		if err != nil {
			w.Error(req.ID, ipc.ErrInternal, err.Error())
			return
		}
		data, _ := json.Marshal(vr)
		w.Result(req.ID, data)
	}
}

// taskGroupHandler returns the duplicate group attached to a flagged
// task — used by the scheduler UI to render the per-file inspector
// (path/size/resolution/quality_score) before approval.
func taskGroupHandler(db *database.MediaDB) ipc.Handler {
	return func(ctx context.Context, req ipc.Request, w ipc.FrameWriter) {
		var args taskIDArgs
		if err := json.Unmarshal(req.Args, &args); err != nil || args.ID == 0 {
			w.Error(req.ID, ipc.ErrBadRequest, "id required")
			return
		}
		t, err := db.GetHousekeepingTask(args.ID)
		if err != nil {
			w.Error(req.ID, ipc.ErrInternal, err.Error())
			return
		}
		if t == nil {
			w.Error(req.ID, ipc.ErrNotFound, "task not found")
			return
		}
		mediaType, _ := t.Payload["media_type"].(string)
		title, _ := t.Payload["normalized_title"].(string)
		if mediaType == "" || title == "" {
			w.Error(req.ID, ipc.ErrBadRequest, "task has no duplicate group payload")
			return
		}
		cs := service.NewCleanupService(db)
		group, err := cs.FindDuplicateGroup(mediaType, title,
			payloadIntPtrLocal(t.Payload, "year"),
			payloadIntPtrLocal(t.Payload, "season"),
			payloadIntPtrLocal(t.Payload, "episode"))
		if err != nil {
			w.Error(req.ID, ipc.ErrInternal, err.Error())
			return
		}
		if group == nil {
			data, _ := json.Marshal(map[string]any{"resolved": true})
			w.Result(req.ID, data)
			return
		}
		level, reasons := cs.GroupConfidence(*group)
		files := make([]map[string]any, 0, len(group.Files))
		for _, f := range group.Files {
			files = append(files, map[string]any{
				"id":            f.ID,
				"path":          f.Path,
				"size":          f.Size,
				"resolution":    f.Resolution,
				"source_type":   f.SourceType,
				"quality_score": f.QualityScore,
				"would_keep":    f.ID == group.BestFileID,
			})
		}
		data, _ := json.Marshal(map[string]any{
			"group_id":          group.ID,
			"media_type":        group.MediaType,
			"title":             group.Title,
			"year":              group.Year,
			"season":            group.Season,
			"episode":           group.Episode,
			"best_file_id":      group.BestFileID,
			"reclaimable_bytes": group.ReclaimableBytes,
			"confidence":        level,
			"confidence_reasons": reasons,
			"files":             files,
		})
		w.Result(req.ID, data)
	}
}

// taskApproveHandler converts a flagged duplicate task
// (cross_volume_duplicate, year_mismatch) into an executable
// consolidate_duplicate task, resetting status to pending so the
// housekeeping engine picks it up on the next tick.
func taskApproveHandler(db *database.MediaDB) ipc.Handler {
	return func(ctx context.Context, req ipc.Request, w ipc.FrameWriter) {
		var args taskIDArgs
		if err := json.Unmarshal(req.Args, &args); err != nil || args.ID == 0 {
			w.Error(req.ID, ipc.ErrBadRequest, "id required")
			return
		}
		t, err := db.GetHousekeepingTask(args.ID)
		if err != nil {
			w.Error(req.ID, ipc.ErrInternal, err.Error())
			return
		}
		if t == nil {
			w.Error(req.ID, ipc.ErrNotFound, "task not found")
			return
		}
		switch t.Kind {
		case database.TaskKindCrossVolumeDuplicate, database.TaskKindYearMismatch:
			// approvable
		default:
			w.Error(req.ID, ipc.ErrBadRequest, "task kind not approvable: "+t.Kind)
			return
		}
		payload := t.Payload
		if payload == nil {
			payload = map[string]any{}
		}
		payload["approved_from"] = t.Kind
		if err := db.UpdateHousekeepingTask(args.ID,
			database.TaskKindConsolidateDuplicate,
			database.TaskStatusPending, payload); err != nil {
			w.Error(req.ID, ipc.ErrInternal, err.Error())
			return
		}
		w.Result(req.ID, json.RawMessage(`{"approved":true}`))
	}
}

// payloadIntPtrLocal mirrors housekeeping.payloadIntPtr — JSON numbers
// arrive as float64; coerce to *int.
func payloadIntPtrLocal(p map[string]any, key string) *int {
	v, ok := p[key]
	if !ok || v == nil {
		return nil
	}
	switch x := v.(type) {
	case float64:
		i := int(x)
		return &i
	case int:
		return &x
	case int64:
		i := int(x)
		return &i
	}
	return nil
}
