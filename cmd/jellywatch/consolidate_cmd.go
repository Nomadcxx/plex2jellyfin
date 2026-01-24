package main

import (
	"context"
	"fmt"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/consolidate"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/spf13/cobra"
)

func newConsolidateCmd() *cobra.Command {
	var (
		generate bool
		dryRun   bool
		execute  bool
		status   bool
	)

	cmd := &cobra.Command{
		Use:   "consolidate [flags]",
		Short: "Detect and remove duplicate files using CONDOR system",
		Long: `Detect duplicate media files and non-compliant filenames using the CONDOR
database-driven system.

The CONDOR system analyzes your media_files database to identify:
  - Duplicate files (same movie/episode with different quality)
  - Non-Jellyfin-compliant filenames

Workflow:
  1. jellywatch scan                    # Populate database
  2. jellywatch consolidate --generate  # Generate consolidation plans
  3. jellywatch consolidate --dry-run   # Review what will happen
  4. jellywatch consolidate --execute   # Execute the plans

Examples:
  jellywatch consolidate --generate     # Generate plans from database
  jellywatch consolidate --status       # Show plan summary
  jellywatch consolidate --dry-run      # Preview pending plans
  jellywatch consolidate --execute      # Execute pending plans
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConsolidate(generate, dryRun, execute, status)
		},
	}

	cmd.Flags().BoolVar(&generate, "generate", false, "Generate consolidation plans from database")
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "d", false, "Show what would be done without making changes")
	cmd.Flags().BoolVar(&execute, "execute", false, "Execute pending consolidation plans")
	cmd.Flags().BoolVar(&status, "status", false, "Show plan summary")

	return cmd
}

func runConsolidate(generate, dryRun, execute, status bool) error {
	// Open database
	db, err := database.Open()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Generate plans if requested
	if generate {
		return runGeneratePlans(ctx, db)
	}

	// Show status if requested
	if status {
		return runConsolidateStatus(db)
	}

	// Execute plans (dry-run or actual)
	if execute || dryRun {
		return runExecutePlans(ctx, db, dryRun)
	}

	// Default: Show summary and guide user
	return runConsolidateSummary(db)
}

func clearPendingPlans(db *database.MediaDB) error {
	query := `DELETE FROM consolidation_plans WHERE status = 'pending'`
	_, err := db.DB().Exec(query)
	return err
}

func runGeneratePlans(ctx context.Context, db *database.MediaDB) error {
	fmt.Println("🔍 Analyzing database for consolidation opportunities...")

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	consolidator := consolidate.NewConsolidator(db, cfg)

	err = clearPendingPlans(db)
	if err != nil {
		return fmt.Errorf("failed to clear pending plans: %w", err)
	}

	plans, err := consolidator.GenerateAllPlans()
	if err != nil {
		return fmt.Errorf("failed to generate plans: %w", err)
	}

	var planCount, failedPlanCount, moveCount, spaceToReclaim int64
	totalConflicts := int64(len(plans))

	for _, plan := range plans {
		err := consolidator.StorePlan(plan)
		if err != nil {
			fmt.Printf("Warning: Failed to store plan for %s: %v\n", plan.Title, err)
			failedPlanCount++
			continue
		}

		planCount++
		moveCount += int64(len(plan.Operations))
		spaceToReclaim += plan.TotalBytes
	}

	fmt.Println("\n✅ Plans generated successfully!")
	fmt.Printf("\nConsolidation Summary:\n")
	fmt.Printf("  Conflicts analyzed:        %d\n", totalConflicts)
	fmt.Printf("  Consolidation opportunities: %d\n", planCount)
	fmt.Printf("  Move operations:           %d\n", moveCount)
	fmt.Printf("  Space to reclaim:          %s\n", formatBytes(spaceToReclaim))
	if failedPlanCount > 0 {
		fmt.Printf("  Failed to store:          %d plans\n", failedPlanCount)
	}

	if planCount > 0 {
		fmt.Println("\nNext steps:")
		fmt.Println("  jellywatch consolidate --dry-run   # Preview what will happen")
		fmt.Println("  jellywatch consolidate --execute   # Execute the plans")
	} else {
		fmt.Println("\n✨ No consolidation needed - your library is already organized!")
	}

	return nil
}

func runConsolidateStatus(db *database.MediaDB) error {
	planner := consolidate.NewPlanner(db)
	summary, err := planner.GetPlanSummary()
	if err != nil {
		return fmt.Errorf("failed to get plan summary: %w", err)
	}

	fmt.Println("=== Consolidation Status ===")
	fmt.Printf("\nPending Plans:\n")
	fmt.Printf("  Total:    %d\n", summary.TotalPlans)
	fmt.Printf("  Delete:   %d\n", summary.DeletePlans)
	fmt.Printf("  Move:     %d\n", summary.MovePlans)
	fmt.Printf("  Rename:   %d\n", summary.RenamePlans)
	fmt.Printf("\nSpace to reclaim: %s\n", formatBytes(summary.SpaceToReclaim))

	if summary.TotalPlans == 0 {
		fmt.Println("\nNo pending plans. Run 'jellywatch consolidate --generate' to create new plans.")
	} else {
		fmt.Println("\nRun 'jellywatch consolidate --dry-run' to preview actions.")
		fmt.Println("Run 'jellywatch consolidate --execute' to execute plans.")
	}

	return nil
}

func runExecutePlans(ctx context.Context, db *database.MediaDB, dryRun bool) error {
	planner := consolidate.NewPlanner(db)
	executor := consolidate.NewExecutor(db, dryRun)

	// Get pending plans
	plans, err := planner.GetPendingPlans()
	if err != nil {
		return fmt.Errorf("failed to get pending plans: %w", err)
	}

	if len(plans) == 0 {
		fmt.Println("No pending plans to execute.")
		fmt.Println("Run 'jellywatch consolidate --generate' to create plans.")
		return nil
	}

	if dryRun {
		fmt.Println("🔍 DRY RUN - No changes will be made")
	} else {
		fmt.Println("⚠️  Executing consolidation plans...")
	}

	fmt.Printf("Found %d pending plans:\n\n", len(plans))

	deleteCount := 0
	moveCount := 0
	renameCount := 0

	for i, plan := range plans {
		switch plan.Action {
		case "delete":
			deleteCount++
			// Get file info for size
			file, _ := db.GetMediaFileByID(plan.SourceFileID)
			size := ""
			if file != nil {
				size = formatBytes(file.Size)
			}
			fmt.Printf("%d. DELETE: %s (%s)\n", i+1, plan.SourcePath, size)
			fmt.Printf("   Reason: %s\n", plan.Reason)
			if plan.ReasonDetails != "" {
				fmt.Printf("   %s\n", plan.ReasonDetails)
			}
		case "move":
			moveCount++
			fmt.Printf("%d. MOVE: %s\n", i+1, plan.SourcePath)
			fmt.Printf("   To: %s\n", plan.TargetPath)
			fmt.Printf("   Reason: %s\n", plan.Reason)
		case "rename":
			renameCount++
			fmt.Printf("%d. RENAME: %s\n", i+1, plan.SourcePath)
			fmt.Printf("   To: %s\n", plan.TargetPath)
			fmt.Printf("   Reason: %s\n", plan.Reason)
		}
		fmt.Println()
	}

	fmt.Printf("Summary: %d deletes, %d moves, %d renames\n\n", deleteCount, moveCount, renameCount)

	if dryRun {
		fmt.Println("✅ Dry run complete - no changes made")
		fmt.Println("\nTo execute these plans, run:")
		fmt.Println("  jellywatch consolidate --execute")
		return nil
	}

	// Execute plans
	fmt.Println("Executing plans...")
	result, err := executor.ExecutePlans(ctx)
	if err != nil {
		return fmt.Errorf("failed to execute plans: %w", err)
	}

	// Show results
	fmt.Println("\n=== Execution Complete ===")
	fmt.Printf("Plans executed:  %d\n", result.PlansExecuted)
	fmt.Printf("Succeeded:       %d\n", result.PlansSucceeded)
	fmt.Printf("Failed:          %d\n", result.PlansFailed)
	fmt.Printf("Files deleted:   %d\n", result.FilesDeleted)
	fmt.Printf("Files moved:     %d\n", result.FilesMoved)
	fmt.Printf("Files renamed:   %d\n", result.FilesRenamed)
	fmt.Printf("Space reclaimed: %s\n", formatBytes(result.SpaceReclaimed))
	fmt.Printf("Duration:        %s\n", result.Duration)

	if len(result.Errors) > 0 {
		fmt.Printf("\n⚠️  Errors encountered:\n")
		for _, err := range result.Errors {
			fmt.Printf("  - %v\n", err)
		}
	}

	return nil
}

func runConsolidateSummary(db *database.MediaDB) error {
	// Check for pending plans
	planner := consolidate.NewPlanner(db)
	summary, err := planner.GetPlanSummary()
	if err != nil {
		return fmt.Errorf("failed to get plan summary: %w", err)
	}

	fmt.Println("=== CONDOR Consolidation System ===")

	if summary.TotalPlans > 0 {
		fmt.Printf("You have %d pending consolidation plans:\n", summary.TotalPlans)
		fmt.Printf("  Delete:   %d files (%s to reclaim)\n", summary.DeletePlans, formatBytes(summary.SpaceToReclaim))
		fmt.Printf("  Move:     %d files\n", summary.MovePlans)
		fmt.Printf("  Rename:   %d files\n", summary.RenamePlans)
		fmt.Println("\nNext steps:")
		fmt.Println("  jellywatch consolidate --dry-run   # Preview actions")
		fmt.Println("  jellywatch consolidate --execute   # Execute plans")
	} else {
		fmt.Println("No pending consolidation plans found.")
		fmt.Println("\nTo analyze your library and generate plans:")
		fmt.Println("  1. jellywatch scan                    # Scan libraries into database")
		fmt.Println("  2. jellywatch consolidate --generate  # Generate consolidation plans")
		fmt.Println("  3. jellywatch consolidate --dry-run   # Preview plans")
		fmt.Println("  4. jellywatch consolidate --execute   # Execute plans")
	}

	return nil
}
