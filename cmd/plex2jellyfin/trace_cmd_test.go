package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
)

// traceTestDB returns an opener the command can call (and close) repeatedly,
// plus an open handle for seeding rows.
func traceTestDB(t *testing.T) (func() (*database.MediaDB, error), *database.MediaDB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "media.db")
	db, err := database.OpenPath(path)
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return func() (*database.MediaDB, error) { return database.OpenPath(path) }, db
}

func intPtr(v int) *int { return &v }

func TestTraceCmdRendersJourney(t *testing.T) {
	openDB, db := traceTestDB(t)

	moved := time.Date(2026, 3, 2, 10, 5, 0, 0, time.UTC)
	resolved := moved.Add(2 * time.Minute)
	_, err := db.InsertDecision(database.ParseDecision{
		SourcePath:           "/downloads/tv/Show.Name.S01E01.1080p.WEB-DL.mkv",
		SourceFilename:       "Show.Name.S01E01.1080p.WEB-DL.mkv",
		EventAt:              moved.Add(-time.Minute),
		MediaTypeGuessed:     "tv",
		ParseMethod:          "regex",
		ParsedTitle:          "Show Name",
		ParsedYear:           intPtr(2019),
		ParsedSeason:         intPtr(1),
		ParsedEpisode:        intPtr(1),
		ParserStrippedTokens: "1080p WEB-DL",
		TargetPath:           "/media/TV Shows/Show Name (2019)/Season 01/Show Name (2019) S01E01.mkv",
		TargetAt:             &moved,
		OrganizeOutcome:      "SUCCESS",
		JellyfinItemID:       "abc123",
		JellyfinTvdbID:       "78901",
		JellyfinResolvedAt:   &resolved,
	})
	if err != nil {
		t.Fatalf("InsertDecision: %v", err)
	}

	var out bytes.Buffer
	cmd := newTraceCmdWithDeps(openDB, &out)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"Show.Name.S01E01.1080p.WEB-DL.mkv",
		"detected",
		"via regex",
		`"Show Name" (2019) S01E01`,
		"stripped: 1080p WEB-DL",
		"moved      -> /media/TV Shows/Show Name (2019)/Season 01/",
		"matched item abc123",
		"tvdb 78901",
		"outcome    SUCCESS",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\n---\n%s", want, got)
		}
	}
}

func TestTraceCmdFiltersAndHandlesFailures(t *testing.T) {
	openDB, db := traceTestDB(t)

	if _, err := db.InsertDecision(database.ParseDecision{
		SourcePath:      "/downloads/movies/garbled.filename.mkv",
		SourceFilename:  "garbled.filename.mkv",
		EventAt:         time.Now(),
		ParseMethod:     "regex",
		OrganizeOutcome: "FAIL",
		OrganizeError:   "unparseable filename",
	}); err != nil {
		t.Fatalf("InsertDecision: %v", err)
	}

	var out bytes.Buffer
	cmd := newTraceCmdWithDeps(openDB, &out)
	cmd.SetArgs([]string{"garbled"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := out.String()
	for _, want := range []string{"(not moved)", "not confirmed yet", "error      unparseable filename"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\n---\n%s", want, got)
		}
	}

	out.Reset()
	cmd = newTraceCmdWithDeps(openDB, &out)
	cmd.SetArgs([]string{"no-such-file"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out.String(), `No pipeline activity found matching "no-such-file"`) {
		t.Errorf("expected empty-result message, got: %s", out.String())
	}
}
