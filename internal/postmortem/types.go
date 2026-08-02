package postmortem

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin"
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
	RunID                         string    `json:"run_id"`
	GeneratedAt                   time.Time `json:"generated_at"`
	Since                         time.Time `json:"since"`
	ProcessedDecisions            int       `json:"processed_decisions"`
	RepairEvents                  int       `json:"repair_events"`
	SuspiciousItems               int       `json:"suspicious_items"`
	MetadataProblems              int       `json:"metadata_problems"`
	DriftLabels                   int       `json:"drift_labels"`
	FailLabels                    int       `json:"fail_labels"`
	PendingLabels                 int       `json:"pending_labels"`
	OverdueUnlabeled              int       `json:"overdue_unlabeled"`
	HousekeepingFailed            int       `json:"housekeeping_failed"` // created in window
	ManualReview                  int       `json:"manual_review"`       // created in window
	HousekeepingOutstandingFailed int       `json:"housekeeping_outstanding_failed"`
	HousekeepingOutstandingReview int       `json:"housekeeping_outstanding_review"`
	UnknownSeasonActionable       int       `json:"unknown_season_actionable"`
}

// LabelOverdueAfter is how long an unlabeled decision may wait before counting
// as overdue in the postmortem window (matches the sweeper lookback grace).
const LabelOverdueAfter = 24 * time.Hour

type HousekeepingWindowCounts struct {
	CreatedInWindow map[string]int `json:"created_in_window"`
	Outstanding     map[string]int `json:"outstanding"`
}

// SummarizeDecisionMetrics counts windowed convergence signals, deduplicating
// by decision ID so a duplicated row cannot inflate totals.
func SummarizeDecisionMetrics(decisions []*database.ParseDecision, now time.Time) Summary {
	seen := make(map[int64]struct{})
	var s Summary
	for _, d := range decisions {
		if d == nil {
			continue
		}
		if _, ok := seen[d.ID]; ok {
			continue
		}
		seen[d.ID] = struct{}{}
		s.ProcessedDecisions++

		switch strings.ToUpper(strings.TrimSpace(d.AutoLabel)) {
		case "DRIFT":
			s.DriftLabels++
		case "FAIL":
			s.FailLabels++
		case "":
			s.PendingLabels++
			if !d.EventAt.IsZero() && now.Sub(d.EventAt) > LabelOverdueAfter {
				s.OverdueUnlabeled++
			}
		}

		if isMetadataProblem(d.MetadataState) {
			s.MetadataProblems++
		}
	}
	return s
}

func isMetadataProblem(state string) bool {
	switch strings.TrimSpace(state) {
	case "", "identified", "recent_import_waiting":
		return false
	default:
		return true
	}
}

// CountHousekeepingWindow splits tasks into created-in-window vs current
// outstanding totals so cumulative backlog is not mixed with window metrics.
func CountHousekeepingWindow(tasks []database.HousekeepingTask, since time.Time, outstanding map[string]int) HousekeepingWindowCounts {
	created := make(map[string]int)
	for _, t := range tasks {
		if t.CreatedAt.Before(since) {
			continue
		}
		created[t.Status]++
	}
	out := make(map[string]int, len(outstanding))
	for k, v := range outstanding {
		out[k] = v
	}
	return HousekeepingWindowCounts{CreatedInWindow: created, Outstanding: out}
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
