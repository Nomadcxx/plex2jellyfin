package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/Nomadcxx/jellywatch/internal/plans"
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

func TestNewAuditCmd_UsesSubcommandsOnly(t *testing.T) {
	cmd := newAuditCmd()
	names := subcommandNames(cmd)

	for _, required := range []string{"generate", "dry-run", "execute"} {
		if !hasSubcommand(required, names) {
			t.Fatalf("expected audit subcommand %q, got %v", required, names)
		}
	}

	for _, legacyFlag := range []string{"generate", "dry-run", "execute"} {
		if cmd.Flags().Lookup(legacyFlag) != nil {
			t.Fatalf("expected legacy mode flag --%s to be removed", legacyFlag)
		}
	}
}

func TestAuditGenerate_DefaultLimitIsUncapped(t *testing.T) {
	cmd := newAuditCmd()
	generate := findSubcommand(cmd, "generate")
	if generate == nil {
		t.Fatal("expected generate subcommand")
	}

	limitFlag := generate.Flags().Lookup("limit")
	if limitFlag == nil {
		t.Fatal("expected --limit flag on audit generate")
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
	if !strings.Contains(details, "Run 'jellywatch audit execute' to apply changes") {
		t.Fatalf("expected execute hint in details:\n%s", details)
	}
}
