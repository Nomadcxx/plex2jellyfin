package consolidate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SafetyIssues returns reasons a consolidation plan must not be executed.
func SafetyIssues(plan *Plan) []string {
	if plan == nil {
		return nil
	}

	var issues []string
	if hasSeasonComponent(plan.TargetPath) {
		issues = append(issues, fmt.Sprintf("target location includes season/release structure: %s", plan.TargetPath))
	}

	sourceCounts := make(map[string]int)
	for _, op := range plan.Operations {
		if op == nil {
			continue
		}
		sourceCounts[filepath.Clean(op.SourcePath)]++
		if pathIsWithin(plan.TargetPath, op.SourcePath) {
			issues = append(issues, fmt.Sprintf("source already under target location; not a consolidation move: %s", op.SourcePath))
		}
		if hasSeasonComponent(op.SourcePath) && !hasSeasonComponent(op.DestinationPath) {
			issues = append(issues, fmt.Sprintf("target would drop season structure: %s -> %s", op.SourcePath, op.DestinationPath))
		}
		if _, err := os.Stat(op.DestinationPath); err == nil {
			issues = append(issues, fmt.Sprintf("target already exists: %s", op.DestinationPath))
		}
	}

	for source, count := range sourceCounts {
		if count > 1 {
			issues = append(issues, fmt.Sprintf("duplicate source appears in %d planned moves: %s", count, source))
		}
	}

	return issues
}

// DBPlanSafetyIssue returns a reason a DB-backed move/rename plan must not run.
func DBPlanSafetyIssue(plan *ConsolidationPlan) string {
	if plan == nil {
		return "missing consolidation plan"
	}
	if plan.Action != "move" && plan.Action != "rename" {
		return ""
	}
	if isExtraContentPath(plan.SourcePath) || isExtraContentPath(plan.TargetPath) {
		return "source or target is sample/extra content"
	}
	if hasSeasonComponent(plan.SourcePath) && !hasSeasonComponent(plan.TargetPath) {
		return "target would drop season structure"
	}
	if sameVolumeSeriesRoot(plan.SourcePath, plan.TargetPath) && pathIsWithin(filepath.Dir(plan.TargetPath), plan.SourcePath) {
		return "source already under target location"
	}
	return ""
}

func pathIsWithin(parent, child string) bool {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func hasSeasonComponent(path string) bool {
	for _, part := range strings.Split(filepath.Clean(path), string(filepath.Separator)) {
		lower := strings.ToLower(strings.TrimSpace(part))
		if strings.HasPrefix(lower, "season ") || strings.HasPrefix(lower, "season_") || strings.HasPrefix(lower, "season-") {
			return true
		}
	}
	return false
}

func isExtraContentPath(path string) bool {
	components := strings.FieldsFunc(strings.ToLower(filepath.Clean(path)), func(r rune) bool {
		return r == filepath.Separator
	})
	for _, component := range components {
		switch strings.Trim(component, " ._-") {
		case "sample", "samples", "trailer", "trailers", "extras", "extra", "featurette", "featurettes":
			return true
		}
	}
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	base = strings.Trim(base, " ._-")
	return base == "sample" || base == "trailer" ||
		strings.HasPrefix(base, "sample.") ||
		strings.HasPrefix(base, "sample-") ||
		strings.HasPrefix(base, "sample_") ||
		strings.Contains(base, ".sample") ||
		strings.Contains(base, "-sample") ||
		strings.Contains(base, "_sample")
}

func sameVolumeSeriesRoot(source, target string) bool {
	sourceParts := strings.Split(filepath.Clean(source), string(filepath.Separator))
	targetParts := strings.Split(filepath.Clean(target), string(filepath.Separator))
	if len(sourceParts) < 4 || len(targetParts) < 4 {
		return false
	}
	return sourceParts[1] == targetParts[1] && sourceParts[2] == targetParts[2] && sourceParts[3] == targetParts[3]
}
