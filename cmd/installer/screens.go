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

		b.WriteString("  Model:  [↑/↓] to select\n")
		for i, model := range m.aiModels {
			prefix := "    "
			if i == m.aiModelIndex {
				prefix = lipgloss.NewStyle().Foreground(Primary).Render("  ▸ ")
			}
			b.WriteString(fmt.Sprintf("%s%s\n", prefix, model))
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

	b.WriteString("\n\n" + lipgloss.NewStyle().Foreground(FgMuted).Render("[E] Toggle  [↑/↓] Select model  [T] Test  [P] Prompt  [R] Retry  [S] Skip"))

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
	b.WriteString(fmt.Sprintf("  • AI:     %s\n", boolToEnabled(m.aiEnabled)))

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
			err := task.errorDetails
			b.WriteString(lipgloss.NewStyle().Foreground(ErrorColor).Render(
				fmt.Sprintf("  └─ Error: %s\n", err.message)))
			if err.command != "" {
				b.WriteString(lipgloss.NewStyle().Foreground(FgMuted).Render(
					fmt.Sprintf("  └─ Command: %s\n", err.command)))
			}
			b.WriteString(lipgloss.NewStyle().Foreground(FgMuted).Render(
				fmt.Sprintf("  └─ See full logs: %s\n", err.logFile)))
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
	b.WriteString(lipgloss.NewStyle().Foreground(SuccessColor).Bold(true).Render("Installation complete"))
	b.WriteString("\n\n")

	// Show database build results if we scanned
	if m.scanResult != nil {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(Primary).Render("Database Built"))
		b.WriteString("\n")
		b.WriteString(strings.Repeat("─", 40))
		b.WriteString("\n")

		b.WriteString(fmt.Sprintf("  Files indexed:  %s\n",
			lipgloss.NewStyle().Foreground(SuccessColor).Bold(true).Render(fmt.Sprintf("%d", m.scanResult.FilesAdded))))
		b.WriteString(fmt.Sprintf("  Build time:     %s\n",
			lipgloss.NewStyle().Foreground(FgMuted).Render(m.scanResult.Duration.Round(100*time.Millisecond).String())))

		b.WriteString(strings.Repeat("─", 40))
		b.WriteString("\n\n")

		if len(m.scanResult.Errors) > 0 {
			b.WriteString(lipgloss.NewStyle().Foreground(WarningColor).Render(
				fmt.Sprintf("⚠ %d errors during indexing (check logs)\n\n", len(m.scanResult.Errors))))
		}

		b.WriteString(lipgloss.NewStyle().Foreground(FgMuted).Italic(true).Render(
			"Your library is now indexed. Run the commands below to analyze it."))
		b.WriteString("\n\n")
	} else {
		b.WriteString("JellyWatch is ready. The daemon is running and watching your\nconfigured directories.\n\n")
	}

	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(Primary).Render("What's Next?"))
	b.WriteString("\n")

	cmdStyle := lipgloss.NewStyle().Foreground(Secondary)
	descStyle := lipgloss.NewStyle().Foreground(FgMuted)

	b.WriteString(fmt.Sprintf("  %s  %s\n", cmdStyle.Render("jellywatch duplicates generate"), descStyle.Render("Find duplicate files")))
	b.WriteString(fmt.Sprintf("  %s  %s\n", cmdStyle.Render("jellywatch audit generate"), descStyle.Render("Find naming issues")))
	b.WriteString(fmt.Sprintf("  %s  %s\n", cmdStyle.Render("jellywatch consolidate generate"), descStyle.Render("Find scattered series")))
	b.WriteString(fmt.Sprintf("  %s  %s\n", cmdStyle.Render("jellywatch scan"), descStyle.Render("Re-index after changes")))

	b.WriteString("\n")

	labelStyle := lipgloss.NewStyle().Foreground(FgMuted)
	pathStyle := lipgloss.NewStyle().Foreground(FgMuted).Italic(true)
	b.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("Config:  "), pathStyle.Render("~/.config/jellywatch/config.toml")))
	b.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("Database:"), pathStyle.Render("~/.config/jellywatch/media.db")))
	b.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("Logs:    "), pathStyle.Render("journalctl -u jellywatchd -f")))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(FgMuted).Render("Press Enter to exit"))

	if len(m.errors) > 0 {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(WarningColor).Render("Warnings"))
		b.WriteString("\n")
		for _, err := range m.errors {
			b.WriteString(lipgloss.NewStyle().Foreground(WarningColor).Render("  • " + err))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().
		Foreground(FgMuted).
		Italic(true).
		Render("you could trust sonarr/radarr... or you know.. not do that"))

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
