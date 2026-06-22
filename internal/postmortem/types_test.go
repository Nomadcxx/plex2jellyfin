package postmortem

import "testing"

func TestReportBundlePathUsesTimestampAndLatest(t *testing.T) {
	root := "/tmp/jellywatch/reports"
	runID := "2026-06-19T0200"

	paths := NewBundlePaths(root, runID)

	if paths.Dir != "/tmp/jellywatch/reports/2026-06-19T0200" {
		t.Fatalf("Dir = %q", paths.Dir)
	}
	if paths.LatestLink != "/tmp/jellywatch/reports/latest" {
		t.Fatalf("LatestLink = %q", paths.LatestLink)
	}
	if paths.File("summary.json") != "/tmp/jellywatch/reports/2026-06-19T0200/summary.json" {
		t.Fatalf("summary path = %q", paths.File("summary.json"))
	}
}
