package service

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

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
		item := ScatteredItem{
			ID:        c.ID,
			Title:     c.Title,
			Year:      c.Year,
			MediaType: c.MediaType,
			Locations: c.Locations,
		}

		// Determine target location (one with most files)
		if len(c.Locations) > 0 {
			item.TargetLocation = c.Locations[0] // Default to first
			// Could enhance to pick location with most files
		}

		// Count files to move (from non-target locations)
		for _, loc := range c.Locations {
			if loc != item.TargetLocation {
				// Count would need filesystem access or DB query
				item.FilesToMove++ // Simplified: 1 per location for now
			}
		}

		analysis.Items = append(analysis.Items, item)
	}

	analysis.TotalItems = len(analysis.Items)
	return analysis, nil
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
