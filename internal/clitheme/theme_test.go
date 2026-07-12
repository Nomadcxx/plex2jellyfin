package clitheme

import (
	"bytes"
	"strings"
	"testing"

	"charm.land/fang/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

func TestBannerString_ContainsASCIIArt(t *testing.T) {
	s := BannerString("dev")
	if !strings.Contains(s, "▄▄▄▄▄") {
		t.Fatalf("banner missing ASCII art, got:\n%s", s)
	}
	if !strings.Contains(s, "Version: dev") {
		t.Fatalf("banner missing version line, got:\n%s", s)
	}
}

func TestBannerString_UsesPlexYellowWhenColorForced(t *testing.T) {
	prev := lipgloss.Writer.Profile
	lipgloss.Writer.Profile = colorprofile.TrueColor
	t.Cleanup(func() { lipgloss.Writer.Profile = prev })

	s := BannerString("1.2.3")
	// Plex yellow #e5a00d => RGB 229,160,13
	if !strings.Contains(s, "38;2;229;160;13") {
		t.Fatalf("expected plex yellow SGR in banner, got:\n%q", s)
	}
}

func TestFangScheme_ProgramIsPlexYellow(t *testing.T) {
	cs := FangScheme(lipgloss.LightDark(true))
	if !ColorEqual(cs.Program, PlexYellow) {
		t.Fatalf("Program = %v, want plex yellow", cs.Program)
	}
	if !ColorEqual(cs.Title, PlexYellow) {
		t.Fatalf("Title = %v, want plex yellow", cs.Title)
	}
	_ = fang.ColorScheme{}
}

func TestSection_OK_Warn_Err(t *testing.T) {
	var buf bytes.Buffer
	Section(&buf, "Media paths")
	OK(&buf, "Sonarr connection OK")
	Warn(&buf, "renameEpisodes needs true")
	Err(&buf, "fixing failed")
	Muted(&buf, "run health --fix later")

	out := buf.String()
	for _, want := range []string{"Media paths", "[ok]", "[warn]", "[error]", "run health"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "💡") || strings.Contains(out, "✅") || strings.Contains(out, "⚠️") {
		t.Fatalf("emoji leaked into themed output:\n%s", out)
	}
}
