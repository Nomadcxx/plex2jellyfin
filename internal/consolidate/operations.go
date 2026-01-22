package consolidate

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/transfer"
)

// ExecutePlan executes a consolidation plan
func (c *Consolidator) ExecutePlan(plan *Plan, dryRun bool) error {
	if !plan.CanProceed {
		return fmt.Errorf("cannot execute plan: %v", plan.Reasons)
	}

	var failures []string

	// Ensure target directory exists
	if !dryRun {
		if err := os.MkdirAll(plan.TargetPath, 0755); err != nil {
			return fmt.Errorf("failed to create target directory: %w", err)
		}
	}

	// Execute each operation
	for _, op := range plan.Operations {
		if err := c.executeOperation(op, dryRun); err != nil {
			failures = append(failures, fmt.Sprintf("%s -> %s: %v", op.SourcePath, op.DestinationPath, err))
			continue
		}

		if !dryRun {
			c.stats.FilesMoved++
			c.stats.BytesMoved += op.Size
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("%d operations failed: %v", len(failures), failures)
	}

	// Mark conflict as resolved in database
	if !dryRun && len(failures) == 0 {
		if err := c.db.ResolveConflict(plan.ConflictID, plan.TargetPath); err != nil {
			return fmt.Errorf("failed to mark conflict as resolved: %w", err)
		}
	}

	return nil
}

// executeOperation executes a single move operation
func (c *Consolidator) executeOperation(op *Operation, dryRun bool) error {
	// Ensure destination directory exists
	destDir := filepath.Dir(op.DestinationPath)
	if !dryRun {
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return fmt.Errorf("failed to create destination directory: %w", err)
		}
	}

	// Skip if source doesn't exist (should not happen, but safe)
	if _, err := os.Stat(op.SourcePath); os.IsNotExist(err) {
		return fmt.Errorf("source file does not exist")
	}

	// Check if destination already exists (should have been already filtered, but double-check)
	if _, err := os.Stat(op.DestinationPath); err == nil {
		return fmt.Errorf("destination file already exists")
	}

	if dryRun {
		fmt.Printf("[DRY RUN] Would move: %s -> %s (%d bytes)\n",
			op.SourcePath, op.DestinationPath, op.Size)
		return nil
	}

	// Create transferer for robustness with timeout/retry
	transf, err := transfer.New(transfer.BackendAuto)
	if err != nil {
		return fmt.Errorf("failed to create transferer: %w", err)
	}

	opts := transfer.DefaultOptions()
	opts.Checksum = c.cfg.Options.VerifyChecksums

	// Move file (rsync backend handles failing disks)
	result, err := transf.Move(op.SourcePath, op.DestinationPath, opts)
	if err != nil {
		return fmt.Errorf("transfer failed: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("transfer failed: %v", result.Error)
	}

	return nil
}

// GenerateAllPlans generates consolidation plans for all conflicts
func (c *Consolidator) GenerateAllPlans() ([]*Plan, error) {
	// Detect conflicts first
	conflicts, err := c.db.DetectConflicts()
	if err != nil {
		return nil, fmt.Errorf("failed to detect conflicts: %w", err)
	}

	c.stats.ConflictsFound = len(conflicts)

	var plans []*Plan
	for _, conflict := range conflicts {
		plan, err := c.GeneratePlan(&conflict)
		if err != nil {
			fmt.Printf("Warning: Failed to generate plan for conflict %d (%s): %v\n",
				conflict.ID, conflict.Title, err)
			continue
		}

		plans = append(plans, plan)
	}

	return plans, nil
}

// DryRun executes a dry run of all consolidation plans
func (c *Consolidator) DryRun() error {
	plans, err := c.GenerateAllPlans()
	if err != nil {
		return fmt.Errorf("failed to generate plans: %w", err)
	}

	if len(plans) == 0 {
		fmt.Println("No consolidation plans to execute.")
		return nil
	}

	var totalFiles int
	var totalBytes int64

	for _, plan := range plans {
		totalFiles += plan.TotalFiles
		totalBytes += plan.TotalBytes

		yearStr := "unknown"
		if plan.Year != nil {
			yearStr = fmt.Sprintf("%d", *plan.Year)
		}
		fmt.Printf("\n=== Plan for %s (%s) ===\n", plan.Title, yearStr)
		fmt.Printf("Conflict ID: %d\n", plan.ConflictID)
		fmt.Printf("Media Type: %s\n", plan.MediaType)
		fmt.Printf("Target Path: %s\n", plan.TargetPath)
		fmt.Printf("Source Paths: %v\n", plan.SourcePaths)
		fmt.Printf("Files to move: %d (%s)\n", plan.TotalFiles, formatBytes(plan.TotalBytes))

		if !plan.CanProceed {
			fmt.Printf("⚠️  Cannot proceed: %v\n", plan.Reasons)
		}

		// Show first 5 files to move
		for i, op := range plan.Operations {
			if i >= 5 {
				fmt.Printf("  ... and %d more files\n", len(plan.Operations)-i)
				break
			}
			fmt.Printf("  %s -> %s (%s)\n",
				filepath.Base(op.SourcePath),
				filepath.Base(op.DestinationPath),
				formatBytes(op.Size))
		}
	}

	fmt.Printf("\n=== SUMMARY ===\n")
	fmt.Printf("Conflicts found: %d\n", c.stats.ConflictsFound)
	fmt.Printf("Plans generated: %d\n", len(plans))
	fmt.Printf("Total files to move: %d\n", totalFiles)
	fmt.Printf("Total bytes to move: %s\n", formatBytes(totalBytes))

	return nil
}

// ExecuteAll executes all consolidation plans
func (c *Consolidator) ExecuteAll(dryRun bool) error {
	plans, err := c.GenerateAllPlans()
	if err != nil {
		return fmt.Errorf("failed to generate plans: %w", err)
	}

	if len(plans) == 0 {
		fmt.Println("No consolidation plans to execute.")
		return nil
	}

	var succeeded, failed int
	for i, plan := range plans {
		fmt.Printf("\n--- [%d/%d] Consolidating %s (%v) ---\n",
			i+1, len(plans), plan.Title, plan.Year)

		if !plan.CanProceed {
			fmt.Printf("⚠️  Skipping: %v\n", plan.Reasons)
			failed++
			continue
		}

		if err := c.ExecutePlan(plan, dryRun); err != nil {
			fmt.Printf("❌ Failed: %v\n", err)
			failed++
		} else {
			fmt.Printf("✅ Successfully moved %d files (%s)\n",
				plan.TotalFiles, formatBytes(plan.TotalBytes))
			succeeded++
		}
	}

	c.stats.EndTime = time.Now()

	fmt.Printf("\n=== EXECUTION SUMMARY ===\n")
	fmt.Printf("Execution mode: %s\n", map[bool]string{true: "DRY RUN", false: "LIVE"}[dryRun])
	fmt.Printf("Duration: %s\n", c.stats.EndTime.Sub(c.stats.StartTime))
	fmt.Printf("Plans attempted: %d\n", len(plans))
	fmt.Printf("Plans succeeded: %d\n", succeeded)
	fmt.Printf("Plans failed: %d\n", failed)
	fmt.Printf("Total files moved: %d\n", c.stats.FilesMoved)
	fmt.Printf("Total bytes moved: %s\n", formatBytes(c.stats.BytesMoved))

	if dryRun {
		fmt.Printf("\n⚠️  This was a DRY RUN. No files were actually moved.\n")
	}

	return nil
}

// GetStats returns consolidation statistics
func (c *Consolidator) GetStats() Stats {
	return c.stats
}

// formatBytes formats bytes for display
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTP"[exp])
}

// StorePlan stores a consolidation plan into the database
// Creates individual MOVE plans for each operation in the plan
func (c *Consolidator) StorePlan(plan *Plan) error {
	for _, op := range plan.Operations {
		reason := fmt.Sprintf("Consolidate conflict: %s", plan.Title)
		details := fmt.Sprintf("Moving %s to target location", op.SourcePath)

		var sourceFileID *int64 = nil

		file, err := c.db.GetMediaFile(op.SourcePath)
		if err == nil && file != nil {
			id := file.ID
			sourceFileID = &id
		}

		query := `
			INSERT INTO consolidation_plans (
				action, source_file_id, source_path, target_path, reason, reason_details
			) VALUES (?, ?, ?, ?, ?, ?)
		`

		_, err = c.db.DB().Exec(query, "move", sourceFileID, op.SourcePath, op.DestinationPath, reason, details)
		if err != nil {
			return fmt.Errorf("failed to insert consolidation plan: %w", err)
		}
	}

	return nil
}
