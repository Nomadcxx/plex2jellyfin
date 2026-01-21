package main

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *model) initPathsInputs() {
	m.inputs = make([]textinput.Model, 0)

	for i := range m.watchFolders {
		ti := textinput.New()
		ti.Placeholder = "e.g., /mnt/downloads/tv, /data/torrents"
		ti.Width = 50
		ti.CharLimit = 500
		ti.SetValue(m.watchFolders[i].Paths)
		ti.PromptStyle = lipgloss.NewStyle().Foreground(Secondary)
		ti.TextStyle = lipgloss.NewStyle().Foreground(FgPrimary)
		m.inputs = append(m.inputs, ti)
	}

	tiTV := textinput.New()
	tiTV.Placeholder = "e.g., /srv/jellyfin/TV Shows"
	tiTV.Width = 50
	tiTV.CharLimit = 500
	tiTV.SetValue(m.tvLibraryPaths)
	tiTV.PromptStyle = lipgloss.NewStyle().Foreground(Secondary)
	tiTV.TextStyle = lipgloss.NewStyle().Foreground(FgPrimary)
	m.inputs = append(m.inputs, tiTV)

	tiMovies := textinput.New()
	tiMovies.Placeholder = "e.g., /srv/jellyfin/Movies"
	tiMovies.Width = 50
	tiMovies.CharLimit = 500
	tiMovies.SetValue(m.movieLibraryPaths)
	tiMovies.PromptStyle = lipgloss.NewStyle().Foreground(Secondary)
	tiMovies.TextStyle = lipgloss.NewStyle().Foreground(FgPrimary)
	m.inputs = append(m.inputs, tiMovies)
}

func (m *model) savePathsInputs() {
	for i := range m.watchFolders {
		if i < len(m.inputs) {
			m.watchFolders[i].Paths = m.inputs[i].Value()
		}
	}
	libraryStartIdx := len(m.watchFolders)
	if libraryStartIdx < len(m.inputs) {
		m.tvLibraryPaths = m.inputs[libraryStartIdx].Value()
	}
	if libraryStartIdx+1 < len(m.inputs) {
		m.movieLibraryPaths = m.inputs[libraryStartIdx+1].Value()
	}
}

func (m *model) initSonarrInputs() {
	m.inputs = make([]textinput.Model, 2)

	m.inputs[0] = textinput.New()
	m.inputs[0].Placeholder = "http://localhost:8989"
	m.inputs[0].Width = 40
	m.inputs[0].CharLimit = 200
	m.inputs[0].SetValue(m.sonarrURL)
	m.inputs[0].PromptStyle = lipgloss.NewStyle().Foreground(Secondary)
	m.inputs[0].TextStyle = lipgloss.NewStyle().Foreground(FgPrimary)

	m.inputs[1] = textinput.New()
	m.inputs[1].Placeholder = "API Key"
	m.inputs[1].Width = 40
	m.inputs[1].CharLimit = 100
	m.inputs[1].SetValue(m.sonarrAPIKey)
	m.inputs[1].EchoMode = textinput.EchoPassword
	m.inputs[1].EchoCharacter = '•'
	m.inputs[1].PromptStyle = lipgloss.NewStyle().Foreground(Secondary)
	m.inputs[1].TextStyle = lipgloss.NewStyle().Foreground(FgPrimary)
}

func (m *model) saveSonarrInputs() {
	if len(m.inputs) >= 2 {
		m.sonarrURL = m.inputs[0].Value()
		m.sonarrAPIKey = m.inputs[1].Value()
	}
}

func (m *model) initRadarrInputs() {
	m.inputs = make([]textinput.Model, 2)

	m.inputs[0] = textinput.New()
	m.inputs[0].Placeholder = "http://localhost:7878"
	m.inputs[0].Width = 40
	m.inputs[0].CharLimit = 200
	m.inputs[0].SetValue(m.radarrURL)
	m.inputs[0].PromptStyle = lipgloss.NewStyle().Foreground(Secondary)
	m.inputs[0].TextStyle = lipgloss.NewStyle().Foreground(FgPrimary)

	m.inputs[1] = textinput.New()
	m.inputs[1].Placeholder = "API Key"
	m.inputs[1].Width = 40
	m.inputs[1].CharLimit = 100
	m.inputs[1].SetValue(m.radarrAPIKey)
	m.inputs[1].EchoMode = textinput.EchoPassword
	m.inputs[1].EchoCharacter = '•'
	m.inputs[1].PromptStyle = lipgloss.NewStyle().Foreground(Secondary)
	m.inputs[1].TextStyle = lipgloss.NewStyle().Foreground(FgPrimary)
}

func (m *model) saveRadarrInputs() {
	if len(m.inputs) >= 2 {
		m.radarrURL = m.inputs[0].Value()
		m.radarrAPIKey = m.inputs[1].Value()
	}
}

func (m *model) initAIInputs() {
	m.inputs = make([]textinput.Model, 1)

	m.inputs[0] = textinput.New()
	m.inputs[0].Placeholder = "http://localhost:11434"
	m.inputs[0].Width = 40
	m.inputs[0].CharLimit = 200
	m.inputs[0].SetValue(m.aiOllamaURL)
	m.inputs[0].PromptStyle = lipgloss.NewStyle().Foreground(Secondary)
	m.inputs[0].TextStyle = lipgloss.NewStyle().Foreground(FgPrimary)
}

func (m *model) saveAIInputs() {
	if len(m.inputs) >= 1 {
		m.aiOllamaURL = m.inputs[0].Value()
	}
	if m.aiModelIndex >= 0 && m.aiModelIndex < len(m.aiModels) {
		m.aiModel = m.aiModels[m.aiModelIndex]
	}
}

func (m *model) initPermissionsInputs() {
	m.inputs = make([]textinput.Model, 4)

	m.inputs[0] = textinput.New()
	m.inputs[0].Placeholder = "jellyfin"
	m.inputs[0].Width = 20
	m.inputs[0].CharLimit = 50
	m.inputs[0].SetValue(m.permUser)
	m.inputs[0].PromptStyle = lipgloss.NewStyle().Foreground(Secondary)
	m.inputs[0].TextStyle = lipgloss.NewStyle().Foreground(FgPrimary)

	m.inputs[1] = textinput.New()
	m.inputs[1].Placeholder = "jellyfin"
	m.inputs[1].Width = 20
	m.inputs[1].CharLimit = 50
	m.inputs[1].SetValue(m.permGroup)
	m.inputs[1].PromptStyle = lipgloss.NewStyle().Foreground(Secondary)
	m.inputs[1].TextStyle = lipgloss.NewStyle().Foreground(FgPrimary)

	m.inputs[2] = textinput.New()
	m.inputs[2].Placeholder = "0644"
	m.inputs[2].Width = 10
	m.inputs[2].CharLimit = 4
	m.inputs[2].SetValue(m.permFileMode)
	m.inputs[2].PromptStyle = lipgloss.NewStyle().Foreground(Secondary)
	m.inputs[2].TextStyle = lipgloss.NewStyle().Foreground(FgPrimary)

	m.inputs[3] = textinput.New()
	m.inputs[3].Placeholder = "0755"
	m.inputs[3].Width = 10
	m.inputs[3].CharLimit = 4
	m.inputs[3].SetValue(m.permDirMode)
	m.inputs[3].PromptStyle = lipgloss.NewStyle().Foreground(Secondary)
	m.inputs[3].TextStyle = lipgloss.NewStyle().Foreground(FgPrimary)
}

func (m *model) savePermissionsInputs() {
	if len(m.inputs) >= 4 {
		m.permUser = m.inputs[0].Value()
		m.permGroup = m.inputs[1].Value()
		m.permFileMode = m.inputs[2].Value()
		m.permDirMode = m.inputs[3].Value()
	}
}

func (m model) addWatchFolder() (tea.Model, tea.Cmd) {
	newFolder := WatchFolder{
		Label: "Custom",
		Type:  "movies",
		Paths: "",
	}
	m.watchFolders = append(m.watchFolders, newFolder)
	m.initPathsInputs()
	return m, nil
}

func (m model) removeWatchFolder() (tea.Model, tea.Cmd) {
	if len(m.watchFolders) > 2 {
		idx := len(m.watchFolders) - 1
		m.watchFolders = m.watchFolders[:idx]
		m.initPathsInputs()
	}
	return m, nil
}
