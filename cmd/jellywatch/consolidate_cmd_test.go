package main

import (
	"strings"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/consolidate"
	"github.com/Nomadcxx/jellywatch/internal/plans"
)

func TestConsolidatePlanSafetyIssuesDetectsDuplicateSource(t *testing.T) {
	plan := &plans.ConsolidatePlan{
		Plans: []plans.ConsolidateGroup{
			{
				Title:          "Example A",
				TargetLocation: "/library/A",
				Operations: []plans.MoveOperation{
					{Action: "move", SourcePath: "/library/source/file.mkv", TargetPath: "/library/A/Season 01/file.mkv"},
				},
			},
			{
				Title:          "Example B",
				TargetLocation: "/library/B",
				Operations: []plans.MoveOperation{
					{Action: "move", SourcePath: "/library/source/file.mkv", TargetPath: "/library/B/Season 01/file.mkv"},
				},
			},
		},
	}

	issues := consolidatePlanSafetyIssues(plan)
	if !containsIssue(issues, "duplicate source") {
		t.Fatalf("expected duplicate source issue, got %#v", issues)
	}
}

func TestConsolidatePlanSafetyIssuesDetectsSourceInsideTarget(t *testing.T) {
	plan := &plans.ConsolidatePlan{
		Plans: []plans.ConsolidateGroup{
			{
				Title:          "Example",
				TargetLocation: "/library/Show (2020)",
				Operations: []plans.MoveOperation{
					{
						Action:     "move",
						SourcePath: "/library/Show (2020)/Season 01/Release/hash.mkv",
						TargetPath: "/library/Show (2020)/hash.mkv",
					},
				},
			},
		},
	}

	issues := consolidatePlanSafetyIssues(plan)
	if !containsIssue(issues, "already under target") {
		t.Fatalf("expected source-inside-target issue, got %#v", issues)
	}
	if !containsIssue(issues, "season structure") {
		t.Fatalf("expected season flattening issue, got %#v", issues)
	}
}

func TestConsolidatePlanSafetyIssuesAllowsCrossRootSeasonMove(t *testing.T) {
	plan := &plans.ConsolidatePlan{
		Plans: []plans.ConsolidateGroup{
			{
				Title:          "Example",
				TargetLocation: "/library-b/Show (2020)",
				Operations: []plans.MoveOperation{
					{
						Action:     "move",
						SourcePath: "/library-a/Show (2020)/Season 01/Show S01E01.mkv",
						TargetPath: "/library-b/Show (2020)/Season 01/Show S01E01.mkv",
					},
				},
			},
		},
	}

	if issues := consolidatePlanSafetyIssues(plan); len(issues) != 0 {
		t.Fatalf("expected safe plan, got %#v", issues)
	}
}

func TestShouldReportSkippedConsolidationPlanSuppressesZeroSourcePlans(t *testing.T) {
	plan := &consolidate.Plan{
		Title:       "_jellywatch_quarantine_20260607",
		SourcePaths: nil,
		CanProceed:  false,
		Reasons:     []string{"Failed to choose target path: no locations found for conflict"},
	}

	if shouldReportSkippedConsolidationPlan(plan) {
		t.Fatalf("zero-source skipped plan should be suppressed from CLI output")
	}
}

func containsIssue(issues []string, needle string) bool {
	for _, issue := range issues {
		if strings.Contains(issue, needle) {
			return true
		}
	}
	return false
}
