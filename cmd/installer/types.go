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
	stepIntegrationsJellyfin
	stepIntegrationsAI
	stepSystemPermissions
	stepSystemService
	stepSystemWeb
	stepConfirm
	stepUninstallConfirm // Confirm uninstall and choose to delete config/db
	stepInstalling
	stepArrIssues // Show arr configuration issues and offer to fix
	stepScanning  // Library scan with progress
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

	// Jellyfin configuration
	jellyfinEnabled bool
	jellyfinURL     string
	jellyfinAPIKey  string
	webhookSecret   string
	jellyfinTested  bool
	jellyfinVersion string
	jellyfinTesting bool

	// AI configuration
	aiEnabled            bool
	aiState              aiState
	aiOllamaURL          string
	aiModels             []string
	aiModelIndex         int
	aiModel              string
	aiFallbackModelIndex int // -1 = none
	aiFallbackModel      string
	aiTesting            bool
	aiTestResult         string
	aiPromptTesting      bool
	aiPromptResult       string

	// Permissions
	permUser     string
	permGroup    string
	permFileMode string
	permDirMode  string

	// Systemd service
	serviceEnabled  bool
	serviceStartNow bool
	scanFrequency   int // 0=5m, 1=10m, 2=30m, 3=hourly, 4=daily
	webEnabled      bool
	webStartNow     bool
	webPort         string

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

	// Arr configuration issues
	arrIssues       []ArrIssue
	arrIssuesChoice int // 0=fix, 1=skip

	// Timing config
	inputDelay time.Duration
}

// ArrIssue represents a configuration issue found in Sonarr/Radarr.
type ArrIssue struct {
	Service  string // "sonarr", "radarr"
	Setting  string // "enableCompletedDownloadHandling", "renameEpisodes", etc.
	Current  string
	Expected string
	Severity string // "critical", "warning"
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
	service string // "sonarr", "radarr", "jellyfin", "ollama"
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

type arrIssuesMsg struct {
	issues []ArrIssue
	err    error
}

type arrFixMsg struct {
	fixed int
	err   error
}

// Scan frequency options
var scanFrequencyOptions = []string{
	"Every 5 minutes",
	"Every 10 minutes",
	"Every 30 minutes",
	"Hourly",
	"Every 24 hours",
}
