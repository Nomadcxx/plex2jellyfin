package postmortem

import (
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
)

func TestClassifySuspiciousNamePollutedReleaseMarker(t *testing.T) {
	got := ClassifySuspiciousName("Ratatouille RoDubbed (2007)", "/mnt/STORAGE1/MOVIES/Ratatouille RoDubbed (2007)")
	if got.Category != "polluted_name" {
		t.Fatalf("Category = %q", got.Category)
	}
	if got.Marker != "RoDubbed" {
		t.Fatalf("Marker = %q", got.Marker)
	}
}

func TestClassifySuspiciousNameDoesNotMatchMarkersInsideWords(t *testing.T) {
	tests := []string{
		"Star City",
		"Maximum Pleasure Guaranteed",
		"My Adventures with Superman",
		"Last Week Tonight with John Oliver",
		"Project Runway S22E03 - In It to Win It",
	}

	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			got := ClassifySuspiciousName(name, "")
			if got.Category != "" {
				t.Fatalf("Category = %q, marker = %q", got.Category, got.Marker)
			}
		})
	}
}

func TestClassifySuspiciousNameFlagsHDRipToken(t *testing.T) {
	got := ClassifySuspiciousName("Masters Of The Universe Hdrip (2026)", "")
	if got.Category != "polluted_name" {
		t.Fatalf("Category = %q", got.Category)
	}
	if got.Marker != "HDRip" {
		t.Fatalf("Marker = %q", got.Marker)
	}
}

func TestClassifySuspiciousNameFlagsTUBIToken(t *testing.T) {
	got := ClassifySuspiciousName("Breaking Bear S01E01 - TUBI", "")
	if got.Category != "polluted_name" || got.Marker != "TUBI" {
		t.Fatalf("got %#v, want TUBI polluted_name", got)
	}
}

func TestClassifyPathTranslationMismatch(t *testing.T) {
	got := ClassifyPathMismatch("/mnt/STORAGE5/TVSHOWS/The Daily Show/file.mkv", "/tv5/The Daily Show/file.mkv")
	if got.Category != "path_translation_false_positive" {
		t.Fatalf("Category = %q", got.Category)
	}
}

func TestSuspiciousFromDecisionsFlagsParserDrift(t *testing.T) {
	suspicious, _ := suspiciousFromDecisions([]*database.ParseDecision{{
		SourcePath:       "/downloads/The.Sheep.Detectives.2026.2160p.AMZN.WEB-DL.DV.HDR10+.MULTi-Ben.The.Men/The.Sheep.Detectives.2026.2160p.AMZN.WEB-DL.DV.HDR10+.MULTi-Ben.The.Men.mp4",
		SourceFilename:   "The.Sheep.Detectives.2026.2160p.AMZN.WEB-DL.DV.HDR10+.MULTi-Ben.The.Men.mp4",
		MediaTypeGuessed: "movie",
		ParsedTitle:      "The Sheep Detectives Ben The Men",
		TargetPath:       "/mnt/STORAGE1/MOVIES/The Sheep Detectives Ben The Men (2026)/The Sheep Detectives Ben The Men (2026).mp4",
	}})
	if len(suspicious) != 1 {
		t.Fatalf("suspicious = %d, want 1: %#v", len(suspicious), suspicious)
	}
	if suspicious[0].Category != "parser_drift" {
		t.Fatalf("Category = %q", suspicious[0].Category)
	}
	if !strings.Contains(suspicious[0].Reason, "The Sheep Detectives") {
		t.Fatalf("Reason = %q", suspicious[0].Reason)
	}
}

func TestSuspiciousFromDecisionsSkipsSeasonPackAggregateDrift(t *testing.T) {
	suspicious, _ := suspiciousFromDecisions([]*database.ParseDecision{{
		SourcePath:       "/downloads/Among.Us.S01.1080p-NoTrace/Among.Us.S01E09.Im.Just.Venting.1080p-NoTrace.mkv",
		SourceFilename:   "Among.Us.S01E09.Im.Just.Venting.1080p-NoTrace.mkv",
		MediaTypeGuessed: "tv",
		ParseMethod:      "season_pack",
		ParsedTitle:      "Among Us",
		TargetPath:       "/mnt/STORAGE4/TVSHOWS/Among Us/Season 01/Among Us S01E01.mkv",
	}})
	if len(suspicious) != 0 {
		t.Fatalf("suspicious = %#v, want none", suspicious)
	}
}

func TestSuspiciousFromDecisionsFlagsTargetCollision(t *testing.T) {
	target := "/mnt/STORAGE6/TVSHOWS/Maisy/Season 01/Maisy S01E06.mkv"
	suspicious, _ := suspiciousFromDecisions([]*database.ParseDecision{
		{
			SourcePath:       "/downloads/Maisy.S01E061-062-063-064/Maisy.S01E061-062-063-064.mkv",
			SourceFilename:   "Maisy.S01E061-062-063-064.mkv",
			MediaTypeGuessed: "tv",
			ParsedTitle:      "Maisy",
			TargetPath:       target,
			OrganizeOutcome:  "success",
		},
		{
			SourcePath:       "/downloads/Maisy.S01E065-066-067-068/Maisy.S01E065-066-067-068.mkv",
			SourceFilename:   "Maisy.S01E065-066-067-068.mkv",
			MediaTypeGuessed: "tv",
			ParsedTitle:      "Maisy",
			TargetPath:       target,
			OrganizeOutcome:  "success",
		},
	})
	if len(suspicious) != 1 {
		t.Fatalf("suspicious = %d, want 1: %#v", len(suspicious), suspicious)
	}
	if suspicious[0].Category != "target_collision" {
		t.Fatalf("Category = %q", suspicious[0].Category)
	}
	if suspicious[0].Path != target {
		t.Fatalf("Path = %q, want %q", suspicious[0].Path, target)
	}
}

func TestSuspiciousFromDecisionsFlagsRepeatedSameSourceWrite(t *testing.T) {
	source := "/downloads/Seeking.Persephone.S01E04/Seeking.Persephone.S01E04.mkv"
	target := "/mnt/STORAGE4/TVSHOWS/Seeking Persephone/Season 01/Seeking Persephone S01E04.mkv"
	decision := database.ParseDecision{
		SourcePath:       source,
		SourceFilename:   "Seeking.Persephone.S01E04.mkv",
		MediaTypeGuessed: "tv",
		ParsedTitle:      "Seeking Persephone",
		ParsedSeason:     intPtr(1),
		ParsedEpisode:    intPtr(4),
		TargetPath:       target,
		OrganizeOutcome:  "success",
	}

	suspicious, _ := suspiciousFromDecisions([]*database.ParseDecision{&decision, &decision})
	if len(suspicious) != 1 || suspicious[0].Category != "target_collision" {
		t.Fatalf("suspicious = %#v, want repeated target_collision", suspicious)
	}
}

func TestSuspiciousFromDecisionsChecksVisibleTargetName(t *testing.T) {
	suspicious, _ := suspiciousFromDecisions([]*database.ParseDecision{{
		SourcePath:       "/downloads/Breaking.Bear.S01E01.TUBI.WEB.H264-DARK.mp4",
		SourceFilename:   "Breaking.Bear.S01E01.TUBI.WEB.H264-DARK.mp4",
		MediaTypeGuessed: "tv",
		ParsedTitle:      "Breaking Bear",
		ParsedSeason:     intPtr(1),
		ParsedEpisode:    intPtr(1),
		TargetPath:       "/mnt/STORAGE4/TVSHOWS/Breaking Bear/Season 01/Breaking Bear S01E01 - TUBI.mp4",
		OrganizeOutcome:  "success",
	}})
	if len(suspicious) != 1 || suspicious[0].Marker != "TUBI" {
		t.Fatalf("suspicious = %#v, want visible TUBI pollution", suspicious)
	}
}

func TestSummarizeDecisionMetricsCountsMetadataProblemsMissedBySuspiciousClassifier(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	decisions := []*database.ParseDecision{{
		ID:             42,
		EventAt:        now.Add(-time.Hour),
		SourceFilename: "Clean Title.mkv",
		ParsedTitle:    "Clean Title",
		TargetPath:     "/mnt/STORAGE1/MOVIES/Clean Title (2024)/Clean Title (2024).mkv",
		MetadataState:  "jellyfin_item_missing",
		AutoLabel:      "",
	}}

	suspicious, _ := suspiciousFromDecisions(decisions)
	if len(suspicious) != 0 {
		t.Fatalf("suspicious classifier should stay quiet for clean names, got %#v", suspicious)
	}
	metrics := SummarizeDecisionMetrics(decisions, now)
	if metrics.MetadataProblems != 1 {
		t.Fatalf("MetadataProblems = %d, want 1", metrics.MetadataProblems)
	}
	if metrics.PendingLabels != 1 {
		t.Fatalf("PendingLabels = %d, want 1", metrics.PendingLabels)
	}
}

func intPtr(v int) *int {
	return &v
}
