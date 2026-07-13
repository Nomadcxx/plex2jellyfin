package main

import (
	"fmt"
	"net/url"
	"strings"

	configpkg "github.com/Nomadcxx/plex2jellyfin/internal/config"
	setuppkg "github.com/Nomadcxx/plex2jellyfin/internal/setup"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func styleTextInput(ti *textinput.Model) {
	ti.PromptStyle = lipgloss.NewStyle().Foreground(Secondary).Background(BgBase)
	ti.TextStyle = lipgloss.NewStyle().Foreground(FgPrimary).Background(BgBase)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(FgMuted).Background(BgBase)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(Primary).Background(BgBase)
}

func (m *model) initPathsInputs() {
	m.inputs = make([]textinput.Model, 0)

	for i := range m.watchFolders {
		ti := textinput.New()
		ti.Placeholder = "e.g., /mnt/downloads/tv, /data/torrents"
		ti.Width = 50
		ti.CharLimit = 500
		ti.SetValue(m.watchFolders[i].Paths)
		styleTextInput(&ti)
		m.inputs = append(m.inputs, ti)
	}

	tiTV := textinput.New()
	tiTV.Placeholder = "e.g., /srv/jellyfin/TV Shows"
	tiTV.Width = 50
	tiTV.CharLimit = 500
	tiTV.SetValue(m.tvLibraryPaths)
	styleTextInput(&tiTV)
	m.inputs = append(m.inputs, tiTV)

	tiMovies := textinput.New()
	tiMovies.Placeholder = "e.g., /srv/jellyfin/Movies"
	tiMovies.Width = 50
	tiMovies.CharLimit = 500
	tiMovies.SetValue(m.movieLibraryPaths)
	styleTextInput(&tiMovies)
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
	styleTextInput(&m.inputs[0])

	m.inputs[1] = textinput.New()
	m.inputs[1].Placeholder = "API Key"
	m.inputs[1].Width = 40
	m.inputs[1].CharLimit = 100
	m.inputs[1].SetValue(m.sonarrAPIKey)
	m.inputs[1].EchoMode = textinput.EchoPassword
	m.inputs[1].EchoCharacter = '•'
	styleTextInput(&m.inputs[1])
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
	styleTextInput(&m.inputs[0])

	m.inputs[1] = textinput.New()
	m.inputs[1].Placeholder = "API Key"
	m.inputs[1].Width = 40
	m.inputs[1].CharLimit = 100
	m.inputs[1].SetValue(m.radarrAPIKey)
	m.inputs[1].EchoMode = textinput.EchoPassword
	m.inputs[1].EchoCharacter = '•'
	styleTextInput(&m.inputs[1])
}

func (m *model) saveRadarrInputs() {
	if len(m.inputs) >= 2 {
		m.radarrURL = m.inputs[0].Value()
		m.radarrAPIKey = m.inputs[1].Value()
	}
}

func (m *model) initJellyfinInputs() {
	m.inputs = make([]textinput.Model, 3)

	m.inputs[0] = textinput.New()
	m.inputs[0].Placeholder = "http://localhost:8096"
	m.inputs[0].Width = 40
	m.inputs[0].CharLimit = 200
	m.inputs[0].SetValue(m.jellyfinURL)
	styleTextInput(&m.inputs[0])

	m.inputs[1] = textinput.New()
	m.inputs[1].Placeholder = "API Key"
	m.inputs[1].Width = 40
	m.inputs[1].CharLimit = 120
	m.inputs[1].SetValue(m.jellyfinAPIKey)
	m.inputs[1].EchoMode = textinput.EchoPassword
	m.inputs[1].EchoCharacter = '•'
	styleTextInput(&m.inputs[1])

	m.inputs[2] = textinput.New()
	m.inputs[2].Placeholder = "Auto-generated if empty"
	m.inputs[2].Width = 40
	m.inputs[2].CharLimit = 128
	m.inputs[2].SetValue(m.webhookSecret)
	m.inputs[2].EchoMode = textinput.EchoPassword
	m.inputs[2].EchoCharacter = '•'
	styleTextInput(&m.inputs[2])
}

func (m *model) saveJellyfinInputs() {
	if len(m.inputs) >= 2 {
		m.jellyfinURL = m.inputs[0].Value()
		m.jellyfinAPIKey = m.inputs[1].Value()
	}
	if len(m.inputs) >= 3 {
		m.webhookSecret = m.inputs[2].Value()
	}
}

// ensurePathMappingInputs appends Jellyfin→P2J mapping fields after a successful Test.
func (m *model) ensurePathMappingInputs() {
	if len(m.inputs) >= 5 {
		return
	}
	for len(m.inputs) < 3 {
		ti := textinput.New()
		styleTextInput(&ti)
		m.inputs = append(m.inputs, ti)
	}
	jf := textinput.New()
	jf.Placeholder = "/movies1"
	jf.Width = 30
	jf.CharLimit = 200
	styleTextInput(&jf)
	daemon := textinput.New()
	daemon.Placeholder = "/mnt/STORAGE1/MOVIES"
	daemon.Width = 40
	daemon.CharLimit = 300
	styleTextInput(&daemon)
	m.inputs = append(m.inputs, jf, daemon)
}

func (m *model) recomputeJellyfinUnmapped() {
	m.jellyfinUnmapped = setuppkg.FindUnmappedJellyfinRoots(
		m.jellyfinFolders,
		splitPaths(m.movieLibraryPaths),
		splitPaths(m.tvLibraryPaths),
		m.pathMappings,
	)
}

func (m *model) tryAddPathMapping() bool {
	if len(m.inputs) < 5 {
		return false
	}
	jf := strings.TrimSpace(m.inputs[3].Value())
	daemon := strings.TrimSpace(m.inputs[4].Value())
	if jf == "" || daemon == "" {
		m.pathMappingsError = "both Jellyfin and P2J paths are required"
		return false
	}
	m.pathMappings = append(m.pathMappings, configpkg.JellyfinPathMapping{Jellyfin: jf, Daemon: daemon})
	m.inputs[3].SetValue("")
	m.inputs[4].SetValue("")
	m.pathMappingsError = ""
	m.recomputeJellyfinUnmapped()
	return true
}

func (m *model) removeLastPathMapping() {
	if len(m.pathMappings) == 0 {
		return
	}
	m.pathMappings = m.pathMappings[:len(m.pathMappings)-1]
	m.recomputeJellyfinUnmapped()
}

func (m *model) initAIInputs() {
	m.inputs = make([]textinput.Model, 1)

	m.inputs[0] = textinput.New()
	m.inputs[0].Placeholder = "http://localhost:11434"
	m.inputs[0].Width = 40
	m.inputs[0].CharLimit = 200
	m.inputs[0].SetValue(m.aiOllamaURL)
	styleTextInput(&m.inputs[0])
}

func (m *model) saveAIInputs() {
	if len(m.inputs) >= 1 {
		m.aiOllamaURL = m.inputs[0].Value()
	}
	if m.aiModelIndex >= 0 && m.aiModelIndex < len(m.aiModels) {
		m.aiModel = m.aiModels[m.aiModelIndex]
	}
	if m.aiFallbackModelIndex >= 0 && m.aiFallbackModelIndex < len(m.aiModels) {
		m.aiFallbackModel = m.aiModels[m.aiFallbackModelIndex]
	} else {
		m.aiFallbackModel = ""
	}
}

func (m *model) initPermissionsInputs() {
	m.inputs = make([]textinput.Model, 4)

	m.inputs[0] = textinput.New()
	m.inputs[0].Placeholder = "(empty = preserve)"
	m.inputs[0].Width = 20
	m.inputs[0].CharLimit = 50
	m.inputs[0].SetValue(m.permUser)
	styleTextInput(&m.inputs[0])

	m.inputs[1] = textinput.New()
	m.inputs[1].Placeholder = "media"
	m.inputs[1].Width = 20
	m.inputs[1].CharLimit = 50
	m.inputs[1].SetValue(m.permGroup)
	styleTextInput(&m.inputs[1])

	m.inputs[2] = textinput.New()
	m.inputs[2].Placeholder = "0644"
	m.inputs[2].Width = 10
	m.inputs[2].CharLimit = 4
	m.inputs[2].SetValue(m.permFileMode)
	styleTextInput(&m.inputs[2])

	m.inputs[3] = textinput.New()
	m.inputs[3].Placeholder = "0755"
	m.inputs[3].Width = 10
	m.inputs[3].CharLimit = 4
	m.inputs[3].SetValue(m.permDirMode)
	styleTextInput(&m.inputs[3])
}

func (m *model) savePermissionsInputs() {
	if len(m.inputs) >= 4 {
		m.permUser = m.inputs[0].Value()
		m.permGroup = m.inputs[1].Value()
		m.permFileMode = m.inputs[2].Value()
		m.permDirMode = m.inputs[3].Value()
	}
}

// defaultCallbackURL derives where Jellyfin's companion plugin posts events:
// the web UI when it will exist, otherwise the daemon's health endpoint
// (which mounts the same /api/v1/webhooks/jellyfin route).
func (m *model) defaultCallbackURL() string {
	host := setuppkg.DetectAdvertiseIP()
	if host == "" {
		host = "localhost"
	}
	port := normalizedWebPort(m.webPort)
	if !m.webEnabled {
		port = "8686"
	}
	return "http://" + host + ":" + port
}

func validatePluginCallbackURL(raw string) error {
	value := strings.TrimSpace(raw)
	u, err := url.ParseRequestURI(value)
	if err != nil || strings.ContainsAny(value, "\"\r\n") ||
		(u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("enter an absolute http:// or https:// URL")
	}
	return nil
}

func (m *model) initWebInputs() {
	m.inputs = make([]textinput.Model, 1)

	m.inputs[0] = textinput.New()
	m.inputs[0].Placeholder = "5522"
	m.inputs[0].Width = 10
	m.inputs[0].CharLimit = 5
	m.inputs[0].SetValue(m.webPort)
	styleTextInput(&m.inputs[0])

	if m.jellyfinEnabled && m.pluginInstall {
		ti := textinput.New()
		ti.Placeholder = "http://<lan-ip>:<port>"
		ti.Width = 40
		ti.CharLimit = 200
		ti.Validate = validatePluginCallbackURL
		if m.pluginDaemonURL != "" {
			ti.SetValue(m.pluginDaemonURL)
		} else {
			ti.SetValue(m.defaultCallbackURL())
		}
		styleTextInput(&ti)
		m.inputs = append(m.inputs, ti)
	}
}

func (m *model) saveWebInputs() {
	if len(m.inputs) >= 1 {
		m.webPort = m.inputs[0].Value()
	}
	if len(m.inputs) >= 2 {
		m.pluginDaemonURL = strings.TrimSpace(m.inputs[1].Value())
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
