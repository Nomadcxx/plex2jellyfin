package postmortem

import (
	"fmt"
	"strings"
)

func MarkdownReport(s Summary, suspicious []SuspiciousItem) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# JellyWatch Postmortem")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "Run: %s\n\n", s.RunID)
	fmt.Fprintln(&b, "## Summary")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "- Processed decisions: %d\n", s.ProcessedDecisions)
	fmt.Fprintf(&b, "- Repair events: %d\n", s.RepairEvents)
	fmt.Fprintf(&b, "- Suspicious items: %d\n", s.SuspiciousItems)
	fmt.Fprintf(&b, "- Housekeeping failed: %d\n", s.HousekeepingFailed)
	fmt.Fprintf(&b, "- Manual review: %d\n\n", s.ManualReview)

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

	fmt.Fprintln(&b, "## Recommended review")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Read `agent-prompt.md` and inspect the JSON evidence before changing media files.")
	return b.String()
}
