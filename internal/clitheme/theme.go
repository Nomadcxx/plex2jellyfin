package clitheme

import (
	_ "embed"
	"fmt"
	"image/color"
	"io"

	"charm.land/fang/v2"
	"charm.land/lipgloss/v2"
)

//go:embed header.txt
var asciiHeader string

// PlexYellow is the Plex brand accent used for the CLI ASCII header and Fang chrome.
var PlexYellow = lipgloss.Color("#e5a00d")

var (
	successColor = lipgloss.Color("#3d9a5f")
	warningColor = lipgloss.Color("#c9a227")
	errorColor   = lipgloss.Color("#c44b4b")
	mutedColor   = lipgloss.Color("#888888")
)

func bannerStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(PlexYellow)
}

func successStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(successColor)
}

func warningStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(warningColor)
}

func errorStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(errorColor)
}

func mutedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(mutedColor)
}

func sectionStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(PlexYellow)
}

// BannerString returns the yellow ASCII header plus a version line.
func BannerString(version string) string {
	return bannerStyle().Render(stringsTrimRightNewline(asciiHeader)) + "\n" +
		bannerStyle().Render("Version: "+version) + "\n"
}

// PrintBanner writes the yellow ASCII header to w.
func PrintBanner(w io.Writer, version string) {
	fmt.Fprint(w, BannerString(version))
	fmt.Fprintln(w)
}

// Section writes a branded section heading like "— Media paths —".
func Section(w io.Writer, title string) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, sectionStyle().Render("— "+title+" —"))
}

// OK writes a success line with an ASCII marker.
func OK(w io.Writer, msg string) {
	fmt.Fprintln(w, successStyle().Render("[ok] ")+msg)
}

// Warn writes a warning line with an ASCII marker.
func Warn(w io.Writer, msg string) {
	fmt.Fprintln(w, warningStyle().Render("[warn] ")+msg)
}

// Err writes an error line with an ASCII marker.
func Err(w io.Writer, msg string) {
	fmt.Fprintln(w, errorStyle().Render("[error] ")+msg)
}

// Muted writes secondary / hint text.
func Muted(w io.Writer, msg string) {
	fmt.Fprintln(w, mutedStyle().Render(msg))
}

// FangScheme returns a Fang colorscheme with Plex yellow brand accents.
func FangScheme(ld lipgloss.LightDarkFunc) fang.ColorScheme {
	cs := fang.DefaultColorScheme(ld)
	cs.Title = PlexYellow
	cs.Program = PlexYellow
	return cs
}

func stringsTrimRightNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

// ColorEqual reports whether two colors share the same RGBA values.
func ColorEqual(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == b
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}
