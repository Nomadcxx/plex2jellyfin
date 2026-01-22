package consolidate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/database"
)

const (
	MinConsolidationFileSize = 100 * 1024 * 1024 // 100MB minimum
)

// Consolidator handles consolidation operations
type Consolidator struct {
	db    *database.MediaDB
	cfg   *config.Config
	stats Stats
}

// Stats tracks consolidation statistics
type Stats struct {
	ConflictsFound int
	PlansGenerated int
	FilesMoved     int
	BytesMoved     int64
	StartTime      time.Time
	EndTime        time.Time
}

// NewConsolidator creates a new consolidator
func NewConsolidator(db *database.MediaDB, cfg *config.Config) *Consolidator {
	return &Consolidator{
		db:    db,
		cfg:   cfg,
		stats: Stats{StartTime: time.Now()},
	}
}

// Plan represents a consolidation plan for a specific conflict
type Plan struct {
	ConflictID      int64
	MediaType       string
	Title           string
	TitleNormalized string
	Year            *int

	SourcePaths []string
	TargetPath  string

	// Files to move
	Operations []*Operation

	// Statistics
	TotalFiles int
	TotalBytes int64
	CanProceed bool
	Reasons    []string
}

// Operation represents a single file move operation
type Operation struct {
	SourcePath      string
	DestinationPath string
	Size            int64
	Type            string // "movie", "episode"
}

// GeneratePlan creates a consolidation plan for a conflict
func (c *Consolidator) GeneratePlan(conflict *database.Conflict) (*Plan, error) {
	plan := &Plan{
		ConflictID:      conflict.ID,
		MediaType:       conflict.MediaType,
		Title:           conflict.Title,
		TitleNormalized: conflict.TitleNormalized,
		Year:            conflict.Year,
		SourcePaths:     conflict.Locations,
		CanProceed:      true,
	}

	// Determine target path (choose the location with most content)
	targetPath, err := c.chooseTargetPath(conflict)
	if err != nil {
		plan.CanProceed = false
		plan.Reasons = append(plan.Reasons, fmt.Sprintf("Failed to choose target path: %v", err))
		return plan, nil
	}

	plan.TargetPath = targetPath

	// Find all files to move from other locations to target
	for _, sourcePath := range conflict.Locations {
		if sourcePath == targetPath {
			continue // Skip target location
		}

		// Get files at source location
		ops, err := c.getFilesToMove(sourcePath, targetPath, conflict)
		if err != nil {
			plan.CanProceed = false
			plan.Reasons = append(plan.Reasons, fmt.Sprintf("Failed to list files at %s: %v", sourcePath, err))
			return plan, nil
		}

		plan.Operations = append(plan.Operations, ops...)
	}

	// Calculate totals
	for _, op := range plan.Operations {
		plan.TotalFiles++
		plan.TotalBytes += op.Size
	}

	c.stats.PlansGenerated++
	return plan, nil
}

// chooseTargetPath selects the best location to consolidate into
func (c *Consolidator) chooseTargetPath(conflict *database.Conflict) (string, error) {
	if len(conflict.Locations) == 0 {
		return "", fmt.Errorf("no locations found for conflict")
	}

	// Simple heuristic: choose location with most files
	var bestPath string
	maxEpisodes := -1

	for _, path := range conflict.Locations {
		// Count files in this location
		count, err := countFilesInPath(path, conflict)
		if err != nil {
			continue
		}

		if count > maxEpisodes {
			maxEpisodes = count
			bestPath = path
		}
	}

	if bestPath == "" {
		// Fallback: first location
		bestPath = conflict.Locations[0]
	}

	return bestPath, nil
}

// getFilesToMove finds all files to move from source to target
func (c *Consolidator) getFilesToMove(sourcePath, targetPath string, conflict *database.Conflict) ([]*Operation, error) {
	var operations []*Operation

	// Walk the source directory
	err := filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil // Skip directories
		}

		// SIZE FILTER: Skip files under 100MB
		if info.Size() < MinConsolidationFileSize {
			return nil
		}

		// Check if it's a media file
		ext := strings.ToLower(filepath.Ext(path))
		if !isMediaFile(ext) {
			return nil
		}

		// Calculate destination path
		relPath, err := filepath.Rel(sourcePath, path)
		if err != nil {
			return nil
		}

		destPath := filepath.Join(targetPath, relPath)

		// Check if destination already exists
		if _, err := os.Stat(destPath); err == nil {
			// File already exists at destination, skip
			return nil
		}

		op := &Operation{
			SourcePath:      path,
			DestinationPath: destPath,
			Size:            info.Size(),
			Type:            conflict.MediaType,
		}

		operations = append(operations, op)
		return nil
	})

	return operations, err
}

// countFilesInPath counts media files in a directory
func countFilesInPath(path string, conflict *database.Conflict) (int, error) {
	count := 0

	err := filepath.Walk(path, func(subpath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(subpath))
		if isMediaFile(ext) {
			count++
		}

		return nil
	})

	return count, err
}

// isMediaFile checks if extension is a media file
func isMediaFile(ext string) bool {
	mediaExts := map[string]bool{
		".mkv":  true,
		".mp4":  true,
		".avi":  true,
		".mov":  true,
		".m4v":  true,
		".webm": true,
	}
	return mediaExts[ext]
}
