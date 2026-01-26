package plans

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
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
	GroupID   string    `json:"group_id"`
	Title     string    `json:"title"`
	Year      *int      `json:"year"`
	MediaType string    `json:"media_type"`
	Season    *int      `json:"season,omitempty"`
	Episode   *int      `json:"episode,omitempty"`
	Keep      FileInfo  `json:"keep"`
	Delete    FileInfo  `json:"delete"`
}

// DuplicateSummary contains summary stats for duplicate plans
type DuplicateSummary struct {
	TotalGroups      int   `json:"total_groups"`
	FilesToDelete    int   `json:"files_to_delete"`
	SpaceReclaimable int64 `json:"space_reclaimable"`
}

// DuplicatePlan represents the full duplicate deletion plan
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

// ConsolidatePlan represents the full consolidation plan
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

// ensurePlansDir creates the plans directory if it doesn't exist
func ensurePlansDir() error {
	dir, err := GetPlansDir()
	if err != nil {
		return err
	}
	return os.MkdirAll(dir, 0755)
}

// getDuplicatePlansPath returns the path to duplicates.json
func getDuplicatePlansPath() (string, error) {
	dir, err := GetPlansDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "duplicates.json"), nil
}

// getConsolidatePlansPath returns the path to consolidate.json
func getConsolidatePlansPath() (string, error) {
	dir, err := GetPlansDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "consolidate.json"), nil
}
