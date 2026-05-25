package service

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

	// Get movie duplicates
	movieGroups, err := s.db.FindDuplicateMovies()
	if err != nil {
		return nil, err
	}

	for _, mg := range movieGroups {
		if len(mg.Files) < 2 {
			continue
		}

		group := DuplicateGroup{
			ID:               generateGroupID(mg.NormalizedTitle, mg.Year, nil, nil),
			Title:            mg.NormalizedTitle,
			Year:             mg.Year,
			MediaType:        "movie",
			Files:            make([]MediaFile, len(mg.Files)),
			ReclaimableBytes: mg.SpaceReclaimable,
		}

		for i, f := range mg.Files {
			group.Files[i] = MediaFile{
				ID:           f.ID,
				Path:         f.Path,
				Size:         f.Size,
				Resolution:   f.Resolution,
				SourceType:   f.SourceType,
				QualityScore: f.QualityScore,
			}
		}

		if mg.BestFile != nil {
			group.BestFileID = mg.BestFile.ID
		}

		analysis.Groups = append(analysis.Groups, group)
		analysis.TotalFiles += len(mg.Files)
		analysis.ReclaimableBytes += mg.SpaceReclaimable
	}

	// Get episode duplicates
	episodeGroups, err := s.db.FindDuplicateEpisodes()
	if err != nil {
		return nil, err
	}

	for _, eg := range episodeGroups {
		if len(eg.Files) < 2 {
			continue
		}

		group := DuplicateGroup{
			ID:               generateGroupID(eg.NormalizedTitle, eg.Year, eg.Season, eg.Episode),
			Title:            eg.NormalizedTitle,
			Year:             eg.Year,
			MediaType:        "series",
			Season:           eg.Season,
			Episode:          eg.Episode,
			Files:            make([]MediaFile, len(eg.Files)),
			ReclaimableBytes: eg.SpaceReclaimable,
		}

		for i, f := range eg.Files {
			group.Files[i] = MediaFile{
				ID:           f.ID,
				Path:         f.Path,
				Size:         f.Size,
				Resolution:   f.Resolution,
				SourceType:   f.SourceType,
				QualityScore: f.QualityScore,
			}
		}

		if eg.BestFile != nil {
			group.BestFileID = eg.BestFile.ID
		}

		analysis.Groups = append(analysis.Groups, group)
		analysis.TotalFiles += len(eg.Files)
		analysis.ReclaimableBytes += eg.SpaceReclaimable
	}

	analysis.TotalGroups = len(analysis.Groups)
	return analysis, nil
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
	switch strings.ToLower(ext) {
	case ".mkv", ".mp4", ".avi", ".mov", ".wmv", ".flv", ".webm", ".m4v", ".mpg", ".mpeg", ".m2ts", ".ts":
		return true
	default:
		return false
	}
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
