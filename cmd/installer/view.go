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
			Bold(true).
			Render(fmt.Sprintf(
				"Terminal too small!\n\nMinimum: 80x24\nCurrent: %dx%d\n\nPlease resize.",
				m.width, m.height,
			))
	}

	var content strings.Builder

	if m.beams != nil {
		beamsOutput := m.beams.Render()
		content.WriteString(beamsOutput)
		content.WriteString("\n")
	}

	headerLines := strings.Split(asciiHeader, "\n")
	for _, line := range headerLines {
		centered := lipgloss.NewStyle().
			Width(m.width).
			Align(lipgloss.Center).
			Foreground(Primary).
			Bold(true).
			Render(line)
		content.WriteString(centered)
		content.WriteString("\n")
	}
	content.WriteString("\n")

	if m.ticker != nil {
		tickerText := m.ticker.Render(m.width - 4)
		tickerStyled := lipgloss.NewStyle().
			Foreground(FgMuted).
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
	case stepIntegrationsAI:
		mainContent = m.renderAI()
	case stepSystemPermissions:
		mainContent = m.renderPermissions()
	case stepSystemService:
		mainContent = m.renderService()
	case stepConfirm:
		mainContent = m.renderConfirm()
	case stepInstalling:
		mainContent = m.renderInstalling()
	case stepComplete:
		mainContent = m.renderComplete()
	}

	mainStyle := lipgloss.NewStyle().
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Secondary).
		Foreground(FgPrimary).
		Width(m.width - 4)
	content.WriteString(mainStyle.Render(mainContent))
	content.WriteString("\n")

	helpText := m.getHelpText()
	if helpText != "" {
		helpStyle := lipgloss.NewStyle().
			Foreground(FgMuted).
			Italic(true).
			Width(m.width).
			Align(lipgloss.Center)
		content.WriteString("\n" + helpStyle.Render(helpText))
	}

	bgStyle := lipgloss.NewStyle().
		Background(BgBase).
		Foreground(FgPrimary).
		Width(m.width).
		Height(m.height)

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
	case stepIntegrationsSonarr, stepIntegrationsRadarr:
		return "Tab: Next field  •  T: Test  •  S: Skip  •  Enter: Continue  •  Esc: Back"
	case stepIntegrationsAI:
		return "Tab: Next field  •  T: Test  •  P: Prompt  •  R: Retry  •  S: Skip  •  Esc: Back"
	case stepSystemPermissions:
		return "Tab: Next field  •  Enter: Continue  •  Esc: Back"
	case stepSystemService:
		return "Tab: Next option  •  ↑/↓: Change value  •  Enter: Continue  •  Esc: Back"
	case stepConfirm:
		return "Enter: Install  •  Esc: Back"
	case stepInstalling:
		return "Please wait..."
	case stepComplete:
		return "Enter: Exit"
	}
	return ""
}
