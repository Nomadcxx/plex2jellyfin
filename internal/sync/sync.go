package sync

import (
	"context"
	"fmt"
	"log/slog"
	stdsync "sync"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/radarr"
	"github.com/Nomadcxx/jellywatch/internal/scanner"
	"github.com/Nomadcxx/jellywatch/internal/sonarr"
)

// SyncService manages periodic synchronization from multiple sources
type SyncService struct {
	db             *database.MediaDB
	sonarr         *sonarr.Client
	radarr         *radarr.Client
	tvLibraries    []string
	movieLibraries []string
	logger         *slog.Logger
	aiHelper       *scanner.AIHelper

	syncHour int
	stopCh   chan struct{}
	stopOnce stdsync.Once

	syncChan      chan SyncRequest
	retryInterval time.Duration
}

// SyncRequest represents a request to sync a media item to external services
type SyncRequest struct {
	MediaType string
	ID        int64
}

// SyncConfig holds configuration for SyncService
type SyncConfig struct {
	DB             *database.MediaDB
	Sonarr         *sonarr.Client    // nil if not configured
	Radarr         *radarr.Client    // nil if not configured
	AIHelper       *scanner.AIHelper // Optional AI helper for auto-trigger
	TVLibraries    []string
	MovieLibraries []string
	SyncHour       int // Hour for daily sync, default 3
	Logger         *slog.Logger
}

// NewSyncService creates a new sync service
func NewSyncService(cfg SyncConfig) *SyncService {
	if cfg.SyncHour < 0 || cfg.SyncHour > 23 {
		cfg.SyncHour = 3
	}

	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &SyncService{
		db:             cfg.DB,
		sonarr:         cfg.Sonarr,
		radarr:         cfg.Radarr,
		aiHelper:       cfg.AIHelper,
		tvLibraries:    cfg.TVLibraries,
		movieLibraries: cfg.MovieLibraries,
		syncHour:       cfg.SyncHour,
		logger:         cfg.Logger,
		stopCh:         make(chan struct{}),
		syncChan:       make(chan SyncRequest, 100),
		retryInterval:  5 * time.Minute,
	}
}

// Start begins the daily sync scheduler in a background goroutine
func (s *SyncService) Start() {
	go s.runScheduler()
	go s.runSyncWorker()
}

// Stop stops the scheduler (safe to call multiple times)
func (s *SyncService) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
		close(s.syncChan)
	})
}

// runScheduler runs the daily sync at the configured hour
func (s *SyncService) runScheduler() {
	for {
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day(), s.syncHour, 0, 0, 0, now.Location())
		if now.After(next) {
			next = next.Add(24 * time.Hour)
		}

		waitDuration := next.Sub(now)
		s.logger.Info("next sync scheduled", "at", next, "in", waitDuration)

		select {
		case <-time.After(waitDuration):
			s.RunFullSync(context.Background())
		case <-s.stopCh:
			s.logger.Info("sync scheduler stopped")
			return
		}
	}
}

// RunFullSync runs a complete sync from all sources
func (s *SyncService) RunFullSync(ctx context.Context) error {
	s.logger.Info("starting full sync")
	startTime := time.Now()

	// 1. Sync from Sonarr (if available)
	if s.sonarr != nil {
		if err := s.SyncFromSonarr(ctx); err != nil {
			s.logger.Error("sonarr sync failed", "error", err)
			// Continue with other syncs
		}
	} else {
		s.logger.Debug("sonarr not configured, skipping")
	}

	// 2. Sync from Radarr (if available)
	if s.radarr != nil {
		if err := s.SyncFromRadarr(ctx); err != nil {
			s.logger.Error("radarr sync failed", "error", err)
		}
	} else {
		s.logger.Debug("radarr not configured, skipping")
	}

	// 3. Always scan filesystem (catches manual additions, verifies counts)
	if _, err := s.SyncFromFilesystem(ctx); err != nil {
		s.logger.Error("filesystem sync failed", "error", err)
	}

	duration := time.Since(startTime)
	s.logger.Info("full sync completed", "duration", duration)
	return nil
}

// SyncNow triggers an immediate sync (for CLI use)
func (s *SyncService) SyncNow(ctx context.Context) error {
	return s.RunFullSync(ctx)
}

// runSyncWorker processes immediate sync requests from channel
func (s *SyncService) runSyncWorker() {
	for {
		select {
		case req := <-s.syncChan:
			s.processSyncRequest(context.Background(), req)
		case <-s.stopCh:
			s.logger.Info("sync worker stopped")
			return
		}
	}
}

// processSyncRequest handles a single sync request
func (s *SyncService) processSyncRequest(ctx context.Context, req SyncRequest) {
	switch req.MediaType {
	case "series":
		if s.sonarr == nil {
			s.logger.Debug("sonarr not configured, skipping series sync")
			return
		}
		series, err := s.db.GetSeriesByID(req.ID)
		if err != nil || series == nil {
			s.logger.Warn("failed to get series for sync", "id", req.ID, "error", err)
			return
		}

		if series.SonarrID == nil || *series.SonarrID <= 0 {
			s.logger.Debug("series has no Sonarr ID, skipping", "id", req.ID)
			return
		}

		s.logger.Info("syncing series to Sonarr", "id", req.ID, "sonarr_id", *series.SonarrID, "path", series.CanonicalPath)
		err = s.sonarr.UpdateSeriesPath(*series.SonarrID, series.CanonicalPath)
		if err != nil {
			s.logger.Error("failed to update Sonarr path", "id", req.ID, "error", err)
			return
		}

		if err := s.db.MarkSeriesSynced(req.ID); err != nil {
			s.logger.Error("failed to mark series synced", "id", req.ID, "error", err)
		}

	case "movie":
		if s.radarr == nil {
			s.logger.Debug("radarr not configured, skipping movie sync")
			return
		}
		movie, err := s.db.GetMovieByID(req.ID)
		if err != nil || movie == nil {
			s.logger.Warn("failed to get movie for sync", "id", req.ID, "error", err)
			return
		}

		if movie.RadarrID == nil || *movie.RadarrID <= 0 {
			s.logger.Debug("movie has no Radarr ID, skipping", "id", req.ID)
			return
		}

		s.logger.Info("syncing movie to Radarr", "id", req.ID, "radarr_id", *movie.RadarrID, "path", movie.CanonicalPath)
		err = s.radarr.UpdateMoviePath(*movie.RadarrID, movie.CanonicalPath)
		if err != nil {
			s.logger.Error("failed to update Radarr path", "id", req.ID, "error", err)
			return
		}

		if err := s.db.MarkMovieSynced(req.ID); err != nil {
			s.logger.Error("failed to mark movie synced", "id", req.ID, "error", err)
		}
	}
}

// UpdateSonarrPath updates a series path in Sonarr to match JellyWatch
func (s *SyncService) UpdateSonarrPath(ctx context.Context, sonarrID int, newPath string) error {
	if s.sonarr == nil {
		s.logger.Debug("sonarr not configured, cannot update path")
		return nil
	}

	s.logger.Info("updating Sonarr path", "sonarr_id", sonarrID, "new_path", newPath)

	if err := s.sonarr.UpdateSeriesPath(sonarrID, newPath); err != nil {
		return fmt.Errorf("failed to update Sonarr path: %w", err)
	}

	s.logger.Info("successfully updated Sonarr path", "sonarr_id", sonarrID)
	return nil
}

// UpdateRadarrPath updates a movie path in Radarr to match JellyWatch
func (s *SyncService) UpdateRadarrPath(ctx context.Context, radarrID int, newPath string) error {
	if s.radarr == nil {
		s.logger.Debug("radarr not configured, cannot update path")
		return nil
	}

	s.logger.Info("updating Radarr path", "radarr_id", radarrID, "new_path", newPath)

	if err := s.radarr.UpdateMoviePath(radarrID, newPath); err != nil {
		return fmt.Errorf("failed to update Radarr path: %w", err)
	}

	s.logger.Info("successfully updated Radarr path", "radarr_id", radarrID)
	return nil
}
