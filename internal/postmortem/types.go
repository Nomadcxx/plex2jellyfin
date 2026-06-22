package postmortem

import (
	"path/filepath"
	"time"
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
	RunID              string    `json:"run_id"`
	GeneratedAt        time.Time `json:"generated_at"`
	Since              time.Time `json:"since"`
	ProcessedDecisions int       `json:"processed_decisions"`
	RepairEvents       int       `json:"repair_events"`
	SuspiciousItems    int       `json:"suspicious_items"`
	HousekeepingFailed int       `json:"housekeeping_failed"`
	ManualReview       int       `json:"manual_review"`
}
