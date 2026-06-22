package postmortem

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/database"
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
		Workspace: "/home/nomadx/Documents/jellywatch",
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
