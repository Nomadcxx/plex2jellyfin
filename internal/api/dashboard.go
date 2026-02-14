package api

import (
	"net/http"

	"github.com/Nomadcxx/jellywatch/api"
	"github.com/Nomadcxx/jellywatch/internal/config"
)

// GetDashboard returns aggregate dashboard data
func (s *Server) GetDashboard(w http.ResponseWriter, r *http.Request) {
	// Get library stats from database
	libStats, err := s.db.GetLibraryStats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "stats_error", "Failed to get library stats: "+err.Error())
		return
	}

	// Build library stats response
	libraryStats := api.LibraryStats{
		TotalFiles:       &libStats.TotalFiles,
		TotalSize:        &libStats.TotalSize,
		MovieCount:       &libStats.MovieCount,
		SeriesCount:      &libStats.SeriesCount,
		EpisodeCount:     &libStats.EpisodeCount,
		DuplicateGroups:  &libStats.DuplicateGroups,
		ReclaimableBytes: &libStats.ReclaimableBytes,
		ScatteredSeries:  &libStats.ScatteredSeries,
	}

	// Build media managers list from config
	mediaManagers := buildMediaManagers(s.cfg)

	// Build response
	response := api.DashboardData{
		LibraryStats:  &libraryStats,
		MediaManagers: &mediaManagers,
	}

	writeJSON(w, http.StatusOK, response)
}

// buildMediaManagers creates a list of media managers from config
func buildMediaManagers(cfg *config.Config) []api.MediaManagerSummary {
	var managers []api.MediaManagerSummary

	// Add Sonarr if enabled
	if cfg.Sonarr.Enabled && cfg.Sonarr.URL != "" {
		id := "sonarr"
		name := "Sonarr"
		managerType := "sonarr"
		online := false // Will be updated when actual health check is implemented
		queueSize := 0
		stuckCount := 0

		managers = append(managers, api.MediaManagerSummary{
			Id:         &id,
			Name:       &name,
			Type:       &managerType,
			Online:     &online,
			QueueSize:  &queueSize,
			StuckCount: &stuckCount,
		})
	}

	// Add Radarr if enabled
	if cfg.Radarr.Enabled && cfg.Radarr.URL != "" {
		id := "radarr"
		name := "Radarr"
		managerType := "radarr"
		online := false // Will be updated when actual health check is implemented
		queueSize := 0
		stuckCount := 0

		managers = append(managers, api.MediaManagerSummary{
			Id:         &id,
			Name:       &name,
			Type:       &managerType,
			Online:     &online,
			QueueSize:  &queueSize,
			StuckCount: &stuckCount,
		})
	}

	return managers
}