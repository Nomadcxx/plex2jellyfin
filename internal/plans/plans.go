package plans

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/paths"
	"github.com/Nomadcxx/plex2jellyfin/internal/permissions"
	"github.com/Nomadcxx/plex2jellyfin/internal/transfer"
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
	return paths.PlansDir()
}

// --- generic plan file operations ---

func savePlan[T any](plan *T, filename string) error {
	dir, err := GetPlansDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
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

func loadPlan[T any](filename string) (*T, error) {
	dir, err := GetPlansDir()
	if err != nil {
		return nil, fmt.Errorf("failed to open plans file: %w", err)
	}
	path := filepath.Join(dir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read plans file: %w", err)
	}
	var plan T
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("failed to parse plans file: %w", err)
	}
	return &plan, nil
}

func deletePlan(filename string) error {
	dir, err := GetPlansDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, filename)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete plan file: %w", err)
	}
	return nil
}

func archivePlan(filename string) error {
	dir, err := GetPlansDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, filename)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	oldPath := path + ".old"
	os.Remove(oldPath)
	if err := os.Rename(path, oldPath); err != nil {
		return fmt.Errorf("failed to archive plan file: %w", err)
	}
	return nil
}

// --- Consolidate wrappers ---

func SaveConsolidatePlans(plan *ConsolidatePlan) error       { return savePlan(plan, "consolidate.json") }
func LoadConsolidatePlans() (*ConsolidatePlan, error)        { return loadPlan[ConsolidatePlan]("consolidate.json") }
func DeleteConsolidatePlans() error                          { return deletePlan("consolidate.json") }
func ArchiveConsolidatePlans() error                         { return archivePlan("consolidate.json") }

// --- Duplicate wrappers ---

func SaveDuplicatePlans(plan *DuplicatePlan) error           { return savePlan(plan, "duplicates.json") }
func LoadDuplicatePlans() (*DuplicatePlan, error)            { return loadPlan[DuplicatePlan]("duplicates.json") }
func DeleteDuplicatePlans() error                            { return deletePlan("duplicates.json") }
func ArchiveDuplicatePlans() error                           { return archivePlan("duplicates.json") }

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
	ItemIndex  int     `json:"item_index"` // index into AuditPlan.Items for correct pairing
	Action     string  `json:"action"`     // "rename" or "delete"
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
	AICandidateCount      int `json:"ai_candidate_count"`
	AISuccessCount        int `json:"ai_success_count"`
	AIErrorCount          int `json:"ai_error_count"`
	DeterministicSkipped  int `json:"deterministic_skipped"`
	ManualReviewSkipped   int `json:"manual_review_skipped"`
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

// --- Audit wrappers ---

func SaveAuditPlans(plan *AuditPlan) error   { return savePlan(plan, "audit.json") }
func LoadAuditPlans() (*AuditPlan, error)    { return loadPlan[AuditPlan]("audit.json") }
func DeleteAuditPlans() error                { return deletePlan("audit.json") }
func ArchiveAuditPlans() error               { return archivePlan("audit.json") }

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
				return permissions.NewPermissionError(file.Path, "delete", removeErr, uid, gid)
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

	// Fast path: for normal in-place renames, os.Rename is atomic and should
	// complete in milliseconds. The generic transfer backend may copy the whole
	// file, which is only appropriate for cross-device moves.
	if err := os.MkdirAll(filepath.Dir(action.NewPath), 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}
	transferOpts := transfer.OptionsFromConfig(cfg)
	if err := os.Rename(file.Path, action.NewPath); err == nil {
		if permErr := transfer.ApplyPermissions(action.NewPath, transferOpts); permErr != nil {
			return fmt.Errorf("file renamed but permission application failed: %w", permErr)
		}
	} else {
		if !errors.Is(err, syscall.EXDEV) {
			return fmt.Errorf("failed to rename file: %w", err)
		}
		start := time.Now()
		fmt.Printf("  Cross-device rename detected; using transfer fallback...\n")

		// Perform filesystem move using transfer package (handles cross-device)
		transferer, transferErr := transfer.New(transfer.BackendAuto)
		if transferErr != nil {
			return fmt.Errorf("failed to create transferer: %w", transferErr)
		}

		result, transferErr := transferer.Move(file.Path, action.NewPath, transferOpts)
		if transferErr != nil {
			return fmt.Errorf("failed to move file: %w", transferErr)
		}
		if !result.Success {
			return fmt.Errorf("file transfer failed: %v", result.Error)
		}
		duration := result.Duration
		if duration == 0 {
			duration = time.Since(start)
		}
		fmt.Printf("  Transfer fallback completed via %s in %s (%d bytes, %d attempt(s))\n",
			transferer.Name(), duration.Round(time.Millisecond), result.BytesCopied, result.Attempts)
	}

	var originalPath string
	originalPath = file.Path

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
	if action.Confidence > 0 {
		file.Confidence = action.Confidence
		file.ParseMethod = "audit"
		file.NeedsReview = action.Confidence < 0.8
	}

	if err := db.UpdateMediaFile(file); err != nil {
		// Attempt rollback on DB failure
		if rollbackErr := os.Rename(action.NewPath, originalPath); rollbackErr != nil {
			fmt.Printf("Failed to rollback file move: %v\n", rollbackErr)
		}
		return fmt.Errorf("failed to update database: %w", err)
	}

	return nil
}
