package service

import (
	"fmt"
)

// Confidence levels for housekeeper auto-execution gating.
const (
	ConfidenceHigh = "high"
	ConfidenceLow  = "low"
)

// GroupConfidence classifies a duplicate group into "high" (safe to auto
// delete the inferior copies) or "low" (must be flagged for human review).
//
// A group is downgraded to "low" when ANY of:
//   - BestFileID is unset (no clear winner from quality scoring)
//   - The two top files have identical quality scores
//   - File sizes within the group differ by more than 50% AND resolution
//     is identical (suggests different cuts or rip qualities, not pure
//     duplicates: e.g. theatrical vs extended, or two different episodes
//     misclassified as the same S/E)
//   - Any file in the group has a non-positive size (record looks stale)
func (s *CleanupService) GroupConfidence(group DuplicateGroup) (level string, reasons []string) {
	if group.BestFileID == 0 || len(group.Files) < 2 {
		return ConfidenceLow, []string{"no clear best file"}
	}

	var best MediaFile
	var rest []MediaFile
	for _, f := range group.Files {
		if f.ID == group.BestFileID {
			best = f
		} else {
			rest = append(rest, f)
		}
	}
	if best.ID == 0 {
		return ConfidenceLow, []string{"best file not present in group"}
	}

	for _, f := range group.Files {
		if f.Size <= 0 {
			reasons = append(reasons, fmt.Sprintf("file id=%d has zero size", f.ID))
		}
	}

	for _, f := range rest {
		if f.QualityScore == best.QualityScore {
			// A tie is only ambiguous if the files actually differ.
			// Auto-resolve when:
			//   * files are byte-identical (same Size) — pure clones
			//   * best is overwhelmingly larger (>=10x) — the smaller
			//     is almost certainly a sample/trailer/broken artifact
			switch {
			case f.Size == best.Size && best.Size > 0:
				// byte-identical clone, safe to delete the inferior copy
			case best.Size > 0 && f.Size > 0 && best.Size >= 10*f.Size:
				// extreme size ratio, smaller looks like a sample
			default:
				reasons = append(reasons,
					fmt.Sprintf("quality_score tie with id=%d (both %d)", f.ID, best.QualityScore))
			}
		}
		if best.Resolution != "" && f.Resolution == best.Resolution {
			if sizeDivergesMaterially(best.Size, f.Size) {
				// Auto-resolve when best is the larger AND the ratio is
				// large enough to be a sample (>=10x) or a clearly
				// inferior encode (>=3x). Below 3x we keep flagging —
				// that is the band where two legitimate encodes (e.g.
				// x265 vs x264 of the same source) overlap.
				switch {
				case best.Size > f.Size && best.Size >= 3*f.Size:
					// keep best, drop the smaller copy
				default:
					reasons = append(reasons,
						fmt.Sprintf("same resolution (%s) but size differs >50%% with id=%d (%d vs %d)",
							best.Resolution, f.ID, best.Size, f.Size))
				}
			}
		}
	}

	if len(reasons) > 0 {
		return ConfidenceLow, reasons
	}
	return ConfidenceHigh, nil
}

// sizeDivergesMaterially returns true when a and b differ by more than 50%
// of the larger value. Same-resolution files in genuine duplicate groups
// should be within ~10-20% of each other; a >50% gap is a strong signal
// that they are not, in fact, the same logical content.
func sizeDivergesMaterially(a, b int64) bool {
	if a <= 0 || b <= 0 {
		return false
	}
	larger := a
	smaller := b
	if b > a {
		larger, smaller = b, a
	}
	return float64(larger-smaller)/float64(larger) > 0.5
}

// FindDuplicateGroup looks up a duplicate group by its key fields. Returns
// nil if no matching group exists in the current analysis (e.g., the group
// was already resolved by a previous run, or the underlying files were
// deleted out-of-band).
//
// Used by the housekeeping executor to re-fetch a group at execute time
// using only the lightweight payload it persists in housekeeping_tasks.
func (s *CleanupService) FindDuplicateGroup(mediaType, normalizedTitle string,
	year, season, episode *int) (*DuplicateGroup, error) {

	analysis, err := s.AnalyzeDuplicates()
	if err != nil {
		return nil, err
	}
	for i := range analysis.Groups {
		g := &analysis.Groups[i]
		if g.MediaType != mediaType || g.Title != normalizedTitle {
			continue
		}
		if !intPtrEq(g.Year, year) {
			continue
		}
		if !intPtrEq(g.Season, season) {
			continue
		}
		if !intPtrEq(g.Episode, episode) {
			continue
		}
		return g, nil
	}
	return nil, nil
}

func intPtrEq(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// ResolveDuplicateGroup deletes the inferior copies in a group, keeping
// BestFileID. Thin wrapper around DeleteDuplicateFiles so the housekeeping
// engine has a stable per-group entry point.
func (s *CleanupService) ResolveDuplicateGroup(group *DuplicateGroup) (deleted int, reclaimed int64, err error) {
	if group == nil {
		return 0, 0, fmt.Errorf("nil group")
	}
	if group.BestFileID == 0 {
		return 0, 0, fmt.Errorf("group %s has no best file", group.ID)
	}
	return s.DeleteDuplicateFiles(group.ID, group.BestFileID)
}
