package scanner

import (
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin"
	"github.com/Nomadcxx/plex2jellyfin/internal/logging"
	"github.com/Nomadcxx/plex2jellyfin/internal/radarr"
	"github.com/Nomadcxx/plex2jellyfin/internal/sonarr"
	"github.com/Nomadcxx/plex2jellyfin/internal/watcher"
)

type OrphanChecker interface {
	GetOrphanedEpisodes() ([]jellyfin.Item, error)
}

// ScannerConfig holds configuration for periodic scanner
type ScannerConfig struct {
	Interval    time.Duration
	WatchPaths  []string
	Handler     watcher.Handler
	Logger      *logging.Logger
	ActivityDir string
	OrphanCheck OrphanChecker

	// Arr health check (optional)
	SonarrClient *sonarr.Client
	RadarrClient *radarr.Client
}

// ScannerStatus holds the current state for health reporting
type ScannerStatus struct {
	Healthy      bool      `json:"healthy"`
	LastScan     time.Time `json:"last_scan,omitempty"`
	LastSuccess  time.Time `json:"last_success,omitempty"`
	LastError    string    `json:"last_error,omitempty"`
	SkippedTicks int64     `json:"skipped_ticks"`
	Scanning     bool      `json:"scanning"`
}
