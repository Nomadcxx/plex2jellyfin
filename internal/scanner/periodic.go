package scanner

import (
	"sync"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/daemon"
	"github.com/Nomadcxx/jellywatch/internal/logging"
)

// PeriodicScanner runs periodic directory scans to catch missed files
type PeriodicScanner struct {
	// Configuration
	interval    time.Duration
	watchPaths  []string
	handler     *daemon.MediaHandler
	logger      *logging.Logger
	activityDir string

	// State tracking
	mu           sync.Mutex
	scanning     bool
	lastScan     time.Time
	lastSuccess  time.Time
	lastError    error
	skippedTicks int64

	// Health tracking
	healthy bool
}

// NewPeriodicScanner creates a new scanner with the given config
func NewPeriodicScanner(cfg ScannerConfig) *PeriodicScanner {
	return &PeriodicScanner{
		interval:    cfg.Interval,
		watchPaths:  cfg.WatchPaths,
		handler:     cfg.Handler,
		logger:      cfg.Logger,
		activityDir: cfg.ActivityDir,
		healthy:     true,
	}
}
