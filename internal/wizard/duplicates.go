package wizard

import (
	"fmt"
	"os"

	"github.com/Nomadcxx/jellywatch/internal/service"
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

		switch action {
		case "k", "":
			for _, f := range group.Files {
				if f.ID != group.BestFileID {
					if w.dryRun {
						fmt.Printf("  Would delete: %s\n", f.Path)
					} else {
						if err := os.Remove(f.Path); err != nil {
							fmt.Printf("  ❌ Failed to delete %s: %v\n", f.Path, err)
							continue
						}
						_ = w.db.DeleteMediaFile(f.Path)
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
		case "a":
			applyAll = true
			for _, f := range group.Files {
				if f.ID != group.BestFileID {
					if w.dryRun {
						fmt.Printf("  Would delete: %s\n", f.Path)
					} else {
						if err := os.Remove(f.Path); err != nil {
							fmt.Printf("  ❌ Failed to delete %s: %v\n", f.Path, err)
							continue
						}
						_ = w.db.DeleteMediaFile(f.Path)
						fmt.Printf("  ✅ Deleted: %s\n", f.Path)
					}
					deletedCount++
					reclaimedBytes += f.Size
				}
			}
		case "q":
			fmt.Println("Quitting duplicates step.")
			break
		}
		fmt.Println()
	}

	if w.dryRun {
		fmt.Printf("✅ Would delete %d files, reclaim %s\n\n", deletedCount, formatBytes(reclaimedBytes))
	} else {
		fmt.Printf("✅ Deleted %d files, reclaimed %s\n\n", deletedCount, formatBytes(reclaimedBytes))
	}

	return nil
}
