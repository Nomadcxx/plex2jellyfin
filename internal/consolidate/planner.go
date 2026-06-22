package consolidate

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Nomadcxx/jellywatch/internal/database"
)

const minQualityScore = 1

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
	SourceFileID  sql.NullInt64
	SourcePath    string
	TargetPath    string
	Reason        string
	ReasonDetails string
	ExecutedAt    string
	ErrorMessage  string
	ConflictID    int64
}

// GeneratePlans creates consolidation plans from database duplicate detection
func (p *Planner) GeneratePlans(ctx context.Context) (*PlanSummary, error) {
	summary := &PlanSummary{}

	tx, err := p.db.DB().Begin()
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	if err := p.clearPendingPlansTx(tx); err != nil {
		return nil, fmt.Errorf("failed to clear old plans: %w", err)
	}

	movieDeletes, err := p.generateDuplicateMoviePlansTx(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate movie duplicate plans: %w", err)
	}
	summary.DeletePlans += movieDeletes
	summary.TotalPlans += movieDeletes

	episodeDeletes, movieGroups, episodeGroups, err := p.generateDuplicateEpisodePlansTx(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate episode duplicate plans: %w", err)
	}
	summary.DeletePlans += episodeDeletes
	summary.TotalPlans += episodeDeletes
	summary.DuplicateGroups = movieGroups + episodeGroups

	renameCount, err := p.generateNonCompliantPlansTx(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate non-compliant plans: %w", err)
	}
	summary.RenamePlans += renameCount
	summary.TotalPlans += renameCount

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit plans: %w", err)
	}

	stats, err := p.db.GetConsolidationStats()
	if err != nil {
		return nil, fmt.Errorf("failed to get consolidation stats: %w", err)
	}

	summary.SpaceToReclaim = stats.SpaceReclaimable
	summary.FilesToProcess = stats.DuplicateFiles + stats.NonCompliantFiles

	return summary, nil
}

func (p *Planner) generateDuplicateMoviePlansTx(ctx context.Context, tx *sql.Tx) (int, error) {
	duplicateGroups, err := p.db.FindDuplicateMovies()
	if err != nil {
		return 0, err
	}

	planCount := 0

	for _, group := range duplicateGroups {
		select {
		case <-ctx.Done():
			return planCount, ctx.Err()
		default:
		}

		if group.BestFile == nil || group.BestFile.QualityScore < minQualityScore {
			continue
		}

		for _, file := range group.Files {
			if file.ID == group.BestFile.ID {
				continue
			}
			if file.QualityScore >= group.BestFile.QualityScore {
				continue
			}

			reason := "Duplicate of better quality file"
			details := fmt.Sprintf("Keeping: %s (score: %d, %s %s)",
				group.BestFile.Path,
				group.BestFile.QualityScore,
				group.BestFile.Resolution,
				group.BestFile.SourceType)

			if err := p.insertPlanTx(tx, "delete", file.ID, file.Path, "", reason, details); err != nil {
				return planCount, err
			}
			planCount++
		}
	}

	return planCount, nil
}

func (p *Planner) generateDuplicateEpisodePlansTx(ctx context.Context, tx *sql.Tx) (int, int, int, error) {
	duplicateGroups, err := p.db.FindDuplicateEpisodes()
	if err != nil {
		return 0, 0, 0, err
	}

	planCount := 0
	movieGroupCount := 0
	episodeGroupCount := len(duplicateGroups)

	for _, group := range duplicateGroups {
		select {
		case <-ctx.Done():
			return planCount, movieGroupCount, episodeGroupCount, ctx.Err()
		default:
		}

		if group.BestFile == nil || group.BestFile.QualityScore < minQualityScore {
			continue
		}

		for _, file := range group.Files {
			if file.ID == group.BestFile.ID {
				continue
			}
			if file.QualityScore >= group.BestFile.QualityScore {
				continue
			}

			reason := "Duplicate of better quality file"
			details := fmt.Sprintf("Keeping: %s (score: %d, %s %s)",
				group.BestFile.Path,
				group.BestFile.QualityScore,
				group.BestFile.Resolution,
				group.BestFile.SourceType)

			if err := p.insertPlanTx(tx, "delete", file.ID, file.Path, "", reason, details); err != nil {
				return planCount, movieGroupCount, episodeGroupCount, err
			}
			planCount++
		}
	}

	return planCount, movieGroupCount, episodeGroupCount, nil
}

func (p *Planner) generateNonCompliantPlansTx(ctx context.Context, tx *sql.Tx) (int, error) {
	nonCompliantFiles, err := p.db.FindNonCompliantFiles()
	if err != nil {
		return 0, err
	}

	planCount := 0

	for _, file := range nonCompliantFiles {
		select {
		case <-ctx.Done():
			return planCount, ctx.Err()
		default:
		}

		reason := "Non-Jellyfin-compliant filename"
		details := fmt.Sprintf("Issues: %v", file.ComplianceIssues)

		if err := p.insertPlanTx(tx, "rename", file.ID, file.Path, "", reason, details); err != nil {
			return planCount, err
		}
		planCount++
	}

	return planCount, nil
}

func (p *Planner) insertPlanTx(tx *sql.Tx, action string, fileID int64, sourcePath, targetPath, reason, details string) error {
	query := `
		INSERT INTO consolidation_plans (
			action, source_file_id, source_path, target_path, reason, reason_details
		) VALUES (?, ?, ?, ?, ?, ?)
	`
	_, err := tx.Exec(query, action, fileID, sourcePath, targetPath, reason, details)
	return err
}

func (p *Planner) clearPendingPlansTx(tx *sql.Tx) error {
	_, err := tx.Exec(`DELETE FROM consolidation_plans WHERE status = 'pending'`)
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
