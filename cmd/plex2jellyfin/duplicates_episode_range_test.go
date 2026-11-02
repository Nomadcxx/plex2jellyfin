package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/naming"
	"github.com/Nomadcxx/plex2jellyfin/internal/service"
)

func TestBuildSafeDuplicatePlanEntryRequiresEpisodeRangeCoverage(t *testing.T) {
	root := t.TempDir()
	singleEpisodePath := filepath.Join(root, "The Show S04E01.mkv")
	multiEpisodePath := filepath.Join(root, "The Show S04E01E02.mkv")
	if err := os.WriteFile(singleEpisodePath, make([]byte, 100), 0o644); err != nil {
		t.Fatalf("create single-episode file: %v", err)
	}
	if err := os.WriteFile(multiEpisodePath, make([]byte, 80), 0o644); err != nil {
		t.Fatalf("create multi-episode file: %v", err)
	}
	parsed, err := naming.ParseTVShowFromPath(multiEpisodePath)
	if err != nil || parsed.Season != 4 || parsed.Episode != 1 || parsed.EpisodeEnd != 2 {
		t.Fatalf("parsed multi-episode identity = %+v, %v; want S04E01E02", parsed, err)
	}

	season, episode := 4, 1
	group := service.DuplicateGroup{
		ID:         "episode-range",
		Title:      "the show",
		MediaType:  "series",
		Season:     &season,
		Episode:    &episode,
		BestFileID: 1,
		Files: []service.MediaFile{
			{ID: 1, Path: singleEpisodePath, Size: 100, Resolution: "1080p", QualityScore: 100},
			{ID: 2, Path: multiEpisodePath, Size: 80, Resolution: "1080p", QualityScore: 0},
		},
	}
	_, reason := buildSafeDuplicatePlanEntry(group, service.NewCleanupService(nil), nil)
	if reason == "" {
		t.Fatal("multi-episode file entered an automatic E01 deletion plan")
	}

	group.BestFileID = 2
	group.Files[0].QualityScore = 0
	group.Files[1].QualityScore = 100
	if _, reason := buildSafeDuplicatePlanEntry(group, service.NewCleanupService(nil), nil); reason != "" {
		t.Fatalf("keeper covering E01 and E02 should allow deleting the single-episode copy, got %q", reason)
	}

	xRangePath := filepath.Join(root, "The Show 4x01-02.mkv")
	if err := os.WriteFile(xRangePath, make([]byte, 80), 0o644); err != nil {
		t.Fatalf("create x-format multi-episode file: %v", err)
	}
	group.BestFileID = 1
	group.Files[0].QualityScore = 100
	group.Files[1].QualityScore = 0
	group.Files[1].Path = xRangePath
	_, reason = buildSafeDuplicatePlanEntry(group, service.NewCleanupService(nil), nil)
	if reason == "" {
		t.Fatal("unsupported 4x01-02 file entered an automatic E01 deletion plan")
	}
}
