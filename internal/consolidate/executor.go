package consolidate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/transfer"
)

// Executor executes consolidation plans
type Executor struct {
	db         *database.MediaDB
	planner    *Planner
	transferer transfer.Transferer
	dryRun     bool
}

// ExecutionResult contains statistics from plan execution
type ExecutionResult struct {
	PlansExecuted  int
	PlansSucceeded int
	PlansFailed    int
	FilesDeleted   int
	FilesMoved     int
	FilesRenamed   int
	SpaceReclaimed int64
	Duration       time.Duration
	Errors         []error
}

// NewExecutor creates a new plan executor
func NewExecutor(db *database.MediaDB, dryRun bool) *Executor {
	transferer, _ := transfer.New(transfer.BackendRsync)
	return &Executor{
		db:         db,
		planner:    NewPlanner(db),
		transferer: transferer,
		dryRun:     dryRun,
	}
}

// ExecutePlans runs all pending consolidation plans
func (e *Executor) ExecutePlans(ctx context.Context) (*ExecutionResult, error) {
	startTime := time.Now()
	result := &ExecutionResult{}

	// Get all pending plans
	plans, err := e.planner.GetPendingPlans()
	if err != nil {
		return nil, fmt.Errorf("failed to get pending plans: %w", err)
	}

	if len(plans) == 0 {
		return result, nil
	}

	// Execute each plan
	for _, plan := range plans {
		// Check context cancellation
		select {
		case <-ctx.Done():
			result.Duration = time.Since(startTime)
			return result, ctx.Err()
		default:
		}

		result.PlansExecuted++

		// For delete actions, get file size BEFORE deleting
		var fileSizeToReclaim int64
		if plan.Action == "delete" {
			file, err := e.db.GetMediaFileByID(plan.SourceFileID)
			if err == nil && file != nil {
				fileSizeToReclaim = file.Size
			}
		}

		var execErr error
		switch plan.Action {
		case "delete":
			execErr = e.executeDelete(ctx, plan)
			if execErr == nil {
				result.FilesDeleted++
				result.SpaceReclaimed += fileSizeToReclaim
			}
		case "move":
			execErr = e.executeMove(ctx, plan)
			if execErr == nil {
				result.FilesMoved++
			}
		case "rename":
			execErr = e.executeRename(ctx, plan)
			if execErr == nil {
				result.FilesRenamed++
			}
		default:
			execErr = fmt.Errorf("unknown action: %s", plan.Action)
		}

		// Update plan status in database
		if execErr != nil {
			result.PlansFailed++
			result.Errors = append(result.Errors, fmt.Errorf("plan %d: %w", plan.ID, execErr))
			e.markPlanFailed(plan.ID, execErr.Error())
		} else {
			result.PlansSucceeded++
			e.markPlanCompleted(plan.ID)
		}
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

// ExecutePlan executes a single plan by ID
func (e *Executor) ExecutePlan(ctx context.Context, planID int64) error {
	plan, err := e.planner.GetPlanByID(planID)
	if err != nil {
		return fmt.Errorf("failed to get plan: %w", err)
	}

	if plan.Status != "pending" {
		return fmt.Errorf("plan is not pending (status: %s)", plan.Status)
	}

	var execErr error
	switch plan.Action {
	case "delete":
		execErr = e.executeDelete(ctx, plan)
	case "move":
		execErr = e.executeMove(ctx, plan)
	case "rename":
		execErr = e.executeRename(ctx, plan)
	default:
		return fmt.Errorf("unknown action: %s", plan.Action)
	}

	if execErr != nil {
		e.markPlanFailed(plan.ID, execErr.Error())
		return execErr
	}

	e.markPlanCompleted(plan.ID)
	return nil
}

// executeDelete removes an inferior duplicate file
func (e *Executor) executeDelete(ctx context.Context, plan *ConsolidationPlan) error {
	if e.dryRun {
		return nil // Dry run - don't actually delete
	}

	// Check if file exists
	if _, err := os.Stat(plan.SourcePath); os.IsNotExist(err) {
		// File already gone - mark as complete
		return nil
	}

	// Delete the file
	if err := os.Remove(plan.SourcePath); err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	// Remove from database
	if err := e.db.DeleteMediaFile(plan.SourcePath); err != nil {
		return fmt.Errorf("failed to remove from database: %w", err)
	}

	return nil
}

// executeMove moves a file to a new location
func (e *Executor) executeMove(ctx context.Context, plan *ConsolidationPlan) error {
	if e.dryRun {
		return nil // Dry run - don't actually move
	}

	// Check if source exists
	if _, err := os.Stat(plan.SourcePath); os.IsNotExist(err) {
		return fmt.Errorf("source file does not exist")
	}

	// Use transfer package for reliable file move (handles failing disks)
	result, err := e.transferer.Move(plan.SourcePath, plan.TargetPath, transfer.TransferOptions{
		Timeout:   5 * time.Minute,
		TargetUID: -1, // Preserve ownership
		TargetGID: -1,
	})

	if err != nil {
		return fmt.Errorf("transfer failed: %w", err)
	}

	if result.Error != nil {
		return fmt.Errorf("transfer failed: %w", result.Error)
	}

	// Update database - remove old path, add new path
	file, err := e.db.GetMediaFile(plan.SourcePath)
	if err != nil {
		return fmt.Errorf("failed to get file from database: %w", err)
	}

	// Delete old entry
	if err := e.db.DeleteMediaFile(plan.SourcePath); err != nil {
		return fmt.Errorf("failed to delete old database entry: %w", err)
	}

	// Add new entry
	file.Path = plan.TargetPath
	if err := e.db.UpsertMediaFile(file); err != nil {
		return fmt.Errorf("failed to insert new database entry: %w", err)
	}

	// Cleanup empty parent directory
	parentDir := filepath.Dir(plan.SourcePath)
	if err := CleanupEmptyDir(parentDir); err != nil {
		// Log warning but don't fail the move
		fmt.Printf("Warning: Failed to cleanup empty directory %s: %v\n", parentDir, err)
	}

	return nil
}

// executeRename renames a file to be Jellyfin-compliant
func (e *Executor) executeRename(ctx context.Context, plan *ConsolidationPlan) error {
	if e.dryRun {
		return nil // Dry run - don't actually rename
	}

	// Check if source exists
	if _, err := os.Stat(plan.SourcePath); os.IsNotExist(err) {
		return fmt.Errorf("source file does not exist")
	}

	// Rename the file
	if err := os.Rename(plan.SourcePath, plan.TargetPath); err != nil {
		return fmt.Errorf("failed to rename file: %w", err)
	}

	// Update database path
	file, err := e.db.GetMediaFile(plan.SourcePath)
	if err != nil {
		return fmt.Errorf("failed to get file from database: %w", err)
	}

	// Delete old entry
	if err := e.db.DeleteMediaFile(plan.SourcePath); err != nil {
		return fmt.Errorf("failed to delete old database entry: %w", err)
	}

	// Add new entry with compliant flag set
	file.Path = plan.TargetPath
	file.IsJellyfinCompliant = true
	file.ComplianceIssues = []string{} // Clear issues
	if err := e.db.UpsertMediaFile(file); err != nil {
		return fmt.Errorf("failed to insert new database entry: %w", err)
	}

	return nil
}

// markPlanCompleted marks a plan as successfully completed
func (e *Executor) markPlanCompleted(planID int64) error {
	query := `
		UPDATE consolidation_plans
		SET status = 'completed', executed_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	_, err := e.db.DB().Exec(query, planID)
	return err
}

// markPlanFailed marks a plan as failed with error message
func (e *Executor) markPlanFailed(planID int64, errorMsg string) error {
	query := `
		UPDATE consolidation_plans
		SET status = 'failed', executed_at = CURRENT_TIMESTAMP, error_message = ?
		WHERE id = ?
	`
	_, err := e.db.DB().Exec(query, errorMsg, planID)
	return err
}
