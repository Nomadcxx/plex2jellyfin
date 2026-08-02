package postmortem

import (
	"strings"
	"testing"
)

func TestMarkdownReportIncludesCountsAndNextSteps(t *testing.T) {
	s := Summary{
		RunID:                         "2026-06-19T0200",
		ProcessedDecisions:            42,
		RepairEvents:                  2,
		SuspiciousItems:               3,
		MetadataProblems:              13,
		DriftLabels:                   1,
		FailLabels:                    2,
		PendingLabels:                 4,
		OverdueUnlabeled:              1,
		HousekeepingFailed:            1,
		ManualReview:                  2,
		HousekeepingOutstandingFailed: 5,
		HousekeepingOutstandingReview: 78,
		UnknownSeasonActionable:       5,
	}
	report := MarkdownReport(s, []SuspiciousItem{
		{Category: "polluted_name", Name: "Ratatouille RoDubbed (2007)", Marker: "RoDubbed"},
	}, UnknownSeasonEvidence{ActionablePollutionEpisodes: 5})
	for _, want := range []string{
		"# Plex2Jellyfin Postmortem",
		"Processed decisions: 42",
		"Repair events: 2",
		"Metadata problems: 13",
		"DRIFT labels: 1",
		"FAIL labels: 2",
		"Pending labels: 4",
		"Overdue unlabeled: 1",
		"Housekeeping failed (created in window): 1",
		"Manual review (created in window): 2",
		"Housekeeping failed (outstanding): 5",
		"Manual review (outstanding): 78",
		"Season Unknown actionable: 5",
		"Ratatouille RoDubbed",
		"Unknown Season Pollution",
		"Recommended review",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}
