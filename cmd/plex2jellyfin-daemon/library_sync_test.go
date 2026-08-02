package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/scheduler"
	"github.com/Nomadcxx/plex2jellyfin/internal/sync"
)

func TestRegisterLibrarySyncJob_SeedsDailySchedule(t *testing.T) {
	db, err := database.OpenPath(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer db.Close()

	sched := scheduler.New(db, nil)
	svc := sync.NewSyncService(sync.SyncConfig{
		DB:          db,
		TVLibraries: []string{t.TempDir()},
		MovieLibraries: []string{t.TempDir()},
		SyncHour:    3,
	})

	if err := registerLibrarySyncJob(sched, svc); err != nil {
		t.Fatalf("registerLibrarySyncJob: %v", err)
	}

	row, err := db.GetScheduledJob("library.sync")
	if err != nil {
		t.Fatalf("GetScheduledJob: %v", err)
	}
	if row == nil {
		t.Fatal("expected library.sync job row")
	}
	if row.Schedule != "03:00" {
		t.Fatalf("schedule = %q, want 03:00", row.Schedule)
	}
	if !row.Enabled {
		t.Fatal("library.sync should be enabled")
	}
}

func TestRegisterLibrarySyncJob_RunsBoundedFilesystemSync(t *testing.T) {
	db, err := database.OpenPath(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer db.Close()

	sched := scheduler.New(db, nil)
	svc := sync.NewSyncService(sync.SyncConfig{
		DB:             db,
		TVLibraries:    []string{t.TempDir()},
		MovieLibraries: []string{t.TempDir()},
	})
	if err := registerLibrarySyncJob(sched, svc); err != nil {
		t.Fatalf("registerLibrarySyncJob: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sched.RunNow(ctx, "library.sync"); err != nil {
		t.Fatalf("RunNow: %v", err)
	}
	sched.Wait()

	row, err := db.GetScheduledJob("library.sync")
	if err != nil {
		t.Fatalf("GetScheduledJob: %v", err)
	}
	if row == nil || row.Running {
		t.Fatalf("expected completed job, got %#v", row)
	}
	if row.LastError.Valid && strings.TrimSpace(row.LastError.String) != "" {
		t.Fatalf("library.sync last_error = %q", row.LastError.String)
	}
	if !row.LastRunAt.Valid {
		t.Fatal("expected LastRunAt after RunNow")
	}
}

func TestLibrarySyncJobResultFormat(t *testing.T) {
	got := formatLibrarySyncResult(12, 3, 1, 0)
	want := "scanned=12 added=3 updated=1 skipped=0"
	if got != want {
		t.Fatalf("formatLibrarySyncResult() = %q, want %q", got, want)
	}
}
