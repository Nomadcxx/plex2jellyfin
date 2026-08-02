package main

import (
	"context"
	"fmt"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/scheduler"
	syncsvc "github.com/Nomadcxx/plex2jellyfin/internal/sync"
)

// registerLibrarySyncJob seeds a bounded daily filesystem inventory reconciliation.
// Scheduler single-flight prevents overlapping runs; the job itself only scans
// configured libraries via SyncFromFilesystem (not full arr sync).
func registerLibrarySyncJob(sched *scheduler.Scheduler, svc *syncsvc.SyncService) error {
	if sched == nil || svc == nil {
		return fmt.Errorf("scheduler and sync service are required")
	}
	return sched.Register(scheduler.Job{
		Name:     "library.sync",
		Schedule: "03:00",
		Run: func(ctx context.Context) (string, error) {
			jobCtx, cancel := context.WithTimeout(ctx, 2*time.Hour)
			defer cancel()
			result, err := svc.SyncFromFilesystem(jobCtx)
			if err != nil {
				return "", err
			}
			if result == nil {
				return "scanned=0 added=0 updated=0 skipped=0", nil
			}
			return formatLibrarySyncResult(result.FilesScanned, result.FilesAdded, result.FilesUpdated, result.FilesSkipped), nil
		},
	})
}

func formatLibrarySyncResult(scanned, added, updated, skipped int) string {
	return fmt.Sprintf("scanned=%d added=%d updated=%d skipped=%d", scanned, added, updated, skipped)
}
