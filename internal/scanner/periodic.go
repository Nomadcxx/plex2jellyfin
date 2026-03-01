package scanner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/logging"
	"github.com/Nomadcxx/jellywatch/internal/radarr"
	"github.com/Nomadcxx/jellywatch/internal/service"
	"github.com/Nomadcxx/jellywatch/internal/sonarr"
	"github.com/Nomadcxx/jellywatch/internal/watcher"
)

// PeriodicScanner runs periodic directory scans to catch missed files
type PeriodicScanner struct {
	// Configuration
	interval    time.Duration
	watchPaths  []string
	handler     watcher.Handler
	logger      *logging.Logger
	activityDir string
	orphanCheck OrphanChecker

	// Arr clients for health checks
	sonarrClient *sonarr.Client
	radarrClient *radarr.Client

	// State tracking
	mu           sync.Mutex
	scanning     bool
	lastScan     time.Time
	lastSuccess  time.Time
	lastError    error
	skippedTicks int64

	// Health tracking
	healthy         bool
	lastHealthCheck time.Time
}

// NewPeriodicScanner creates a new scanner with the given config
func NewPeriodicScanner(cfg ScannerConfig) *PeriodicScanner {
	return &PeriodicScanner{
		interval:     cfg.Interval,
		watchPaths:   cfg.WatchPaths,
		handler:      cfg.Handler,
		logger:       cfg.Logger,
		activityDir:  cfg.ActivityDir,
		orphanCheck:  cfg.OrphanCheck,
		sonarrClient: cfg.SonarrClient,
		radarrClient: cfg.RadarrClient,
		healthy:      true,
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

	s.checkOrphans()
	s.checkArrHealth()

	elapsed := time.Since(start)
	s.logger.Info("scanner", "Periodic scan complete",
		logging.F("duration_ms", elapsed.Milliseconds()),
		logging.F("processed", processed),
		logging.F("errors", errors),
		logging.F("retried", retried),
		logging.F("cleaned", cleaned))

	return nil
}

func (s *PeriodicScanner) checkOrphans() {
	if s.orphanCheck == nil {
		return
	}

	orphans, err := s.orphanCheck.GetOrphanedEpisodes()
	if err != nil {
		s.logger.Warn("scanner", "Orphan check failed", logging.F("error", err.Error()))
		return
	}

	if len(orphans) == 0 {
		return
	}

	s.logger.Warn("scanner", "Orphaned episodes detected", logging.F("count", len(orphans)))
	for _, orphan := range orphans {
		s.logger.Warn("scanner", "Orphaned episode",
			logging.F("id", orphan.ID),
			logging.F("name", orphan.Name),
			logging.F("path", orphan.Path),
		)
	}
}

func (s *PeriodicScanner) scanWatchDirectories() (processed int, errors int) {
	for _, watchPath := range s.watchPaths {
		s.logger.Info("scanner", "Scanning directory", logging.F("path", watchPath))

		err := filepath.Walk(watchPath, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				s.logger.Warn("scanner", "Directory inaccessible during scan",
					logging.F("path", path),
					logging.F("error", walkErr.Error()))
				return nil // Continue scanning other directories
			}

			if info.IsDir() {
				return nil
			}

			if !s.handler.IsMediaFile(path) {
				return nil
			}

			event := watcher.FileEvent{
				Type: watcher.EventCreate,
				Path: path,
			}

			if err := s.handler.HandleFileEvent(event); err != nil {
				s.logger.Warn("scanner", "Failed to process file during scan",
					logging.F("path", path),
					logging.F("error", err.Error()))
				errors++
			} else {
				processed++
			}

			return nil
		})

		if err != nil {
			s.logger.Warn("scanner", "Error walking directory",
				logging.F("path", watchPath),
				logging.F("error", err.Error()))
		}
	}

	s.logger.Info("scanner", "Watch directory scan complete",
		logging.F("processed", processed),
		logging.F("errors", errors))

	return processed, errors
}

// checkArrHealth checks Sonarr/Radarr configuration health and logs warnings.
// Rate-limited to once per hour to avoid log spam.
func (s *PeriodicScanner) checkArrHealth() {
	s.mu.Lock()
	if time.Since(s.lastHealthCheck) < time.Hour {
		s.mu.Unlock()
		return
	}
	s.lastHealthCheck = time.Now()
	s.mu.Unlock()

	if s.sonarrClient != nil {
		issues, err := service.CheckSonarrConfig(s.sonarrClient)
		if err != nil {
			s.logger.Error("scanner", "Sonarr health check failed", err)
		} else {
			for _, issue := range issues {
				s.logger.Warn("scanner", "Sonarr configuration issue",
					logging.F("setting", issue.Setting),
					logging.F("current", issue.Current),
					logging.F("expected", issue.Expected),
					logging.F("severity", issue.Severity),
				)
			}
		}
	}

	if s.radarrClient != nil {
		issues, err := service.CheckRadarrConfig(s.radarrClient)
		if err != nil {
			s.logger.Error("scanner", "Radarr health check failed", err)
		} else {
			for _, issue := range issues {
				s.logger.Warn("scanner", "Radarr configuration issue",
					logging.F("setting", issue.Setting),
					logging.F("current", issue.Current),
					logging.F("expected", issue.Expected),
					logging.F("severity", issue.Severity),
				)
			}
		}
	}
}
