package consolidate

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/transfer"
)

// Executor executes consolidation plans
type Executor struct {
	db         *database.MediaDB
	planner    *Planner
	transferer transfer.Transferer
	dryRun     bool
	writer     io.Writer
	cfg        *config.Config
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
func NewExecutor(db *database.MediaDB, dryRun bool, writer io.Writer, cfg *config.Config) *Executor {
	if writer == nil {
		writer = os.Stdout
	}
	transferer, err := transfer.New(transfer.BackendRsync)
	if err != nil {
		// ponytail: use native when optional rsync is missing; install rsync for resumable transfers.
		transferer = transfer.NewNativeTransferer(32 * 1024 * 1024)
	}
	return &Executor{
		db:         db,
		planner:    NewPlanner(db),
		transferer: transferer,
		dryRun:     dryRun,
		writer:     writer,
		cfg:        cfg,
	}
}

// Printf writes formatted output to the configured writer
func (e *Executor) Printf(format string, a ...interface{}) {
	fmt.Fprintf(e.writer, format, a...)
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
		if plan.Action == "delete" && plan.SourceFileID.Valid {
			file, err := e.db.GetMediaFileByID(plan.SourceFileID.Int64)
			if err == nil && file != nil {
				fileSizeToReclaim = file.Size
			}
		}

		var execErr error
		if reason := DBPlanSafetyIssue(plan); reason != "" {
			execErr = fmt.Errorf("unsafe consolidation plan: %s", reason)
		} else {
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
	if reason := DBPlanSafetyIssue(plan); reason != "" {
		err := fmt.Errorf("unsafe consolidation plan: %s", reason)
		e.markPlanFailed(plan.ID, err.Error())
		return err
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
	if err := e.db.CheckMediaFileMoveDestination(plan.SourcePath, plan.TargetPath); err != nil {
		return fmt.Errorf("cannot move file: %w", err)
	}
	if err := CheckDestinationAvailable(plan.TargetPath); err != nil {
		return err
	}

	// Check if source exists
	if _, err := os.Stat(plan.SourcePath); os.IsNotExist(err) {
		return fmt.Errorf("source file does not exist")
	}

	file, err := e.db.GetMediaFile(plan.SourcePath)
	if err != nil {
		return fmt.Errorf("failed to get file from database: %w", err)
	}
	canonicalPath := ""
	if file != nil {
		canonicalPath, err = e.targetCanonicalPath(file, plan)
		if err != nil {
			return err
		}
		file.LibraryRoot = libraryRootForPath(plan.TargetPath, file.MediaType, e.cfg, file.LibraryRoot)
	}

	// Use transfer package for reliable file move (handles failing disks)
	opts := transfer.TransferOptions{
		Timeout:   5 * time.Minute,
		TargetUID: -1, // Preserve ownership
		TargetGID: -1,
	}
	if file != nil {
		file.Path = plan.TargetPath
	}
	moved := false
	movedIdentity := ""
	move := func(sourceIdentity, sourceQuarantinePath string, recordPublished func(string, string) error) (database.MediaFileMoveAttempt, error) {
		opts.MoveSourceIdentity = sourceIdentity
		opts.MoveSourceQuarantinePath = sourceQuarantinePath
		opts.OnPublished = recordPublished
		result, err := e.transferer.Move(plan.SourcePath, plan.TargetPath, opts)
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
			return attempt, fmt.Errorf("transfer completed without removing source: %s", plan.SourcePath)
		}
		moved = true
		movedIdentity = result.PublishedIdentity
		if file != nil {
			info, err := os.Stat(plan.TargetPath)
			if err != nil {
				return attempt, fmt.Errorf("inspect moved file: %w", err)
			}
			file.Size = info.Size()
			file.ModifiedAt = info.ModTime()
		}
		return attempt, nil
	}
	if file == nil {
		// File not in DB: move it without a row to update.
		if _, err := move("", "", nil); err != nil {
			return err
		}
		return nil
	}
	if err := e.db.MoveMediaFileWith(file, plan.SourcePath, canonicalPath, false, move); err != nil {
		if moved {
			if rollbackErr := transfer.RollbackMove(e.transferer, plan.SourcePath, plan.TargetPath, opts, movedIdentity); rollbackErr != nil {
				return fmt.Errorf("failed to update media file after move: %v; filesystem rollback failed: %w", err, rollbackErr)
			}
			if cancelErr := e.db.CancelMediaFileMove(plan.SourcePath); cancelErr != nil {
				return fmt.Errorf("failed to update media file after move: %v; file restored but reservation cleanup failed: %w", err, cancelErr)
			}
			return fmt.Errorf("failed to update media file after move; file restored to source: %w", err)
		}
		return fmt.Errorf("failed to update media file after move: %w", err)
	}

	// Verify file exists at new path
	if _, err := os.Stat(plan.TargetPath); err != nil {
		e.Printf("Warning: post-move verification failed for %s: %v\n", plan.TargetPath, err)
	}

	if plan.ConflictID > 0 {
		if err := e.db.ResolveConflict(plan.ConflictID, plan.TargetPath); err != nil {
			return fmt.Errorf("failed to mark conflict as resolved: %w", err)
		}
	}

	parentDir := filepath.Dir(plan.SourcePath)
	if err := CleanupEmptyDir(parentDir); err != nil {
		e.Printf("Warning: Failed to cleanup empty directory %s: %v\n", parentDir, err)
	}

	return nil
}

// executeRename renames a file to be Jellyfin-compliant
func (e *Executor) executeRename(ctx context.Context, plan *ConsolidationPlan) error {
	if e.dryRun {
		return nil // Dry run - don't actually rename
	}
	if err := e.db.CheckMediaFileMoveDestination(plan.SourcePath, plan.TargetPath); err != nil {
		return fmt.Errorf("cannot rename file: %w", err)
	}
	if err := CheckDestinationAvailable(plan.TargetPath); err != nil {
		return err
	}

	// Check if source exists
	if _, err := os.Stat(plan.SourcePath); os.IsNotExist(err) {
		return fmt.Errorf("source file does not exist")
	}

	file, err := e.db.GetMediaFile(plan.SourcePath)
	if err != nil {
		return fmt.Errorf("failed to get file from database: %w", err)
	}
	if file == nil {
		return fmt.Errorf("media file not found in database for rename: %s", plan.SourcePath)
	}
	canonicalPath, err := e.targetCanonicalPath(file, plan)
	if err != nil {
		return err
	}
	file.LibraryRoot = libraryRootForPath(plan.TargetPath, file.MediaType, e.cfg, file.LibraryRoot)

	opts := transfer.TransferOptions{
		Timeout:   5 * time.Minute,
		TargetUID: -1,
		TargetGID: -1,
	}
	file.Path = plan.TargetPath
	file.IsJellyfinCompliant = true
	file.ComplianceIssues = []string{} // Clear issues
	moved := false
	movedIdentity := ""
	move := func(sourceIdentity, sourceQuarantinePath string, recordPublished func(string, string) error) (database.MediaFileMoveAttempt, error) {
		opts.MoveSourceIdentity = sourceIdentity
		opts.MoveSourceQuarantinePath = sourceQuarantinePath
		opts.OnPublished = recordPublished
		result, err := e.transferer.Move(plan.SourcePath, plan.TargetPath, opts)
		attempt := database.MediaFileMoveAttempt{}
		if result != nil {
			attempt.Published = result.Published
			attempt.SourceRemoved = result.SourceRemoved
			attempt.PublishedIdentity = result.PublishedIdentity
			attempt.PendingSourceRemoval = result.PendingSourceRemoval
		}
		if err != nil {
			return attempt, fmt.Errorf("failed to move file: %w", err)
		}
		if err := result.Validate(); err != nil {
			return attempt, fmt.Errorf("file transfer failed: %w", err)
		}
		if !result.SourceRemoved {
			return attempt, fmt.Errorf("transfer completed without removing source: %s", plan.SourcePath)
		}
		moved = true
		movedIdentity = result.PublishedIdentity
		info, err := os.Stat(plan.TargetPath)
		if err != nil {
			return attempt, fmt.Errorf("inspect renamed file: %w", err)
		}
		file.Size = info.Size()
		file.ModifiedAt = info.ModTime()
		return attempt, nil
	}
	if err := e.db.MoveMediaFileWith(file, plan.SourcePath, canonicalPath, true, move); err != nil {
		if moved {
			if rollbackErr := transfer.RollbackMove(e.transferer, plan.SourcePath, plan.TargetPath, opts, movedIdentity); rollbackErr != nil {
				return fmt.Errorf("failed to update media file after rename: %v; filesystem rollback failed: %w", err, rollbackErr)
			}
			if cancelErr := e.db.CancelMediaFileMove(plan.SourcePath); cancelErr != nil {
				return fmt.Errorf("failed to update media file after rename: %v; file restored but reservation cleanup failed: %w", err, cancelErr)
			}
			return fmt.Errorf("failed to update media file after rename; file restored to source: %w", err)
		}
		return fmt.Errorf("failed to update media file after rename: %w", err)
	}

	return nil
}

func (e *Executor) targetCanonicalPath(file *database.MediaFile, plan *ConsolidationPlan) (string, error) {
	if plan.ConflictID > 0 {
		conflict, err := e.db.GetConflict(plan.ConflictID)
		if err != nil {
			return "", fmt.Errorf("read move conflict %d: %w", plan.ConflictID, err)
		}
		if conflict != nil {
			var target string
			for _, location := range conflict.Locations {
				location = filepath.Clean(location)
				if pathIsWithin(location, plan.TargetPath) && len(location) > len(target) {
					target = location
				}
			}
			if target != "" {
				return target, nil
			}
		}
	}

	switch file.MediaType {
	case "movie":
		return filepath.Dir(plan.TargetPath), nil
	case "episode":
		for current := filepath.Dir(plan.TargetPath); current != filepath.Dir(current); current = filepath.Dir(current) {
			base := strings.ToLower(strings.TrimSpace(filepath.Base(current)))
			if strings.HasPrefix(base, "season ") || strings.HasPrefix(base, "season_") ||
				strings.HasPrefix(base, "season-") || base == "specials" {
				return filepath.Dir(current), nil
			}
		}
		return filepath.Dir(plan.TargetPath), nil
	default:
		return "", nil
	}
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
