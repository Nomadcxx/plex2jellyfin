package main

import (
	"context"
	"fmt"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/consolidate"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/service"
	"github.com/spf13/cobra"
)

func newConsolidateCmd() *cobra.Command {
	var (
		dryRun  bool
		execute bool
		status  bool
	)

	cmd := &cobra.Command{
		Use:   "consolidate [flags]",
		Short: "Consolidate scattered media files",
		Long: `Find and consolidate media files scattered across multiple locations.

This command identifies media with the same title in different folders
and offers to move them to a single location.

Examples:
  jellywatch consolidate              # Show what needs consolidation
  jellywatch consolidate --dry-run    # Preview the consolidation plan
  jellywatch consolidate --execute    # Execute consolidation
  jellywatch consolidate --status     # Show pending plan status
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConsolidate(dryRun, execute, status)
		},
	}

	cmd.Flags().BoolVarP(&dryRun, "dry-run", "d", false, "Show what would be done without making changes")
	cmd.Flags().BoolVar(&execute, "execute", false, "Execute consolidation")
	cmd.Flags().BoolVar(&status, "status", false, "Show plan summary")

	return cmd
}

func runConsolidate(dryRun, execute, status bool) error {
	db, err := database.Open()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	ctx := context.Background()

	if status {
		return runConsolidateStatus(db)
	}

	if execute || dryRun {
		return runExecutePlans(ctx, db, dryRun)
	}

	svc := service.NewCleanupService(db)
	analysis, err := svc.AnalyzeScattered()
	if err != nil {
		return fmt.Errorf("failed to analyze: %w", err)
	}

	if analysis.TotalItems == 0 {
		fmt.Println("✨ No scattered media found - your library is organized!")
		return nil
	}

	fmt.Printf("Found %d items scattered across multiple locations:\n\n", analysis.TotalItems)

	for _, item := range analysis.Items {
		yearStr := ""
		if item.Year != nil {
			yearStr = fmt.Sprintf(" (%d)", *item.Year)
		}
		fmt.Printf("[%s] %s%s\n", item.MediaType, item.Title, yearStr)
		for _, loc := range item.Locations {
			fmt.Printf("  - %s\n", loc)
		}
	}

	fmt.Println("\nNext steps:")
	fmt.Println("  jellywatch consolidate --dry-run   # Preview what will happen")
	fmt.Println("  jellywatch consolidate --execute   # Execute consolidation")

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
		fmt.Println("\nNo pending plans. Run 'jellywatch consolidate --dry-run' to generate new plans.")
	} else {
		fmt.Println("\nRun 'jellywatch consolidate --dry-run' to preview actions.")
		fmt.Println("Run 'jellywatch consolidate --execute' to execute plans.")
	}

	return nil
}

func runExecutePlans(ctx context.Context, db *database.MediaDB, dryRun bool) error {
	planner := consolidate.NewPlanner(db)
	executor := consolidate.NewExecutor(db, dryRun)

	// Auto-generate plans on the fly
	fmt.Println("🔍 Analyzing database for consolidation opportunities...")

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Clear old pending plans and regenerate
	if _, err := db.DB().Exec(`DELETE FROM consolidation_plans WHERE status = 'pending'`); err != nil {
		return fmt.Errorf("failed to clear old plans: %w", err)
	}

	consolidator := consolidate.NewConsolidator(db, cfg)
	generatedPlans, err := consolidator.GenerateAllPlans()
	if err != nil {
		return fmt.Errorf("failed to generate plans: %w", err)
	}

	// Store plans
	for _, plan := range generatedPlans {
		if err := consolidator.StorePlan(plan); err != nil {
			fmt.Printf("Warning: Failed to store plan for %s: %v\n", plan.Title, err)
		}
	}

	plans, err := planner.GetPendingPlans()
	if err != nil {
		return fmt.Errorf("failed to get pending plans: %w", err)
	}

	if len(plans) == 0 {
		fmt.Println("✨ No consolidation needed - your library is already organized!")
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
			var file *database.MediaFile
			if plan.SourceFileID.Valid {
				file, _ = db.GetMediaFileByID(plan.SourceFileID.Int64)
			}
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

	fmt.Println("Executing plans...")
	result, err := executor.ExecutePlans(ctx)
	if err != nil {
		return fmt.Errorf("failed to execute plans: %w", err)
	}

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
