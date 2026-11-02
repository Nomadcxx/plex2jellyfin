package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/consolidate"
	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/fileidentity"
	"github.com/Nomadcxx/plex2jellyfin/internal/plans"
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

func TestConsolidatePlanSafetyIssuesDetectsDuplicateDestination(t *testing.T) {
	destination := "/library/Show (2020)/Season 01/episode.mkv"
	plan := &plans.ConsolidatePlan{Plans: []plans.ConsolidateGroup{{
		TargetLocation: "/library/Show (2020)",
		Operations: []plans.MoveOperation{
			{Action: "move", SourcePath: "/source-a/episode.mkv", TargetPath: destination},
			{Action: "move", SourcePath: "/source-b/episode.mkv", TargetPath: filepath.Join(filepath.Dir(destination), ".", filepath.Base(destination))},
		},
	}}}

	if issues := consolidatePlanSafetyIssues(plan); !containsIssue(issues, "duplicate destination") {
		t.Fatalf("expected duplicate destination issue, got %#v", issues)
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

func TestConsolidatePlanRootIssuesBlocksPathsOutsideConfiguredLibraries(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Libraries.TV = []string{"/library-tv"}
	cfg.Libraries.Movies = []string{"/library-movies"}
	plan := &plans.ConsolidatePlan{
		Plans: []plans.ConsolidateGroup{{
			Title:          "Example",
			TargetLocation: "/tmp/outside",
			Operations: []plans.MoveOperation{{
				Action:     "move",
				SourcePath: "/library-tv/Show (2020)/Season 01/Show S01E01.mkv",
				TargetPath: "/tmp/outside/Show S01E01.mkv",
			}},
		}},
	}

	issues := consolidatePlanRootIssues(plan, cfg)
	if !containsIssue(issues, "outside configured library roots") {
		t.Fatalf("expected root safety issue, got %#v", issues)
	}
}

func TestConsolidatePlanRootIssuesAllowsConfiguredLibraries(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Libraries.TV = []string{"/library-tv-a", "/library-tv-b"}
	cfg.Libraries.Movies = nil
	plan := &plans.ConsolidatePlan{
		Plans: []plans.ConsolidateGroup{{
			Title:          "Example",
			TargetLocation: "/library-tv-b/Show (2020)",
			Operations: []plans.MoveOperation{{
				Action:     "move",
				SourcePath: "/library-tv-a/Show (2020)/Season 01/Show S01E01.mkv",
				TargetPath: "/library-tv-b/Show (2020)/Season 01/Show S01E01.mkv",
			}},
		}},
	}

	if issues := consolidatePlanRootIssues(plan, cfg); len(issues) != 0 {
		t.Fatalf("expected no root safety issues, got %#v", issues)
	}
}

func TestConsolidatePlanDatabaseIssuesRejectsTrackedDestination(t *testing.T) {
	db, err := database.OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	if err := db.UpsertMediaFile(&database.MediaFile{
		Path: "/library/Show (2020)/Season 01/episode.mkv", Size: 100,
		MediaType: "episode", NormalizedTitle: "show",
	}); err != nil {
		t.Fatalf("insert destination media row: %v", err)
	}
	plan := &plans.ConsolidatePlan{Plans: []plans.ConsolidateGroup{{
		Operations: []plans.MoveOperation{{
			Action: "move", SourcePath: "/source/episode.mkv",
			TargetPath: "/library/Show (2020)/Season 01/episode.mkv",
		}},
	}}}

	issues := consolidatePlanDatabaseIssues(db, plan)
	if !containsIssue(issues, "already tracked") {
		t.Fatalf("expected database destination conflict, got %#v", issues)
	}
}

func TestConfiguredRootForPathChoosesMostSpecificRoot(t *testing.T) {
	got := configuredRootForPath("/library/tv/Show (2020)/Season 01/Episode.mkv", []string{"/library", "/library/tv"})
	if got != "/library/tv" {
		t.Fatalf("configuredRootForPath() = %q, want /library/tv", got)
	}
}

func TestUpdateDatabaseAfterMoveTracksUntrackedSourceBeforeMoving(t *testing.T) {
	db, err := database.OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	root := t.TempDir()
	sourcePath := filepath.Join(root, "download", "Film.mkv")
	targetPath := filepath.Join(root, "Movies", "Film (2020)", "Film.mkv")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("film"), 0o644); err != nil {
		t.Fatal(err)
	}
	move := func(sourceIdentity, sourceQuarantinePath string, recordPublished func(string, string) error) (database.MediaFileMoveAttempt, error) {
		tracked, err := db.GetMediaFile(sourcePath)
		if err != nil || tracked == nil {
			return database.MediaFileMoveAttempt{}, errors.New("source was not tracked before transfer")
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return database.MediaFileMoveAttempt{}, err
		}
		if err := os.WriteFile(targetPath, []byte("film"), 0o644); err != nil {
			return database.MediaFileMoveAttempt{}, err
		}
		identity, ok := fileidentity.FromPath(targetPath)
		if !ok {
			return database.MediaFileMoveAttempt{}, errors.New("no file identity")
		}
		targetQuarantinePath, err := fileidentity.NewQuarantinePath(targetPath)
		if err != nil {
			return database.MediaFileMoveAttempt{}, err
		}
		if err := recordPublished(identity, targetQuarantinePath); err != nil {
			return database.MediaFileMoveAttempt{Published: true, PublishedIdentity: identity}, err
		}
		if err := os.Remove(sourcePath); err != nil {
			return database.MediaFileMoveAttempt{}, err
		}
		return database.MediaFileMoveAttempt{Published: true, SourceRemoved: true, PublishedIdentity: identity, DestinationQuarantinePath: targetQuarantinePath}, nil
	}

	if err := updateDatabaseAfterMove(db, sourcePath, targetPath, 4, filepath.Dir(targetPath), root, move); err != nil {
		t.Fatalf("updateDatabaseAfterMove: %v", err)
	}
	moved, err := db.GetMediaFile(targetPath)
	if err != nil || moved == nil {
		t.Fatalf("moved media row = %#v, error = %v", moved, err)
	}
	if moved.MediaType != "movie" || moved.Source != "consolidate" {
		t.Fatalf("moved media metadata = %#v, want movie/consolidate", moved)
	}
	if source, err := db.GetMediaFile(sourcePath); err != nil || source != nil {
		t.Fatalf("source media row = %#v, error = %v; want no stale row", source, err)
	}
}

func TestShouldReportSkippedConsolidationPlanSuppressesZeroSourcePlans(t *testing.T) {
	plan := &consolidate.Plan{
		Title:       "_plex2jellyfin_quarantine_20260607",
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
