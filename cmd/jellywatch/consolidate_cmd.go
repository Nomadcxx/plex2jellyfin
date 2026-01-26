package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/consolidate"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/plans"
	"github.com/Nomadcxx/jellywatch/internal/service"
	"github.com/Nomadcxx/jellywatch/internal/transfer"
	"github.com/spf13/cobra"
)

func newConsolidateCmd() *cobra.Command {
	var (
		generate bool
		dryRun   bool
		execute  bool
	)

	cmd := &cobra.Command{
		Use:   "consolidate [flags]",
		Short: "Consolidate scattered media files",
		Long: `Find and consolidate media files scattered across multiple locations.

Workflow:
  1. jellywatch consolidate              # Show scattered media analysis
  2. jellywatch consolidate --generate   # Generate consolidation plans
  3. jellywatch consolidate --dry-run    # Preview pending plans
  4. jellywatch consolidate --execute    # Execute plans (move files)

Examples:
  jellywatch consolidate              # Show what needs consolidation
  jellywatch consolidate --generate   # Generate/refresh plans
  jellywatch consolidate --dry-run    # Preview pending plans
  jellywatch consolidate --execute    # Execute pending plans
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConsolidate(generate, dryRun, execute)
		},
	}

	cmd.Flags().BoolVar(&generate, "generate", false, "Generate consolidation plans")
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "d", false, "Preview pending plans")
	cmd.Flags().BoolVar(&execute, "execute", false, "Execute pending plans")

	return cmd
}

func runConsolidate(generate, dryRun, execute bool) error {
	db, err := database.Open()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Generate plans
	if generate {
		return runConsolidateGenerate(db)
	}

	// Preview plans
	if dryRun {
		return runConsolidateDryRun()
	}

	// Execute plans
	if execute {
		return runConsolidateExecute(db)
	}

	// Default: show analysis
	return runConsolidateAnalysis(db)
}

func runConsolidateAnalysis(db *database.MediaDB) error {
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
	fmt.Println("  jellywatch consolidate --generate   # Generate consolidation plans")
	fmt.Println("  jellywatch consolidate --dry-run    # Preview plans")
	fmt.Println("  jellywatch consolidate --execute    # Execute plans")

	return nil
}

func runConsolidateGenerate(db *database.MediaDB) error {
	fmt.Println("🔍 Analyzing database for consolidation opportunities...")

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	consolidator := consolidate.NewConsolidator(db, cfg)
	generatedPlans, err := consolidator.GenerateAllPlans()
	if err != nil {
		return fmt.Errorf("failed to generate plans: %w", err)
	}

	if len(generatedPlans) == 0 {
		fmt.Println("✨ No consolidation needed - your library is already organized!")
		return nil
	}

	plan := &plans.ConsolidatePlan{
		CreatedAt: time.Now(),
		Command:   "consolidate",
		Summary:   plans.ConsolidateSummary{},
		Plans:     []plans.ConsolidateGroup{},
	}

	for _, gp := range generatedPlans {
		group := plans.ConsolidateGroup{
			ConflictID:     gp.ConflictID,
			Title:          gp.Title,
			Year:           gp.Year,
			MediaType:      gp.MediaType,
			TargetLocation: gp.TargetPath,
			Operations:     []plans.MoveOperation{},
		}

		for _, op := range gp.Operations {
			group.Operations = append(group.Operations, plans.MoveOperation{
				Action:     "move",
				SourcePath: op.SourcePath,
				TargetPath: op.DestinationPath,
				Size:       op.Size,
			})
			plan.Summary.TotalMoves++
			plan.Summary.TotalBytes += op.Size
		}

		plan.Plans = append(plan.Plans, group)
		plan.Summary.TotalConflicts++
	}

	if err := plans.SaveConsolidatePlans(plan); err != nil {
		return fmt.Errorf("failed to save plans: %w", err)
	}

	fmt.Println("\n✅ Plans generated successfully!")
	fmt.Printf("\nConsolidation Summary:\n")
	fmt.Printf("  Conflicts found:    %d\n", plan.Summary.TotalConflicts)
	fmt.Printf("  Move operations:    %d\n", plan.Summary.TotalMoves)
	fmt.Printf("  Data to relocate:  %s\n", formatBytes(plan.Summary.TotalBytes))

	plansDir, _ := plans.GetPlansDir()
	fmt.Printf("\nPlan saved to: %s/consolidate.json\n", plansDir)

	fmt.Println("\nNext steps:")
	fmt.Println("  jellywatch consolidate --dry-run    # Preview what will happen")
	fmt.Println("  jellywatch consolidate --execute    # Execute the plans")

	return nil
}

func runConsolidateDryRun() error {
	plan, err := plans.LoadConsolidatePlans()
	if err != nil {
		return fmt.Errorf("failed to load plans: %w", err)
	}

	if plan == nil {
		fmt.Println("No pending plans found.")
		fmt.Println("Run 'jellywatch consolidate --generate' first to create plans.")
		return nil
	}

	fmt.Println("🔍 DRY RUN - No changes will be made")
	fmt.Printf("\nPlan created: %s\n", plan.CreatedAt.Format(time.RFC3339))
	fmt.Printf("Conflicts to resolve: %d\n", plan.Summary.TotalConflicts)
	fmt.Printf("Files to move: %d\n", plan.Summary.TotalMoves)
	fmt.Printf("Data to relocate: %s\n\n", formatBytes(plan.Summary.TotalBytes))

	for i, group := range plan.Plans {
		yearStr := ""
		if group.Year != nil {
			yearStr = fmt.Sprintf(" (%d)", *group.Year)
		}

		fmt.Printf("[%d] %s%s\n", i+1, group.Title, yearStr)
		fmt.Printf("    Target: %s\n", group.TargetLocation)

		for _, op := range group.Operations {
			fmt.Printf("    %s: %s\n", strings.ToUpper(op.Action), op.SourcePath)
			if op.Action == "move" {
				fmt.Printf("         → %s\n", op.TargetPath)
			}
		}
		fmt.Println()
	}

	fmt.Println("To execute these plans, run:")
	fmt.Println("  jellywatch consolidate --execute")

	return nil
}

func runConsolidateExecute(db *database.MediaDB) error {
	plan, err := plans.LoadConsolidatePlans()
	if err != nil {
		return fmt.Errorf("failed to load plans: %w", err)
	}

	if plan == nil {
		fmt.Println("No pending plans found.")
		fmt.Println("Run 'jellywatch consolidate --generate' first to create plans.")
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

	transferer, err := transfer.New(transfer.BackendRsync)
	if err != nil {
		return fmt.Errorf("failed to create transferer: %w", err)
	}

	movedCount := 0
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

			// Update database - get file, update path, upsert
			file, err := db.GetMediaFile(op.SourcePath)
			if err == nil && file != nil {
				// Delete old entry
				if err := db.DeleteMediaFile(op.SourcePath); err != nil {
					fmt.Printf("  ⚠️  Moved but failed to delete old database entry: %v\n", err)
				}
				// Update path and insert new entry
				file.Path = op.TargetPath
				if err := db.UpsertMediaFile(file); err != nil {
					fmt.Printf("  ⚠️  Moved but failed to update database: %v\n", err)
				}
			}

			movedCount++
			movedBytes += result.BytesCopied
			fmt.Printf("  ✅ Moved (%s)\n", formatBytes(result.BytesCopied))
		}
	}

	// Delete plans file on success
	if failedCount == 0 {
		if err := plans.DeleteConsolidatePlans(); err != nil {
			fmt.Printf("⚠️  Failed to clean up plans file: %v\n", err)
		}
	}

	fmt.Println("\n=== Execution Complete ===")
	fmt.Printf("✅ Successfully moved: %d files\n", movedCount)
	if failedCount > 0 {
		fmt.Printf("❌ Failed to move:     %d files\n", failedCount)
	}
	fmt.Printf("📦 Data relocated:     %s\n", formatBytes(movedBytes))

	return nil
}
