// cmd/installer/view.go
package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	if m.width < 80 || m.height < 24 {
		return lipgloss.NewStyle().
			Foreground(ErrorColor).
			Background(BgBase).
			Bold(true).
			Width(m.width).
			Height(m.height).
			Render(fmt.Sprintf(
				"Terminal too small!\n\nMinimum: 80x24\nCurrent: %dx%d\n\nPlease resize.",
				m.width, m.height,
			))
	}

	var content strings.Builder

	// Render the animated ASCII header using BeamsTextEffect
	if m.beams != nil {
		beamsOutput := m.beams.Render()
		content.WriteString(beamsOutput)
		content.WriteString("\n")
	} else {
		// Fallback: render static header if beams not initialized
		blockWidth := 0
		for _, line := range asciiHeaderLines {
			lineWidth := lipgloss.Width(line)
			if lineWidth > blockWidth {
				blockWidth = lineWidth
			}
		}

		padding := (m.width - blockWidth) / 2
		if padding < 0 {
			padding = 0
		}

		for _, line := range asciiHeaderLines {
			styled := lipgloss.NewStyle().
				PaddingLeft(padding).
				Foreground(Primary).
				Background(BgBase).
				Bold(true).
				Render(line)
			content.WriteString(styled)
			content.WriteString("\n")
		}
	}
	content.WriteString("\n")

	if m.ticker != nil {
		tickerText := m.ticker.Render(m.width - 4)
		tickerStyled := lipgloss.NewStyle().
			Foreground(FgMuted).
			Background(BgBase).
			Italic(true).
			Width(m.width).
			Align(lipgloss.Center).
			Render(tickerText)
		content.WriteString(tickerStyled)
		content.WriteString("\n\n")
	}

	var mainContent string
	switch m.step {
	case stepWelcome:
		mainContent = m.renderWelcome()
	case stepPaths:
		mainContent = m.renderPaths()
	case stepIntegrationsSonarr:
		mainContent = m.renderSonarr()
	case stepIntegrationsRadarr:
		mainContent = m.renderRadarr()
	case stepIntegrationsJellyfin:
		mainContent = m.renderJellyfin()
	case stepIntegrationsAI:
		mainContent = m.renderAI()
	case stepSystemPermissions:
		mainContent = m.renderPermissions()
	case stepSystemService:
		mainContent = m.renderService()
	case stepSystemWeb:
		mainContent = m.renderWebService()
	case stepConfirm:
		mainContent = m.renderConfirm()
	case stepUninstallConfirm:
		mainContent = m.renderUninstallConfirm()
	case stepInstalling:
		mainContent = m.renderInstalling()
	case stepArrIssues:
		mainContent = m.renderArrIssues()
	case stepScanning:
		mainContent = m.renderScanning()
	case stepComplete:
		mainContent = m.renderComplete()
	}

	mainStyle := lipgloss.NewStyle().
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Secondary).
		Foreground(FgPrimary).
		Background(BgBase).
		Width(m.width - 4)

	// Calculate content height and use Place to fill background completely
	mainRendered := mainStyle.Render(mainContent)
	mainHeight := lipgloss.Height(mainRendered)

	// Place content with filled background to prevent terminal bleed-through
	mainPlaced := lipgloss.Place(
		m.width-4, mainHeight,
		lipgloss.Left, lipgloss.Top,
		mainRendered,
		lipgloss.WithWhitespaceBackground(BgBase),
	)
	content.WriteString(mainPlaced)
	content.WriteString("\n")

	helpText := m.getHelpText()
	if helpText != "" {
		helpStyle := lipgloss.NewStyle().
			Foreground(FgMuted).
			Background(BgBase).
			Italic(true).
			Width(m.width).
			Align(lipgloss.Center)
		content.WriteString("\n" + helpStyle.Render(helpText))
	}

	// Wrap everything in full-screen background with centering
	bgStyle := lipgloss.NewStyle().
		Background(BgBase).
		Foreground(FgPrimary).
		Width(m.width).
		Height(m.height).
		Align(lipgloss.Center, lipgloss.Top)

	return bgStyle.Render(content.String())
}

func (m model) getHelpText() string {
	switch m.step {
	case stepWelcome:
		help := "↑/↓: Navigate  •  Enter: Continue  •  q: Quit"
		if m.existingDBDetected {
			help += "  •  W: Full wizard"
		}
		return help
	case stepPaths:
		return "Tab: Next field  •  +/-: Add/Remove folder  •  Enter: Continue  •  Esc: Back"
	case stepIntegrationsSonarr, stepIntegrationsRadarr, stepIntegrationsJellyfin:
		return "Tab: Next field  •  T: Test  •  S: Skip  •  Enter: Continue  •  Esc: Back"
	case stepIntegrationsAI:
		return "Tab: Next field  •  T: Test  •  P: Prompt  •  R: Retry  •  S: Skip  •  Esc: Back"
	case stepSystemPermissions:
		return "Tab: Next field  •  Enter: Continue  •  Esc: Back"
	case stepSystemService:
		return "Tab: Next option  •  ↑/↓: Change value  •  Enter: Continue  •  Esc: Back"
	case stepSystemWeb:
		return "Tab: Next option  •  ↑/↓: Toggle  •  Enter: Continue  •  Esc: Back"
	case stepConfirm:
		return "Enter: Install  •  Esc: Back"
	case stepUninstallConfirm:
		return "↑/↓: Navigate  •  Enter: Uninstall  •  Esc: Back"
	case stepInstalling:
		return "Please wait..."
	case stepArrIssues:
		return "↑/↓: Navigate  •  F: Fix issues  •  S: Skip  •  Enter: Continue"
	case stepScanning:
		return "Scanning libraries..."
	case stepComplete:
		return "Enter: Exit"
	}
	return ""
}
