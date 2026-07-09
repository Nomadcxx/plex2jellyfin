package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/logging"
)

func newTestScheduler(t *testing.T) (*Scheduler, *database.MediaDB) {
	t.Helper()
	db, err := database.OpenPath(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return New(db, nil), db
}

func TestSchedulerShutdownCancelsAndWaitsForRunningJobs(t *testing.T) {
	sched, _ := newTestScheduler(t)

	started := make(chan struct{})
	done := make(chan struct{})
	if err := sched.Register(Job{
		Name:     "long.running",
		Schedule: "@hourly",
		Run: func(ctx context.Context) (string, error) {
			close(started)
			<-ctx.Done()
			close(done)
			return "cancelled", ctx.Err()
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := sched.RunNow(context.Background(), "long.running"); err != nil {
		t.Fatalf("RunNow: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("job did not start")
	}

	sched.Shutdown()

	select {
	case <-done:
	default:
		t.Fatal("Shutdown returned before running job observed cancellation")
	}
	row, err := sched.db.GetScheduledJob("long.running")
	if err != nil {
		t.Fatalf("GetScheduledJob: %v", err)
	}
	if row == nil || row.Running {
		t.Fatalf("expected job running flag to be cleared after shutdown, got %#v", row)
	}
}

func TestSchedulerRunNowRejectsConcurrentStart(t *testing.T) {
	sched, _ := newTestScheduler(t)

	var runs int32
	release := make(chan struct{})
	if err := sched.Register(Job{
		Name:     "single.flight",
		Schedule: "@hourly",
		Run: func(ctx context.Context) (string, error) {
			atomic.AddInt32(&runs, 1)
			<-release
			return "ok", nil
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := sched.RunNow(context.Background(), "single.flight"); err != nil {
		t.Fatalf("first RunNow: %v", err)
	}
	err := sched.RunNow(context.Background(), "single.flight")
	close(release)
	sched.Wait()

	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("second RunNow error = %v, want already running", err)
	}
	if got := atomic.LoadInt32(&runs); got != 1 {
		t.Fatalf("job ran %d times, want 1", got)
	}
}

func TestSchedulerDoesNotInfoLogEmptySuccessfulResult(t *testing.T) {
	db, err := database.OpenPath(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer db.Close()

	logPath := filepath.Join(t.TempDir(), "scheduler.log")
	logger, err := logging.New(logging.Config{
		Level:      "info",
		File:       logPath,
		MaxSizeMB:  1,
		MaxBackups: 1,
	})
	if err != nil {
		t.Fatalf("logging.New: %v", err)
	}
	defer logger.Close()

	sched := New(db, logger)
	if err := sched.Register(Job{
		Name:     "quiet.noop",
		Schedule: "@hourly",
		Run: func(ctx context.Context) (string, error) {
			return "", nil
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := sched.RunNow(context.Background(), "quiet.noop"); err != nil {
		t.Fatalf("RunNow: %v", err)
	}
	sched.Wait()
	logger.Close()

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(data), "job quiet.noop done") {
		t.Fatalf("empty successful result should not produce info log, got:\n%s", data)
	}
}

func TestSchedulerFireRecoversPanicWithoutDB(t *testing.T) {
	sched := New(nil, nil)
	rj := &registeredJob{
		def: Job{
			Name: "panic.job",
			Run: func(ctx context.Context) (string, error) {
				panic("boom")
			},
		},
		running: true,
	}

	sched.wg.Add(1)
	sched.fire(context.Background(), "panic.job", rj)

	if rj.running {
		t.Fatal("panic recovery should clear running flag")
	}
}
