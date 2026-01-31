package plans

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/permissions"
	"github.com/Nomadcxx/jellywatch/internal/transfer"
)

// FileInfo represents a media file in a plan
type FileInfo struct {
	ID           int64  `json:"id"`
	Path         string `json:"path"`
	Size         int64  `json:"size"`
	QualityScore int    `json:"quality_score"`
	Resolution   string `json:"resolution"`
	SourceType   string `json:"source_type"`
}

// DuplicateGroup represents a group of duplicate files
type DuplicateGroup struct {
	GroupID   string   `json:"group_id"`
	Title     string   `json:"title"`
	Year      *int     `json:"year"`
	MediaType string   `json:"media_type"`
	Season    *int     `json:"season,omitempty"`
	Episode   *int     `json:"episode,omitempty"`
	Keep      FileInfo `json:"keep"`
	Delete    FileInfo `json:"delete"`
}

// DuplicateSummary contains summary stats for duplicate plans
type DuplicateSummary struct {
	TotalGroups      int   `json:"total_groups"`
	FilesToDelete    int   `json:"files_to_delete"`
	SpaceReclaimable int64 `json:"space_reclaimable"`
}

// DuplicatePlan represents a full duplicate deletion plan
type DuplicatePlan struct {
	CreatedAt time.Time        `json:"created_at"`
	Command   string           `json:"command"`
	Summary   DuplicateSummary `json:"summary"`
	Plans     []DuplicateGroup `json:"plans"`
}

// MoveOperation represents a single file move
type MoveOperation struct {
	Action     string `json:"action"`
	SourcePath string `json:"source_path"`
	TargetPath string `json:"target_path"`
	Size       int64  `json:"size"`
}

// ConsolidateGroup represents files to consolidate for one title
type ConsolidateGroup struct {
	ConflictID     int64           `json:"conflict_id"`
	Title          string          `json:"title"`
	Year           *int            `json:"year"`
	MediaType      string          `json:"media_type"`
	TargetLocation string          `json:"target_location"`
	Operations     []MoveOperation `json:"operations"`
}

// ConsolidateSummary contains summary stats for consolidate plans
type ConsolidateSummary struct {
	TotalConflicts int   `json:"total_conflicts"`
	TotalMoves     int   `json:"total_moves"`
	TotalBytes     int64 `json:"total_bytes"`
}

// ConsolidatePlan represents a full consolidation plan
type ConsolidatePlan struct {
	CreatedAt time.Time          `json:"created_at"`
	Command   string             `json:"command"`
	Summary   ConsolidateSummary `json:"summary"`
	Plans     []ConsolidateGroup `json:"plans"`
}

// GetPlansDir returns the directory for plan files
func GetPlansDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".config", "jellywatch", "plans"), nil
}

// getConsolidatePlansPath returns the path to consolidate.json
func getConsolidatePlansPath() (string, error) {
	dir, err := GetPlansDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "consolidate.json"), nil
}

// getDuplicatePlansPath returns the path to duplicates.json
func getDuplicatePlansPath() (string, error) {
	dir, err := GetPlansDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "duplicates.json"), nil
}

// SaveConsolidatePlans saves a consolidate plan to JSON file
func SaveConsolidatePlans(plan *ConsolidatePlan) error {
	path, err := getConsolidatePlansPath()
	if err != nil {
		return err
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create plans directory: %w", err)
	}

	data, err := json.MarshalIndent(plan, "", " 	")
	if err != nil {
		return fmt.Errorf("failed to marshal plan: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write plan file: %w", err)
	}

	return nil
}

// LoadConsolidatePlans loads a consolidate plan from JSON file
func LoadConsolidatePlans() (*ConsolidatePlan, error) {
	path, err := getConsolidatePlansPath()
	if err != nil {
		return nil, fmt.Errorf("failed to open plans file: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read plans file: %w", err)
	}

	var plan ConsolidatePlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("failed to parse plans file: %w", err)
	}

	return &plan, nil
}

// DeleteConsolidatePlans removes the consolidate plans file
func DeleteConsolidatePlans() error {
	path, err := getConsolidatePlansPath()
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete plan file: %w", err)
	}

	return nil
}

// ArchiveConsolidatePlans renames consolidate.json to consolidate.json.old
func ArchiveConsolidatePlans() error {
	path, err := getConsolidatePlansPath()
	if err != nil {
		return err
	}

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil // Nothing to archive
	}

	oldPath := path + ".old"

	// Remove old archive if exists
	os.Remove(oldPath)

	// Rename current to .old
	if err := os.Rename(path, oldPath); err != nil {
		return fmt.Errorf("failed to archive plan file: %w", err)
	}

	return nil
}

// SaveDuplicatePlans saves a duplicate plan to JSON file
func SaveDuplicatePlans(plan *DuplicatePlan) error {
	path, err := getDuplicatePlansPath()
	if err != nil {
		return err
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create plans directory: %w", err)
	}

	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal plan: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write plan file: %w", err)
	}

	return nil
}

// LoadDuplicatePlans loads a duplicate plan from JSON file
func LoadDuplicatePlans() (*DuplicatePlan, error) {
	path, err := getDuplicatePlansPath()
	if err != nil {
		return nil, fmt.Errorf("failed to open plans file: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read plans file: %w", err)
	}

	var plan DuplicatePlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("failed to parse plans file: %w", err)
	}

	return &plan, nil
}

// DeleteDuplicatePlans removes the duplicate plans file
func DeleteDuplicatePlans() error {
	path, err := getDuplicatePlansPath()
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete plan file: %w", err)
	}

	return nil
}

// ArchiveDuplicatePlans renames duplicates.json to duplicates.json.old
func ArchiveDuplicatePlans() error {
	path, err := getDuplicatePlansPath()
	if err != nil {
		return err
	}

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil // Nothing to archive
	}

	oldPath := path + ".old"

	// Remove old archive if exists
	os.Remove(oldPath)

	// Rename current to .old
	if err := os.Rename(path, oldPath); err != nil {
		return fmt.Errorf("failed to archive plan file: %w", err)
	}

	return nil
}

// AuditItem represents a low-confidence file that needs review
type AuditItem struct {
	ID         int64   `json:"id"`
	Path       string  `json:"path"`
	Size       int64   `json:"size"`
	MediaType  string  `json:"media_type"`
	Title      string  `json:"title"`
	Year       *int    `json:"year"`
	Season     *int    `json:"season,omitempty"`
	Episode    *int    `json:"episode,omitempty"`
	Confidence float64 `json:"confidence"`
	Resolution string  `json:"resolution,omitempty"`
	SourceType string  `json:"source_type,omitempty"`
	SkipReason string  `json:"skip_reason,omitempty"`
}

// AuditAction represents an AI-suggested correction
type AuditAction struct {
	Action     string  `json:"action"` // "rename" or "delete"
	NewTitle   string  `json:"new_title,omitempty"`
	NewYear    *int    `json:"new_year,omitempty"`
	NewSeason  *int    `json:"new_season,omitempty"`
	NewEpisode *int    `json:"new_episode,omitempty"`
	NewPath    string  `json:"new_path,omitempty"`
	Reasoning  string  `json:"reasoning,omitempty"`
	Confidence float64 `json:"confidence"`
}

// AuditSummary contains summary stats for audit plans
type AuditSummary struct {
	TotalFiles    int     `json:"total_files"`
	FilesToRename int     `json:"files_to_rename"`
	FilesToDelete int     `json:"files_to_delete"`
	FilesToSkip   int     `json:"files_to_skip"`
	AvgConfidence float64 `json:"avg_confidence"`

	// AI processing statistics
	AITotalCalls          int `json:"ai_total_calls"`
	AISuccessCount        int `json:"ai_success_count"`
	AIErrorCount          int `json:"ai_error_count"`
	TypeMismatchesSkipped int `json:"type_mismatches_skipped"`
	ConfidenceTooLow      int `json:"confidence_too_low"`
	TitleUnchanged        int `json:"title_unchanged"`
}

// AuditPlan represents a full audit plan
type AuditPlan struct {
	CreatedAt time.Time     `json:"created_at"`
	Command   string        `json:"command"`
	Summary   AuditSummary  `json:"summary"`
	Items     []AuditItem   `json:"items"`
	Actions   []AuditAction `json:"actions,omitempty"`
}

// getAuditPlansPath returns path to audit.json
func getAuditPlansPath() (string, error) {
	dir, err := GetPlansDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "audit.json"), nil
}

// SaveAuditPlans saves an audit plan to JSON file
func SaveAuditPlans(plan *AuditPlan) error {
	path, err := getAuditPlansPath()
	if err != nil {
		return err
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create plans directory: %w", err)
	}

	data, err := json.MarshalIndent(plan, "", "	")
	if err != nil {
		return fmt.Errorf("failed to marshal plan: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write plan file: %w", err)
	}

	return nil
}

// LoadAuditPlans loads an audit plan from JSON file
func LoadAuditPlans() (*AuditPlan, error) {
	path, err := getAuditPlansPath()
	if err != nil {
		return nil, fmt.Errorf("failed to open plans file: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read plans file: %w", err)
	}

	var plan AuditPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("failed to parse plans file: %w", err)
	}

	return &plan, nil
}

// DeleteAuditPlans removes the audit plans file
func DeleteAuditPlans() error {
	path, err := getAuditPlansPath()
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete plan file: %w", err)
	}

	return nil
}

// ExecuteAuditAction executes an audit action (rename or delete) on a file
func ExecuteAuditAction(db *database.MediaDB, item AuditItem, action AuditAction, dryRun bool, cfg *config.Config) error {
	switch action.Action {
	case "rename":
		return executeRename(db, item, action, dryRun, cfg)
	case "delete":
		return executeDelete(db, item, action, dryRun, cfg)
	default:
		return fmt.Errorf("unknown action: %s", action.Action)
	}
}

// executeDelete performs a delete operation on a media file
func executeDelete(db *database.MediaDB, item AuditItem, action AuditAction, dryRun bool, cfg *config.Config) error {
	// Get the current file record
	file, err := db.GetMediaFileByID(item.ID)
	if err != nil {
		return fmt.Errorf("failed to get media file: %w", err)
	}
	if file == nil {
		return fmt.Errorf("media file not found: %d", item.ID)
	}

	// In dry-run mode, just print what would happen
	if dryRun {
		fmt.Printf("[DRY RUN] Would delete: %s\n", file.Path)
		return nil
	}

	// Check if file exists before attempting deletion
	if _, err := os.Stat(file.Path); os.IsNotExist(err) {
		// File already gone - clean up database and return success
		_ = db.DeleteMediaFileByID(file.ID)
		return nil
	}

	// Check if we can delete the file (using uid/gid = -1 to not change ownership)
	canDelete, err := permissions.CanDelete(file.Path)
	if err != nil {
		return fmt.Errorf("failed to check permissions: %w", err)
	}

	if !canDelete {
		var uid, gid int = -1, -1
		if cfg != nil && cfg.Permissions.WantsOwnership() {
			uid, _ = cfg.Permissions.ResolveUID()
			gid, _ = cfg.Permissions.ResolveGID()
		}
		if err := permissions.FixPermissions(file.Path, uid, gid); err != nil {
			if removeErr := os.Remove(file.Path); removeErr != nil {
				_ = db.DeleteMediaFileByID(file.ID)
				return fmt.Errorf("permission denied (tried chmod but failed: %v): %w", err, removeErr)
			}
		}
	}

	if err := os.Remove(file.Path); err != nil {
		_ = db.DeleteMediaFileByID(file.ID)
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to delete file: %w", err)
	}

	if err := db.DeleteMediaFileByID(file.ID); err != nil {
		fmt.Printf("Warning: File deleted but database cleanup failed: %v\n", err)
	}

	return nil
}

// executeRename performs a rename operation on a media file
func executeRename(db *database.MediaDB, item AuditItem, action AuditAction, dryRun bool, cfg *config.Config) error {
	// Get the current file record
	file, err := db.GetMediaFileByID(item.ID)
	if err != nil {
		return fmt.Errorf("failed to get media file: %w", err)
	}
	if file == nil {
		return fmt.Errorf("media file not found: %d", item.ID)
	}

	// In dry-run mode, just print what would happen
	if dryRun {
		fmt.Printf("[DRY RUN] Would rename: %s -> %s\n", file.Path, action.NewPath)
		return nil
	}

	// Perform filesystem move using transfer package (handles cross-device)
	transferer, err := transfer.New(transfer.BackendAuto)
	if err != nil {
		return fmt.Errorf("failed to create transferer: %w", err)
	}

	result, err := transferer.Move(file.Path, action.NewPath, transfer.DefaultOptions())
	if err != nil {
		return fmt.Errorf("failed to move file: %w", err)
	}
	if !result.Success {
		return fmt.Errorf("file transfer failed: %v", result.Error)
	}

	// Update database record
	file.Path = action.NewPath
	file.NormalizedTitle = action.NewTitle
	if action.NewYear != nil {
		file.Year = action.NewYear
	}
	if action.NewSeason != nil {
		file.Season = action.NewSeason
	}
	if action.NewEpisode != nil {
		file.Episode = action.NewEpisode
	}

	if err := db.UpdateMediaFile(file); err != nil {
		// Attempt rollback on DB failure
		rollbackResult, rollbackErr := transferer.Move(action.NewPath, file.Path, transfer.DefaultOptions())
		if rollbackErr != nil {
			fmt.Printf("Failed to rollback file move: %v\n", rollbackErr)
		} else if !rollbackResult.Success {
			fmt.Printf("Rollback transfer failed: %v\n", rollbackResult.Error)
		}
		return fmt.Errorf("failed to update database: %w", err)
	}

	return nil
}

// ArchiveAuditPlans renames audit.json to audit.json.old
func ArchiveAuditPlans() error {
	path, err := getAuditPlansPath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil // Nothing to archive
	}

	oldPath := path + ".old"

	os.Remove(oldPath)

	if err := os.Rename(path, oldPath); err != nil {
		return fmt.Errorf("failed to archive plan file: %w", err)
	}

	return nil
}
