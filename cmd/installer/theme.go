// cmd/installer/theme.go
package main

import "github.com/charmbracelet/lipgloss"

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

// Styles
var (
	checkMark   = lipgloss.NewStyle().Foreground(SuccessColor).SetString("[OK]")
	failMark    = lipgloss.NewStyle().Foreground(ErrorColor).SetString("[FAIL]")
	skipMark    = lipgloss.NewStyle().Foreground(WarningColor).SetString("[SKIP]")
	headerStyle = lipgloss.NewStyle().Foreground(Primary).Bold(true)
)

var asciiHeaderLines = []string{
	"    ███  ██████  ██       ██      ██   ██  ██   ██   ▄█████  ██████   ▄█████  ██   ██",
	"▄▄   ██  ██▄▄    ██       ██      ██▄  ██  ██▄█▄██  ██▀  ██    ██    ██▀      ██▄▄▄██",
	"▀██▄▄██  ██▄▄▄▄  ██▄▄██   ██▄▄██   ▀█████  ███▀███  ███████    ██    ██▄▄▄▄▄  ██▀▀▀██",
	"  ▀▀▀▀▀  ▀▀▀▀▀▀  ▀▀▀▀▀▀   ▀▀▀▀▀▀       ▀▀  ▀▀   ▀▀  ▀▀   ▀▀    ▀▀    ▀▀▀▀▀▀▀  ▀▀   ▀▀",
}

const asciiHeader = `    ███  ██████  ██       ██      ██   ██  ██   ██   ▄█████  ██████   ▄█████  ██   ██
▄▄   ██  ██▄▄    ██       ██      ██▄  ██  ██▄█▄██  ██▀  ██    ██    ██▀      ██▄▄▄██
▀██▄▄██  ██▄▄▄▄  ██▄▄██   ██▄▄██   ▀█████  ███▀███  ███████    ██    ██▄▄▄▄▄  ██▀▀▀██
  ▀▀▀▀▀  ▀▀▀▀▀▀  ▀▀▀▀▀▀   ▀▀▀▀▀▀       ▀▀  ▀▀   ▀▀  ▀▀   ▀▀    ▀▀    ▀▀▀▀▀▀▀  ▀▀   ▀▀`
