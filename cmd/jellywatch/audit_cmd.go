package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/Nomadcxx/jellywatch/internal/ai"
	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/plans"
)

type auditOptions struct {
	Generate  bool
	DryRun    bool
	Execute   bool
	Threshold float64
	Limit     int
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
	if err := checkDatabasePopulated(); err != nil {
		return err
	}

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
	if plan == nil {
		fmt.Println("No audit plans found. Run with --generate to create one.")
		return nil
	}

	// Dry run - just show to plan
	if auditOpts.DryRun {
		return displayAuditPlan(plan, true)
	}

	// Execute mode - apply to plan
	if auditOpts.Execute {
		return executeAuditPlan(db, plan)
	}

	// Default - show plan
	return displayAuditPlan(plan, false)
}

// dbProvider implements ai.DatabaseProvider interface
type dbProvider struct {
	mediaDB *database.MediaDB
}

func (p *dbProvider) DB() *sql.DB {
	return p.mediaDB.DB()
}

// buildCorrectPath constructs a new file path based on AI-suggested metadata
func buildCorrectPath(currentPath, newTitle string, newYear, newSeason, newEpisode *int) string {
	dir := filepath.Dir(currentPath)
	ext := filepath.Ext(currentPath)

	var filename string
	if newSeason != nil && newEpisode != nil {
		// TV show episode
		if newYear != nil {
			filename = fmt.Sprintf("%s (%d) - S%02dE%02d%s", newTitle, *newYear, *newSeason, *newEpisode, ext)
		} else {
			filename = fmt.Sprintf("%s - S%02dE%02d%s", newTitle, *newSeason, *newEpisode, ext)
		}
	} else {
		// Movie
		if newYear != nil {
			filename = fmt.Sprintf("%s (%d)%s", newTitle, *newYear, ext)
		} else {
			filename = fmt.Sprintf("%s%s", newTitle, ext)
		}
	}

	return filepath.Join(dir, filename)
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

	matcher, err := ai.NewMatcher(cfg.AI)
	if err != nil {
		return fmt.Errorf("failed to initialize AI matcher: %w", err)
	}

	actions := make([]plans.AuditAction, 0, len(files))
	items := make([]plans.AuditItem, len(files))
	stats := NewAuditStats()
	progress := NewProgressBar(len(files))

	fmt.Printf("\nProcessing with AI...\n")

	for i, file := range files {
		items[i] = plans.AuditItem{
			ID:         file.ID,
			Path:       file.Path,
			Size:       file.Size,
			MediaType:  file.MediaType,
			Title:      file.NormalizedTitle,
			Year:       file.Year,
			Season:     file.Season,
			Episode:    file.Episode,
			Confidence: file.Confidence,
			Resolution: file.Resolution,
			SourceType: file.SourceType,
		}

		ctx := context.Background()

		libraryType := "unknown"
		if file.MediaType == "movie" {
			libraryType = "movie library"
		} else if file.MediaType == "episode" {
			libraryType = "TV show library"
		}

		folderPath := filepath.Dir(file.Path)

		aiResult, err := matcher.ParseWithContext(
			ctx,
			filepath.Base(file.Path),
			libraryType,
			folderPath,
			file.NormalizedTitle,
			file.Confidence,
		)

		if err != nil {
			stats.RecordAICall(false)
			items[i].SkipReason = fmt.Sprintf("AI error: %v", err)
			progress.Update(len(actions), stats.AIErrorCount)
			continue
		}

		stats.RecordAICall(true)

		if aiResult.Confidence < cfg.AI.ConfidenceThreshold {
			stats.RecordSkip("confidence too low")
			items[i].SkipReason = fmt.Sprintf("AI confidence %.2f below threshold %.2f",
				aiResult.Confidence, cfg.AI.ConfidenceThreshold)
			progress.Update(len(actions), stats.AIErrorCount)
			continue
		}

		if aiResult.Title == file.NormalizedTitle {
			stats.RecordSkip("title unchanged")
			items[i].SkipReason = "Title unchanged after AI analysis"
			progress.Update(len(actions), stats.AIErrorCount)
			continue
		}

		valid, reason := validateMediaType(file, aiResult, cfg)
		if !valid {
			stats.RecordSkip(reason)
			items[i].SkipReason = reason
			progress.Update(len(actions), stats.AIErrorCount)
			continue
		}

		var newSeason, newEpisode *int
		if aiResult.Season != nil {
			newSeason = aiResult.Season
		} else {
			newSeason = file.Season
		}

		if len(aiResult.Episodes) > 0 {
			newEpisode = &aiResult.Episodes[0]
		} else {
			newEpisode = file.Episode
		}

		action := plans.AuditAction{
			Action:     "rename",
			NewTitle:   aiResult.Title,
			NewYear:    aiResult.Year,
			NewSeason:  newSeason,
			NewEpisode: newEpisode,
			NewPath:    buildCorrectPath(file.Path, aiResult.Title, aiResult.Year, newSeason, newEpisode),
			Reasoning:  fmt.Sprintf("AI suggested: %s (confidence: %.2f)", aiResult.Title, aiResult.Confidence),
			Confidence: aiResult.Confidence,
		}

		actions = append(actions, action)
		stats.RecordAction()
		progress.Update(len(actions), stats.AIErrorCount)
	}

	progress.Finish()

	plan := &plans.AuditPlan{
		CreatedAt: time.Now(),
		Command:   "audit",
		Summary: plans.AuditSummary{
			TotalFiles:            len(files),
			FilesToRename:         len(actions),
			FilesToDelete:         0,
			FilesToSkip:           len(files) - len(actions),
			AvgConfidence:         calculateAvgConfidence(files),
			AITotalCalls:          stats.AITotalCalls,
			AISuccessCount:        stats.AISuccessCount,
			AIErrorCount:          stats.AIErrorCount,
			TypeMismatchesSkipped: stats.TypeMismatches,
			ConfidenceTooLow:      stats.ConfidenceTooLow,
			TitleUnchanged:        stats.TitleUnchanged,
		},
		Items:   items,
		Actions: actions,
	}

	err = plans.SaveAuditPlans(plan)
	if err != nil {
		return fmt.Errorf("failed to save audit plans: %w", err)
	}

	printAuditSummary(plan)

	return nil
}

// printAuditSummary displays the audit generation results
func printAuditSummary(plan *plans.AuditPlan) {
	fmt.Printf("\n✓ Processing complete\n\n")
	fmt.Printf("Summary:\n")
	fmt.Printf("  Total files analyzed: %d\n", plan.Summary.TotalFiles)
	fmt.Printf("  AI calls: %d (%d successful, %d errors)\n",
		plan.Summary.AITotalCalls, plan.Summary.AISuccessCount, plan.Summary.AIErrorCount)
	fmt.Printf("  Actions created: %d (renames)\n", plan.Summary.FilesToRename)
	fmt.Printf("  Skipped: %d\n", plan.Summary.FilesToSkip)

	if plan.Summary.FilesToSkip > 0 {
		if plan.Summary.ConfidenceTooLow > 0 {
			fmt.Printf("    - AI confidence too low: %d\n", plan.Summary.ConfidenceTooLow)
		}
		if plan.Summary.TypeMismatchesSkipped > 0 {
			fmt.Printf("    - Type validation failed: %d\n", plan.Summary.TypeMismatchesSkipped)
		}
		if plan.Summary.TitleUnchanged > 0 {
			fmt.Printf("    - Title unchanged: %d\n", plan.Summary.TitleUnchanged)
		}
		otherSkips := plan.Summary.FilesToSkip - plan.Summary.ConfidenceTooLow -
			plan.Summary.TypeMismatchesSkipped - plan.Summary.TitleUnchanged
		if otherSkips > 0 {
			fmt.Printf("    - Other (AI errors, etc.): %d\n", otherSkips)
		}
	}

	fmt.Printf("\n📁 Plan saved to: %s\n", getAuditPlansPath())
	if plan.Summary.FilesToRename > 0 {
		fmt.Printf("💡 Run 'jellywatch audit --dry-run' to preview changes\n")
	}
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

	// Filter actions with confidence >= 0.8
	var filteredIndices []int
	for i, action := range plan.Actions {
		if action.Confidence >= 0.8 {
			filteredIndices = append(filteredIndices, i)
		}
	}

	if len(filteredIndices) == 0 {
		fmt.Println("No actions meet the confidence threshold (>= 0.8)")
		return nil
	}

	fmt.Printf("Found %d actions to execute (confidence >= 0.8)\n\n", len(filteredIndices))

	// Show actions and prompt for confirmation
	for _, idx := range filteredIndices {
		action := plan.Actions[idx]
		item := plan.Items[idx]
		fmt.Printf("[%s] %s\n", action.Action, filepath.Base(item.Path))
		fmt.Printf("  -> %s (confidence: %.2f)\n", action.NewPath, action.Confidence)
		fmt.Printf("  Reason: %s\n\n", action.Reasoning)
	}

	fmt.Printf("Execute %d actions? [y/N/all]: ", len(filteredIndices))
	var response string
	fmt.Scanln(&response)

	confirmedAll := false
	switch response {
	case "y", "Y":
		// Continue with execution
	case "all", "All", "ALL":
		confirmedAll = true
	default:
		fmt.Println("Cancelled.")
		return nil
	}

	// Track results
	succeeded := 0
	failed := 0

	// Execute actions
	for _, idx := range filteredIndices {
		action := plan.Actions[idx]
		item := plan.Items[idx]

		if !confirmedAll {
			fmt.Printf("\nExecute: %s %s -> %s? [y/N]: ", action.Action, filepath.Base(item.Path), filepath.Base(action.NewPath))
			var confirm string
			fmt.Scanln(&confirm)
			if confirm != "y" && confirm != "Y" {
				fmt.Printf("  Skipped\n")
				continue
			}
		}

		if err := plans.ExecuteAuditAction(db, item, action, auditOpts.DryRun); err != nil {
			fmt.Printf("  ✗ Failed: %v\n", err)
			failed++
		} else {
			fmt.Printf("  ✓ Success\n")
			succeeded++
		}
	}

	// Print summary
	fmt.Printf("\n📊 Summary:\n")
	fmt.Printf("  Succeeded: %d\n", succeeded)
	fmt.Printf("  Failed: %d\n", failed)

	// Delete plan file on success (all actions completed successfully)
	if failed == 0 && succeeded > 0 {
		if err := plans.DeleteAuditPlans(); err != nil {
			fmt.Printf("  ⚠️  Warning: failed to delete plan file: %v\n", err)
		} else {
			fmt.Printf("  ✓ Plan file deleted\n")
		}
	}

	return nil
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

// newAuditCmd returns the audit command
func newAuditCmd() *cobra.Command {
	return auditCmd
}
