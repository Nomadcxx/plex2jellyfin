package postmortem

import (
	"path/filepath"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/jellyfin"
)

const TimestampLayout = "2006-01-02T1504"

type BundlePaths struct {
	Root       string
	RunID      string
	Dir        string
	LatestLink string
}

func NewBundlePaths(root, runID string) BundlePaths {
	return BundlePaths{
		Root:       root,
		RunID:      runID,
		Dir:        filepath.Join(root, runID),
		LatestLink: filepath.Join(root, "latest"),
	}
}

func (p BundlePaths) File(name string) string {
	return filepath.Join(p.Dir, name)
}

func RunID(t time.Time) string {
	return t.Format(TimestampLayout)
}

type Summary struct {
	RunID                   string    `json:"run_id"`
	GeneratedAt             time.Time `json:"generated_at"`
	Since                   time.Time `json:"since"`
	ProcessedDecisions      int       `json:"processed_decisions"`
	RepairEvents            int       `json:"repair_events"`
	SuspiciousItems         int       `json:"suspicious_items"`
	HousekeepingFailed      int       `json:"housekeeping_failed"`
	ManualReview            int       `json:"manual_review"`
	UnknownSeasonActionable int       `json:"unknown_season_actionable"`
}

type UnknownSeasonEvidence struct {
	UserID                      string                        `json:"user_id,omitempty"`
	Total                       int                           `json:"total"`
	RefreshRepairableSeasons    int                           `json:"refresh_repairable_seasons"`
	RefreshRepairableEpisodes   int                           `json:"refresh_repairable_episodes"`
	RefreshCandidateSeasons     int                           `json:"refresh_candidate_seasons"`
	RefreshCandidateEpisodes    int                           `json:"refresh_candidate_episodes"`
	RandomishBasenameEpisodes   int                           `json:"randomish_basename_episodes"`
	ActionablePollutionEpisodes int                           `json:"actionable_pollution_episodes"`
	FolderContext               int                           `json:"folder_context"`
	MixedReview                 int                           `json:"mixed_review"`
	ManualUnknown               int                           `json:"manual_unknown"`
	Empty                       int                           `json:"empty"`
	Indexed                     int                           `json:"indexed"`
	Issues                      []jellyfin.UnknownSeasonIssue `json:"issues,omitempty"`
	Error                       string                        `json:"error,omitempty"`
}
