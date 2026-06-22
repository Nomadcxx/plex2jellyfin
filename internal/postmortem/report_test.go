package postmortem

import (
	"strings"
	"testing"
)

func TestMarkdownReportIncludesCountsAndNextSteps(t *testing.T) {
	s := Summary{
		RunID:              "2026-06-19T0200",
		ProcessedDecisions: 42,
		RepairEvents:       2,
		SuspiciousItems:    3,
		HousekeepingFailed: 21,
		ManualReview:       136,
	}
	report := MarkdownReport(s, []SuspiciousItem{
		{Category: "polluted_name", Name: "Ratatouille RoDubbed (2007)", Marker: "RoDubbed"},
	})
	for _, want := range []string{
		"# JellyWatch Postmortem",
		"Processed decisions: 42",
		"Repair events: 2",
		"Ratatouille RoDubbed",
		"Recommended review",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}
