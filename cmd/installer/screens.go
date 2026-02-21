package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func (m model) renderWelcome() string {
	var b strings.Builder

	b.WriteString("Select an option:\n\n")

	options := []struct {
		label string
		desc  string
	}{
		{"Install JellyWatch", "Fresh installation with full configuration"},
	}

	if m.existingDBDetected {
		options = append([]struct {
			label string
			desc  string
		}{
			{"Update JellyWatch", "Update binaries, preserve configuration"},
		}, options...)
	}

	options = append(options, struct {
		label string
		desc  string
	}{"Uninstall JellyWatch", "Remove JellyWatch from your system"})

	for i, opt := range options {
		prefix := "  "
		if i == m.selectedOption {
			prefix = lipgloss.NewStyle().Foreground(Primary).Render("▸ ")
		}
		b.WriteString(prefix + opt.label + "\n")
		b.WriteString("    " + lipgloss.NewStyle().Foreground(FgMuted).Render(opt.desc) + "\n\n")
	}

	if m.existingDBDetected {
		b.WriteString(lipgloss.NewStyle().Foreground(FgMuted).Render(
			fmt.Sprintf("Existing installation detected: %s", m.existingDBPath)))
		b.WriteString("\n")
	}

	b.WriteString("\n" + lipgloss.NewStyle().Foreground(FgMuted).Render("Requires root privileges (sudo)"))

	return b.String()
}

func (m model) renderUninstallConfirm() string {
	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(Primary).Render("Uninstall JellyWatch"))
	b.WriteString("\n\n")

	b.WriteString("This will remove:\n")
	b.WriteString("  • JellyWatch binaries from /usr/local/bin\n")
	b.WriteString("  • Systemd service (jellywatchd)\n\n")

	// Three options for config/database handling
	prefixes := []string{"  ", "  ", "  "}
	prefixes[m.selectedOption] = lipgloss.NewStyle().Foreground(Primary).Render("▸ ")

	b.WriteString("Configuration and database:\n\n")

	b.WriteString(prefixes[0] + "Keep configuration and database\n")
	b.WriteString("    " + lipgloss.NewStyle().Foreground(FgMuted).Render("Preserve everything for future reinstall") + "\n\n")

	b.WriteString(prefixes[1] + "Keep configuration, delete database\n")
	b.WriteString("    " + lipgloss.NewStyle().Foreground(FgMuted).Render("Keep settings but rebuild media.db on next install") + "\n\n")

	b.WriteString(prefixes[2] + "Delete configuration and database\n")
	b.WriteString("    " + lipgloss.NewStyle().Foreground(FgMuted).Render("Remove all JellyWatch data permanently") + "\n\n")

	if m.existingDBDetected {
		b.WriteString(lipgloss.NewStyle().Foreground(FgMuted).Render(
			fmt.Sprintf("Database: %s", m.existingDBPath)))
		b.WriteString("\n")
	}

	return b.String()
}

func (m model) renderPaths() string {
	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(Primary).Render("Watch Folders"))
	b.WriteString("\n\n")

	// Render watch folder inputs
	for i, wf := range m.watchFolders {
		prefix := "  "
		if m.focusedInput == i {
			prefix = lipgloss.NewStyle().Foreground(Primary).Render("▸ ")
		}
		b.WriteString(fmt.Sprintf("%s%s (%s)\n", prefix, wf.Label, wf.Type))
		// Render the actual text input widget
		if i < len(m.inputs) {
			b.WriteString(fmt.Sprintf("    Paths: %s\n\n", m.inputs[i].View()))
		} else {
			b.WriteString(fmt.Sprintf("    Paths: %s\n\n", wf.Paths))
		}
	}

	b.WriteString(lipgloss.NewStyle().Foreground(FgMuted).Render("[+] Add folder  [-] Remove folder"))
	b.WriteString("\n\n")

	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(Primary).Render("Library Paths"))
	b.WriteString("\n\n")

	// Render library path inputs
	libraryStartIdx := len(m.watchFolders)
	tvPrefix := "  "
	if m.focusedInput == libraryStartIdx {
		tvPrefix = lipgloss.NewStyle().Foreground(Primary).Render("▸ ")
	}
	if libraryStartIdx < len(m.inputs) {
		b.WriteString(fmt.Sprintf("%sTV Libraries:    %s\n", tvPrefix, m.inputs[libraryStartIdx].View()))
	} else {
		b.WriteString(fmt.Sprintf("%sTV Libraries:    %s\n", tvPrefix, m.tvLibraryPaths))
	}

	moviePrefix := "  "
	if m.focusedInput == libraryStartIdx+1 {
		moviePrefix = lipgloss.NewStyle().Foreground(Primary).Render("▸ ")
	}
	if libraryStartIdx+1 < len(m.inputs) {
		b.WriteString(fmt.Sprintf("%sMovie Libraries: %s\n", moviePrefix, m.inputs[libraryStartIdx+1].View()))
	} else {
		b.WriteString(fmt.Sprintf("%sMovie Libraries: %s\n", moviePrefix, m.movieLibraryPaths))
	}

	return b.String()
}

func (m model) renderSonarr() string {
	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(Primary).Render("Sonarr Integration"))
	b.WriteString("\n\n")

	enablePrefix := "  "
	if m.focusedInput == 0 {
		enablePrefix = lipgloss.NewStyle().Foreground(Primary).Render("▸ ")
	}
	enabledStr := "No"
	if m.sonarrEnabled {
		enabledStr = "Yes"
	}
	b.WriteString(fmt.Sprintf("%sEnable: %s  (↑/↓ to toggle)\n\n", enablePrefix, enabledStr))

	if m.sonarrEnabled {
		// Render URL input
		urlPrefix := "  "
		if m.focusedInput == 1 && len(m.inputs) > 0 {
			urlPrefix = lipgloss.NewStyle().Foreground(Primary).Render("▸ ")
		}
		if len(m.inputs) > 0 {
			b.WriteString(fmt.Sprintf("%sURL:     %s\n", urlPrefix, m.inputs[0].View()))
		} else {
			b.WriteString(fmt.Sprintf("%sURL:     %s\n", urlPrefix, m.sonarrURL))
		}

		// Render API Key input
		keyPrefix := "  "
		if m.focusedInput == 2 && len(m.inputs) > 1 {
			keyPrefix = lipgloss.NewStyle().Foreground(Primary).Render("▸ ")
		}
		if len(m.inputs) > 1 {
			b.WriteString(fmt.Sprintf("%sAPI Key: %s\n\n", keyPrefix, m.inputs[1].View()))
		} else {
			b.WriteString(fmt.Sprintf("%sAPI Key: %s\n\n", keyPrefix, strings.Repeat("•", len(m.sonarrAPIKey))))
		}

		if m.sonarrTesting {
			b.WriteString("  " + m.spinner.View() + " Testing connection...\n")
		} else if m.sonarrVersion != "" {
			if m.sonarrTested {
				b.WriteString(fmt.Sprintf("  %s Connected - %s\n", checkMark.String(), m.sonarrVersion))
			} else {
				b.WriteString(fmt.Sprintf("  %s Failed - %s\n", failMark.String(), m.sonarrVersion))
			}
		}
	}

	b.WriteString("\n" + lipgloss.NewStyle().Foreground(FgMuted).Render("[T] Test connection  [S] Skip"))

	return b.String()
}

func (m model) renderRadarr() string {
	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(Primary).Render("Radarr Integration"))
	b.WriteString("\n\n")

	enablePrefix := "  "
	if m.focusedInput == 0 {
		enablePrefix = lipgloss.NewStyle().Foreground(Primary).Render("▸ ")
	}
	enabledStr := "No"
	if m.radarrEnabled {
		enabledStr = "Yes"
	}
	b.WriteString(fmt.Sprintf("%sEnable: %s  (↑/↓ to toggle)\n\n", enablePrefix, enabledStr))

	if m.radarrEnabled {
		// Render URL input
		urlPrefix := "  "
		if m.focusedInput == 1 && len(m.inputs) > 0 {
			urlPrefix = lipgloss.NewStyle().Foreground(Primary).Render("▸ ")
		}
		if len(m.inputs) > 0 {
			b.WriteString(fmt.Sprintf("%sURL:     %s\n", urlPrefix, m.inputs[0].View()))
		} else {
			b.WriteString(fmt.Sprintf("%sURL:     %s\n", urlPrefix, m.radarrURL))
		}

		// Render API Key input
		keyPrefix := "  "
		if m.focusedInput == 2 && len(m.inputs) > 1 {
			keyPrefix = lipgloss.NewStyle().Foreground(Primary).Render("▸ ")
		}
		if len(m.inputs) > 1 {
			b.WriteString(fmt.Sprintf("%sAPI Key: %s\n\n", keyPrefix, m.inputs[1].View()))
		} else {
			b.WriteString(fmt.Sprintf("%sAPI Key: %s\n\n", keyPrefix, strings.Repeat("•", len(m.radarrAPIKey))))
		}

		if m.radarrTesting {
			b.WriteString("  " + m.spinner.View() + " Testing connection...\n")
		} else if m.radarrVersion != "" {
			if m.radarrTested {
				b.WriteString(fmt.Sprintf("  %s Connected - %s\n", checkMark.String(), m.radarrVersion))
			} else {
				b.WriteString(fmt.Sprintf("  %s Failed - %s\n", failMark.String(), m.radarrVersion))
			}
		}
	}

	b.WriteString("\n" + lipgloss.NewStyle().Foreground(FgMuted).Render("[T] Test connection  [S] Skip"))

	return b.String()
}

func (m model) renderJellyfin() string {
	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(Primary).Render("Jellyfin Integration"))
	b.WriteString("\n\n")

	enablePrefix := "  "
	if m.focusedInput == 0 {
		enablePrefix = lipgloss.NewStyle().Foreground(Primary).Render("▸ ")
	}
	enabledStr := "No"
	if m.jellyfinEnabled {
		enabledStr = "Yes"
	}
	b.WriteString(fmt.Sprintf("%sEnable: %s  (↑/↓ to toggle)\n\n", enablePrefix, enabledStr))

	if m.jellyfinEnabled {
		urlPrefix := "  "
		if m.focusedInput == 1 && len(m.inputs) > 0 {
			urlPrefix = lipgloss.NewStyle().Foreground(Primary).Render("▸ ")
		}
		if len(m.inputs) > 0 {
			b.WriteString(fmt.Sprintf("%sURL:     %s\n", urlPrefix, m.inputs[0].View()))
		} else {
			b.WriteString(fmt.Sprintf("%sURL:     %s\n", urlPrefix, m.jellyfinURL))
		}

		keyPrefix := "  "
		if m.focusedInput == 2 && len(m.inputs) > 1 {
			keyPrefix = lipgloss.NewStyle().Foreground(Primary).Render("▸ ")
		}
		if len(m.inputs) > 1 {
			b.WriteString(fmt.Sprintf("%sAPI Key: %s\n\n", keyPrefix, m.inputs[1].View()))
		} else {
			b.WriteString(fmt.Sprintf("%sAPI Key: %s\n\n", keyPrefix, strings.Repeat("•", len(m.jellyfinAPIKey))))
		}

		if m.jellyfinTesting {
			b.WriteString("  " + m.spinner.View() + " Testing connection...\n")
		} else if m.jellyfinVersion != "" {
			if m.jellyfinTested {
				b.WriteString(fmt.Sprintf("  %s Connected - %s\n", checkMark.String(), m.jellyfinVersion))
			} else {
				b.WriteString(fmt.Sprintf("  %s Failed - %s\n", failMark.String(), m.jellyfinVersion))
			}
		}
	}

	b.WriteString("\n" + lipgloss.NewStyle().Foreground(FgMuted).Render("[T] Test connection  [S] Skip"))
	return b.String()
}

func (m model) renderAI() string {
	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(Primary).Render("AI / Ollama Integration"))
	b.WriteString("\n\n")

	enabledStr := "No"
	if m.aiEnabled {
		enabledStr = "Yes"
	}
	b.WriteString(fmt.Sprintf("  Enable: %s  [E] to toggle\n\n", enabledStr))

	if !m.aiEnabled {
		b.WriteString(lipgloss.NewStyle().Foreground(FgMuted).Render("  AI features disabled"))
		b.WriteString("\n\n[S] Skip")
		return b.String()
	}

	switch m.aiState {
	case aiStateNotInstalled:
		b.WriteString(lipgloss.NewStyle().Foreground(WarningColor).Render("  ⚠ Ollama not detected"))
		b.WriteString("\n\n  Install Ollama with:\n")
		b.WriteString(lipgloss.NewStyle().Foreground(FgSecondary).Render("  curl -fsSL https://ollama.com/install.sh | sh"))
		b.WriteString("\n\n  After installing, run: ollama serve")
		b.WriteString("\n  Then pull a model:     ollama pull llama3.2")

	case aiStateNotRunning:
		b.WriteString(lipgloss.NewStyle().Foreground(WarningColor).Render("  ⚠ Ollama installed but not running"))
		b.WriteString("\n\n  Start Ollama with:\n")
		b.WriteString(lipgloss.NewStyle().Foreground(FgSecondary).Render("  ollama serve"))
		b.WriteString("\n\n  Or enable the systemd service:\n")
		b.WriteString(lipgloss.NewStyle().Foreground(FgSecondary).Render("  systemctl --user enable --now ollama"))

	case aiStateNoModels:
		b.WriteString(lipgloss.NewStyle().Foreground(SuccessColor).Render("  ● Connected"))
		b.WriteString(" - No models found\n\n")
		b.WriteString("  Pull a model with:\n")
		b.WriteString(lipgloss.NewStyle().Foreground(FgSecondary).Render("  ollama pull llama3.2      # Recommended, 2GB"))
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(FgSecondary).Render("  ollama pull mistral       # Alternative, 4GB"))

	case aiStateReady:
		b.WriteString(lipgloss.NewStyle().Foreground(SuccessColor).Render("  ● Connected"))
		b.WriteString(fmt.Sprintf(" - %d models available\n\n", len(m.aiModels)))

		b.WriteString(lipgloss.NewStyle().Foreground(FgMuted).Render("  [Space] Select  (1st = Primary  2nd = Fallback)") + "\n\n")

		for i, mdl := range m.aiModels {
			cursor := "   "
			if i == m.aiModelIndex {
				cursor = lipgloss.NewStyle().Foreground(Primary).Render(" ▸ ")
			}
			var badge string
			if mdl == m.aiModel && m.aiModel != "" {
				badge = lipgloss.NewStyle().Foreground(SuccessColor).Bold(true).Render("[P] ")
			} else if mdl == m.aiFallbackModel && m.aiFallbackModel != "" {
				badge = lipgloss.NewStyle().Foreground(WarningColor).Bold(true).Render("[F] ")
			} else {
				badge = "    "
			}
			b.WriteString(fmt.Sprintf("%s%s%s\n", cursor, badge, mdl))
		}

		if m.aiModel != "" {
			b.WriteString(fmt.Sprintf("\n  Primary:  %s\n", lipgloss.NewStyle().Foreground(SuccessColor).Render(m.aiModel)))
		}
		if m.aiFallbackModel != "" {
			b.WriteString(fmt.Sprintf("  Fallback: %s\n", lipgloss.NewStyle().Foreground(WarningColor).Render(m.aiFallbackModel)))
		}

		if m.aiTestResult != "" {
			b.WriteString(fmt.Sprintf("\n  Last test: %s\n", m.aiTestResult))
		}
		if m.aiPromptResult != "" {
			b.WriteString(fmt.Sprintf("  Prompt test: %s\n", m.aiPromptResult))
		}

	default:
		if m.aiTesting {
			b.WriteString("  " + m.spinner.View() + " Detecting Ollama...\n")
		}
	}

	b.WriteString("\n\n" + lipgloss.NewStyle().Foreground(FgMuted).Render("[E] Toggle  [↑/↓] Navigate  [Space] Select  [T] Test  [P] Prompt  [R] Retry  [S] Skip"))

	return b.String()
}

func (m model) renderPermissions() string {
	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(Primary).Render("File Permissions"))
	b.WriteString("\n\n")

	fields := []struct {
		label string
		idx   int
	}{
		{"User:     ", 0},
		{"Group:    ", 1},
		{"File Mode:", 2},
		{"Dir Mode: ", 3},
	}

	for _, f := range fields {
		prefix := "  "
		if m.focusedInput == f.idx {
			prefix = lipgloss.NewStyle().Foreground(Primary).Render("▸ ")
		}
		if f.idx < len(m.inputs) {
			b.WriteString(fmt.Sprintf("%s%s %s\n", prefix, f.label, m.inputs[f.idx].View()))
		}
	}

	b.WriteString("\n")

	tipStyle := lipgloss.NewStyle().Foreground(FgMuted)
	warnStyle := lipgloss.NewStyle().Foreground(WarningColor)

	if mediaServer := detectMediaServer(); mediaServer != nil {
		b.WriteString(tipStyle.Render(fmt.Sprintf("  Detected: %s (user/group auto-filled)", mediaServer.Name)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(tipStyle.Render("  Recommended settings:"))
	b.WriteString("\n")
	b.WriteString(tipStyle.Render("  • User: empty (root owns) or your media server user"))
	b.WriteString("\n")
	b.WriteString(tipStyle.Render("  • Group: shared group (e.g., 'media') for multi-app access"))
	b.WriteString("\n")
	b.WriteString(tipStyle.Render("  • Dir Mode: 0775 allows group to delete files"))
	b.WriteString("\n\n")

	actualUser := getActualUser()
	currentGroup := m.inputs[1].Value()
	if currentGroup != "" && !isUserInGroup(actualUser, currentGroup) {
		b.WriteString(warnStyle.Render(fmt.Sprintf("  ⚠ User '%s' is not in group '%s'", actualUser, currentGroup)))
		b.WriteString("\n")
		b.WriteString(tipStyle.Render(fmt.Sprintf("  Run: sudo usermod -aG %s %s", currentGroup, actualUser)))
		b.WriteString("\n\n")
	}

	b.WriteString(tipStyle.Render("  📖 Full guide: https://github.com/Nomadcxx/jellywatch/blob/main/docs/permissions.md"))

	return b.String()
}

func (m model) renderService() string {
	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(Primary).Render("Systemd Service"))
	b.WriteString("\n\n")

	fields := []struct {
		label string
		value string
	}{
		{"Enable on boot", boolToYesNo(m.serviceEnabled)},
		{"Start now", boolToYesNo(m.serviceStartNow)},
		{"Scan frequency", scanFrequencyOptions[m.scanFrequency]},
	}

	for i, f := range fields {
		prefix := "  "
		if i == m.focusedInput {
			prefix = lipgloss.NewStyle().Foreground(Primary).Render("▸ ")
		}
		b.WriteString(fmt.Sprintf("%s%-15s %s\n", prefix, f.label+":", f.value))
	}

	b.WriteString("\n" + lipgloss.NewStyle().Foreground(FgMuted).Render(
		"The daemon monitors watch folders and runs periodic scans to catch missed files"))

	return b.String()
}

func (m model) renderConfirm() string {
	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(Primary).Render("Confirm Installation"))
	b.WriteString("\n\n")

	b.WriteString("Watch Folders:\n")
	for _, wf := range m.watchFolders {
		if wf.Paths != "" {
			b.WriteString(fmt.Sprintf("  • %s: %s\n", wf.Label, wf.Paths))
		}
	}

	b.WriteString("\nLibraries:\n")
	if m.tvLibraryPaths != "" {
		b.WriteString(fmt.Sprintf("  • TV: %s\n", m.tvLibraryPaths))
	}
	if m.movieLibraryPaths != "" {
		b.WriteString(fmt.Sprintf("  • Movies: %s\n", m.movieLibraryPaths))
	}

	b.WriteString("\nIntegrations:\n")
	b.WriteString(fmt.Sprintf("  • Sonarr: %s\n", boolToEnabled(m.sonarrEnabled)))
	b.WriteString(fmt.Sprintf("  • Radarr: %s\n", boolToEnabled(m.radarrEnabled)))
	b.WriteString(fmt.Sprintf("  • Jellyfin: %s\n", boolToEnabled(m.jellyfinEnabled)))
	if m.aiEnabled && m.aiModel != "" {
		b.WriteString(fmt.Sprintf("  • AI:     Enabled (model: %s)\n", m.aiModel))
		if m.aiFallbackModel != "" {
			b.WriteString(fmt.Sprintf("            Fallback: %s\n", m.aiFallbackModel))
		}
	} else {
		b.WriteString(fmt.Sprintf("  • AI:     %s\n", boolToEnabled(m.aiEnabled)))
	}

	b.WriteString("\nService:\n")
	b.WriteString(fmt.Sprintf("  • Enable on boot: %s\n", boolToYesNo(m.serviceEnabled)))
	b.WriteString(fmt.Sprintf("  • Scan frequency: %s\n", scanFrequencyOptions[m.scanFrequency]))

	b.WriteString("\n" + lipgloss.NewStyle().Bold(true).Foreground(Primary).Render("Press Enter to install"))

	return b.String()
}

func (m model) renderInstalling() string {
	var b strings.Builder

	for _, task := range m.tasks {
		var line string
		switch task.status {
		case statusPending:
			line = lipgloss.NewStyle().Foreground(FgMuted).Render("  " + task.name)
		case statusRunning:
			line = m.spinner.View() + " " + lipgloss.NewStyle().Foreground(Secondary).Render(task.description)
		case statusComplete:
			line = checkMark.String() + " " + task.name
		case statusFailed:
			line = failMark.String() + " " + task.name
		case statusSkipped:
			line = skipMark.String() + " " + task.name
		}
		b.WriteString(line + "\n")

		if len(task.subTasks) > 0 {
			for j, subTask := range task.subTasks {
				isLast := (j == len(task.subTasks)-1)
				prefix := "  ├─ "
				if isLast {
					prefix = "  └─ "
				}

				var subLine string
				switch subTask.status {
				case statusPending:
					subLine = lipgloss.NewStyle().Foreground(FgMuted).Render(subTask.name)
				case statusRunning:
					subLine = m.spinner.View() + " " + subTask.name
				case statusComplete:
					subLine = checkMark.String() + " " + subTask.name
				case statusFailed:
					subLine = failMark.String() + " " + subTask.name
				case statusSkipped:
					subLine = skipMark.String() + " " + subTask.name
				}

				b.WriteString(prefix + subLine + "\n")
			}
		}

		if task.status == statusFailed && task.errorDetails != nil {
			if task.errorDetails.logFile != "" {
				b.WriteString(lipgloss.NewStyle().Foreground(FgMuted).Render(
					fmt.Sprintf("     See log: %s\n", task.errorDetails.logFile)))
			}
		}
	}

	if len(m.errors) > 0 {
		b.WriteString("\n")
		for _, err := range m.errors {
			b.WriteString(lipgloss.NewStyle().Foreground(WarningColor).Render(err))
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (m model) renderScanning() string {
	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(Primary).Render("Scanning Libraries"))
	b.WriteString("\n\n")

	// Calculate percentage from files or libraries
	percent := 0.0
	if m.scanProgress.LibrariesTotal > 0 {
		percent = float64(m.scanProgress.LibrariesDone) / float64(m.scanProgress.LibrariesTotal) * 100
	}

	// Progress bar (50 chars wide)
	barWidth := 50
	filled := int((percent / 100.0) * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}

	// Build the progress bar with colors
	barFilled := lipgloss.NewStyle().Foreground(Secondary).Render(strings.Repeat("█", filled))
	barEmpty := lipgloss.NewStyle().Foreground(FgMuted).Render(strings.Repeat("░", barWidth-filled))
	progressBar := fmt.Sprintf("[%s%s]", barFilled, barEmpty)

	b.WriteString(progressBar)
	b.WriteString(fmt.Sprintf(" %.1f%%\n\n", percent))

	// Library progress
	if m.scanProgress.LibrariesTotal > 0 {
		b.WriteString(fmt.Sprintf("  Libraries: %d/%d\n",
			m.scanProgress.LibrariesDone,
			m.scanProgress.LibrariesTotal))
	}

	// Files scanned counter with spinner
	b.WriteString(fmt.Sprintf("  %s Files scanned: %s\n",
		m.spinner.View(),
		lipgloss.NewStyle().Foreground(Secondary).Bold(true).Render(fmt.Sprintf("%d", m.scanProgress.FilesScanned))))

	// Show error count if any
	if m.scanProgress.ErrorCount > 0 {
		b.WriteString(fmt.Sprintf("  %s Errors: %d\n",
			lipgloss.NewStyle().Foreground(WarningColor).Render("!"),
			m.scanProgress.ErrorCount))
	}

	// Current file being scanned
	if m.scanProgress.CurrentPath != "" {
		// Truncate long paths intelligently
		displayPath := m.scanProgress.CurrentPath
		maxLen := 65

		if len(displayPath) > maxLen {
			// Show ...end of path
			displayPath = "..." + displayPath[len(displayPath)-(maxLen-3):]
		}

		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(FgMuted).Italic(true).Render(
			fmt.Sprintf("  %s", displayPath)))
	}

	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Foreground(FgMuted).Render(
		"Building media database from your libraries..."))

	return b.String()
}

func (m model) renderComplete() string {
	hasCriticalFailure := false
	for _, task := range m.tasks {
		if task.status == statusFailed && !task.optional {
			hasCriticalFailure = true
			break
		}
	}

	if hasCriticalFailure {
		return lipgloss.NewStyle().Foreground(ErrorColor).Render(
			"Installation failed.\nCheck errors above.\n\nPress Enter to exit")
	}

	if m.uninstallMode {
		var msg strings.Builder
		msg.WriteString("Uninstall complete.\nJellyWatch has been removed.\n\n")

		if m.keepConfig && m.keepDatabase {
			msg.WriteString(lipgloss.NewStyle().Foreground(FgMuted).Render("Config and database preserved: ~/.config/jellywatch/"))
			msg.WriteString("\n(Delete manually if no longer needed)")
		} else if m.keepConfig && !m.keepDatabase {
			msg.WriteString(lipgloss.NewStyle().Foreground(FgMuted).Render("Config preserved: ~/.config/jellywatch/config.toml"))
			msg.WriteString("\n")
			msg.WriteString(lipgloss.NewStyle().Foreground(FgMuted).Render("Database deleted."))
			msg.WriteString("\n\n")
			msg.WriteString(lipgloss.NewStyle().Bold(true).Foreground(Primary).Render("To rebuild the database after reinstalling:"))
			msg.WriteString("\n")
			msg.WriteString("  jellywatch scan\n")
			msg.WriteString("\n")
			msg.WriteString(lipgloss.NewStyle().Foreground(FgMuted).Render("Or with Sonarr/Radarr sync:"))
			msg.WriteString("\n")
			msg.WriteString("  jellywatch scan --sonarr --radarr\n")
		} else {
			msg.WriteString(lipgloss.NewStyle().Foreground(FgMuted).Render("Config and database deleted."))
		}
		msg.WriteString("\n\nPress Enter to exit")
		return msg.String()
	}

	var b strings.Builder

	bold := lipgloss.NewStyle().Bold(true).Foreground(Primary)
	muted := lipgloss.NewStyle().Foreground(FgMuted)
	cmd := lipgloss.NewStyle().Foreground(Secondary)
	border := lipgloss.NewStyle().Foreground(FgMuted)

	b.WriteString(bold.Render("Installation complete"))
	b.WriteString("\n\n")

	// ── Database stats table ──────────────────────────────────────────────
	if m.scanResult != nil {
		b.WriteString(border.Render("┌─────────────────────┬──────────────────────────┐"))
		b.WriteString("\n")
		b.WriteString(border.Render("│ ") + bold.Render(fmt.Sprintf("%-19s", "Scan Complete")) + border.Render(" │ ") +
			muted.Render(fmt.Sprintf("%-24s", m.scanResult.Duration.Round(100*time.Millisecond).String())) + border.Render(" │"))
		b.WriteString("\n")
		b.WriteString(border.Render("├─────────────────────┼──────────────────────────┤"))
		b.WriteString("\n")

		var rows []struct{ label, value string }
		if m.scanStats != nil {
			rows = []struct{ label, value string }{
				{"TV shows", fmt.Sprintf("%d", m.scanStats.TVShows)},
				{"Movies", fmt.Sprintf("%d", m.scanStats.Movies)},
			}
			if m.scanStats.DuplicateGroups > 0 {
				rows = append(rows, struct{ label, value string }{"Duplicates", fmt.Sprintf("%d sets", m.scanStats.DuplicateGroups)})
			}
		} else {
			rows = []struct{ label, value string }{
				{"Files indexed", fmt.Sprintf("%d", m.scanResult.FilesScanned)},
			}
		}
		for _, row := range rows {
			b.WriteString(border.Render("│ ") + muted.Render(fmt.Sprintf("%-19s", row.label)) + border.Render(" │ ") +
				lipgloss.NewStyle().Foreground(Primary).Render(fmt.Sprintf("%-24s", row.value)) + border.Render(" │"))
			b.WriteString("\n")
		}

		if len(m.scanResult.Errors) > 0 {
			b.WriteString(border.Render("├─────────────────────┴──────────────────────────┤"))
			b.WriteString("\n")
			warn := fmt.Sprintf("  ⚠  %d errors during indexing (check logs)", len(m.scanResult.Errors))
			b.WriteString(border.Render("│") + lipgloss.NewStyle().Foreground(WarningColor).Render(fmt.Sprintf("%-48s", warn)) + border.Render("│"))
			b.WriteString("\n")
		}

		b.WriteString(border.Render("└─────────────────────────────────────────────────┘"))
		b.WriteString("\n\n")
	} else {
		b.WriteString(muted.Render("JellyWatch is ready. The daemon is running and watching your configured directories."))
		b.WriteString("\n\n")
	}

	// ── What's Next ───────────────────────────────────────────────────────
	b.WriteString(bold.Render("What's Next?") + "\n\n")
	b.WriteString("  " + cmd.Render("jellywatch scan") + "  " + muted.Render("analyze your library for issues") + "\n")
	b.WriteString("\n")
	b.WriteString(muted.Render("  The scan will detect duplicates, scattered series, and") + "\n")
	b.WriteString(muted.Render("  low-confidence parses, then guide you through next steps.") + "\n")

	b.WriteString("\n")

	// ── Web UI ────────────────────────────────────────────────────────────
	b.WriteString(bold.Render("Web UI") + "\n\n")
	b.WriteString("  " + cmd.Render("sudo systemctl start jellyweb") + "  " + muted.Render("start the web interface") + "\n")
	b.WriteString("  " + muted.Render("Then open ") + cmd.Render("http://localhost:5522") + muted.Render(" in your browser") + "\n")
	b.WriteString("\n")
	b.WriteString(muted.Render("  💡 Set a password in config.toml to enable authentication") + "\n")
	b.WriteString("\n")

	// ── Config paths ──────────────────────────────────────────────────────
	pathStyle := muted.Italic(true)
	b.WriteString(muted.Render("Config:   ") + pathStyle.Render("~/.config/jellywatch/config.toml") + "\n")
	b.WriteString(muted.Render("Database: ") + pathStyle.Render("~/.config/jellywatch/media.db") + "\n")
	b.WriteString(muted.Render("Logs:     ") + pathStyle.Render("journalctl -u jellywatchd -f") + "\n")
	b.WriteString("\n")

	if len(m.errors) > 0 {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(WarningColor).Render("Warnings") + "\n")
		for _, err := range m.errors {
			b.WriteString(lipgloss.NewStyle().Foreground(WarningColor).Render("  • "+err) + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(muted.Render("Press Enter to exit"))
	b.WriteString("\n\n")
	b.WriteString(muted.Italic(true).Render("you could trust sonarr/radarr... or you know.. not do that"))

	return b.String()
}

func boolToYesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

func boolToEnabled(b bool) string {
	if b {
		return "Enabled"
	}
	return "Disabled"
}
