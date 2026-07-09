package postmortem

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
)

func TestCollectorWritesSummaryAndParseDecisions(t *testing.T) {
	db, err := database.OpenPath(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer db.Close()

	now := time.Date(2026, 6, 19, 2, 0, 0, 0, time.UTC)
	_, err = db.InsertDecision(database.ParseDecision{
		SourcePath:       "/downloads/Ratatouille.mkv",
		SourceFilename:   "Ratatouille.2007.720p.BluRay.RoDubbed.mkv",
		EventAt:          now.Add(-time.Hour),
		ParseMethod:      "regex",
		ParsedTitle:      "Ratatouille RoDubbed",
		MediaTypeGuessed: "movie",
		OrganizeOutcome:  "success",
		TargetPath:       "/mnt/STORAGE1/MOVIES/Ratatouille RoDubbed (2007)/Ratatouille RoDubbed (2007).mkv",
	})
	if err != nil {
		t.Fatalf("InsertDecision: %v", err)
	}

	root := t.TempDir()
	c := Collector{
		DB:        db,
		Root:      root,
		Now:       func() time.Time { return now },
		Since:     now.Add(-96 * time.Hour),
		LogDir:    t.TempDir(),
		Workspace: "/home/nomadx/Documents/plex2jellyfin",
	}
	bundle, err := c.Collect()
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	for _, file := range []string{
		"summary.json",
		"repair-events.json",
		"jellyfin-diff.json",
		"parse-decisions.json",
		"housekeeping.json",
		"suspicious-items.json",
		"unknown-seasons.json",
		"daemon-log-excerpt.txt",
		"context.md",
		"agent-prompt.md",
		"report.md",
	} {
		if bundle.File(file) == "" {
			t.Fatalf("empty path for %s", file)
		}
		if _, err := os.Stat(bundle.File(file)); err != nil {
			t.Fatalf("expected %s: %v", file, err)
		}
	}

	data, err := os.ReadFile(bundle.File("parse-decisions.json"))
	if err != nil {
		t.Fatal(err)
	}
	var decisions []map[string]any
	if err := json.Unmarshal(data, &decisions); err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 1 {
		t.Fatalf("decisions = %d, want 1", len(decisions))
	}
	if decisions[0]["source_filename"] != "Ratatouille.2007.720p.BluRay.RoDubbed.mkv" {
		t.Fatalf("source_filename = %v", decisions[0]["source_filename"])
	}
}

func TestCollectorWritesUnknownSeasonEvidence(t *testing.T) {
	db, err := database.OpenPath(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer db.Close()

	now := time.Date(2026, 7, 1, 2, 0, 0, 0, time.UTC)
	bundle, err := Collector{
		DB:     db,
		Root:   t.TempDir(),
		Now:    func() time.Time { return now },
		Since:  now.Add(-96 * time.Hour),
		LogDir: t.TempDir(),
		UnknownSeasons: func() UnknownSeasonEvidence {
			return UnknownSeasonEvidence{
				Total:                       2,
				RefreshRepairableSeasons:    1,
				RefreshCandidateEpisodes:    3,
				RandomishBasenameEpisodes:   4,
				ActionablePollutionEpisodes: 7,
			}
		},
	}.Collect()
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	data, err := os.ReadFile(bundle.File("unknown-seasons.json"))
	if err != nil {
		t.Fatal(err)
	}
	var evidence UnknownSeasonEvidence
	if err := json.Unmarshal(data, &evidence); err != nil {
		t.Fatal(err)
	}
	if evidence.ActionablePollutionEpisodes != 7 {
		t.Fatalf("actionable = %d, want 7", evidence.ActionablePollutionEpisodes)
	}

	summaryData, err := os.ReadFile(bundle.File("summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	var summary Summary
	if err := json.Unmarshal(summaryData, &summary); err != nil {
		t.Fatal(err)
	}
	if summary.UnknownSeasonActionable != 7 {
		t.Fatalf("summary unknown actionable = %d, want 7", summary.UnknownSeasonActionable)
	}
}

func TestCollectorWritesEmptySuspiciousItemsAsArray(t *testing.T) {
	db, err := database.OpenPath(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer db.Close()

	now := time.Date(2026, 6, 19, 2, 0, 0, 0, time.UTC)
	_, err = db.InsertDecision(database.ParseDecision{
		SourcePath:       "/downloads/Inception.2010.1080p.WEB-DL-FLUX.mkv",
		SourceFilename:   "Inception.2010.1080p.WEB-DL-FLUX.mkv",
		EventAt:          now.Add(-time.Hour),
		ParseMethod:      "regex",
		ParsedTitle:      "Inception",
		MediaTypeGuessed: "movie",
		OrganizeOutcome:  "success",
		TargetPath:       "/mnt/STORAGE1/MOVIES/Inception (2010)/Inception (2010).mkv",
	})
	if err != nil {
		t.Fatalf("InsertDecision: %v", err)
	}

	bundle, err := Collector{
		DB:        db,
		Root:      t.TempDir(),
		Now:       func() time.Time { return now },
		Since:     now.Add(-96 * time.Hour),
		LogDir:    t.TempDir(),
		Workspace: "/home/nomadx/Documents/plex2jellyfin",
	}.Collect()
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	data, err := os.ReadFile(bundle.File("suspicious-items.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) == "null" {
		t.Fatalf("suspicious-items.json = null, want []")
	}
	var items []SuspiciousItem
	if err := json.Unmarshal(data, &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("items = %d, want 0", len(items))
	}
}

func TestDaemonLogExcerptReportsUnavailableWhenNoSourceWorks(t *testing.T) {
	oldJournalctlExcerpt := journalctlExcerpt
	journalctlExcerpt = func(time.Time) (string, error) {
		return "", fmt.Errorf("permission denied")
	}
	t.Cleanup(func() { journalctlExcerpt = oldJournalctlExcerpt })

	c := Collector{LogDir: filepath.Join(t.TempDir(), "missing")}

	got := c.daemonLogExcerpt()

	if !strings.Contains(got, "daemon log unavailable") {
		t.Fatalf("daemonLogExcerpt() = %q, want explicit unavailable message", got)
	}
	if !strings.Contains(got, "permission denied") {
		t.Fatalf("daemonLogExcerpt() = %q, want journalctl error", got)
	}
}

func TestDaemonLogExcerptFallsBackToJournalctl(t *testing.T) {
	oldJournalctlExcerpt := journalctlExcerpt
	journalctlExcerpt = func(time.Time) (string, error) {
		return "Jun 26 plex2jellyfin-daemon[1]: scanner complete", nil
	}
	t.Cleanup(func() { journalctlExcerpt = oldJournalctlExcerpt })

	c := Collector{LogDir: filepath.Join(t.TempDir(), "missing")}

	got := c.daemonLogExcerpt()

	if !strings.Contains(got, "scanner complete") {
		t.Fatalf("daemonLogExcerpt() = %q, want journalctl lines", got)
	}
	if strings.Contains(got, "daemon log unavailable") {
		t.Fatalf("daemonLogExcerpt() = %q, should not report unavailable when journalctl works", got)
	}
}
