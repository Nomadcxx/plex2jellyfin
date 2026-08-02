package postmortem

import (
	"fmt"
	"strings"
)

func MarkdownReport(s Summary, suspicious []SuspiciousItem, unknownSeasons UnknownSeasonEvidence) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# Plex2Jellyfin Postmortem")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "Run: %s\n\n", s.RunID)
	fmt.Fprintln(&b, "## Summary")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "- Processed decisions: %d\n", s.ProcessedDecisions)
	fmt.Fprintf(&b, "- Repair events: %d\n", s.RepairEvents)
	fmt.Fprintf(&b, "- Suspicious items: %d\n", s.SuspiciousItems)
	fmt.Fprintf(&b, "- Metadata problems: %d\n", s.MetadataProblems)
	fmt.Fprintf(&b, "- DRIFT labels: %d\n", s.DriftLabels)
	fmt.Fprintf(&b, "- FAIL labels: %d\n", s.FailLabels)
	fmt.Fprintf(&b, "- Pending labels: %d\n", s.PendingLabels)
	fmt.Fprintf(&b, "- Overdue unlabeled: %d\n", s.OverdueUnlabeled)
	fmt.Fprintf(&b, "- Housekeeping failed (created in window): %d\n", s.HousekeepingFailed)
	fmt.Fprintf(&b, "- Manual review (created in window): %d\n", s.ManualReview)
	fmt.Fprintf(&b, "- Housekeeping failed (outstanding): %d\n", s.HousekeepingOutstandingFailed)
	fmt.Fprintf(&b, "- Manual review (outstanding): %d\n\n", s.HousekeepingOutstandingReview)
	fmt.Fprintf(&b, "- Season Unknown actionable: %d\n\n", s.UnknownSeasonActionable)

	fmt.Fprintln(&b, "## Suspicious Items")
	fmt.Fprintln(&b)
	if len(suspicious) == 0 {
		fmt.Fprintln(&b, "No suspicious items found.")
		fmt.Fprintln(&b)
	} else {
		for _, item := range suspicious {
			fmt.Fprintf(&b, "- [%s] %s", item.Category, item.Name)
			if item.Marker != "" {
				fmt.Fprintf(&b, " marker=%s", item.Marker)
			}
			if item.Path != "" {
				fmt.Fprintf(&b, " path=%s", item.Path)
			}
			fmt.Fprintln(&b)
		}
		fmt.Fprintln(&b)
	}

	fmt.Fprintln(&b, "## Unknown Season Pollution")
	fmt.Fprintln(&b)
	if unknownSeasons.Error != "" {
		fmt.Fprintf(&b, "Unknown-season audit unavailable: %s\n\n", unknownSeasons.Error)
	} else if unknownSeasons.ActionablePollutionEpisodes == 0 {
		fmt.Fprintln(&b, "No actionable Season Unknown pollution found.")
		fmt.Fprintln(&b)
	} else {
		fmt.Fprintf(&b, "- Refresh candidates: %d seasons / %d episodes\n", unknownSeasons.RefreshCandidateSeasons, unknownSeasons.RefreshCandidateEpisodes)
		fmt.Fprintf(&b, "- Randomish/obfuscated basenames: %d episodes\n", unknownSeasons.RandomishBasenameEpisodes)
		fmt.Fprintf(&b, "- Actionable total: %d episodes\n\n", unknownSeasons.ActionablePollutionEpisodes)
	}

	fmt.Fprintln(&b, "## Recommended review")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Read `agent-prompt.md` and inspect the JSON evidence before changing media files.")
	return b.String()
}
