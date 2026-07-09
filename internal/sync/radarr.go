package sync

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
)

const radarrSourcePriority = 25

// SyncFromRadarr imports movie data from Radarr API
func (s *SyncService) SyncFromRadarr(ctx context.Context) (retErr error) {
	s.logger.Info("syncing from Radarr")

	logID, err := s.db.StartSyncLog("radarr")
	if err != nil {
		return err
	}

	// Recover from panics to avoid leaving sync_log stuck in "running"
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("panic in SyncFromRadarr: %v", r)
			if err := s.db.CompleteSyncLog(logID, "failed", 0, 0, 0, retErr.Error()); err != nil {
				s.logger.Error("sync", "Failed to complete sync log after panic", err)
			}
		}
	}()

	// Timeout: large libraries need several minutes to upsert all items
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	movies, err := s.radarr.GetMoviesContext(ctx)
	if err != nil {
		if logErr := s.db.CompleteSyncLog(logID, "failed", 0, 0, 0, err.Error()); logErr != nil {
			s.logger.Error("sync", "Failed to complete sync log", logErr)
		}
		return err
	}

	var processed, added, updated int

	for _, movie := range movies {
		select {
		case <-ctx.Done():
			if logErr := s.db.CompleteSyncLog(logID, "failed", processed, added, updated, "context cancelled"); logErr != nil {
				s.logger.Error("sync", "Failed to complete sync log", logErr)
			}
			return ctx.Err()
		default:
		}

		processed++

		record := &database.Movie{
			Title:          movie.Title,
			Year:           movie.Year,
			TmdbID:         &movie.TmdbID,
			RadarrID:       &movie.ID,
			CanonicalPath:  movie.Path,
			LibraryRoot:    filepath.Dir(movie.Path),
			Source:         "radarr",
			SourcePriority: radarrSourcePriority,
		}

		// Set IMDB ID if available
		if movie.ImdbID != "" {
			record.ImdbID = &movie.ImdbID
		}

		// Check if this is new
		existing, _ := s.db.GetMovieByTitle(movie.Title, movie.Year)
		isNew := (existing == nil)

		// UpsertMovie respects source priority - won't overwrite plex2jellyfin paths
		_, err := s.db.UpsertMovie(record)
		if err != nil {
			s.logger.Warn("failed to upsert movie", "title", movie.Title, "error", err)
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
	s.logger.Info("radarr sync completed", "processed", processed, "added", added, "updated", updated)

	return nil
}
