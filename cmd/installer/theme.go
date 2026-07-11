// cmd/installer/theme.go
package main

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Theme colors - Monochrome (sysc-family style)
var (
	BgBase       = lipgloss.Color("#1a1a1a")
	BgElevated   = lipgloss.Color("#2a2a2a")
	Primary      = lipgloss.Color("#ffffff")
	Secondary    = lipgloss.Color("#cccccc")
	Accent       = lipgloss.Color("#ffffff")
	FgPrimary    = lipgloss.Color("#ffffff")
	FgSecondary  = lipgloss.Color("#cccccc")
	FgMuted      = lipgloss.Color("#666666")
	ErrorColor   = lipgloss.Color("#ffffff") // White (monochrome)
	WarningColor = lipgloss.Color("#888888") // Medium gray
	SuccessColor = lipgloss.Color("#ffffff") // White (monochrome)
)

// fg returns a style that always paints BgBase behind the glyph. Nested
// Foreground-only styles emit ESC[0m and punch holes through the full-screen
// background; every on-screen span must carry Background.
func fg(c lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(c).Background(BgBase)
}

func fgBold(c lipgloss.Color) lipgloss.Style {
	return fg(c).Bold(true)
}

// Styles
var (
	checkMark   = fg(SuccessColor).SetString("[OK]")
	failMark    = fg(ErrorColor).SetString("[FAIL]")
	skipMark    = fg(WarningColor).SetString("[SKIP]")
	headerStyle = fgBold(Primary)
)

const asciiHeader = `                                  ██                                  ██        
▄▄▄▄▄   ██    ▄▄▄▄ ▄▄  ▄▄ ████▄   ▄▄    ▄▄▄▄  ██   ██  ▄▄   ▄▄  ▄████ ▄▄ ▄▄▄▄▄  
██▀▀██▄ ██  ▄█████ ▀████▀ ▄▄▄███  ██  ▄█████  ██   ██  ██   ██ ██▀    ██ ██▀▀██▄
██▄▄▄██ ██▄ ██▄▄▄▄ ▄████▄ ██▄▄▄▄  ██  ██▄▄▄▄  ██▄  ██▄ ▀██▄▄██ █████  ██ ██   ██
██▀▀▀▀▀ ▀▀▀ ▀▀▀▀▀▀ ▀▀  ▀▀ ▀▀▀▀▀▀ ▄██  ▀▀▀▀▀▀  ▀▀▀  ▀▀▀   ▀▀▀██ ▀▀     ▀▀ ▀▀   ▀▀
▀▀                               ▀▀                         ▀▀                  `

var asciiHeaderLines = strings.Split(asciiHeader, "\n")

// ensureTrueColorHint sets COLORTERM when the terminal looks capable but sudo
// stripped the variable. lipgloss reads this to prefer TrueColor over ANSI16.
func ensureTrueColorHint() {
	if os.Getenv("COLORTERM") != "" {
		return
	}
	term := strings.ToLower(os.Getenv("TERM"))
	if strings.Contains(term, "truecolor") || strings.Contains(term, "256color") ||
		strings.HasPrefix(term, "xterm") || strings.HasPrefix(term, "tmux") ||
		strings.HasPrefix(term, "screen") || strings.Contains(term, "alacritty") ||
		strings.Contains(term, "kitty") || strings.Contains(term, "foot") {
		_ = os.Setenv("COLORTERM", "truecolor")
	}
}
