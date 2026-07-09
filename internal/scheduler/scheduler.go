// Package scheduler provides a small cron-style runner for jellywatchd's
// recurring jobs (housekeeping detection, drain, etc.).
//
// Schedules are simple strings:
//
//	"HH:MM"        - daily at the given local time
//	"@hourly"      - top of every hour
//	"@continuous"  - run, then re-run as soon as previous finishes
//	"every:Nm"     - every N minutes
//
// Each registered Job is executed at most once at a time; concurrent
// runs are skipped with last_result="skipped (already running)".
package scheduler

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/logging"
)

// Job is a single registered task.
type Job struct {
	Name     string
	Schedule string // overridden by DB row when persisted
	Run      func(ctx context.Context) (string, error)
}

// Scheduler ticks once a minute (or every continuousTick for continuous
// jobs) and fires due jobs.
type Scheduler struct {
	db      *database.MediaDB
	logger  *logging.Logger
	mu      sync.Mutex
	wg      sync.WaitGroup
	jobs    map[string]*registeredJob
	cancels map[string]context.CancelFunc
}

type registeredJob struct {
	def     Job
	running bool
}

func New(db *database.MediaDB, logger *logging.Logger) *Scheduler {
	return &Scheduler{
		db:      db,
		logger:  logger,
		jobs:    map[string]*registeredJob{},
		cancels: map[string]context.CancelFunc{},
	}
}

// Register adds a job. If the DB has no row for it yet, it's seeded with
// the default schedule from j.Schedule.
func (s *Scheduler) Register(j Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[j.Name] = &registeredJob{def: j}
	if err := s.db.UpsertScheduledJob(j.Name, j.Schedule, true, "{}"); err != nil {
		return fmt.Errorf("seed job %s: %w", j.Name, err)
	}
	return nil
}

// Run blocks until ctx is cancelled, ticking every minute.
func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	s.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	rows, err := s.db.ListScheduledJobs()
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("scheduler", fmt.Sprintf("list jobs failed: %v", err))
		}
		return
	}
	now := time.Now()
	for _, row := range rows {
		if !row.Enabled || row.Running {
			continue
		}
		s.mu.Lock()
		rj, ok := s.jobs[row.Name]
		s.mu.Unlock()
		if !ok || rj.running {
			continue
		}
		if !shouldRun(row, now) {
			continue
		}
		if err := s.start(ctx, row.Name); err != nil && s.logger != nil && !strings.Contains(err.Error(), "already running") {
			s.logger.Warn("scheduler", fmt.Sprintf("start job failed name=%s err=%v", row.Name, err))
		}
	}
}

func (s *Scheduler) start(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	rj, ok := s.jobs[name]
	if !ok || rj.running {
		s.mu.Unlock()
		if !ok {
			return fmt.Errorf("unknown job %q", name)
		}
		return fmt.Errorf("job %q is already running", name)
	}
	rj.running = true
	jobCtx, cancel := context.WithCancel(ctx)
	s.cancels[name] = cancel
	s.wg.Add(1)
	s.mu.Unlock()

	go s.fire(jobCtx, name, rj)
	return nil
}

func (s *Scheduler) fire(ctx context.Context, name string, rj *registeredJob) {
	defer func() {
		if r := recover(); r != nil {
			if s.logger != nil {
				s.logger.Error("scheduler", fmt.Sprintf("job %s panicked: %v", name, r), nil)
			}
			if s.db != nil {
				_ = s.db.RecordScheduledJobRun(name, "", fmt.Sprintf("panic: %v", r), 0, time.Time{})
			}
		}
		s.mu.Lock()
		rj.running = false
		delete(s.cancels, name)
		s.mu.Unlock()
		s.wg.Done()
	}()

	if s.db != nil {
		_ = s.db.MarkScheduledJobRunning(name, true)
	}
	start := time.Now()
	result, err := rj.def.Run(ctx)
	dur := time.Since(start)

	var row *database.ScheduledJob
	if s.db != nil {
		row, _ = s.db.GetScheduledJob(name)
	}
	next := time.Time{}
	if row != nil {
		next = nextRun(row.Schedule, time.Now())
	}
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	if s.db != nil {
		if recErr := s.db.RecordScheduledJobRun(name, result, errStr, dur, next); recErr != nil && s.logger != nil {
			s.logger.Warn("scheduler", fmt.Sprintf("record run failed name=%s err=%v", name, recErr))
		}
	}
	if s.logger != nil {
		if err != nil {
			s.logger.Error("scheduler", fmt.Sprintf("job %s failed in %s: %v", name, dur, err), nil)
		} else if strings.TrimSpace(result) != "" {
			s.logger.Info("scheduler", fmt.Sprintf("job %s done in %s: %s", name, dur, result))
		}
	}
}

// RunNow triggers a job out-of-band (used by IPC/API).
func (s *Scheduler) RunNow(ctx context.Context, name string) error {
	return s.start(ctx, name)
}

// Stop cancels a running job.
func (s *Scheduler) Stop(name string) error {
	s.mu.Lock()
	cancel, ok := s.cancels[name]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("job %q is not running", name)
	}
	cancel()
	return nil
}

// Shutdown cancels all running jobs and waits for them to finish.
func (s *Scheduler) Shutdown() {
	s.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(s.cancels))
	for _, cancel := range s.cancels {
		cancels = append(cancels, cancel)
	}
	s.mu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
	s.Wait()
}

// Wait blocks until all currently running jobs have finished.
func (s *Scheduler) Wait() {
	s.wg.Wait()
}

// shouldRun decides if `row` should fire now.
func shouldRun(row database.ScheduledJob, now time.Time) bool {
	sched := strings.TrimSpace(row.Schedule)
	switch {
	case sched == "@continuous":
		// Always eligible — Scheduler.tick will skip if Running=true.
		return true
	case sched == "@hourly":
		if !row.LastRunAt.Valid {
			return true
		}
		return now.Sub(row.LastRunAt.Time) >= time.Hour
	case strings.HasPrefix(sched, "every:"):
		dur, err := parseEvery(sched)
		if err != nil {
			return false
		}
		if !row.LastRunAt.Valid {
			return true
		}
		return now.Sub(row.LastRunAt.Time) >= dur
	default:
		// HH:MM: fire when current minute matches and we haven't already
		// run today.
		hh, mm, ok := parseHHMM(sched)
		if !ok {
			return false
		}
		if now.Hour() != hh || now.Minute() != mm {
			return false
		}
		if row.LastRunAt.Valid && sameMinute(row.LastRunAt.Time, now) {
			return false
		}
		return true
	}
}

func nextRun(sched string, now time.Time) time.Time {
	sched = strings.TrimSpace(sched)
	switch {
	case sched == "@continuous":
		return now.Add(5 * time.Second)
	case sched == "@hourly":
		return now.Add(time.Hour).Truncate(time.Hour)
	case strings.HasPrefix(sched, "every:"):
		dur, err := parseEvery(sched)
		if err != nil {
			return time.Time{}
		}
		return now.Add(dur)
	default:
		hh, mm, ok := parseHHMM(sched)
		if !ok {
			return time.Time{}
		}
		next := time.Date(now.Year(), now.Month(), now.Day(), hh, mm, 0, 0, now.Location())
		if !next.After(now) {
			next = next.Add(24 * time.Hour)
		}
		return next
	}
}

func parseHHMM(s string) (int, int, bool) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	hh, err1 := strconv.Atoi(parts[0])
	mm, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	if hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, 0, false
	}
	return hh, mm, true
}

func parseEvery(s string) (time.Duration, error) {
	rest := strings.TrimPrefix(s, "every:")
	return time.ParseDuration(rest)
}

func sameMinute(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day() &&
		a.Hour() == b.Hour() && a.Minute() == b.Minute()
}
