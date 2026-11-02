package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/consolidate"
	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/plans"
	"github.com/Nomadcxx/plex2jellyfin/internal/privilege"
	"github.com/Nomadcxx/plex2jellyfin/internal/transfer"
)

func runConsolidateExecute(db *database.MediaDB, cfg *config.Config) error {
	plan, err := plans.LoadConsolidatePlans()
	if err != nil {
		return fmt.Errorf("failed to load plans: %w", err)
	}

	if plan == nil {
		fmt.Println("No pending plans found.")
		fmt.Println("Run 'plex2jellyfin consolidate generate' first to create plans.")
		return nil
	}

	issues := append(consolidatePlanSafetyIssues(plan), consolidatePlanRootIssues(plan, cfg)...)
	issues = append(issues, consolidatePlanDatabaseIssues(db, plan)...)
	if len(issues) > 0 {
		fmt.Println("[error] Consolidation plan failed safety validation; refusing to execute.")
		printConsolidateSafetyIssues(issues)
		fmt.Println("\nRegenerate after planner fixes, or inspect with 'plex2jellyfin consolidate dry-run'.")
		return nil
	}

	// Escalate only after validating the plan. Unsafe plans should not require sudo.
	if privilege.NeedsRoot() {
		return privilege.Escalate("move files across filesystems and set ownership")
	}

	fmt.Printf("[warn] This will move %d files (%s).\n", plan.Summary.TotalMoves, formatBytes(plan.Summary.TotalBytes))
	fmt.Print("Continue? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read confirmation: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response != "y" && response != "yes" {
		fmt.Println("[error] Execution cancelled.")
		return nil
	}

	fmt.Println("\nExecuting consolidation plans...")

	allLibraryRoots := append(cfg.Libraries.TV, cfg.Libraries.Movies...)

	transferer, err := transfer.New(transfer.BackendAuto)
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
				fmt.Printf("  [skip] Already moved: %s\n", filepath.Base(op.SourcePath))
				alreadyGoneCount++
				continue
			}
			if err := consolidate.CheckDestinationAvailable(op.TargetPath); err != nil {
				fmt.Printf("  [error] Failed to move: %v\n", err)
				failedCount++
				continue
			}

			fmt.Printf("  Moving: %s\n", op.SourcePath)

			// Ensure target directory exists
			targetDir := op.TargetPath[:strings.LastIndex(op.TargetPath, "/")]
			if err := os.MkdirAll(targetDir, 0755); err != nil {
				fmt.Printf("  [error] Failed to create directory: %v\n", err)
				failedCount++
				continue
			}

			// Move file
			transferOptions := transfer.OptionsFromConfig(cfg)
			// Update database
			targetLibraryRoot := configuredRootForPath(group.TargetLocation, allLibraryRoots)
			moveBytes := int64(0)
			moved := false
			movedIdentity := ""
			move := func(sourceIdentity, sourceQuarantinePath string, recordPublished func(string, string) error) (database.MediaFileMoveAttempt, error) {
				transferOptions.MoveSourceIdentity = sourceIdentity
				transferOptions.MoveSourceQuarantinePath = sourceQuarantinePath
				transferOptions.OnPublished = recordPublished
				result, err := transferer.Move(op.SourcePath, op.TargetPath, transferOptions)
				attempt := database.MediaFileMoveAttempt{}
				if result != nil {
					attempt.Published = result.Published
					attempt.SourceRemoved = result.SourceRemoved
					attempt.PublishedIdentity = result.PublishedIdentity
					attempt.PendingSourceRemoval = result.PendingSourceRemoval
				}
				if err != nil {
					return attempt, fmt.Errorf("transfer failed: %w", err)
				}
				if err := result.Validate(); err != nil {
					return attempt, fmt.Errorf("transfer failed: %w", err)
				}
				if !result.SourceRemoved {
					return attempt, fmt.Errorf("transfer completed without removing source: %s", op.SourcePath)
				}
				moveBytes = result.BytesCopied
				moved = true
				movedIdentity = result.PublishedIdentity
				return attempt, nil
			}
			if err := updateDatabaseAfterMove(db, op.SourcePath, op.TargetPath, op.Size, group.TargetLocation, targetLibraryRoot, move); err != nil {
				if moved {
					if rollbackErr := transfer.RollbackMove(transferer, op.SourcePath, op.TargetPath, transferOptions, movedIdentity); rollbackErr != nil {
						fmt.Printf("  [warn] Database update issue: %v; filesystem rollback failed: %v\n", err, rollbackErr)
					} else {
						if cancelErr := db.CancelMediaFileMove(op.SourcePath); cancelErr != nil {
							fmt.Printf("  [warn] Database update issue; file restored but reservation cleanup failed: %v\n", cancelErr)
						} else {
							fmt.Printf("  [warn] Database update issue; file restored to source: %v\n", err)
						}
					}
				} else {
					fmt.Printf("  [error] Failed to move: %v\n", err)
				}
				failedCount++
				continue
			}

			// Cleanup source directory (delete cruft and empty dirs)
			sourceDir := filepath.Dir(op.SourcePath)
			if err := cleanupSourceDir(sourceDir, allLibraryRoots); err != nil {
				fmt.Printf("  [warn] Cleanup warning: %v\n", err)
			}

			movedCount++
			movedBytes += moveBytes
			fmt.Printf("  [ok] Moved (%s)\n", formatBytes(moveBytes))
		}
	}

	// Handle plan file based on results
	if failedCount == 0 {
		if err := plans.DeleteConsolidatePlans(); err != nil {
			fmt.Printf("[warn] Failed to clean up plans file: %v\n", err)
		}
		fmt.Println("\n[ok] Plan completed and removed")
	} else {
		if err := plans.ArchiveConsolidatePlans(); err != nil {
			fmt.Printf("[warn] Failed to archive plans file: %v\n", err)
		}
		fmt.Println("\n[warn] Plan archived to consolidate.json.old due to failures")
	}

	fmt.Println("\n=== Execution Complete ===")
	fmt.Printf("[ok] Successfully moved: %d files\n", movedCount)
	if alreadyGoneCount > 0 {
		fmt.Printf("[skip] Already moved:     %d files\n", alreadyGoneCount)
	}
	if failedCount > 0 {
		fmt.Printf("[error] Failed to move:     %d files\n", failedCount)
	}
	fmt.Printf("Data relocated:     %s\n", formatBytes(movedBytes))

	return nil
}

func consolidatePlanRootIssues(plan *plans.ConsolidatePlan, cfg *config.Config) []string {
	roots := configuredLibraryRoots(cfg)
	if len(roots) == 0 || plan == nil {
		return nil
	}
	var issues []string
	for _, group := range plan.Plans {
		if issue := rootBoundPathIssue("target location", group.TargetLocation, roots); issue != "" {
			issues = append(issues, issue)
		}
		for _, op := range group.Operations {
			if op.Action != "move" {
				continue
			}
			if issue := rootBoundPathIssue("source path", op.SourcePath, roots); issue != "" {
				issues = append(issues, issue)
			}
			if issue := rootBoundPathIssue("target path", op.TargetPath, roots); issue != "" {
				issues = append(issues, issue)
			}
		}
	}
	return issues
}

func consolidatePlanSafetyIssues(plan *plans.ConsolidatePlan) []string {
	if plan == nil {
		return nil
	}

	var issues []string
	sourceCounts := make(map[string]int)
	destinationCounts := make(map[string]int)
	for _, group := range plan.Plans {
		if hasSeasonComponent(group.TargetLocation) {
			issues = append(issues, fmt.Sprintf("target location includes season/release structure: %s", group.TargetLocation))
		}
		for _, op := range group.Operations {
			if op.Action != "move" {
				continue
			}
			sourceCounts[filepath.Clean(op.SourcePath)]++
			destinationCounts[filepath.Clean(op.TargetPath)]++
			if pathIsWithin(group.TargetLocation, op.SourcePath) {
				issues = append(issues, fmt.Sprintf("source already under target location; not a consolidation move: %s", op.SourcePath))
			}
			if hasSeasonComponent(op.SourcePath) && !hasSeasonComponent(op.TargetPath) {
				issues = append(issues, fmt.Sprintf("target would drop season structure: %s -> %s", op.SourcePath, op.TargetPath))
			}
			if _, err := os.Stat(op.TargetPath); err == nil {
				issues = append(issues, fmt.Sprintf("target already exists: %s", op.TargetPath))
			}
		}
	}

	for source, count := range sourceCounts {
		if count > 1 {
			issues = append(issues, fmt.Sprintf("duplicate source appears in %d planned moves: %s", count, source))
		}
	}
	for destination, count := range destinationCounts {
		if count > 1 {
			issues = append(issues, fmt.Sprintf("duplicate destination appears in %d planned moves: %s", count, destination))
		}
	}

	return issues
}

func consolidatePlanDatabaseIssues(db *database.MediaDB, plan *plans.ConsolidatePlan) []string {
	if db == nil || plan == nil {
		return nil
	}
	var issues []string
	for _, group := range plan.Plans {
		for _, op := range group.Operations {
			if op.Action != "move" {
				continue
			}
			if err := db.CheckMediaFileMoveDestination(op.SourcePath, op.TargetPath); err != nil {
				issues = append(issues, fmt.Sprintf("cannot move to %s: %v", op.TargetPath, err))
			}
		}
	}
	return issues
}

func configuredRootForPath(path string, roots []string) string {
	cleanPath := filepath.Clean(path)
	best := ""
	for _, root := range roots {
		cleanRoot := filepath.Clean(root)
		rel, err := filepath.Rel(cleanRoot, cleanPath)
		if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if len(cleanRoot) > len(best) {
			best = cleanRoot
		}
	}
	return best
}

func printConsolidateSafetyIssues(issues []string) {
	const maxIssues = 20
	for i, issue := range issues {
		if i >= maxIssues {
			fmt.Printf("  ... and %d more issue(s)\n", len(issues)-maxIssues)
			return
		}
		fmt.Printf("  - %s\n", issue)
	}
}

func pathIsWithin(parent, child string) bool {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func hasSeasonComponent(path string) bool {
	for _, part := range strings.Split(filepath.Clean(path), string(filepath.Separator)) {
		lower := strings.ToLower(strings.TrimSpace(part))
		if strings.HasPrefix(lower, "season ") || strings.HasPrefix(lower, "season_") || strings.HasPrefix(lower, "season-") {
			return true
		}
	}
	return false
}

// updateDatabaseAfterMove moves the file while reserving its destination row.
func updateDatabaseAfterMove(db *database.MediaDB, sourcePath, targetPath string, size int64, targetCanonicalPath, targetLibraryRoot string, move func(string, string, func(string, string) error) (database.MediaFileMoveAttempt, error)) error {
	// Try to get existing file entry
	file, err := db.GetMediaFile(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to read media file: %w", err)
	}
	if file == nil {
		// Track the source first so a failed move still has a reserved DB row.
		fmt.Printf("  File not tracked, adding to database\n")
		if err := createMediaFileEntry(db, sourcePath, targetPath, size); err != nil {
			return fmt.Errorf("failed to track source file before move: %w", err)
		}
		file, err = db.GetMediaFile(sourcePath)
		if err != nil {
			return fmt.Errorf("failed to read newly tracked source file: %w", err)
		}
		if file == nil {
			return fmt.Errorf("source file was not tracked before move: %s", sourcePath)
		}
	}

	info, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to inspect source file: %w", err)
	}
	file.Path = targetPath
	file.Size = info.Size()
	file.ModifiedAt = info.ModTime()
	if targetLibraryRoot != "" {
		file.LibraryRoot = targetLibraryRoot
	}
	return db.MoveMediaFileWith(file, sourcePath, targetCanonicalPath, false, func(sourceIdentity, sourceQuarantinePath string, recordPublished func(string, string) error) (database.MediaFileMoveAttempt, error) {
		attempt, err := move(sourceIdentity, sourceQuarantinePath, recordPublished)
		if err != nil {
			return attempt, err
		}
		info, err := os.Stat(targetPath)
		if err != nil {
			return attempt, fmt.Errorf("failed to inspect moved file: %w", err)
		}
		file.Size = info.Size()
		file.ModifiedAt = info.ModTime()
		return attempt, nil
	})
}

// createMediaFileEntry creates a minimal database entry for a moved file
func createMediaFileEntry(db *database.MediaDB, path, classificationPath string, size int64) error {
	// Get file info
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	// Determine media type from path
	mediaType := "movie"
	if strings.Contains(strings.ToLower(classificationPath), "tvshow") ||
		strings.Contains(strings.ToLower(classificationPath), "tv show") ||
		strings.Contains(classificationPath, "Season") {
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
