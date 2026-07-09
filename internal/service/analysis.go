package service

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/video"
)

const minConsolidationFileSize = 100 * 1024 * 1024

// DuplicateGroup represents files that are duplicates of each other
type DuplicateGroup struct {
	ID               string // Hash of normalized title + year + episode
	Title            string
	Year             *int
	MediaType        string // "movie" or "series"
	Season           *int   // For TV
	Episode          *int   // For TV
	Files            []MediaFile
	BestFileID       int64
	ReclaimableBytes int64
}

// MediaFile represents a media file in the analysis
type MediaFile struct {
	ID           int64
	Path         string
	Size         int64
	Resolution   string
	SourceType   string
	QualityScore int
}

// ScatteredItem represents media scattered across multiple locations
type ScatteredItem struct {
	ID             int64
	Title          string
	Year           *int
	MediaType      string
	Locations      []string
	TargetLocation string
	FilesToMove    int
	BytesToMove    int64
}

// DuplicateAnalysis contains the full duplicate analysis results
type DuplicateAnalysis struct {
	Groups           []DuplicateGroup
	TotalFiles       int
	TotalGroups      int
	ReclaimableBytes int64
}

// MissingMediaPruneResult summarizes stale media rows removed from the DB.
type MissingMediaPruneResult struct {
	Checked int
	Pruned  int
}

// ScatteredAnalysis contains scattered media analysis results
type ScatteredAnalysis struct {
	Items      []ScatteredItem
	TotalItems int
	TotalMoves int
	TotalBytes int64
}

// AnalyzeDuplicates finds all duplicate media files
func (s *CleanupService) AnalyzeDuplicates() (*DuplicateAnalysis, error) {
	analysis := &DuplicateAnalysis{
		Groups: []DuplicateGroup{},
	}

	if _, err := s.PruneMissingMediaFiles(); err != nil {
		return nil, err
	}

	// Get movie duplicates
	movieGroups, err := s.db.FindDuplicateMovies()
	if err != nil {
		return nil, err
	}

	for _, mg := range movieGroups {
		files, err := s.pruneMissingMediaFiles(mg.Files)
		if err != nil {
			return nil, err
		}
		if len(files) < 2 {
			continue
		}

		group := DuplicateGroup{
			ID:        generateGroupID(mg.NormalizedTitle, mg.Year, nil, nil),
			Title:     mg.NormalizedTitle,
			Year:      mg.Year,
			MediaType: "movie",
			Files:     make([]MediaFile, len(files)),
		}

		for i, f := range files {
			group.Files[i] = MediaFile{
				ID:           f.ID,
				Path:         f.Path,
				Size:         f.Size,
				Resolution:   f.Resolution,
				SourceType:   f.SourceType,
				QualityScore: f.QualityScore,
			}
		}

		group.BestFileID = files[0].ID
		for _, f := range files[1:] {
			group.ReclaimableBytes += f.Size
		}

		analysis.Groups = append(analysis.Groups, group)
		analysis.TotalFiles += len(files)
		analysis.ReclaimableBytes += group.ReclaimableBytes
	}

	// Get episode duplicates
	episodeGroups, err := s.db.FindDuplicateEpisodes()
	if err != nil {
		return nil, err
	}

	for _, eg := range episodeGroups {
		files, err := s.pruneMissingMediaFiles(eg.Files)
		if err != nil {
			return nil, err
		}
		if len(files) < 2 {
			continue
		}

		group := DuplicateGroup{
			ID:        generateGroupID(eg.NormalizedTitle, eg.Year, eg.Season, eg.Episode),
			Title:     eg.NormalizedTitle,
			Year:      eg.Year,
			MediaType: "series",
			Season:    eg.Season,
			Episode:   eg.Episode,
			Files:     make([]MediaFile, len(files)),
		}

		for i, f := range files {
			group.Files[i] = MediaFile{
				ID:           f.ID,
				Path:         f.Path,
				Size:         f.Size,
				Resolution:   f.Resolution,
				SourceType:   f.SourceType,
				QualityScore: f.QualityScore,
			}
		}

		group.BestFileID = files[0].ID
		for _, f := range files[1:] {
			group.ReclaimableBytes += f.Size
		}

		analysis.Groups = append(analysis.Groups, group)
		analysis.TotalFiles += len(files)
		analysis.ReclaimableBytes += group.ReclaimableBytes
	}

	analysis.TotalGroups = len(analysis.Groups)
	return analysis, nil
}

func (s *CleanupService) pruneMissingMediaFiles(files []*database.MediaFile) ([]*database.MediaFile, error) {
	existing := make([]*database.MediaFile, 0, len(files))
	for _, file := range files {
		if file == nil {
			continue
		}

		info, err := os.Stat(file.Path)
		if err == nil && !info.IsDir() {
			existing = append(existing, file)
			continue
		}
		if err == nil {
			continue
		}
		if os.IsNotExist(err) {
			if err := s.db.DeleteMediaFileByID(file.ID); err != nil {
				return nil, fmt.Errorf("pruning missing media file %d: %w", file.ID, err)
			}
			continue
		}
		existing = append(existing, file)
	}
	return existing, nil
}

func (s *CleanupService) PruneMissingMediaFiles() (*MissingMediaPruneResult, error) {
	files, err := s.db.GetAllMediaFiles()
	if err != nil {
		return nil, fmt.Errorf("loading media files for missing-file repair: %w", err)
	}

	result := &MissingMediaPruneResult{Checked: len(files)}
	for _, file := range files {
		if file == nil {
			continue
		}
		info, err := os.Stat(file.Path)
		if err == nil && !info.IsDir() {
			continue
		}
		if err == nil {
			continue
		}
		if !os.IsNotExist(err) {
			continue
		}
		if err := s.db.DeleteMediaFileByID(file.ID); err != nil {
			return nil, fmt.Errorf("pruning missing media file %d: %w", file.ID, err)
		}
		result.Pruned++
	}

	return result, nil
}

// AnalyzeScattered finds media scattered across multiple locations
func (s *CleanupService) AnalyzeScattered() (*ScatteredAnalysis, error) {
	analysis := &ScatteredAnalysis{
		Items: []ScatteredItem{},
	}

	_, err := s.db.DetectConflicts()
	if err != nil {
		return nil, fmt.Errorf("failed to detect conflicts: %w", err)
	}

	conflicts, err := s.db.GetUnresolvedConflicts()
	if err != nil {
		return nil, fmt.Errorf("failed to get conflicts: %w", err)
	}

	for _, c := range conflicts {
		if c.MediaType != "series" {
			continue
		}

		locations := existingDirectories(c.Locations)
		if len(locations) < 2 {
			continue
		}

		targetLocation := chooseScatteredTarget(locations)
		item := ScatteredItem{
			ID:             c.ID,
			Title:          c.Title,
			Year:           c.Year,
			MediaType:      c.MediaType,
			Locations:      locations,
			TargetLocation: targetLocation,
		}

		for _, loc := range locations {
			if loc == targetLocation {
				continue
			}
			files, bytes := countConsolidatableFiles(loc)
			item.FilesToMove += files
			item.BytesToMove += bytes
		}
		if item.FilesToMove == 0 {
			continue
		}

		analysis.Items = append(analysis.Items, item)
		analysis.TotalMoves += item.FilesToMove
		analysis.TotalBytes += item.BytesToMove
	}

	analysis.TotalItems = len(analysis.Items)
	return analysis, nil
}

func existingDirectories(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	existing := make([]string, 0, len(paths))
	for _, path := range paths {
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			continue
		}
		info, err := os.Stat(clean)
		if err != nil || !info.IsDir() {
			continue
		}
		seen[clean] = struct{}{}
		existing = append(existing, clean)
	}
	return existing
}

func chooseScatteredTarget(locations []string) string {
	target := locations[0]
	maxFiles := -1
	for _, loc := range locations {
		files, _ := countConsolidatableFiles(loc)
		if files > maxFiles {
			maxFiles = files
			target = loc
		}
	}
	return target
}

func countConsolidatableFiles(root string) (int, int64) {
	files := 0
	var bytes int64
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if info.Size() <= minConsolidationFileSize {
			return nil
		}
		if !isScatteredMediaFile(filepath.Ext(path)) {
			return nil
		}
		files++
		bytes += info.Size()
		return nil
	})
	return files, bytes
}

func isScatteredMediaFile(ext string) bool {
	return video.IsVideoExt(ext)
}

// generateGroupID creates a unique ID for a duplicate group
func generateGroupID(title string, year *int, season, episode *int) string {
	parts := []string{strings.ToLower(title)}
	if year != nil {
		parts = append(parts, fmt.Sprintf("%d", *year))
	}
	if season != nil {
		parts = append(parts, fmt.Sprintf("s%d", *season))
	}
	if episode != nil {
		parts = append(parts, fmt.Sprintf("e%d", *episode))
	}
	hash := sha256.Sum256([]byte(strings.Join(parts, "-")))
	return fmt.Sprintf("%x", hash[:8])
}
