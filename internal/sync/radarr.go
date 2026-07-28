package sync

import (
	"context"
	"errors"
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
	var reconcileErr error

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
		reconcile, err := s.db.UpsertMovie(record)
		if err != nil {
			s.logger.Warn("failed to upsert movie", "title", movie.Title, "error", err)
			continue
		}
		if reconcile {
			canonical, getErr := s.db.GetMovieByID(record.ID)
			if getErr != nil {
				reconcileErr = errors.Join(reconcileErr, fmt.Errorf("load canonical movie %q: %w", movie.Title, getErr))
				continue
			}
			if canonical == nil {
				reconcileErr = errors.Join(reconcileErr, fmt.Errorf("load canonical movie %q: record disappeared", movie.Title))
				continue
			}
			if err := s.db.SetMovieDirty(canonical.ID); err != nil {
				reconcileErr = errors.Join(reconcileErr, fmt.Errorf("mark movie %q dirty: %w", movie.Title, err))
				continue
			}
			if err := s.radarr.UpdateMoviePathContext(ctx, movie.ID, canonical.CanonicalPath); err != nil {
				reconcileErr = errors.Join(reconcileErr, fmt.Errorf("reconcile Radarr movie %q: %w", movie.Title, err))
				continue
			}
			if _, err := s.radarr.RescanMovie(movie.ID); err != nil {
				reconcileErr = errors.Join(reconcileErr, fmt.Errorf("rescan Radarr movie %q: %w", movie.Title, err))
				continue
			}
			if err := s.db.MarkMovieSynced(canonical.ID); err != nil {
				reconcileErr = errors.Join(reconcileErr, fmt.Errorf("mark movie %q synced: %w", movie.Title, err))
			}
		}

		if isNew {
			added++
		} else {
			updated++
		}
	}

	status, detail := "success", ""
	if reconcileErr != nil {
		status, detail = "failed", reconcileErr.Error()
	}
	if logErr := s.db.CompleteSyncLog(logID, status, processed, added, updated, detail); logErr != nil {
		s.logger.Error("sync", "Failed to complete sync log", logErr)
	}
	s.logger.Info("radarr sync completed", "processed", processed, "added", added, "updated", updated)

	return reconcileErr
}
