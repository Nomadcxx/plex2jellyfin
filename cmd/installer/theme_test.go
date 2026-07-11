package main

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestMain(m *testing.M) {
	// Tests run without a TTY; force TrueColor so style assertions see ANSI.
	_ = os.Setenv("COLORTERM", "truecolor")
	lipgloss.SetColorProfile(termenv.TrueColor)
	os.Exit(m.Run())
}

func TestFgHelper_PreservesBackgroundAcrossNestedRenders(t *testing.T) {
	// Outer paint + nested FG-only used to emit ESC[0m and punch holes.
	outer := lipgloss.NewStyle().Background(BgBase).Foreground(FgPrimary)
	nested := fg(FgMuted).Render("HI")
	out := outer.Render("A" + nested + "B")

	if !strings.Contains(out, "48;2;26;26;26") && !strings.Contains(out, "48;5;") {
		t.Fatalf("expected background color in output, got %q", out)
	}
	if !strings.Contains(nested, "48;") {
		t.Fatalf("fg() helper must set background, got %q", nested)
	}
}

func TestThemeMarks_CarryBackground(t *testing.T) {
	for name, s := range map[string]lipgloss.Style{
		"check":  checkMark,
		"fail":   failMark,
		"skip":   skipMark,
		"header": headerStyle,
	} {
		rendered := s.Render("x")
		if !strings.Contains(rendered, "48;") {
			t.Errorf("%s style missing background ANSI: %q", name, rendered)
		}
	}
}
