package postmortem

import (
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
)

func TestReportBundlePathUsesTimestampAndLatest(t *testing.T) {
	root := "/tmp/plex2jellyfin/reports"
	runID := "2026-06-19T0200"

	paths := NewBundlePaths(root, runID)

	if paths.Dir != "/tmp/plex2jellyfin/reports/2026-06-19T0200" {
		t.Fatalf("Dir = %q", paths.Dir)
	}
	if paths.LatestLink != "/tmp/plex2jellyfin/reports/latest" {
		t.Fatalf("LatestLink = %q", paths.LatestLink)
	}
	if paths.File("summary.json") != "/tmp/plex2jellyfin/reports/2026-06-19T0200/summary.json" {
		t.Fatalf("summary path = %q", paths.File("summary.json"))
	}
}

func TestSummarizeDecisionMetricsSeparatesWindowedCountsAndDedupesByID(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	decisions := []*database.ParseDecision{
		{ID: 1, EventAt: now.Add(-time.Hour), AutoLabel: "DRIFT"},
		{ID: 2, EventAt: now.Add(-time.Hour), AutoLabel: "FAIL"},
		{ID: 3, EventAt: now.Add(-time.Hour), MetadataState: "series_identified_episode_stale"},
		{ID: 4, EventAt: now.Add(-2 * time.Hour), AutoLabel: ""},  // pending, not yet overdue
		{ID: 5, EventAt: now.Add(-30 * time.Hour), AutoLabel: ""}, // overdue unlabeled (>24h)
		{ID: 6, EventAt: now.Add(-time.Hour), MetadataState: "identified", AutoLabel: "PASS"},
		{ID: 7, EventAt: now.Add(-time.Hour), MetadataState: "missing_provider_ids"},
		{ID: 7, EventAt: now.Add(-time.Hour), MetadataState: "missing_provider_ids"},  // duplicate ID
		{ID: 8, EventAt: now.Add(-time.Hour), MetadataState: "recent_import_waiting"}, // not a problem
		nil,
	}

	got := SummarizeDecisionMetrics(decisions, now)

	if got.ProcessedDecisions != 8 {
		t.Fatalf("ProcessedDecisions = %d, want 8 unique IDs", got.ProcessedDecisions)
	}
	if got.DriftLabels != 1 {
		t.Fatalf("DriftLabels = %d, want 1", got.DriftLabels)
	}
	if got.FailLabels != 1 {
		t.Fatalf("FailLabels = %d, want 1", got.FailLabels)
	}
	if got.MetadataProblems != 2 {
		t.Fatalf("MetadataProblems = %d, want 2 (stale + missing_provider, deduped)", got.MetadataProblems)
	}
	if got.PendingLabels != 5 {
		t.Fatalf("PendingLabels = %d, want 5 (unlabeled ids 3,4,5,7,8)", got.PendingLabels)
	}
	if got.OverdueUnlabeled != 1 {
		t.Fatalf("OverdueUnlabeled = %d, want 1 (id 5)", got.OverdueUnlabeled)
	}
}

func TestCountHousekeepingWindowSeparatesCreatedAndOutstanding(t *testing.T) {
	since := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	tasks := []database.HousekeepingTask{
		{ID: 1, Status: database.TaskStatusFlagged, CreatedAt: since.Add(time.Hour)},
		{ID: 2, Status: database.TaskStatusFlagged, CreatedAt: since.Add(-24 * time.Hour)}, // outstanding only
		{ID: 3, Status: database.TaskStatusFailed, CreatedAt: since.Add(2 * time.Hour)},
		{ID: 4, Status: database.TaskStatusDone, CreatedAt: since.Add(3 * time.Hour)},
		{ID: 5, Status: database.TaskStatusFailed, CreatedAt: since.Add(-48 * time.Hour)},
	}
	outstanding := map[string]int{
		database.TaskStatusFlagged: 40,
		database.TaskStatusFailed:  5,
		database.TaskStatusDone:    13,
	}

	got := CountHousekeepingWindow(tasks, since, outstanding)

	if got.CreatedInWindow[database.TaskStatusFlagged] != 1 {
		t.Fatalf("created flagged = %d, want 1", got.CreatedInWindow[database.TaskStatusFlagged])
	}
	if got.CreatedInWindow[database.TaskStatusFailed] != 1 {
		t.Fatalf("created failed = %d, want 1", got.CreatedInWindow[database.TaskStatusFailed])
	}
	if got.Outstanding[database.TaskStatusFlagged] != 40 {
		t.Fatalf("outstanding flagged = %d, want 40", got.Outstanding[database.TaskStatusFlagged])
	}
	if got.Outstanding[database.TaskStatusFailed] != 5 {
		t.Fatalf("outstanding failed = %d, want 5", got.Outstanding[database.TaskStatusFailed])
	}
}
