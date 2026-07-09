package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/plans"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

func hasSubcommand(cmdName string, names []string) bool {
	for _, name := range names {
		if name == cmdName {
			return true
		}
	}
	return false
}

func findSubcommand(cmd *cobra.Command, name string) *cobra.Command {
	for _, sub := range cmd.Commands() {
		if sub.Name() == name {
			return sub
		}
	}
	return nil
}

func subcommandNames(cmd *cobra.Command) []string {
	commands := cmd.Commands()
	names := make([]string, 0, len(commands))
	for _, c := range commands {
		names = append(names, c.Name())
	}
	return names
}

func TestNewAuditCmd_HasGenerateFlag(t *testing.T) {
	cmd := newAuditCmd()
	flag := cmd.Flags().Lookup("generate")
	if flag == nil {
		t.Fatal("expected --generate flag on audit command")
	}
	if flag.DefValue != "false" {
		t.Fatalf("expected --generate default false, got %q", flag.DefValue)
	}
}

func TestNewAuditCmd_HasDryRunFlag(t *testing.T) {
	cmd := newAuditCmd()
	flag := cmd.Flags().Lookup("dry-run")
	if flag == nil {
		t.Fatal("expected --dry-run flag on audit command")
	}
	if flag.DefValue != "false" {
		t.Fatalf("expected --dry-run default false, got %q", flag.DefValue)
	}
}

func TestNewAuditCmd_HasExecuteFlag(t *testing.T) {
	cmd := newAuditCmd()
	flag := cmd.Flags().Lookup("execute")
	if flag == nil {
		t.Fatal("expected --execute flag on audit command")
	}
	if flag.DefValue != "false" {
		t.Fatalf("expected --execute default false, got %q", flag.DefValue)
	}
}

func TestNewAuditCmd_HasThresholdFlag(t *testing.T) {
	cmd := newAuditCmd()
	flag := cmd.Flags().Lookup("threshold")
	if flag == nil {
		t.Fatal("expected --threshold flag on audit command")
	}
}

func TestNewAuditCmd_HasLimitFlag(t *testing.T) {
	cmd := newAuditCmd()
	flag := cmd.Flags().Lookup("limit")
	if flag == nil {
		t.Fatal("expected --limit flag on audit command")
	}
}

func TestNewAuditCmd_NoLegacySubcommands(t *testing.T) {
	cmd := newAuditCmd()
	// Audit uses flags, not subcommands
	if cmd.HasSubCommands() {
		t.Fatal("audit command should not have subcommands — uses --generate, --dry-run, --execute flags")
	}
}

func TestNewAuditCmd_LimitFlagDefault(t *testing.T) {
	cmd := newAuditCmd()
	limitFlag := cmd.Flags().Lookup("limit")
	if limitFlag == nil {
		t.Fatal("expected --limit flag on audit command")
	}
	if limitFlag.DefValue != "0" {
		t.Fatalf("expected --limit default 0 (uncapped), got %q", limitFlag.DefValue)
	}
}

func TestRenderAuditCard_StableVisibleWidth(t *testing.T) {
	year := 2024
	item := plans.AuditItem{
		Path:       filepath.Join("/very/long/path", "The.File.With.A.Really.Long.Name.2160p.REMUX.mkv"),
		Title:      "A Very Long Source Title With Multiple Tokens",
		Year:       &year,
		Confidence: 0.61,
	}
	action := plans.AuditAction{
		NewPath:    filepath.Join("/very/long/path", "The Corrected and Properly Named File (2024).mkv"),
		NewTitle:   "The Corrected And Properly Named Title",
		Confidence: 0.94,
		Reasoning:  "Title normalized from release format and validated against context",
	}

	card := renderAuditCard(0, 5, &item, &action)
	lines := strings.Split(card, "\n")
	if len(lines) < 8 {
		t.Fatalf("expected multi-line card output, got %d lines", len(lines))
	}

	expectedWidth := lipgloss.Width(lines[0])
	if expectedWidth == 0 {
		t.Fatal("expected non-zero visible width for first line")
	}

	for i, line := range lines {
		if got := lipgloss.Width(line); got != expectedWidth {
			t.Fatalf("line %d width mismatch: got %d, want %d\nline=%q", i, got, expectedWidth, line)
		}
	}
}

func TestRenderAuditCard_UsesASCIIMarkers(t *testing.T) {
	item := plans.AuditItem{
		Path:       "/library/input.mkv",
		Title:      "Input",
		Confidence: 0.20,
		SkipReason: "AI confidence too low",
	}

	card := renderAuditCard(1, 3, &item, nil)

	if !strings.Contains(card, "SKIP") {
		t.Fatalf("expected SKIP marker in card output:\n%s", card)
	}
	if strings.Contains(card, "⚠") || strings.Contains(card, "✓") || strings.Contains(card, "⏭") {
		t.Fatalf("expected ASCII markers only, found unicode status markers:\n%s", card)
	}
}

func TestRenderAuditPlanDetails_IncludesExecuteHint(t *testing.T) {
	now := time.Now()
	plan := &plans.AuditPlan{
		CreatedAt: now,
		Summary: plans.AuditSummary{
			TotalFiles:    1,
			FilesToRename: 1,
			FilesToSkip:   0,
		},
		Items: []plans.AuditItem{
			{
				ID:         1,
				Path:       "/library/input.mkv",
				Title:      "Input",
				Confidence: 0.45,
			},
		},
		Actions: []plans.AuditAction{
			{
				Action:     "rename",
				NewTitle:   "Input Corrected",
				NewPath:    "/library/Input Corrected.mkv",
				Confidence: 0.92,
				Reasoning:  "test",
			},
		},
	}

	details := renderAuditPlanDetails(plan, false)
	if !strings.Contains(details, "Run 'plex2jellyfin audit execute' to apply changes") {
		t.Fatalf("expected execute hint in details:\n%s", details)
	}
}

func TestAuditPlanWarningFlagsStaleAIModelErrors(t *testing.T) {
	plan := &plans.AuditPlan{
		Summary: plans.AuditSummary{
			TotalFiles:    1,
			FilesToSkip:   1,
			AITotalCalls:  1,
			AIErrorCount:  1,
			FilesToRename: 0,
			FilesToDelete: 0,
			AvgConfidence: 0.1,
		},
		Items: []plans.AuditItem{
			{SkipReason: `AI error: ollama returned 404: {"error":"model 'qwen2.5vl:7b' not found"}`},
		},
	}
	cfg := config.DefaultConfig()
	cfg.AI.Model = "minimax-m2.5:cloud"

	warning := auditPlanWarning(plan, cfg)
	if !strings.Contains(warning, "qwen2.5vl:7b") {
		t.Fatalf("expected warning to name stale model, got %q", warning)
	}
	if !strings.Contains(warning, "minimax-m2.5:cloud") {
		t.Fatalf("expected warning to name current model, got %q", warning)
	}
	if !strings.Contains(warning, "plex2jellyfin audit --generate") {
		t.Fatalf("expected warning to give regenerate command, got %q", warning)
	}
}

func TestExecutableAuditActionIndicesUsesRequestedThreshold(t *testing.T) {
	plan := &plans.AuditPlan{
		Items: []plans.AuditItem{
			{Path: "/library/A.mkv"},
			{Path: "/library/B.mkv"},
		},
		Actions: []plans.AuditAction{
			{ItemIndex: 0, Confidence: 0.70},
			{ItemIndex: 1, Confidence: 0.90},
		},
	}

	got := executableAuditActionIndices(plan, 0.75)
	if len(got) != 1 || got[0] != 1 {
		t.Fatalf("threshold 0.75 indices = %v, want [1]", got)
	}

	got = executableAuditActionIndices(plan, 0.65)
	if len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Fatalf("threshold 0.65 indices = %v, want [0 1]", got)
	}
}

func TestAuditScopeFromArgsResolvesAbsolutePath(t *testing.T) {
	scope, err := auditScopeFromArgs([]string{"."})
	if err != nil {
		t.Fatalf("auditScopeFromArgs: %v", err)
	}
	if !filepath.IsAbs(scope) {
		t.Fatalf("scope = %q, want absolute path", scope)
	}
}

func TestAuditPreAISkipReasonSkipsObfuscatedFilename(t *testing.T) {
	file := &database.MediaFile{
		Path:            "/library/Show (2020)/Season 01/248fc0bc4f6f454b89a8158018a398a6.mkv",
		MediaType:       "episode",
		NormalizedTitle: "show",
		Confidence:      0.1,
		ParseMethod:     "folder",
		NeedsReview:     true,
	}

	reason := auditPreAISkipReason(file)
	if !strings.Contains(reason, "Obfuscated") {
		t.Fatalf("auditPreAISkipReason = %q, want obfuscated skip", reason)
	}
}

func TestAuditPreAISkipReasonSkipsSampleFile(t *testing.T) {
	file := &database.MediaFile{
		Path:            "/library/Movie (2020)/release/Sample/sample.mkv",
		MediaType:       "movie",
		NormalizedTitle: "sample",
		Confidence:      0.2,
	}

	reason := auditPreAISkipReason(file)
	if !strings.Contains(strings.ToLower(reason), "extra/sample") {
		t.Fatalf("auditPreAISkipReason = %q, want extra/sample skip", reason)
	}
}

func TestAuditPreAISkipReasonSkipsDeterministicMovie(t *testing.T) {
	year := 2022
	file := &database.MediaFile{
		Path:            "/library/The Batman (2022)/The Batman (2022).mkv",
		MediaType:       "movie",
		NormalizedTitle: "thebatman",
		Year:            &year,
		Confidence:      0.3,
		ParseMethod:     "regex",
	}

	reason := auditPreAISkipReason(file)
	if !strings.Contains(reason, "Deterministic") {
		t.Fatalf("auditPreAISkipReason = %q, want deterministic skip", reason)
	}
}

func TestExecutableAuditActionIndicesSkipsUnsafeStalePlanActions(t *testing.T) {
	plan := &plans.AuditPlan{
		Items: []plans.AuditItem{
			{Path: "/library/Movie (2020)/sample.mkv"},
			{Path: "/library/Movie (2020)/Movie (2020).mkv"},
		},
		Actions: []plans.AuditAction{
			{ItemIndex: 0, Confidence: 0.99},
			{ItemIndex: 1, Confidence: 0.99},
		},
	}

	got := executableAuditActionIndices(plan, 0.8)
	if len(got) != 1 || got[0] != 1 {
		t.Fatalf("executable indices = %v, want [1]", got)
	}
}

func TestAuditPlanRootIssuesBlocksPathsOutsideConfiguredLibraries(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Libraries.Movies = []string{"/library-movies"}
	cfg.Libraries.TV = nil
	plan := &plans.AuditPlan{
		Items: []plans.AuditItem{
			{Path: "/library-movies/Movie (2020)/Movie (2020).mkv"},
		},
		Actions: []plans.AuditAction{
			{ItemIndex: 0, Action: "rename", NewPath: "/tmp/evil.mkv", Confidence: 0.99},
		},
	}

	issues := auditPlanRootIssues(plan, []int{0}, cfg)
	if !containsIssue(issues, "outside configured library roots") {
		t.Fatalf("expected root safety issue, got %#v", issues)
	}
}

func TestAuditPlanRootIssuesAllowsConfiguredLibraries(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Libraries.Movies = []string{"/library-movies"}
	cfg.Libraries.TV = nil
	plan := &plans.AuditPlan{
		Items: []plans.AuditItem{
			{Path: "/library-movies/Movie (2020)/bad.mkv"},
		},
		Actions: []plans.AuditAction{
			{ItemIndex: 0, Action: "rename", NewPath: "/library-movies/Movie (2020)/Movie (2020).mkv", Confidence: 0.99},
		},
	}

	if issues := auditPlanRootIssues(plan, []int{0}, cfg); len(issues) != 0 {
		t.Fatalf("expected no root safety issues, got %#v", issues)
	}
}

func TestAuditPreAISkipReasonAllowsNonDeterministicSemanticFilename(t *testing.T) {
	file := &database.MediaFile{
		Path:            "/library/Movies/s7-hangover2.1080.mkv",
		MediaType:       "movie",
		NormalizedTitle: "s7hangover2",
		Confidence:      0.2,
		ParseMethod:     "regex",
	}

	if reason := auditPreAISkipReason(file); reason != "" {
		t.Fatalf("auditPreAISkipReason = %q, want AI candidate", reason)
	}
}

func TestAuditSkipBreakdownCountsPreAISkips(t *testing.T) {
	items := []plans.AuditItem{
		{SkipReason: "Deterministic parse already has media identity; AI skipped"},
		{SkipReason: "Obfuscated filename requires folder/manual review; AI skipped"},
		{SkipReason: "Folder-derived episode is missing episode number; manual review required"},
		{SkipReason: "AI error: model unavailable"},
	}

	got := auditSkipBreakdown(items)
	if got.Deterministic != 1 {
		t.Fatalf("Deterministic = %d, want 1", got.Deterministic)
	}
	if got.ManualReview != 2 {
		t.Fatalf("ManualReview = %d, want 2", got.ManualReview)
	}
	if got.Other != 1 {
		t.Fatalf("Other = %d, want 1", got.Other)
	}
}
