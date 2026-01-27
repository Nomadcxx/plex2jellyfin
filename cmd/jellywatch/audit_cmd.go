package main

import (
	"fmt"
	"os"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/plans"
)

type auditOptions struct {
	Generate  bool
	DryRun    bool
	Execute    bool
	Threshold  float64
	Limit      int
}

var auditOpts auditOptions

func init() {
	auditCmd.Flags().BoolVar(&auditOpts.Generate, "generate", false, "Generate AI suggestions for low-confidence files")
	auditCmd.Flags().BoolVar(&auditOpts.DryRun, "dry-run", false, "Preview changes without executing")
	auditCmd.Flags().BoolVar(&auditOpts.Execute, "execute", false, "Execute generated audit plan")
	auditCmd.Flags().Float64Var(&auditOpts.Threshold, "threshold", 0.8, "Confidence threshold (default: 0.8)")
	auditCmd.Flags().IntVar(&auditOpts.Limit, "limit", 100, "Maximum number of files to audit (default: 100)")
}

var auditCmd = &cobra.Command{
	Use:   "audit [path]",
	Short: "Audit low-confidence media files",
	Long:  "Generate and execute audit plans for files with low confidence scores",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAudit(cmd, args)
	},
}

func runAudit(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	db, err := database.Open()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Default threshold
	if auditOpts.Threshold == 0 {
		auditOpts.Threshold = 0.8
	}

	// Generate mode
	if auditOpts.Generate {
		return generateAudit(db, cfg, auditOpts.Threshold, auditOpts.Limit)
	}

	// Load and execute plan
	plan, err := plans.LoadAuditPlans()
	if err != nil {
		return fmt.Errorf("failed to load audit plans: %w", err)
	}

	if plan == nil {
		fmt.Println("No audit plans found. Run with --generate to create one.")
		return nil
	}

	// Dry run - just show the plan
	if auditOpts.DryRun {
		return displayAuditPlan(plan, true)
	}

	// Execute mode - apply the plan
	if auditOpts.Execute {
		return executeAuditPlan(db, plan)
	}

	// Default - show plan
	return displayAuditPlan(plan, false)
}

func generateAudit(db *database.MediaDB, cfg *config.Config, threshold float64, limit int) error {
	fmt.Printf("🔍 Scanning for files with confidence < %.2f\n", threshold)

	files, err := db.GetLowConfidenceFiles(threshold, limit)
	if err != nil {
		return fmt.Errorf("failed to query low-confidence files: %w", err)
	}

	if len(files) == 0 {
		fmt.Println("✓ No low-confidence files found")
		return nil
	}

	fmt.Printf("Found %d low-confidence files\n", len(files))

	// TODO: Generate AI suggestions (task 8)
	plan := &plans.AuditPlan{
		CreatedAt: time.Now(),
		Command:    "audit",
		Summary: plans.AuditSummary{
			TotalFiles:    len(files),
			FilesToRename: 0,
			FilesToDelete: 0,
			FilesToSkip:   len(files),
			AvgConfidence: calculateAvgConfidence(files),
		},
		Items: filesToAuditItems(files),
	}

	err = plans.SaveAuditPlans(plan)
	if err != nil {
		return fmt.Errorf("failed to save audit plans: %w", err)
	}

	fmt.Printf("✓ Generated audit plan with %d items\n", len(files))
	fmt.Printf("📁 Plan saved to: %s\n", getAuditPlansPath())

	return nil
}

func displayAuditPlan(plan *plans.AuditPlan, showActions bool) error {
	fmt.Printf("\n📋 Audit Plan\n")
	fmt.Printf("Created: %s\n", plan.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("\nSummary:\n")
	fmt.Printf("  Total Files: %d\n", plan.Summary.TotalFiles)
	fmt.Printf("  Files to Rename: %d\n", plan.Summary.FilesToRename)
	fmt.Printf("  Files to Delete: %d\n", plan.Summary.FilesToDelete)
	fmt.Printf("  Files to Skip: %d\n", plan.Summary.FilesToSkip)
	fmt.Printf("  Avg Confidence: %.2f\n", plan.Summary.AvgConfidence)

	if showActions && len(plan.Actions) > 0 {
		fmt.Printf("\nActions:\n")
		for i, action := range plan.Actions {
			fmt.Printf("  %d. %s: %s -> %s\n", i+1, action.Action, action.NewTitle, action.NewPath)
		}
	}

	return nil
}

func executeAuditPlan(db *database.MediaDB, plan *plans.AuditPlan) error {
	fmt.Printf("\n🚀 Executing Audit Plan\n")

	for _, action := range plan.Actions {
		fmt.Printf("%s: %s\n", action.Action, action.Reasoning)

		switch action.Action {
		case "rename":
			// TODO: Implement rename (task 9)
			fmt.Printf("  Skipping: %s -> %s\n", action.NewPath)
		case "delete":
			// TODO: Implement delete
			fmt.Printf("  Skipping: delete %s\n", action.NewPath)
		}
	}

	fmt.Println("✓ Audit plan executed")

	return nil
}

func filesToAuditItems(files []*database.MediaFile) []plans.AuditItem {
	items := make([]plans.AuditItem, len(files))
	for i, file := range files {
		items[i] = plans.AuditItem{
			ID:         file.ID,
			Path:       file.Path,
			Size:       file.Size,
			MediaType: file.MediaType,
			Title:      file.NormalizedTitle,
			Year:       file.Year,
			Season:     file.Season,
			Episode:    file.Episode,
			Confidence: file.Confidence,
			Resolution: file.Resolution,
			SourceType: file.SourceType,
		}
	}
	return items
}

func calculateAvgConfidence(files []*database.MediaFile) float64 {
	if len(files) == 0 {
		return 0
	}
	sum := 0.0
	for _, file := range files {
		sum += file.Confidence
	}
	return sum / float64(len(files))
}

func getAuditPlansPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%s/.config/jellywatch/plans/audit.json", homeDir)
}
