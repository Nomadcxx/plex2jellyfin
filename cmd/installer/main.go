// cmd/installer/main.go
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Nomadcxx/jellywatch/internal/privilege"
)

// globalProgram is used to send messages from goroutines
var globalProgram *tea.Program

func newModel(debugMode bool, logFile *os.File) model {
	s := spinner.New()
	s.Style = lipgloss.NewStyle().Foreground(Secondary)
	s.Spinner = spinner.Dot

	ctx, cancel := context.WithCancel(context.Background())

	// Default watch folders
	watchFolders := []WatchFolder{
		{Label: "TV Shows", Type: "tv", Paths: ""},
		{Label: "Movies", Type: "movies", Paths: ""},
	}

	// Detect existing installation
	existingDB, dbPath, _ := detectExistingInstall()

	// Smart permission defaults based on detected media server
	permUser := ""
	permGroup := "media"
	permFileMode := "0664"
	permDirMode := "0775"

	if mediaServer := detectMediaServer(); mediaServer != nil {
		permUser = mediaServer.User
		permGroup = mediaServer.Group
	} else if mediaGroup := detectMediaGroup(); mediaGroup != "" {
		permGroup = mediaGroup
	}

	m := model{
		step:             stepWelcome,
		tasks:            []installTask{},
		currentTaskIndex: -1,
		spinner:          s,
		errors:           []string{},
		debugMode:        debugMode,
		logFile:          logFile,
		ctx:              ctx,
		cancel:           cancel,

		// Animations (initialized on first resize)
		beams:  nil,
		ticker: NewTypewriterTicker(),

		// Watch folders
		watchFolders: watchFolders,

		// Default values
		sonarrURL:    "http://localhost:8989",
		radarrURL:    "http://localhost:7878",
		aiOllamaURL:  "http://localhost:11434",
		permUser:     permUser,
		permGroup:    permGroup,
		permFileMode: permFileMode,
		permDirMode:  permDirMode,

		// Service defaults
		serviceEnabled:  true,
		serviceStartNow: true,
		scanFrequency:   0, // Every 5 minutes

		// Existing install detection
		existingDBDetected: existingDB,
		existingDBPath:     dbPath,

		// Timing config
		inputDelay: 200 * time.Millisecond,
	}

	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		tickCmd(),
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*50, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func main() {
	// Escalate to root immediately - installer needs privileges throughout
	if privilege.NeedsRoot() {
		if err := privilege.Escalate("install binaries and configure systemd services"); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	// Check for debug flag
	debugMode := false
	for _, arg := range os.Args[1:] {
		if arg == "--debug" || arg == "-d" {
			debugMode = true
			break
		}
	}

	// Create log file with unique name to avoid permission issues
	logFile, err := os.CreateTemp("", "jellywatch-installer-*.log")
	if err != nil {
		// Silently continue without log file - don't print warning on exit
		logFile = nil
	}
	if logFile != nil {
		defer logFile.Close()
		logFile.WriteString(fmt.Sprintf("=== JellyWatch Installer Log ===\n"))
		logFile.WriteString(fmt.Sprintf("Started: %s\n", time.Now().Format("2006-01-02 15:04:05")))
		logFile.WriteString(fmt.Sprintf("Debug Mode: %v\n\n", debugMode))
	}

	m := newModel(debugMode, logFile)
	p := tea.NewProgram(m, tea.WithAltScreen())
	globalProgram = p

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
