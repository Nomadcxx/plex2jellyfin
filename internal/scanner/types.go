package scanner

import (
	"time"

	"github.com/Nomadcxx/jellywatch/internal/daemon"
	"github.com/Nomadcxx/jellywatch/internal/logging"
)

// ScannerConfig holds configuration for the periodic scanner
type ScannerConfig struct {
	Interval    time.Duration
	WatchPaths  []string
	Handler     *daemon.MediaHandler
	Logger      *logging.Logger
	ActivityDir string
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
