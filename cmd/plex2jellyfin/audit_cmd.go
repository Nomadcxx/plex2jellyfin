package main

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Nomadcxx/plex2jellyfin/internal/ai"
	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/naming"
	"github.com/Nomadcxx/plex2jellyfin/internal/paths"
	"github.com/Nomadcxx/plex2jellyfin/internal/plans"
	"github.com/Nomadcxx/plex2jellyfin/internal/privilege"
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
	auditCmd.Flags().IntVar(&auditOpts.Limit, "limit", 0, "Maximum number of files to audit (0 = uncapped)")
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

	if err := checkDatabasePopulated(); err != nil {
		return err
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
		scopePath, err := auditScopeFromArgs(args)
		if err != nil {
			return err
		}
		return generateAudit(db, cfg, auditOpts.Threshold, auditOpts.Limit, scopePath)
	}

	// Load and execute plan
	plan, err := plans.LoadAuditPlans()
	if plan == nil {
		fmt.Println("No audit plans found. Run with --generate to create one.")
		return nil
	}

	// Dry run - just show to plan
	if auditOpts.DryRun {
		if warning := auditPlanWarning(plan, cfg); warning != "" {
			fmt.Println(warning)
		}
		return displayAuditPlan(plan, true)
	}

	// Execute mode - apply to plan
	if auditOpts.Execute {
		return executeAuditPlan(db, plan, cfg)
	}

	// Default - show plan
	if warning := auditPlanWarning(plan, cfg); warning != "" {
		fmt.Println(warning)
	}
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

func auditScopeFromArgs(args []string) (string, error) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return "", nil
	}
	scope, err := filepath.Abs(args[0])
	if err != nil {
		return "", fmt.Errorf("resolving audit path: %w", err)
	}
	return filepath.Clean(scope), nil
}

func generateAudit(db *database.MediaDB, cfg *config.Config, threshold float64, limit int, scopePath string) error {
	if scopePath == "" {
		fmt.Printf("🔍 Scanning for files with confidence < %.2f\n", threshold)
	} else {
		fmt.Printf("🔍 Scanning %s for files with confidence < %.2f\n", scopePath, threshold)
	}

	var files []*database.MediaFile
	var err error
	if scopePath == "" {
		files, err = db.GetLowConfidenceFiles(threshold, limit)
	} else {
		files, err = db.GetLowConfidenceFilesUnderPath(threshold, limit, scopePath)
	}
	if err != nil {
		return fmt.Errorf("failed to query low-confidence files: %w", err)
	}

	if len(files) == 0 {
		fmt.Println("✓ No low-confidence files found")
		return nil
	}

	fmt.Printf("Found %d low-confidence files\n", len(files))

	actions := make([]plans.AuditAction, 0, len(files))
	items := make([]plans.AuditItem, len(files))
	stats := NewAuditStats()
	var matcher *ai.Matcher

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
	}

	candidateIndexes := make([]int, 0, len(files))
	for i, file := range files {
		if reason := auditPreAISkipReason(file); reason != "" {
			stats.RecordSkip(reason)
			items[i].SkipReason = reason
			continue
		}
		candidateIndexes = append(candidateIndexes, i)
	}

	skipBreakdown := auditSkipBreakdown(items)
	fmt.Printf("Pre-classified: %d AI candidates, %d deterministic skips, %d manual-review skips\n",
		len(candidateIndexes), skipBreakdown.Deterministic, skipBreakdown.ManualReview)

	if len(candidateIndexes) > 0 {
		fmt.Printf("\nProcessing %d AI candidates...\n", len(candidateIndexes))
	}

	progress := NewProgressBarWithUnit(len(candidateIndexes), "AI candidates")

	for _, fileIndex := range candidateIndexes {
		file := files[fileIndex]
		ctx := context.Background()

		if matcher == nil {
			var err error
			matcher, err = ai.NewMatcher(cfg.AI)
			if err != nil {
				return fmt.Errorf("failed to initialize AI matcher: %w", err)
			}
		}

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
			items[fileIndex].SkipReason = fmt.Sprintf("AI error: %v", err)
			progress.Update(len(actions), stats.AIErrorCount)
			continue
		}

		stats.RecordAICall(true)

		if aiResult.Confidence < cfg.AI.ConfidenceThreshold {
			stats.RecordSkip("confidence too low")
			items[fileIndex].SkipReason = fmt.Sprintf("AI confidence %.2f below threshold %.2f",
				aiResult.Confidence, cfg.AI.ConfidenceThreshold)
			progress.Update(len(actions), stats.AIErrorCount)
			continue
		}

		if aiResult.Title == file.NormalizedTitle {
			stats.RecordSkip("title unchanged")
			items[fileIndex].SkipReason = "Title unchanged after AI analysis"
			progress.Update(len(actions), stats.AIErrorCount)
			continue
		}

		valid, reason := validateMediaType(file, aiResult, cfg)
		if !valid {
			stats.RecordSkip(reason)
			items[fileIndex].SkipReason = reason
			progress.Update(len(actions), stats.AIErrorCount)
			continue
		}

		var newSeason, newEpisode *int
		if aiResult.Season != nil {
			newSeason = aiResult.Season.Int()
		} else {
			newSeason = file.Season
		}

		if len(aiResult.Episodes) > 0 {
			newEpisode = &aiResult.Episodes[0]
		} else {
			newEpisode = file.Episode
		}

		// Check if file is already Jellyfin-compliant before suggesting changes
		currentFilename := filepath.Base(file.Path)
		if naming.IsJellyfinCompliantFilename(currentFilename, file.MediaType) {
			// File is already compliant - check if AI suggestion is meaningfully different
			suggestedPath := buildCorrectPath(file.Path, aiResult.Title, aiResult.Year.Int(), newSeason, newEpisode)
			suggestedFilename := filepath.Base(suggestedPath)

			// Normalize both for comparison (ignore trivial differences like hyphen vs space)
			if normalizeForComparison(currentFilename) == normalizeForComparison(suggestedFilename) {
				stats.RecordSkip("already compliant")
				items[fileIndex].SkipReason = "File already follows Jellyfin naming conventions"
				progress.Update(len(actions), stats.AIErrorCount)
				continue
			}
		}

		action := plans.AuditAction{
			Action:     "rename",
			ItemIndex:  fileIndex,
			NewTitle:   aiResult.Title,
			NewYear:    aiResult.Year.Int(),
			NewSeason:  newSeason,
			NewEpisode: newEpisode,
			NewPath:    buildCorrectPath(file.Path, aiResult.Title, aiResult.Year.Int(), newSeason, newEpisode),
			Reasoning:  fmt.Sprintf("AI suggested: %s (confidence: %.2f)", aiResult.Title, aiResult.Confidence),
			Confidence: aiResult.Confidence,
		}

		actions = append(actions, action)
		stats.RecordAction()
		progress.Update(len(actions), stats.AIErrorCount)
	}

	if len(candidateIndexes) > 0 {
		progress.Finish()
	}

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
			AICandidateCount:      len(candidateIndexes),
			AISuccessCount:        stats.AISuccessCount,
			AIErrorCount:          stats.AIErrorCount,
			DeterministicSkipped:  skipBreakdown.Deterministic,
			ManualReviewSkipped:   skipBreakdown.ManualReview,
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

func auditPreAISkipReason(file *database.MediaFile) string {
	if file == nil {
		return "Missing media file"
	}

	filename := filepath.Base(file.Path)
	if isAuditExtraContentPath(file.Path) {
		return "Extra/sample content requires cleanup/manual review; AI skipped"
	}

	if naming.IsObfuscatedFilename(filename) {
		return "Obfuscated filename requires folder/manual review; AI skipped"
	}

	if file.ParseMethod == "folder" && file.MediaType == "episode" && file.Episode == nil {
		return "Folder-derived episode is missing episode number; manual review required"
	}

	if auditHasDeterministicIdentity(file) {
		return "Deterministic parse already has media identity; AI skipped"
	}

	return ""
}

func auditHasDeterministicIdentity(file *database.MediaFile) bool {
	if strings.TrimSpace(file.NormalizedTitle) == "" {
		return false
	}
	switch file.MediaType {
	case "episode":
		return file.Season != nil && *file.Season > 0 &&
			file.Episode != nil && *file.Episode > 0 &&
			naming.IsTVEpisodeFromPath(file.Path, naming.SourceUnknown)
	case "movie":
		return file.Year != nil && *file.Year > 0 &&
			!naming.IsObfuscatedFilename(filepath.Base(file.Path))
	default:
		return false
	}
}

func isAuditExtraContentPath(path string) bool {
	components := strings.FieldsFunc(strings.ToLower(filepath.Clean(path)), func(r rune) bool {
		return r == filepath.Separator
	})
	for _, component := range components {
		switch strings.Trim(component, " ._-") {
		case "sample", "samples", "trailer", "trailers", "extras", "extra", "featurette", "featurettes":
			return true
		}
	}

	base := strings.ToLower(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	base = strings.Trim(base, " ._-")
	if base == "sample" || base == "trailer" {
		return true
	}
	return strings.HasPrefix(base, "sample.") ||
		strings.HasPrefix(base, "sample-") ||
		strings.HasPrefix(base, "sample_") ||
		strings.Contains(base, ".sample") ||
		strings.Contains(base, "-sample") ||
		strings.Contains(base, "_sample")
}

// printAuditSummary displays the audit generation results
func printAuditSummary(plan *plans.AuditPlan) {
	fmt.Printf("\n✓ Processing complete\n\n")
	fmt.Printf("Summary:\n")
	fmt.Printf("  Total files analyzed: %d\n", plan.Summary.TotalFiles)
	fmt.Printf("  AI candidates: %d\n", plan.Summary.AICandidateCount)
	fmt.Printf("  AI calls: %d (%d successful, %d errors)\n",
		plan.Summary.AITotalCalls, plan.Summary.AISuccessCount, plan.Summary.AIErrorCount)
	fmt.Printf("  Actions created: %d (renames)\n", plan.Summary.FilesToRename)
	fmt.Printf("  Skipped: %d\n", plan.Summary.FilesToSkip)

	if plan.Summary.FilesToSkip > 0 {
		skipBreakdown := auditPlanSkipBreakdown(plan)
		if plan.Summary.ConfidenceTooLow > 0 {
			fmt.Printf("    - AI confidence too low: %d\n", plan.Summary.ConfidenceTooLow)
		}
		if plan.Summary.TypeMismatchesSkipped > 0 {
			fmt.Printf("    - Type validation failed: %d\n", plan.Summary.TypeMismatchesSkipped)
		}
		if plan.Summary.TitleUnchanged > 0 {
			fmt.Printf("    - Title unchanged: %d\n", plan.Summary.TitleUnchanged)
		}
		if skipBreakdown.Deterministic > 0 {
			fmt.Printf("    - Deterministic parses skipped before AI: %d\n", skipBreakdown.Deterministic)
		}
		if skipBreakdown.ManualReview > 0 {
			fmt.Printf("    - Manual-review items skipped before AI: %d\n", skipBreakdown.ManualReview)
		}
		otherSkips := plan.Summary.FilesToSkip - plan.Summary.ConfidenceTooLow -
			plan.Summary.TypeMismatchesSkipped - plan.Summary.TitleUnchanged -
			skipBreakdown.Deterministic - skipBreakdown.ManualReview
		if otherSkips > 0 {
			fmt.Printf("    - Other (AI errors, etc.): %d\n", otherSkips)
		}
	}

	fmt.Printf("\n📁 Plan saved to: %s\n", getAuditPlansPath())
	if plan.Summary.FilesToRename > 0 {
		printSmallAuditActionList(plan)
		fmt.Printf("💡 Next: run 'plex2jellyfin audit --dry-run' to preview, then 'plex2jellyfin audit --execute' to apply these renames\n")
	} else {
		fmt.Printf("💡 Next: run 'plex2jellyfin audit --dry-run' to inspect skipped/manual-review items\n")
	}
}

func printSmallAuditActionList(plan *plans.AuditPlan) {
	if plan == nil || len(plan.Actions) == 0 || len(plan.Actions) > 30 {
		return
	}

	fmt.Printf("\nPlanned actions:\n")
	for _, action := range plan.Actions {
		if action.ItemIndex < 0 || action.ItemIndex >= len(plan.Items) {
			continue
		}
		item := plan.Items[action.ItemIndex]
		fmt.Printf("  - rename: %s -> %s\n", filepath.Base(item.Path), filepath.Base(action.NewPath))
	}
}

type auditSkipCounts struct {
	Deterministic int
	ManualReview  int
	Other         int
}

func auditSkipBreakdown(items []plans.AuditItem) auditSkipCounts {
	var counts auditSkipCounts
	for _, item := range items {
		reason := strings.ToLower(item.SkipReason)
		switch {
		case reason == "":
			continue
		case strings.Contains(reason, "deterministic"):
			counts.Deterministic++
		case strings.Contains(reason, "manual review") || strings.Contains(reason, "manual-review") || strings.Contains(reason, "obfuscated"):
			counts.ManualReview++
		default:
			counts.Other++
		}
	}
	return counts
}

func auditPlanSkipBreakdown(plan *plans.AuditPlan) auditSkipCounts {
	if plan == nil {
		return auditSkipCounts{}
	}
	counts := auditSkipCounts{
		Deterministic: plan.Summary.DeterministicSkipped,
		ManualReview:  plan.Summary.ManualReviewSkipped,
	}
	if counts.Deterministic == 0 && counts.ManualReview == 0 {
		return auditSkipBreakdown(plan.Items)
	}
	return counts
}

func displayAuditPlan(plan *plans.AuditPlan, showDetails bool) error {
	fmt.Printf("\n📋 Audit Plan\n")
	fmt.Printf("Created: %s\n", plan.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("\nSummary:\n")
	fmt.Printf("  Total Files: %d\n", plan.Summary.TotalFiles)
	fmt.Printf("  Files to Rename: %d\n", plan.Summary.FilesToRename)
	fmt.Printf("  Files to Delete: %d\n", plan.Summary.FilesToDelete)
	fmt.Printf("  Files to Skip: %d\n", plan.Summary.FilesToSkip)
	fmt.Printf("  Avg Confidence: %.2f\n", plan.Summary.AvgConfidence)
	fmt.Printf("  AI Candidates: %d\n", plan.Summary.AICandidateCount)
	fmt.Printf("  AI Calls: %d (%d errors)\n", plan.Summary.AITotalCalls, plan.Summary.AIErrorCount)
	if plan.Summary.DeterministicSkipped > 0 || plan.Summary.ManualReviewSkipped > 0 {
		fmt.Printf("  Pre-AI Skips: %d deterministic, %d manual-review\n",
			plan.Summary.DeterministicSkipped, plan.Summary.ManualReviewSkipped)
	}

	if !showDetails {
		if plan.Summary.FilesToRename > 0 {
			fmt.Printf("\n💡 Run 'plex2jellyfin audit --dry-run' to see detailed changes\n")
		}
		return nil
	}

	// Build action index for items that have actions
	// Actions are created in order for items that pass validation
	actionIdx := 0

	fmt.Printf("\nFiles:\n")
	for i, item := range plan.Items {
		fmt.Printf("\n[%d] %s\n", i+1, filepath.Base(item.Path))
		fmt.Printf("    Path: %s\n", filepath.Dir(item.Path))
		fmt.Printf("    Current: %s (confidence: %.2f)\n", item.Title, item.Confidence)

		if item.SkipReason != "" {
			fmt.Printf("    ⚠️  Skipped: %s\n", item.SkipReason)
		} else if actionIdx < len(plan.Actions) {
			action := plan.Actions[actionIdx]
			fmt.Printf("    ✓ %s → %s\n", action.Action, filepath.Base(action.NewPath))
			fmt.Printf("      New title: %s (confidence: %.2f)\n", action.NewTitle, action.Confidence)
			if action.Reasoning != "" {
				fmt.Printf("      Reason: %s\n", action.Reasoning)
			}
			actionIdx++
		}
	}

	if plan.Summary.FilesToRename > 0 {
		fmt.Printf("\n💡 Run 'plex2jellyfin audit --execute' to apply changes\n")
	}

	return nil
}

func auditPlanWarning(plan *plans.AuditPlan, cfg *config.Config) string {
	if plan == nil || cfg == nil {
		return ""
	}
	if plan.Summary.AITotalCalls == 0 || plan.Summary.AIErrorCount != plan.Summary.AITotalCalls {
		return ""
	}
	if plan.Summary.FilesToRename > 0 || plan.Summary.FilesToDelete > 0 {
		return ""
	}

	models := modelsFromAuditSkipReasons(plan.Items)
	if len(models) == 0 {
		return "Warning: existing audit plan contains only AI errors and no actions. Regenerate it with: plex2jellyfin audit --generate"
	}

	currentModel := strings.TrimSpace(cfg.AI.Model)
	for model := range models {
		if currentModel != "" && model != currentModel {
			return fmt.Sprintf("Warning: existing audit plan contains only AI errors for model %s, but current config uses %s. Regenerate it with: plex2jellyfin audit --generate", model, currentModel)
		}
	}

	return "Warning: existing audit plan contains only AI errors and no actions. Regenerate it with: plex2jellyfin audit --generate"
}

var auditMissingModelPattern = regexp.MustCompile(`model '([^']+)' not found`)

func modelsFromAuditSkipReasons(items []plans.AuditItem) map[string]struct{} {
	models := make(map[string]struct{})
	for _, item := range items {
		if item.SkipReason == "" {
			continue
		}
		match := auditMissingModelPattern.FindStringSubmatch(item.SkipReason)
		if len(match) == 2 {
			models[match[1]] = struct{}{}
		}
	}
	return models
}

func executeAuditPlan(db *database.MediaDB, plan *plans.AuditPlan, cfg *config.Config) error {
	fmt.Printf("\n🚀 Executing Audit Plan\n")

	threshold := auditOpts.Threshold
	if threshold == 0 {
		threshold = 0.8
	}
	filteredIndices := executableAuditActionIndices(plan, threshold)
	blockedActions := blockedAuditActionCount(plan, threshold)

	if len(filteredIndices) == 0 {
		fmt.Printf("No actions meet the confidence threshold (>= %.2f)\n", threshold)
		if blockedActions > 0 {
			fmt.Printf("Skipped %d unsafe stale action(s), such as sample/extra or obfuscated source files. Regenerate the audit plan.\n", blockedActions)
		}
		return nil
	}

	if issues := auditPlanRootIssues(plan, filteredIndices, cfg); len(issues) > 0 {
		fmt.Println("❌ Audit plan failed safety validation; refusing to execute.")
		printConsolidateSafetyIssues(issues)
		fmt.Println("\nRegenerate the audit plan before retrying.")
		return nil
	}

	// Escalate only after validating the plan. Unsafe plans should not require sudo.
	if privilege.NeedsRoot() {
		return privilege.Escalate("rename/delete files and modify ownership")
	}

	fmt.Printf("Found %d actions to execute (confidence >= %.2f)\n\n", len(filteredIndices), threshold)
	if blockedActions > 0 {
		fmt.Printf("Skipped %d unsafe stale action(s), such as sample/extra or obfuscated source files. Regenerate the audit plan after this run.\n\n", blockedActions)
	}

	// Show actions and prompt for confirmation
	for _, idx := range filteredIndices {
		action := plan.Actions[idx]
		item := plan.Items[action.ItemIndex]
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
		item := plan.Items[action.ItemIndex]

		if !confirmedAll {
			fmt.Printf("\nExecute: %s %s -> %s? [y/N]: ", action.Action, filepath.Base(item.Path), filepath.Base(action.NewPath))
			var confirm string
			fmt.Scanln(&confirm)
			if confirm != "y" && confirm != "Y" {
				fmt.Printf("  Skipped\n")
				continue
			}
		}

		start := time.Now()
		if err := plans.ExecuteAuditAction(db, item, action, auditOpts.DryRun, cfg); err != nil {
			fmt.Printf("  ✗ Failed: %v\n", err)
			failed++
		} else {
			fmt.Printf("  ✓ Success (%s)\n", time.Since(start).Round(time.Millisecond))
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

func auditPlanRootIssues(plan *plans.AuditPlan, actionIndices []int, cfg *config.Config) []string {
	roots := configuredLibraryRoots(cfg)
	if len(roots) == 0 || plan == nil {
		return nil
	}
	var issues []string
	for _, idx := range actionIndices {
		if idx < 0 || idx >= len(plan.Actions) {
			issues = append(issues, fmt.Sprintf("action index out of range: %d", idx))
			continue
		}
		action := plan.Actions[idx]
		if action.ItemIndex < 0 || action.ItemIndex >= len(plan.Items) {
			issues = append(issues, fmt.Sprintf("item index out of range: %d", action.ItemIndex))
			continue
		}
		item := plan.Items[action.ItemIndex]
		if issue := rootBoundPathIssue("audit source path", item.Path, roots); issue != "" {
			issues = append(issues, issue)
		}
		if action.NewPath != "" {
			if issue := rootBoundPathIssue("audit target path", action.NewPath, roots); issue != "" {
				issues = append(issues, issue)
			}
		}
	}
	return issues
}

func executableAuditActionIndices(plan *plans.AuditPlan, threshold float64) []int {
	if plan == nil {
		return nil
	}
	var filteredIndices []int
	for i, action := range plan.Actions {
		if action.Confidence < threshold {
			continue
		}
		if auditActionBlocked(plan, action) {
			continue
		}
		filteredIndices = append(filteredIndices, i)
	}
	return filteredIndices
}

func blockedAuditActionCount(plan *plans.AuditPlan, threshold float64) int {
	if plan == nil {
		return 0
	}
	blocked := 0
	for _, action := range plan.Actions {
		if action.Confidence < threshold {
			continue
		}
		if auditActionBlocked(plan, action) {
			blocked++
		}
	}
	return blocked
}

func auditActionBlocked(plan *plans.AuditPlan, action plans.AuditAction) bool {
	if action.ItemIndex < 0 || action.ItemIndex >= len(plan.Items) {
		return true
	}
	item := plan.Items[action.ItemIndex]
	if isAuditExtraContentPath(item.Path) || isAuditExtraContentPath(action.NewPath) {
		return true
	}
	return naming.IsObfuscatedFilename(filepath.Base(item.Path))
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
	plansDir, err := paths.PlansDir()
	if err != nil {
		return ""
	}
	return filepath.Join(plansDir, "audit.json")
}

// newAuditCmd returns the audit command
func newAuditCmd() *cobra.Command {
	return auditCmd
}
