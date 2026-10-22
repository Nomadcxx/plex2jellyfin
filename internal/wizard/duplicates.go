package wizard

import (
	"errors"
	"fmt"
	"os"

	"github.com/Nomadcxx/plex2jellyfin/internal/service"
)

func (w *Wizard) handleDuplicates(analysis *service.DuplicateAnalysis) error {
	fmt.Println("=== Step 1: Duplicates ===")

	if !w.confirm("Handle duplicates now?") {
		fmt.Println("Skipping duplicates.")
		return nil
	}
	fmt.Println()

	deletedCount := 0
	reclaimedBytes := int64(0)
	applyAll := false
	var cleanupErrors []error

	for i, group := range analysis.Groups {
		yearStr := ""
		if group.Year != nil {
			yearStr = fmt.Sprintf(" (%d)", *group.Year)
		}

		episodeStr := ""
		if group.Season != nil && group.Episode != nil {
			episodeStr = fmt.Sprintf(" S%02dE%02d", *group.Season, *group.Episode)
		}

		fmt.Printf("[%d/%d] %s%s%s - %d copies found\n",
			i+1, len(analysis.Groups), group.Title, yearStr, episodeStr, len(group.Files))

		for _, f := range group.Files {
			marker := "DELETE"
			if f.ID == group.BestFileID {
				marker = "KEEP"
			}
			fmt.Printf("  %s: %s %s (%s) - %s\n",
				marker, f.Resolution, f.SourceType, formatBytes(f.Size), f.Path)
		}

		fmt.Printf("\n  Space saved: %s\n\n", formatBytes(group.ReclaimableBytes))

		var action string
		if applyAll {
			action = "k"
		} else {
			action = w.prompt("  [K]eep suggestion / [S]wap / [Skip] / [A]ll remaining / [Q]uit: ")
		}

		if action == "a" {
			applyAll = true
		}

		switch action {
		case "k", "", "a":
			for _, f := range group.Files {
				if f.ID != group.BestFileID {
					if w.dryRun {
						fmt.Printf("  Would delete: %s\n", f.Path)
					} else {
						fileRemoved, err := w.deleteDuplicateFile(f.Path)
						if err != nil && !fileRemoved {
							fmt.Printf("  ❌ Failed to delete %s: %v\n", f.Path, err)
							continue
						}
						if err != nil {
							fmt.Printf("  ❌ Removed from disk, but database cleanup remains incomplete for %s: %v\n", f.Path, err)
							cleanupErrors = append(cleanupErrors, fmt.Errorf("%s: %w", f.Path, err))
							continue
						}
						fmt.Printf("  ✅ Deleted: %s\n", f.Path)
					}
					deletedCount++
					reclaimedBytes += f.Size
				}
			}
		case "s":
			fmt.Println("  Swap not implemented yet - skipping")
		case "skip":
			fmt.Println("  Skipped")
		case "q":
			fmt.Println("Quitting duplicates step.")
			break
		}
		fmt.Println()
	}

	if w.dryRun {
		fmt.Printf("✅ Would delete %d files, reclaim %s\n\n", deletedCount, formatBytes(reclaimedBytes))
	} else if len(cleanupErrors) > 0 {
		fmt.Printf("❌ Cleanup incomplete: %d database reconciliation failure(s); completed %d other deletions, reclaimed %s. Run `plex2jellyfin scan` to retry reconciliation.\n\n",
			len(cleanupErrors), deletedCount, formatBytes(reclaimedBytes))
	} else {
		fmt.Printf("✅ Deleted %d files, reclaimed %s\n\n", deletedCount, formatBytes(reclaimedBytes))
	}

	return errors.Join(cleanupErrors...)
}

func (w *Wizard) deleteDuplicateFile(path string) (bool, error) {
	if err := os.Remove(path); err != nil {
		return false, err
	}
	if err := w.db.DeleteMediaFile(path); err != nil {
		if reconcileErr := w.service.ReconcileMissingMediaFile(path); reconcileErr != nil {
			return true, fmt.Errorf("database cleanup failed: %v; missing-file reconciliation failed: %w; run `plex2jellyfin scan` to retry", err, reconcileErr)
		}
		fmt.Printf("  ⚠ Database cleanup failed, but missing-file reconciliation removed the stale row: %v\n", err)
	}
	return true, nil
}
