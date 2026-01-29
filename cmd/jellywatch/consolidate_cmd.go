package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/plans"
	"github.com/Nomadcxx/jellywatch/internal/transfer"
)

func runConsolidateExecute(db *database.MediaDB) error {
	plan, err := plans.LoadConsolidatePlans()
	if err != nil {
		return fmt.Errorf("failed to load plans: %w", err)
	}

	if plan == nil {
		fmt.Println("No pending plans found.")
		fmt.Println("Run 'jellywatch consolidate generate' first to create plans.")
		return nil
	}

	fmt.Printf("⚠️  This will move %d files (%s).\n", plan.Summary.TotalMoves, formatBytes(plan.Summary.TotalBytes))
	fmt.Print("Continue? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read confirmation: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response != "y" && response != "yes" {
		fmt.Println("❌ Execution cancelled.")
		return nil
	}

	fmt.Println("\n📦 Executing consolidation plans...")

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	allLibraryRoots := append(cfg.Libraries.TV, cfg.Libraries.Movies...)

	transferer, err := transfer.New(transfer.BackendRsync)
	if err != nil {
		return fmt.Errorf("failed to create transferer: %w", err)
	}

	movedCount := 0
	alreadyGoneCount := 0
	failedCount := 0
	movedBytes := int64(0)

	for _, group := range plan.Plans {
		yearStr := ""
		if group.Year != nil {
			yearStr = fmt.Sprintf(" (%d)", *group.Year)
		}
		fmt.Printf("\n[%s] %s%s\n", group.MediaType, group.Title, yearStr)

		for _, op := range group.Operations {
			if op.Action != "move" {
				continue
			}

			// Check if source exists BEFORE attempting move
			if _, err := os.Stat(op.SourcePath); os.IsNotExist(err) {
				fmt.Printf("  ⏭️  Already moved: %s\n", filepath.Base(op.SourcePath))
				alreadyGoneCount++
				continue
			}

			fmt.Printf("  Moving: %s\n", op.SourcePath)

			// Ensure target directory exists
			targetDir := op.TargetPath[:strings.LastIndex(op.TargetPath, "/")]
			if err := os.MkdirAll(targetDir, 0755); err != nil {
				fmt.Printf("  ❌ Failed to create directory: %v\n", err)
				failedCount++
				continue
			}

			// Move file
			result, err := transferer.Move(op.SourcePath, op.TargetPath, transfer.DefaultOptions())
			if err != nil {
				fmt.Printf("  ❌ Failed to move: %v\n", err)
				failedCount++
				continue
			}

			// Update database
			if err := updateDatabaseAfterMove(db, op.SourcePath, op.TargetPath, op.Size); err != nil {
				fmt.Printf("  ⚠️  Moved but database update issue: %v\n", err)
			}

			// Cleanup source directory (delete cruft and empty dirs)
			sourceDir := filepath.Dir(op.SourcePath)
			if err := cleanupSourceDir(sourceDir, allLibraryRoots); err != nil {
				fmt.Printf("  ⚠️  Cleanup warning: %v\n", err)
			}

			movedCount++
			movedBytes += result.BytesCopied
			fmt.Printf("  ✅ Moved (%s)\n", formatBytes(result.BytesCopied))
		}
	}

	// Handle plan file based on results
	if failedCount == 0 {
		if err := plans.DeleteConsolidatePlans(); err != nil {
			fmt.Printf("⚠️  Failed to clean up plans file: %v\n", err)
		}
		fmt.Println("\n✅ Plan completed and removed")
	} else {
		if err := plans.ArchiveConsolidatePlans(); err != nil {
			fmt.Printf("⚠️  Failed to archive plans file: %v\n", err)
		}
		fmt.Println("\n⚠️  Plan archived to consolidate.json.old due to failures")
	}

	fmt.Println("\n=== Execution Complete ===")
	fmt.Printf("✅ Successfully moved: %d files\n", movedCount)
	if alreadyGoneCount > 0 {
		fmt.Printf("⏭️  Already moved:     %d files\n", alreadyGoneCount)
	}
	if failedCount > 0 {
		fmt.Printf("❌ Failed to move:     %d files\n", failedCount)
	}
	fmt.Printf("📦 Data relocated:     %s\n", formatBytes(movedBytes))

	return nil
}

// updateDatabaseAfterMove updates or creates a database entry after a file move
func updateDatabaseAfterMove(db *database.MediaDB, sourcePath, targetPath string, size int64) error {
	// Try to get existing file entry
	file, err := db.GetMediaFile(sourcePath)
	if err != nil || file == nil {
		// File not in DB - create new entry
		fmt.Printf("  ℹ️  File not tracked, adding to database\n")
		return createMediaFileEntry(db, targetPath, size)
	}

	// Delete old entry
	if err := db.DeleteMediaFile(sourcePath); err != nil {
		return fmt.Errorf("failed to delete old entry: %w", err)
	}

	// Update path and upsert
	file.Path = targetPath
	if err := db.UpsertMediaFile(file); err != nil {
		return fmt.Errorf("failed to update entry: %w", err)
	}

	return nil
}

// createMediaFileEntry creates a minimal database entry for a moved file
func createMediaFileEntry(db *database.MediaDB, path string, size int64) error {
	// Get file info
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	// Determine media type from path
	mediaType := "movie"
	if strings.Contains(strings.ToLower(path), "tvshow") ||
		strings.Contains(strings.ToLower(path), "tv show") ||
		strings.Contains(path, "Season") {
		mediaType = "episode"
	}

	// Create minimal entry - will be enriched on next scan
	file := &database.MediaFile{
		Path:            path,
		Size:            info.Size(),
		ModifiedAt:      info.ModTime(),
		MediaType:       mediaType,
		NormalizedTitle: "pending-rescan", // Will be updated on next scan
		Source:          "consolidate",
		SourcePriority:  50,
	}

	return db.UpsertMediaFile(file)
}
