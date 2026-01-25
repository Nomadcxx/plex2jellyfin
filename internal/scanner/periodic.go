package scanner

import (
	"context"
	"fmt"
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

// IsHealthy returns whether the scanner is in a healthy state
func (s *PeriodicScanner) IsHealthy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.healthy
}

// Status returns the current scanner status for health reporting
func (s *PeriodicScanner) Status() ScannerStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	status := ScannerStatus{
		Healthy:      s.healthy,
		LastScan:     s.lastScan,
		LastSuccess:  s.lastSuccess,
		SkippedTicks: s.skippedTicks,
		Scanning:     s.scanning,
	}

	if s.lastError != nil {
		status.LastError = s.lastError.Error()
	}

	return status
}

// Start begins the periodic scanning loop. Blocks until context is cancelled.
func (s *PeriodicScanner) Start(ctx context.Context) error {
	s.logger.Info("scanner", "Periodic scanner starting",
		logging.F("interval", s.interval.String()),
		logging.F("watch_paths", len(s.watchPaths)))

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("scanner", "Periodic scanner stopped")
			return nil
		case <-ticker.C:
			s.tick()
		}
	}
}

func (s *PeriodicScanner) tick() {
	s.mu.Lock()
	if s.scanning {
		s.skippedTicks++
		s.mu.Unlock()
		s.logger.Warn("scanner", "Periodic scan skipped - previous scan still running",
			logging.F("skipped_ticks", s.skippedTicks))
		return
	}
	s.scanning = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.scanning = false
		s.mu.Unlock()
	}()

	if err := s.runScan(); err != nil {
		s.mu.Lock()
		s.lastError = err
		s.healthy = false
		s.mu.Unlock()
		s.logger.Error("scanner", "Periodic scan failed", err)
	} else {
		s.mu.Lock()
		s.lastSuccess = time.Now()
		s.lastError = nil
		s.healthy = true
		s.mu.Unlock()
	}

	s.mu.Lock()
	s.lastScan = time.Now()
	s.mu.Unlock()
}

func (s *PeriodicScanner) runScan() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("scan panic: %v", r)
			s.logger.Error("scanner", "Panic during periodic scan", err)
		}
	}()

	start := time.Now()
	s.logger.Info("scanner", "Periodic scan starting")

	// Phase 1: Scan watch directories
	processed, errors := s.scanWatchDirectories()

	// Phase 2: Reconcile activity logs
	retried, cleaned, reconcileErr := s.reconcileActivity()
	if reconcileErr != nil {
		s.logger.Warn("scanner", "Activity reconciliation had errors",
			logging.F("error", reconcileErr.Error()))
	}

	elapsed := time.Since(start)
	s.logger.Info("scanner", "Periodic scan complete",
		logging.F("duration_ms", elapsed.Milliseconds()),
		logging.F("processed", processed),
		logging.F("errors", errors),
		logging.F("retried", retried),
		logging.F("cleaned", cleaned))

	return nil
}
