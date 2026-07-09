package sync

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
)

const sonarrSourcePriority = 25

// SyncFromSonarr imports series data from Sonarr API
func (s *SyncService) SyncFromSonarr(ctx context.Context) (retErr error) {
	s.logger.Info("syncing from Sonarr")

	logID, err := s.db.StartSyncLog("sonarr")
	if err != nil {
		return err
	}

	// Recover from panics to avoid leaving sync_log stuck in "running"
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("panic in SyncFromSonarr: %v", r)
			if err := s.db.CompleteSyncLog(logID, "failed", 0, 0, 0, retErr.Error()); err != nil {
				s.logger.Error("sync", "Failed to complete sync log after panic", err)
			}
		}
	}()

	// Timeout: large libraries need several minutes to upsert all items
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	series, err := s.sonarr.GetAllSeries()
	if err != nil {
		if logErr := s.db.CompleteSyncLog(logID, "failed", 0, 0, 0, err.Error()); logErr != nil {
			s.logger.Error("sync", "Failed to complete sync log", logErr)
		}
		return err
	}

	var processed, added, updated int

	for _, show := range series {
		select {
		case <-ctx.Done():
			if logErr := s.db.CompleteSyncLog(logID, "failed", processed, added, updated, "context cancelled"); logErr != nil {
				s.logger.Error("sync", "Failed to complete sync log", logErr)
			}
			return ctx.Err()
		default:
		}

		processed++

		// Extract episode count from statistics
		episodeCount := 0
		if show.Statistics != nil {
			episodeCount = show.Statistics.EpisodeFileCount
		}

		record := &database.Series{
			Title:          show.Title,
			Year:           show.Year,
			TvdbID:         &show.TvdbID,
			SonarrID:       &show.ID,
			CanonicalPath:  show.Path,
			LibraryRoot:    filepath.Dir(show.Path),
			Source:         "sonarr",
			SourcePriority: sonarrSourcePriority,
			EpisodeCount:   episodeCount,
		}

		// Set IMDB ID if available
		if show.ImdbID != "" {
			record.ImdbID = &show.ImdbID
		}

		// Check if this is new
		existing, _ := s.db.GetSeriesByTitle(show.Title, show.Year)
		isNew := (existing == nil)

		// UpsertSeries respects source priority - won't overwrite plex2jellyfin paths
		_, err := s.db.UpsertSeries(record)
		if err != nil {
			s.logger.Warn("failed to upsert series", "title", show.Title, "error", err)
			continue
		}

		if isNew {
			added++
		} else {
			updated++
		}
	}

	if logErr := s.db.CompleteSyncLog(logID, "success", processed, added, updated, ""); logErr != nil {
		s.logger.Error("sync", "Failed to complete sync log", logErr)
	}
	s.logger.Info("sonarr sync completed", "processed", processed, "added", added, "updated", updated)

	return nil
}
