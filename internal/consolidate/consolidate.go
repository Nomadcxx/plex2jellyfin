package consolidate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/identity"
	"github.com/Nomadcxx/jellywatch/internal/video"
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
	ConflictsFound   int
	SkippedConflicts int // Conflicts skipped (e.g., movies in consolidation mode)
	PlansGenerated   int
	FilesMoved       int
	BytesMoved       int64
	StartTime        time.Time
	EndTime          time.Time
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
	Collisions []*Collision

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

// Collision represents a source file that cannot be consolidated because the
// computed destination already exists.
type Collision struct {
	SourcePath      string
	DestinationPath string
	Size            int64
	Type            string
}

// GeneratePlan creates a consolidation plan for a conflict
func (c *Consolidator) GeneratePlan(conflict *database.Conflict) (*Plan, error) {
	plan := &Plan{
		ConflictID:      conflict.ID,
		MediaType:       conflict.MediaType,
		Title:           conflict.Title,
		TitleNormalized: conflict.TitleNormalized,
		Year:            conflict.Year,
		CanProceed:      true,
	}
	planningConflict := *conflict
	planningConflict.Locations = c.normalizedConflictLocations(conflict)
	plan.SourcePaths = planningConflict.Locations
	if reasons := c.seriesIdentitySafetyReasons(&planningConflict); len(reasons) > 0 {
		plan.CanProceed = false
		plan.Reasons = append(plan.Reasons, reasons...)
		return plan, nil
	}

	// Determine target path (choose the location with most content)
	targetPath, err := c.chooseTargetPath(&planningConflict)
	if err != nil {
		plan.CanProceed = false
		plan.Reasons = append(plan.Reasons, fmt.Sprintf("Failed to choose target path: %v", err))
		return plan, nil
	}

	plan.TargetPath = targetPath

	// Find all files to move from other locations to target
	for _, sourcePath := range planningConflict.Locations {
		if sourcePath == targetPath || pathIsWithin(targetPath, sourcePath) {
			continue // Skip target location
		}

		// Get files at source location
		ops, collisions, err := c.getFilesToMove(sourcePath, targetPath, &planningConflict)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			plan.CanProceed = false
			plan.Reasons = append(plan.Reasons, fmt.Sprintf("Failed to list files at %s: %v", sourcePath, err))
			return plan, nil
		}

		plan.Operations = append(plan.Operations, ops...)
		plan.Collisions = append(plan.Collisions, collisions...)
	}

	// Calculate totals
	for _, op := range plan.Operations {
		plan.TotalFiles++
		plan.TotalBytes += op.Size
	}
	if plan.TotalFiles == 0 {
		plan.CanProceed = false
		plan.Reasons = append(plan.Reasons, "No files to move")
	}
	if len(plan.Collisions) > 0 {
		plan.CanProceed = false
		plan.Reasons = append(plan.Reasons, fmt.Sprintf("%d files already exist at target; resolve duplicates before consolidation", len(plan.Collisions)))
	}

	c.stats.PlansGenerated++
	return plan, nil
}

func (c *Consolidator) seriesIdentitySafetyReasons(conflict *database.Conflict) []string {
	if conflict.MediaType != "series" || len(conflict.Locations) < 2 {
		return nil
	}

	ids := make([]identity.SeriesIdentity, 0, len(conflict.Locations))
	for _, location := range conflict.Locations {
		ids = append(ids, identity.SeriesIdentity{Path: location})
	}

	var reasons []string
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			decision := identity.CompareSeries(ids[i], ids[j])
			if decision.Verdict == identity.VerdictSame {
				continue
			}
			reasons = append(reasons, fmt.Sprintf(
				"identity safety: %s <-> %s: %s",
				ids[i].Path,
				ids[j].Path,
				strings.Join(decision.Reasons, "; "),
			))
		}
	}
	return reasons
}

func (c *Consolidator) normalizedConflictLocations(conflict *database.Conflict) []string {
	if conflict.MediaType != "series" {
		return cleanUniquePaths(conflict.Locations)
	}

	locations := make([]string, 0, len(conflict.Locations))
	seen := make(map[string]struct{}, len(conflict.Locations))
	for _, location := range conflict.Locations {
		if isJellywatchQuarantinePath(location) {
			continue
		}
		root := c.seriesRootForPath(location)
		if root == "" || isJellywatchQuarantinePath(root) {
			continue
		}
		root = filepath.Clean(root)
		if _, ok := seen[root]; ok {
			continue
		}
		seen[root] = struct{}{}
		locations = append(locations, root)
	}
	return locations
}

func cleanUniquePaths(paths []string) []string {
	cleaned := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		cleaned = append(cleaned, path)
	}
	return cleaned
}

func (c *Consolidator) seriesRootForPath(path string) string {
	clean := filepath.Clean(path)
	if c != nil && c.cfg != nil {
		for _, libraryRoot := range c.cfg.Libraries.TV {
			root := filepath.Clean(libraryRoot)
			rel, err := filepath.Rel(root, clean)
			if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
				continue
			}
			parts := strings.Split(rel, string(filepath.Separator))
			if len(parts) > 0 && parts[0] != "" {
				return filepath.Join(root, parts[0])
			}
		}
	}

	for current := clean; current != "." && current != string(filepath.Separator); current = filepath.Dir(current) {
		if isSeasonDirectoryName(filepath.Base(current)) {
			return filepath.Dir(current)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return clean
}

func isJellywatchQuarantinePath(path string) bool {
	for _, part := range strings.Split(filepath.Clean(path), string(filepath.Separator)) {
		if strings.HasPrefix(part, "_jellywatch_quarantine") {
			return true
		}
	}
	return false
}

func isSeasonDirectoryName(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	return strings.HasPrefix(lower, "season ") ||
		strings.HasPrefix(lower, "season_") ||
		strings.HasPrefix(lower, "season-")
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
func (c *Consolidator) getFilesToMove(sourcePath, targetPath string, conflict *database.Conflict) ([]*Operation, []*Collision, error) {
	var operations []*Operation
	var collisions []*Collision

	// Walk the source directory
	err := filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil // Skip directories
		}

		// SIZE FILTER: Skip files under or equal to 100MB
		if info.Size() <= MinConsolidationFileSize {
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
			collisions = append(collisions, &Collision{
				SourcePath:      path,
				DestinationPath: destPath,
				Size:            info.Size(),
				Type:            conflict.MediaType,
			})
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

	return operations, collisions, err
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

func isMediaFile(ext string) bool {
	return video.IsVideoExt(ext)
}
