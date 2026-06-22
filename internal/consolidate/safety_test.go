package consolidate

import (
	"strings"
	"testing"
)

func TestSafetyIssuesDetectsSourceInsideTargetAndSeasonFlattening(t *testing.T) {
	plan := &Plan{
		TargetPath: "/library/Show (2020)",
		Operations: []*Operation{
			{
				SourcePath:      "/library/Show (2020)/Season 01/Release/hash.mkv",
				DestinationPath: "/library/Show (2020)/hash.mkv",
			},
		},
	}

	issues := SafetyIssues(plan)
	if !hasIssue(issues, "already under target") {
		t.Fatalf("expected source-inside-target issue, got %#v", issues)
	}
	if !hasIssue(issues, "drop season structure") {
		t.Fatalf("expected season flattening issue, got %#v", issues)
	}
}

func TestSafetyIssuesDetectsDuplicateSource(t *testing.T) {
	plan := &Plan{
		TargetPath: "/library/Show (2020)",
		Operations: []*Operation{
			{SourcePath: "/other/Show (2020)/Season 01/file.mkv", DestinationPath: "/library/Show (2020)/Season 01/file.mkv"},
			{SourcePath: "/other/Show (2020)/Season 01/file.mkv", DestinationPath: "/library/Show (2020)/Season 02/file.mkv"},
		},
	}

	if issues := SafetyIssues(plan); !hasIssue(issues, "duplicate source") {
		t.Fatalf("expected duplicate source issue, got %#v", issues)
	}
}

func TestDBPlanSafetyIssueBlocksSampleAndSeasonFlattening(t *testing.T) {
	sample := &ConsolidationPlan{
		Action:     "move",
		SourcePath: "/library/Show (2020)/Sample/sample.mkv",
		TargetPath: "/library/Show (2020)/sample.mkv",
	}
	if reason := DBPlanSafetyIssue(sample); !strings.Contains(reason, "sample/extra") {
		t.Fatalf("sample reason = %q, want sample/extra", reason)
	}

	flatten := &ConsolidationPlan{
		Action:     "move",
		SourcePath: "/library-a/Show (2020)/Season 01/file.mkv",
		TargetPath: "/library-b/Show (2020)/file.mkv",
	}
	if reason := DBPlanSafetyIssue(flatten); !strings.Contains(reason, "season structure") {
		t.Fatalf("flatten reason = %q, want season structure", reason)
	}
}

func hasIssue(issues []string, needle string) bool {
	for _, issue := range issues {
		if strings.Contains(issue, needle) {
			return true
		}
	}
	return false
}
