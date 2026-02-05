// cmd/installer/types.go
package main

import (
	"context"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
)

// Installation steps (grouped flow)
type installStep int

const (
	stepWelcome installStep = iota
	stepPaths
	stepIntegrationsSonarr
	stepIntegrationsRadarr
	stepIntegrationsAI
	stepSystemPermissions
	stepSystemService
	stepConfirm
	stepUninstallConfirm // Confirm uninstall and choose to delete config/db
	stepInstalling
	stepScanning // Library scan with progress
	stepComplete
)

// Task status
type taskStatus int

const (
	statusPending taskStatus = iota
	statusRunning
	statusComplete
	statusFailed
	statusSkipped
)

// WatchFolder represents a configurable watch folder
type WatchFolder struct {
	Label string // Freeform label: "TV Shows", "Movies", "Torrents", etc.
	Type  string // Underlying type: "tv" or "movies"
	Paths string // Comma-separated paths
}

// AI detection state
type aiState int

const (
	aiStateUnknown aiState = iota
	aiStateNotInstalled
	aiStateNotRunning
	aiStateNoModels
	aiStateReady
)

// Installation task
type installTask struct {
	name         string
	description  string
	execute      func(*model) error
	optional     bool
	status       taskStatus
	subTasks     []installSubTask
	currentSub   int
	errorDetails *errorInfo
}

type installSubTask struct {
	name   string
	status taskStatus
}

type errorInfo struct {
	message string
	command string
	logFile string
}

// Main model
type model struct {
	step             installStep
	tasks            []installTask
	postScanTasks    []installTask // Tasks to run after scanning (systemd setup, start service)
	currentTaskIndex int
	width            int
	height           int
	spinner          spinner.Model
	errors           []string
	uninstallMode    bool
	updateMode       bool
	selectedOption   int
	debugMode        bool
	logFile          *os.File

	// Animations
	beams  *BeamsTextEffect
	ticker *TypewriterTicker

	// Text inputs
	inputs       []textinput.Model
	focusedInput int

	// Watch folders (dynamic)
	watchFolders       []WatchFolder
	watchFolderFocused int // Which folder entry is focused

	// Library paths
	tvLibraryPaths    string
	movieLibraryPaths string

	// Sonarr configuration
	sonarrEnabled bool
	sonarrURL     string
	sonarrAPIKey  string
	sonarrTested  bool
	sonarrVersion string
	sonarrTesting bool

	// Radarr configuration
	radarrEnabled bool
	radarrURL     string
	radarrAPIKey  string
	radarrTested  bool
	radarrVersion string
	radarrTesting bool

	// AI configuration
	aiEnabled       bool
	aiState         aiState
	aiOllamaURL     string
	aiModels        []string
	aiModelIndex    int
	aiModel         string
	aiTesting       bool
	aiTestResult    string
	aiPromptTesting bool
	aiPromptResult  string

	// Permissions
	permUser     string
	permGroup    string
	permFileMode string
	permDirMode  string

	// Systemd service
	serviceEnabled  bool
	serviceStartNow bool
	scanFrequency   int // 0=5m, 1=10m, 2=30m, 3=hourly, 4=daily

	// Installation detection
	existingDBDetected bool
	existingDBPath     string
	forceWizard        bool
	daemonWasRunning   bool
	keepDatabase       bool
	keepConfig         bool

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc

	// Scan progress
	scanProgress ScanProgress
	scanResult   *ScanResult
	scanStats    *ScanStats
	scanCancel   context.CancelFunc

	// Timing config
	inputDelay time.Duration
}

// ScanProgress tracks library scanning progress
type ScanProgress struct {
	FilesScanned   int
	CurrentPath    string
	LibrariesDone  int
	LibrariesTotal int
	ErrorCount     int
}

// ScanResult holds the final scan results
type ScanResult struct {
	FilesScanned int
	FilesAdded   int
	Duration     time.Duration
	Errors       []error
}

// ScanStats holds database statistics after scan
type ScanStats struct {
	TVShows         int
	Movies          int
	DuplicateGroups int
}

// Messages
type taskCompleteMsg struct {
	index   int
	success bool
	err     string
	cmdErr  *CommandError // Rich error details from command execution
}

type subTaskUpdateMsg struct {
	parentIndex int
	subIndex    int
	status      taskStatus
}

type apiTestResultMsg struct {
	service string // "sonarr", "radarr", "ollama"
	success bool
	version string
	err     error
}

type aiModelsMsg struct {
	models []string
	err    error
}

type aiPromptTestMsg struct {
	success  bool
	result   string
	duration time.Duration
	err      error
}

type tickMsg time.Time

// Scan messages
type scanStartMsg struct {
	cancel context.CancelFunc
}

type scanProgressMsg struct {
	progress ScanProgress
}

type scanCompleteMsg struct {
	result *ScanResult
	stats  *ScanStats
	err    error
}

// Scan frequency options
var scanFrequencyOptions = []string{
	"Every 5 minutes",
	"Every 10 minutes",
	"Every 30 minutes",
	"Hourly",
	"Every 24 hours",
}
