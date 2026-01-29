package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/consolidate"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/plans"
	"github.com/Nomadcxx/jellywatch/internal/service"
)

func runConsolidateGenerate() error {
	if err := checkDatabasePopulated(); err != nil {
		return err
	}

	db, err := database.Open()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Println("🔍 Analyzing library for scattered content...")

	svc := service.NewCleanupService(db)
	analysis, err := svc.AnalyzeScattered()
	if err != nil {
		return fmt.Errorf("failed to analyze scattered: %w", err)
	}

	if analysis.TotalItems == 0 {
		fmt.Println("✅ No scattered content found!")
		return nil
	}

	fmt.Printf("\nFound %d series conflicts across multiple locations\n", analysis.TotalItems)

	consolidator := consolidate.NewConsolidator(db, cfg)
	consolidatePlans, err := consolidator.GenerateAllPlans()
	if err != nil {
		return fmt.Errorf("failed to generate plans: %w", err)
	}

	plan := &plans.ConsolidatePlan{
		Plans: make([]plans.ConsolidateGroup, 0, len(consolidatePlans)),
		Summary: plans.ConsolidateSummary{
			TotalConflicts: len(consolidatePlans),
			TotalMoves:     0,
			TotalBytes:     0,
		},
	}

	type skippedItem struct {
		title   string
		year    *int
		reasons []string
	}
	var skippedItems []skippedItem

	for _, cp := range consolidatePlans {
		if !cp.CanProceed {
			skippedItems = append(skippedItems, skippedItem{
				title:   cp.Title,
				year:    cp.Year,
				reasons: cp.Reasons,
			})
			continue
		}

		operations := make([]plans.MoveOperation, 0, len(cp.Operations))
		for _, op := range cp.Operations {
			if _, err := os.Stat(op.SourcePath); os.IsNotExist(err) {
				continue
			}

			operations = append(operations, plans.MoveOperation{
				Action:     "move",
				SourcePath: op.SourcePath,
				TargetPath: op.DestinationPath,
				Size:       op.Size,
			})
			plan.Summary.TotalMoves++
			plan.Summary.TotalBytes += op.Size
		}

		if len(operations) == 0 {
			continue
		}

		group := plans.ConsolidateGroup{
			ConflictID:     cp.ConflictID,
			Title:          cp.Title,
			Year:           cp.Year,
			MediaType:      cp.MediaType,
			TargetLocation: cp.TargetPath,
			Operations:     operations,
		}

		plan.Plans = append(plan.Plans, group)
	}

	if len(plan.Plans) == 0 {
		fmt.Println("✅ No consolidation needed (all files already in place)")
		if len(skippedItems) > 0 {
			fmt.Printf("\n⚠️  %d conflicts were skipped:\n", len(skippedItems))
			for _, item := range skippedItems {
				yearStr := ""
				if item.year != nil {
					yearStr = fmt.Sprintf(" (%d)", *item.year)
				}
				fmt.Printf("  • %s%s\n", item.title, yearStr)
				for _, reason := range item.reasons {
					fmt.Printf("    - %s\n", reason)
				}
			}
			fmt.Println("\nThis may indicate permission issues or inaccessible paths.")
		}
		return nil
	}

	if err := plans.SaveConsolidatePlans(plan); err != nil {
		return fmt.Errorf("failed to save plan: %w", err)
	}

	fmt.Println("✅ Consolidation plan generated")
	fmt.Printf("   Conflicts to consolidate: %d\n", len(plan.Plans))
	fmt.Printf("   Files to move: %d\n", plan.Summary.TotalMoves)
	fmt.Printf("   Data to relocate: %s\n", formatBytes(plan.Summary.TotalBytes))

	if len(skippedItems) > 0 {
		fmt.Printf("\n⚠️  %d conflicts were skipped (could not proceed):\n", len(skippedItems))
		for _, item := range skippedItems {
			yearStr := ""
			if item.year != nil {
				yearStr = fmt.Sprintf(" (%d)", *item.year)
			}
			fmt.Printf("  • %s%s\n", item.title, yearStr)
			for _, reason := range item.reasons {
				fmt.Printf("    - %s\n", reason)
			}
		}
		fmt.Println("\n  Check permissions and path accessibility for the above items.")
	}

	fmt.Println("\nNext steps:")
	fmt.Println("  jellywatch consolidate dry-run   # Preview moves")
	fmt.Println("  jellywatch consolidate execute   # Execute moves")

	return nil
}

func runConsolidateDryRun() error {
	plan, err := plans.LoadConsolidatePlans()
	if err != nil {
		return fmt.Errorf("failed to load plans: %w", err)
	}

	if plan == nil {
		fmt.Println("No pending plans found.")
		fmt.Println("Run 'jellywatch consolidate generate' first to create plans.")
		return nil
	}

	fmt.Println("📋 Consolidation Plan (DRY RUN)")
	fmt.Println()
	fmt.Printf("Conflicts to consolidate: %d\n", len(plan.Plans))
	fmt.Printf("Files to move: %d\n", plan.Summary.TotalMoves)
	fmt.Printf("Data to relocate: %s\n\n", formatBytes(plan.Summary.TotalBytes))

	for i, group := range plan.Plans {
		yearStr := ""
		if group.Year != nil {
			yearStr = fmt.Sprintf(" (%d)", *group.Year)
		}

		fmt.Printf("[%d] %s%s\n", i+1, group.Title, yearStr)
		fmt.Printf("  Target: %s\n", group.TargetLocation)
		fmt.Printf("  Operations: %d files\n\n", len(group.Operations))

		showCount := len(group.Operations)
		if showCount > 3 {
			showCount = 3
		}

		for j := 0; j < showCount; j++ {
			op := group.Operations[j]
			sourceDir := filepath.Dir(op.SourcePath)
			sourceFile := filepath.Base(op.SourcePath)
			targetDir := filepath.Dir(op.TargetPath)

			fmt.Printf("    %s\n", sourceFile)
			fmt.Printf("      %s\n", sourceDir)
			fmt.Printf("      → %s\n", targetDir)
			fmt.Printf("      Size: %s\n", formatBytes(op.Size))
		}

		if len(group.Operations) > 3 {
			fmt.Printf("    ... and %d more files\n", len(group.Operations)-3)
		}
		fmt.Println()
	}

	fmt.Println("To execute: jellywatch consolidate execute")
	return nil
}
