package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/permissions"
	"github.com/Nomadcxx/jellywatch/internal/plans"
	"github.com/Nomadcxx/jellywatch/internal/privilege"
)

func deleteDuplicateFile(db *database.MediaDB, filePath string, uid, gid int) error {
	canDelete, err := permissions.CanDelete(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			_ = db.DeleteMediaFile(filePath)
			return nil
		}
		return fmt.Errorf("failed to check permissions: %w", err)
	}

	if !canDelete {
		if err := permissions.FixPermissions(filePath, uid, gid); err != nil {
			if removeErr := os.Remove(filePath); removeErr != nil {
				_ = db.DeleteMediaFile(filePath)
				if os.IsNotExist(removeErr) {
					return nil
				}
				return permissions.NewPermissionError(filePath, "delete", removeErr, uid, gid)
			}
		}
	}

	if err := os.Remove(filePath); err != nil {
		_ = db.DeleteMediaFile(filePath)

		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to delete file: %w", err)
	}

	if err := db.DeleteMediaFile(filePath); err != nil {
		fmt.Printf("  ⚠️  Warning: File deleted but database cleanup failed: %v\n", err)
	}

	return nil
}

func runDuplicatesExecute(db *database.MediaDB, cfg *config.Config) error {
	// Escalate to root if needed for file operations
	if privilege.NeedsRoot() {
		return privilege.Escalate("delete files and modify ownership")
	}

	plan, err := plans.LoadDuplicatePlans()
	if err != nil {
		return fmt.Errorf("failed to load plans: %w", err)
	}

	if plan == nil {
		fmt.Println("No pending plans found.")
		fmt.Println("Run 'jellywatch duplicates generate' first to create plans.")
		return nil
	}

	fmt.Printf("⚠️  WARNING: This will permanently DELETE %d files.\n", plan.Summary.FilesToDelete)
	fmt.Printf("Space to reclaim: %s\n", formatBytes(plan.Summary.SpaceReclaimable))
	fmt.Println("This action CANNOT be undone!")
	fmt.Print("\nContinue? [y/N]: ")

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

	fmt.Println("\n🗑️ Deleting duplicate files...")

	deletedCount := 0
	reclaimedBytes := int64(0)

	for i, p := range plan.Plans {
		yearStr := ""
		if p.Year != nil {
			yearStr = fmt.Sprintf(" (%d)", *p.Year)
		}

		if p.MediaType == "movie" {
			fmt.Printf("[%d] %s%s\n", i+1, p.Title, yearStr)
		} else {
			fmt.Printf("[%d] %s S%02dE%02d\n", i+1, p.Title, p.Season, p.Episode)
		}

		// Only delete the duplicate file, not the keeper
		file := p.Delete
		filePath := file.Path

		var uid, gid int = -1, -1
		if cfg != nil && cfg.Permissions.WantsOwnership() {
			uid, _ = cfg.Permissions.ResolveUID()
			gid, _ = cfg.Permissions.ResolveGID()
		}

		if err := deleteDuplicateFile(db, filePath, uid, gid); err != nil {
			fmt.Printf("  ❌ Failed to delete %s: %v\n", filePath, err)
			continue
		}

		deletedCount++
		reclaimedBytes += file.Size
	}

	// Handle plan file on results
	if deletedCount == 0 {
		if err := plans.DeleteDuplicatePlans(); err != nil {
			fmt.Printf("⚠️  Failed to clean up plans file: %v\n", err)
		}
		fmt.Println("\n✅ Plan completed and removed")
	} else {
		if err := plans.ArchiveDuplicatePlans(); err != nil {
			fmt.Printf("⚠️  Failed to archive plans file: %v\n", err)
		}
		fmt.Println("\n⚠️  Plan archived to duplicates.json.old due to failures")
	}

	fmt.Println("\n=== Execution Complete ===")
	fmt.Printf("✅ Successfully deleted: %d files\n", deletedCount)
	fmt.Printf("📦 Space reclaimed:      %s\n", formatBytes(reclaimedBytes))

	return nil
}
