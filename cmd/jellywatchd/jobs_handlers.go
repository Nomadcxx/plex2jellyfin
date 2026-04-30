package main

import (
	"context"
	"encoding/json"

	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/scheduler"
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
