package consolidate

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Nomadcxx/jellywatch/internal/database"
)

// Planner generates consolidation plans from database queries
type Planner struct {
	db *database.MediaDB
}

// NewPlanner creates a new consolidation planner
func NewPlanner(db *database.MediaDB) *Planner {
	return &Planner{db: db}
}

// PlanSummary contains aggregated statistics about consolidation plans
type PlanSummary struct {
	TotalPlans      int
	DeletePlans     int
	MovePlans       int
	RenamePlans     int
	SpaceToReclaim  int64
	FilesToProcess  int
	DuplicateGroups int
}

// ConsolidationPlan represents a single action in the database
type ConsolidationPlan struct {
	ID            int64
	CreatedAt     string
	Status        string
	Action        string
	SourceFileID  int64
	SourcePath    string
	TargetPath    string
	Reason        string
	ReasonDetails string
	ExecutedAt    string
	ErrorMessage  string
	ConflictID    int64
}

// GeneratePlans creates consolidation plans from database duplicate detection
//
// This is the core of the CONDOR consolidation system:
// 1. Find all duplicate groups (movies/episodes with same normalized title + year)
// 2. For each group, identify the best file (highest quality_score)
// 3. Create DELETE plans for inferior duplicates
// 4. Find non-compliant files that need RENAME/MOVE
// 5. Insert all plans into consolidation_plans table
func (p *Planner) GeneratePlans(ctx context.Context) (*PlanSummary, error) {
	summary := &PlanSummary{}

	// Clear any old pending plans
	if err := p.clearPendingPlans(); err != nil {
		return nil, fmt.Errorf("failed to clear old plans: %w", err)
	}

	// Generate delete plans for duplicate movies
	movieDeletes, err := p.generateDuplicateMoviePlans(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate movie duplicate plans: %w", err)
	}
	summary.DeletePlans += movieDeletes
	summary.TotalPlans += movieDeletes

	// Generate delete plans for duplicate episodes
	episodeDeletes, movieGroups, episodeGroups, err := p.generateDuplicateEpisodePlans(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate episode duplicate plans: %w", err)
	}
	summary.DeletePlans += episodeDeletes
	summary.TotalPlans += episodeDeletes
	summary.DuplicateGroups = movieGroups + episodeGroups

	// Generate rename plans for non-compliant files
	renameCount, err := p.generateNonCompliantPlans(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate non-compliant plans: %w", err)
	}
	summary.RenamePlans += renameCount
	summary.TotalPlans += renameCount

	// Calculate statistics
	stats, err := p.db.GetConsolidationStats()
	if err != nil {
		return nil, fmt.Errorf("failed to get consolidation stats: %w", err)
	}

	summary.SpaceToReclaim = stats.SpaceReclaimable
	summary.FilesToProcess = stats.DuplicateFiles + stats.NonCompliantFiles

	return summary, nil
}

// generateDuplicateMoviePlans creates DELETE plans for duplicate movies
func (p *Planner) generateDuplicateMoviePlans(ctx context.Context) (int, error) {
	duplicateGroups, err := p.db.FindDuplicateMovies()
	if err != nil {
		return 0, err
	}

	planCount := 0

	for _, group := range duplicateGroups {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return planCount, ctx.Err()
		default:
		}

		if group.BestFile == nil {
			continue
		}

		// Create DELETE plans for all inferior files
		for _, file := range group.Files {
			if file.ID == group.BestFile.ID {
				continue // Keep the best file
			}

			reason := fmt.Sprintf("Duplicate of better quality file")
			details := fmt.Sprintf("Keeping: %s (score: %d, %s %s)",
				group.BestFile.Path,
				group.BestFile.QualityScore,
				group.BestFile.Resolution,
				group.BestFile.SourceType)

			if err := p.insertPlan("delete", file.ID, file.Path, "", reason, details); err != nil {
				return planCount, err
			}
			planCount++
		}
	}

	return planCount, nil
}

// generateDuplicateEpisodePlans creates DELETE plans for duplicate episodes
func (p *Planner) generateDuplicateEpisodePlans(ctx context.Context) (int, int, int, error) {
	duplicateGroups, err := p.db.FindDuplicateEpisodes()
	if err != nil {
		return 0, 0, 0, err
	}

	planCount := 0
	movieGroupCount := 0
	episodeGroupCount := len(duplicateGroups)

	for _, group := range duplicateGroups {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return planCount, movieGroupCount, episodeGroupCount, ctx.Err()
		default:
		}

		if group.BestFile == nil {
			continue
		}

		// Create DELETE plans for all inferior files
		for _, file := range group.Files {
			if file.ID == group.BestFile.ID {
				continue // Keep the best file
			}

			reason := fmt.Sprintf("Duplicate of better quality file")
			details := fmt.Sprintf("Keeping: %s (score: %d, %s %s)",
				group.BestFile.Path,
				group.BestFile.QualityScore,
				group.BestFile.Resolution,
				group.BestFile.SourceType)

			if err := p.insertPlan("delete", file.ID, file.Path, "", reason, details); err != nil {
				return planCount, movieGroupCount, episodeGroupCount, err
			}
			planCount++
		}
	}

	return planCount, movieGroupCount, episodeGroupCount, nil
}

// generateNonCompliantPlans creates RENAME plans for non-Jellyfin-compliant files
func (p *Planner) generateNonCompliantPlans(ctx context.Context) (int, error) {
	nonCompliantFiles, err := p.db.FindNonCompliantFiles()
	if err != nil {
		return 0, err
	}

	planCount := 0

	for _, file := range nonCompliantFiles {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return planCount, ctx.Err()
		default:
		}

		// For now, we'll just flag them for manual review
		// In the future, we could compute the correct Jellyfin-compliant name
		// and create a RENAME plan

		reason := "Non-Jellyfin-compliant filename"
		details := fmt.Sprintf("Issues: %v", file.ComplianceIssues)

		// We don't have auto-rename logic yet, so we'll skip creating plans for now
		// This is a future enhancement for Phase 6
		_ = reason
		_ = details
	}

	return planCount, nil
}

// insertPlan inserts a consolidation plan into the database
func (p *Planner) insertPlan(action string, fileID int64, sourcePath, targetPath, reason, details string) error {
	query := `
		INSERT INTO consolidation_plans (
			action, source_file_id, source_path, target_path, reason, reason_details
		) VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err := p.db.DB().Exec(query, action, fileID, sourcePath, targetPath, reason, details)
	return err
}

// clearPendingPlans removes all pending (not executed) plans
func (p *Planner) clearPendingPlans() error {
	query := `DELETE FROM consolidation_plans WHERE status = 'pending'`
	_, err := p.db.DB().Exec(query)
	return err
}

// GetPendingPlans returns all plans that haven't been executed yet
func (p *Planner) GetPendingPlans() ([]*ConsolidationPlan, error) {
	query := `
		SELECT id, created_at, status, action, source_file_id, source_path,
		       target_path, reason, reason_details, executed_at, error_message, conflict_id
		FROM consolidation_plans
		WHERE status = 'pending'
		ORDER BY action DESC, id ASC
	`

	rows, err := p.db.DB().Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plans []*ConsolidationPlan
	for rows.Next() {
		var plan ConsolidationPlan
		var executedAt, errorMsg *string
		var conflictID sql.NullInt64

		err := rows.Scan(
			&plan.ID,
			&plan.CreatedAt,
			&plan.Status,
			&plan.Action,
			&plan.SourceFileID,
			&plan.SourcePath,
			&plan.TargetPath,
			&plan.Reason,
			&plan.ReasonDetails,
			&executedAt,
			&errorMsg,
			&conflictID,
		)
		if err != nil {
			return nil, err
		}

		if executedAt != nil {
			plan.ExecutedAt = *executedAt
		}
		if errorMsg != nil {
			plan.ErrorMessage = *errorMsg
		}
		if conflictID.Valid {
			plan.ConflictID = conflictID.Int64
		}

		plans = append(plans, &plan)
	}

	return plans, rows.Err()
}

// GetPlanByID retrieves a specific plan by ID
func (p *Planner) GetPlanByID(planID int64) (*ConsolidationPlan, error) {
	query := `
		SELECT id, created_at, status, action, source_file_id, source_path,
		       target_path, reason, reason_details, executed_at, error_message, conflict_id
		FROM consolidation_plans
		WHERE id = ?
	`

	var plan ConsolidationPlan
	var executedAt, errorMsg *string
	var conflictID sql.NullInt64

	err := p.db.DB().QueryRow(query, planID).Scan(
		&plan.ID,
		&plan.CreatedAt,
		&plan.Status,
		&plan.Action,
		&plan.SourceFileID,
		&plan.SourcePath,
		&plan.TargetPath,
		&plan.Reason,
		&plan.ReasonDetails,
		&executedAt,
		&errorMsg,
		&conflictID,
	)
	if err != nil {
		return nil, err
	}

	if executedAt != nil {
		plan.ExecutedAt = *executedAt
	}
	if errorMsg != nil {
		plan.ErrorMessage = *errorMsg
	}
	if conflictID.Valid {
		plan.ConflictID = conflictID.Int64
	}

	return &plan, nil
}

// GetPlanSummary returns aggregated statistics about all plans
func (p *Planner) GetPlanSummary() (*PlanSummary, error) {
	summary := &PlanSummary{}

	// Count plans by action (use COALESCE to handle NULL when no rows match)
	query := `
		SELECT
			COUNT(*) as total,
			COALESCE(SUM(CASE WHEN action = 'delete' THEN 1 ELSE 0 END), 0) as deletes,
			COALESCE(SUM(CASE WHEN action = 'move' THEN 1 ELSE 0 END), 0) as moves,
			COALESCE(SUM(CASE WHEN action = 'rename' THEN 1 ELSE 0 END), 0) as renames
		FROM consolidation_plans
		WHERE status = 'pending'
	`

	err := p.db.DB().QueryRow(query).Scan(
		&summary.TotalPlans,
		&summary.DeletePlans,
		&summary.MovePlans,
		&summary.RenamePlans,
	)
	if err != nil {
		return nil, err
	}

	// Calculate space to reclaim from delete plans
	query = `
		SELECT COALESCE(SUM(mf.size), 0)
		FROM consolidation_plans cp
		JOIN media_files mf ON cp.source_file_id = mf.id
		WHERE cp.status = 'pending' AND cp.action = 'delete'
	`

	err = p.db.DB().QueryRow(query).Scan(&summary.SpaceToReclaim)
	if err != nil {
		return nil, err
	}

	summary.FilesToProcess = summary.TotalPlans

	return summary, nil
}
