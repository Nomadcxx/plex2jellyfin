package postmortem

import (
	"strings"
	"testing"
)

func TestMarkdownReportIncludesCountsAndNextSteps(t *testing.T) {
	s := Summary{
		RunID:                   "2026-06-19T0200",
		ProcessedDecisions:      42,
		RepairEvents:            2,
		SuspiciousItems:         3,
		HousekeepingFailed:      21,
		ManualReview:            136,
		UnknownSeasonActionable: 5,
	}
	report := MarkdownReport(s, []SuspiciousItem{
		{Category: "polluted_name", Name: "Ratatouille RoDubbed (2007)", Marker: "RoDubbed"},
	}, UnknownSeasonEvidence{ActionablePollutionEpisodes: 5})
	for _, want := range []string{
		"# JellyWatch Postmortem",
		"Processed decisions: 42",
		"Repair events: 2",
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
