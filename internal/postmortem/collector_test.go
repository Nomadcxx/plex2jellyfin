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

func TestCollectorIncludesAllDecisionsInWindow(t *testing.T) {
	db, err := database.OpenPath(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer db.Close()

	const count = 10001
	now := time.Now().UTC()
	tx, err := db.SQL().Begin()
	if err != nil {
		t.Fatal(err)
	}
	stmt, err := tx.Prepare(`INSERT INTO parse_decisions (source_path, source_filename, event_at) VALUES (?, ?, ?)`)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < count; i++ {
		if _, err := stmt.Exec(fmt.Sprintf("/downloads/%d.mkv", i), fmt.Sprintf("%d.mkv", i), now.Add(-time.Minute)); err != nil {
			t.Fatalf("insert decision %d: %v", i, err)
		}
	}
	if err := stmt.Close(); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	oldJournalctlExcerpt := journalctlExcerpt
	journalctlExcerpt = func(time.Time) (string, error) { return "test log", nil }
	t.Cleanup(func() { journalctlExcerpt = oldJournalctlExcerpt })
	bundle, err := Collector{
		DB:     db,
		Root:   t.TempDir(),
		Now:    func() time.Time { return now },
		Since:  now.Add(-96 * time.Hour),
		LogDir: t.TempDir(),
		UnknownSeasons: func() UnknownSeasonEvidence {
			return UnknownSeasonEvidence{}
		},
	}.Collect()
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	data, err := os.ReadFile(bundle.File("parse-decisions.json"))
	if err != nil {
		t.Fatal(err)
	}
	var decisions []json.RawMessage
	if err := json.Unmarshal(data, &decisions); err != nil {
		t.Fatal(err)
	}
	if len(decisions) != count {
		t.Fatalf("parse decisions = %d, want all %d rows", len(decisions), count)
	}
	var summary Summary
	summaryData, err := os.ReadFile(bundle.File("summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(summaryData, &summary); err != nil {
		t.Fatal(err)
	}
	if summary.ProcessedDecisions != count {
		t.Fatalf("summary processed decisions = %d, want %d", summary.ProcessedDecisions, count)
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

func TestCollectorSummaryIncludesWindowedConvergenceMetrics(t *testing.T) {
	db, err := database.OpenPath(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer db.Close()

	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	insert := func(d database.ParseDecision) int64 {
		t.Helper()
		id, err := db.InsertDecision(d)
		if err != nil {
			t.Fatalf("InsertDecision: %v", err)
		}
		return id
	}

	idDrift := insert(database.ParseDecision{
		SourcePath: "/downloads/a.mkv", SourceFilename: "a.mkv",
		EventAt: now.Add(-time.Hour), OrganizeOutcome: "success", TargetPath: "/movies/a.mkv",
		AutoLabel: "DRIFT",
	})
	idFail := insert(database.ParseDecision{
		SourcePath: "/downloads/b.mkv", SourceFilename: "b.mkv",
		EventAt: now.Add(-2 * time.Hour), OrganizeOutcome: "success", TargetPath: "/movies/b.mkv",
		AutoLabel: "FAIL",
	})
	_ = insert(database.ParseDecision{
		SourcePath: "/downloads/vanished.mkv", SourceFilename: "vanished.mkv",
		EventAt: now.Add(-2 * time.Hour), OrganizeOutcome: "success", TargetPath: "/movies/vanished.mkv",
		AutoLabel: "VANISHED",
	})
	idMeta := insert(database.ParseDecision{
		SourcePath: "/downloads/c.mkv", SourceFilename: "c.mkv",
		EventAt: now.Add(-3 * time.Hour), OrganizeOutcome: "success", TargetPath: "/movies/c.mkv",
	})
	if err := db.UpdateMetadataCheckState(idMeta, "jellyfin_item_missing", "missing", nil); err != nil {
		t.Fatalf("UpdateMetadataCheckState: %v", err)
	}
	_ = insert(database.ParseDecision{
		SourcePath: "/downloads/d.mkv", SourceFilename: "d.mkv",
		EventAt: now.Add(-4 * time.Hour), OrganizeOutcome: "success", TargetPath: "/movies/d.mkv",
	})
	_ = insert(database.ParseDecision{
		SourcePath: "/downloads/e.mkv", SourceFilename: "e.mkv",
		EventAt: now.Add(-30 * time.Hour), OrganizeOutcome: "success", TargetPath: "/movies/e.mkv",
	})
	_, _ = idDrift, idFail

	// Old flagged backlog + one flagged created in-window.
	if _, err := db.EnqueueHousekeepingTask("housekeeping.detect", database.TaskKindPollutedName, map[string]any{"path": "/old"}, 10); err != nil {
		t.Fatalf("enqueue old: %v", err)
	}
	if _, err := db.EnqueueHousekeepingTask("housekeeping.detect", database.TaskKindYearMismatch, map[string]any{"path": "/new"}, 10); err != nil {
		t.Fatalf("enqueue new: %v", err)
	}
	// Backdate the first flagged task outside the window.
	if _, err := db.DB().Exec(`UPDATE housekeeping_tasks SET created_at = ? WHERE id = 1`, now.Add(-10*24*time.Hour)); err != nil {
		t.Fatalf("backdate housekeeping: %v", err)
	}

	bundle, err := Collector{
		DB:     db,
		Root:   t.TempDir(),
		Now:    func() time.Time { return now },
		Since:  now.Add(-96 * time.Hour),
		LogDir: t.TempDir(),
	}.Collect()
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	data, err := os.ReadFile(bundle.File("summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	var summary Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatal(err)
	}
	if summary.DriftLabels != 1 || summary.FailLabels != 1 {
		t.Fatalf("labels drift=%d fail=%d", summary.DriftLabels, summary.FailLabels)
	}
	if summary.VanishedLabels != 1 {
		t.Fatalf("VanishedLabels = %d, want 1", summary.VanishedLabels)
	}
	if summary.MetadataProblems != 1 {
		t.Fatalf("MetadataProblems = %d, want 1", summary.MetadataProblems)
	}
	if summary.MetadataProblemsOutstanding != 1 {
		t.Fatalf("MetadataProblemsOutstanding = %d, want 1", summary.MetadataProblemsOutstanding)
	}
	if summary.PendingLabels < 2 {
		t.Fatalf("PendingLabels = %d, want >= 2", summary.PendingLabels)
	}
	if summary.OverdueUnlabeled != 0 {
		t.Fatalf("OverdueUnlabeled = %d, want 0 inside the labeler TTL", summary.OverdueUnlabeled)
	}
	if summary.ManualReview != 1 {
		t.Fatalf("ManualReview created-in-window = %d, want 1", summary.ManualReview)
	}
	if summary.HousekeepingOutstandingReview != 2 {
		t.Fatalf("outstanding review = %d, want 2", summary.HousekeepingOutstandingReview)
	}

	hkData, err := os.ReadFile(bundle.File("housekeeping.json"))
	if err != nil {
		t.Fatal(err)
	}
	var hk housekeepingSnapshot
	if err := json.Unmarshal(hkData, &hk); err != nil {
		t.Fatal(err)
	}
	if hk.CreatedInWindow[database.TaskStatusFlagged] != 1 {
		t.Fatalf("created_in_window flagged = %d, want 1", hk.CreatedInWindow[database.TaskStatusFlagged])
	}
	if hk.Outstanding[database.TaskStatusFlagged] != 2 {
		t.Fatalf("outstanding flagged = %d, want 2", hk.Outstanding[database.TaskStatusFlagged])
	}
}

func TestDaemonLogExcerptIncludesRotatedSiblingsFilteredFromSince(t *testing.T) {
	oldJournalctlExcerpt := journalctlExcerpt
	journalctlExcerpt = func(time.Time) (string, error) {
		return "", fmt.Errorf("unused")
	}
	t.Cleanup(func() { journalctlExcerpt = oldJournalctlExcerpt })

	dir := t.TempDir()
	since := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	rotated := strings.Join([]string{
		"2026-07-29T23:00:00Z [INFO] [daemon] too old",
		"2026-07-30T01:00:00Z [INFO] [daemon] from rotated",
		"2026-07-30T02:00:00Z [WARN] [web] GET /api/v1/status",
	}, "\n") + "\n"
	current := strings.Join([]string{
		"2026-07-30T03:00:00Z [INFO] [daemon] from current",
		"2026-07-30T04:00:00Z [INFO] [web] access ok",
		"2026-07-31T05:00:00Z [ERROR] [daemon] failure",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "plex2jellyfin.1.log"), []byte(rotated), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plex2jellyfin.log"), []byte(current), 0o644); err != nil {
		t.Fatal(err)
	}
	// Noisy sibling that must not displace daemon evidence.
	if err := os.WriteFile(filepath.Join(dir, "plex2jellyfin-web-access.log"), []byte(
		"2026-07-31T06:00:00Z [INFO] [web] GET /health\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	got := Collector{LogDir: dir, Since: since}.daemonLogExcerpt()

	if !strings.Contains(got, "from rotated") || !strings.Contains(got, "from current") || !strings.Contains(got, "failure") {
		t.Fatalf("missing daemon window lines:\n%s", got)
	}
	if strings.Contains(got, "too old") {
		t.Fatalf("included pre-Since line:\n%s", got)
	}
	if strings.Contains(got, "GET /api/v1/status") || strings.Contains(got, "access ok") || strings.Contains(got, "GET /health") {
		t.Fatalf("included web access noise:\n%s", got)
	}
	if len(strings.Split(strings.TrimSpace(got), "\n")) < 3 {
		t.Fatalf("expected full in-window daemon excerpt, got:\n%s", got)
	}
	if !strings.Contains(got, "web warnings:") || !strings.Contains(got, "warn=1") {
		t.Fatalf("expected separate web warning summary, got:\n%s", got)
	}
}

func TestDaemonLogExcerptJournalFallbackIsDaemonOnlyCapped(t *testing.T) {
	oldJournalctlExcerpt := journalctlExcerpt
	journalctlExcerpt = func(since time.Time) (string, error) {
		_ = since
		var b strings.Builder
		for i := 0; i < 250; i++ {
			fmt.Fprintf(&b, "line-%03d daemon work\n", i)
		}
		return b.String(), nil
	}
	t.Cleanup(func() { journalctlExcerpt = oldJournalctlExcerpt })

	got := Collector{LogDir: filepath.Join(t.TempDir(), "missing"), Since: time.Now().Add(-96 * time.Hour)}.daemonLogExcerpt()

	if strings.Count(got, "line-") != 200 {
		t.Fatalf("journal fallback has %d lines, want the latest 200", strings.Count(got, "line-"))
	}
	if !strings.Contains(got, "line-249 daemon work") || strings.Contains(got, "line-049 daemon work") {
		t.Fatalf("journal fallback did not retain the latest lines: %q", got[:min(len(got), 500)])
	}
	if !strings.Contains(got, "older log lines omitted") {
		t.Fatalf("journal fallback should explain truncation: %q", got[:min(len(got), 500)])
	}
	if strings.Contains(got, "plex2jellyfin-web") {
		t.Fatalf("daemon excerpt should not include web unit noise: %q", got)
	}
}

func TestDaemonLogExcerptCapsFileLogs(t *testing.T) {
	dir := t.TempDir()
	var content strings.Builder
	for i := 0; i < 220; i++ {
		fmt.Fprintf(&content, "line-%03d daemon work\n", i)
	}
	if err := os.WriteFile(filepath.Join(dir, "plex2jellyfin.log"), []byte(content.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	got := Collector{LogDir: dir}.daemonLogExcerpt()
	if strings.Count(got, "line-") != 200 {
		t.Fatalf("file excerpt has %d lines, want the latest 200", strings.Count(got, "line-"))
	}
	if !strings.Contains(got, "line-219 daemon work") || strings.Contains(got, "line-019 daemon work") {
		t.Fatalf("file excerpt did not retain the latest lines: %q", got[:min(len(got), 500)])
	}
}

func TestCollectorSummarizesActivityFailures(t *testing.T) {
	db, err := database.OpenPath(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer db.Close()

	now := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)
	configDir := t.TempDir()
	logDir := filepath.Join(configDir, "logs")
	activityDir := filepath.Join(configDir, "activity")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(activityDir, 0o755); err != nil {
		t.Fatal(err)
	}
	entries := []string{
		`{"ts":"2026-10-18T09:00:00Z","action":"organize","source":"/downloads/ok.mkv","success":true}`,
		`{"ts":"2026-10-18T09:01:00Z","action":"organize","source":"/downloads/a.mkv","success":false,"error":"transfer failed: disk health check failed: insufficient space: need 4 GB, have 400 MB"}`,
		`{"ts":"2026-10-18T09:02:00Z","action":"organize","source":"/downloads/b.mkv","success":false,"error":"transfer failed: disk health check failed: insufficient space: need 8 GB, have 400 MB"}`,
		`{"ts":"2026-10-18T09:03:00Z","action":"organize","source":"/downloads/c.mkv","success":false}`,
		`{"ts":"2026-10-18T09:04:00Z","action":"jellyfin_verification_mismatch","source":"/library/episode.mkv","success":false,"error":"path not confirmed in jellyfin"}`,
		`{"ts":"2026-10-18T09:05:00Z","action":"jellyfin_verification_summary","success":false,"error":"inconclusive: incomplete jellyfin inventory"}`,
		`{"ts":"2026-10-10T09:00:00Z","action":"organize","source":"/downloads/old.mkv","success":false,"error":"old failure"}`,
	}
	activityPath := filepath.Join(activityDir, "activity-2026-10-18.jsonl")
	if err := os.WriteFile(activityPath, []byte(strings.Join(entries, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldJournalctlExcerpt := journalctlExcerpt
	journalctlExcerpt = func(time.Time) (string, error) { return "test log", nil }
	t.Cleanup(func() { journalctlExcerpt = oldJournalctlExcerpt })

	bundle, err := Collector{
		DB:          db,
		Root:        t.TempDir(),
		Now:         func() time.Time { return now },
		Since:       now.Add(-96 * time.Hour),
		LogDir:      logDir,
		ActivityDir: activityDir,
	}.Collect()
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	data, err := os.ReadFile(bundle.File("summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	var summary struct {
		Activity struct {
			OrganizationAttempts                  int `json:"organization_attempts"`
			OrganizationSucceeded                 int `json:"organization_succeeded"`
			OrganizationUnsuccessful              int `json:"organization_unsuccessful"`
			OrganizationUnsuccessfulWithoutReason int `json:"organization_unsuccessful_without_reason"`
			UniqueOrganizationUnsuccessfulSources int `json:"unique_organization_unsuccessful_sources"`
			JellyfinMismatches                    int `json:"jellyfin_verification_mismatches"`
			JellyfinInconclusive                  int `json:"jellyfin_verification_inconclusive"`
			TopNonSuccessReasons                  []struct {
				Reason string `json:"reason"`
				Count  int    `json:"count"`
			} `json:"top_non_success_reasons"`
		} `json:"activity"`
	}
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatal(err)
	}
	a := summary.Activity
	if a.OrganizationAttempts != 4 || a.OrganizationSucceeded != 1 || a.OrganizationUnsuccessful != 3 {
		t.Fatalf("organization counts = attempts:%d succeeded:%d unsuccessful:%d", a.OrganizationAttempts, a.OrganizationSucceeded, a.OrganizationUnsuccessful)
	}
	if a.OrganizationUnsuccessfulWithoutReason != 1 || a.UniqueOrganizationUnsuccessfulSources != 3 {
		t.Fatalf("unclassified unsuccessful outcomes/sources = %d/%d", a.OrganizationUnsuccessfulWithoutReason, a.UniqueOrganizationUnsuccessfulSources)
	}
	if a.JellyfinMismatches != 1 || a.JellyfinInconclusive != 1 {
		t.Fatalf("Jellyfin verification counts = mismatches:%d inconclusive:%d", a.JellyfinMismatches, a.JellyfinInconclusive)
	}
	if len(a.TopNonSuccessReasons) != 1 || a.TopNonSuccessReasons[0].Reason != "insufficient disk space" || a.TopNonSuccessReasons[0].Count != 2 {
		t.Fatalf("top unsuccessful outcome reasons = %+v", a.TopNonSuccessReasons)
	}
}

func TestJournalctlDaemonArgsAreDaemonOnlyWithLineCap(t *testing.T) {
	since := time.Date(2026, 7, 30, 4, 0, 0, 0, time.UTC)
	args := journalctlDaemonArgs(since)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "plex2jellyfin-daemon") {
		t.Fatalf("args missing daemon unit: %v", args)
	}
	if strings.Contains(joined, "plex2jellyfin-web") {
		t.Fatalf("args must not include web unit: %v", args)
	}
	if !strings.Contains(joined, "--lines 200") {
		t.Fatalf("args must cap output at 200 lines: %v", args)
	}
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--lines" && args[i+1] != "200" {
			t.Fatalf("args should request the latest 200 lines: %v", args)
		}
	}
}
