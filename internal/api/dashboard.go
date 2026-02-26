package api

import (
	"log"
	"net/http"

	"github.com/Nomadcxx/jellywatch/api"
	"github.com/Nomadcxx/jellywatch/internal/config"
)

// DashboardData represents the full dashboard response
type DashboardData struct {
	LibraryStats   LibraryStats          `json:"libraryStats"`
	MediaManagers  []MediaManagerSummary `json:"mediaManagers"`
	LLMProvider    *LLMProviderSummary   `json:"llmProvider,omitempty"`
	RecentActivity []ActivityEvent       `json:"recentActivity"`
}

// LibraryStats represents library statistics
type LibraryStats struct {
	TotalFiles       int   `json:"totalFiles"`
	TotalSize        int64 `json:"totalSize"`
	MovieCount       int   `json:"movieCount"`
	SeriesCount      int   `json:"seriesCount"`
	EpisodeCount     int   `json:"episodeCount"`
	DuplicateGroups  int   `json:"duplicateGroups"`
	ReclaimableBytes int64 `json:"reclaimableBytes"`
	ScatteredSeries  int   `json:"scatteredSeries"`
}

// MediaManagerSummary represents a media manager in the dashboard
type MediaManagerSummary struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Online     bool   `json:"online"`
	QueueSize  int    `json:"queueSize"`
	StuckCount int    `json:"stuckCount"`
}

// LLMProviderSummary represents LLM provider status
type LLMProviderSummary struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Online       bool   `json:"online"`
	CurrentModel string `json:"currentModel"`
}

// ActivityEvent represents an activity entry
type ActivityEvent struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

// GetDashboard returns aggregate dashboard data
func (s *Server) GetDashboard(w http.ResponseWriter, r *http.Request) {
	// Get library stats from database
	libStats, err := s.db.GetLibraryStats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "stats_error", "Failed to get library stats: "+err.Error())
		return
	}
	if scattered, err := s.service.AnalyzeScattered(); err == nil {
		libStats.ScatteredSeries = scattered.TotalItems
	} else {
		log.Printf("api: dashboard scattered summary fallback: %v", err)
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
	if cfg == nil {
		return nil
	}

	var managers []api.MediaManagerSummary

	addManager := func(managerID, managerName string) {
		managerType := managerID
		online := false
		queueSize := 0
		stuckCount := 0

		if client, err := getManagerClient(cfg, managerID); err == nil {
			if _, err := client.GetSystemStatus(); err == nil {
				online = true

				if items, err := client.GetAllQueueItems(); err == nil {
					queueSize = len(items)
				}
				if stuckItems, err := client.GetStuckItems(); err == nil {
					stuckCount = len(stuckItems)
				}
			}
		}

		id := managerID
		name := managerName
		managers = append(managers, api.MediaManagerSummary{
			Id:         &id,
			Name:       &name,
			Type:       &managerType,
			Online:     &online,
			QueueSize:  &queueSize,
			StuckCount: &stuckCount,
		})
	}

	if isManagerConfigured(cfg, "sonarr") {
		addManager("sonarr", "Sonarr")
	}
	if isManagerConfigured(cfg, "radarr") {
		addManager("radarr", "Radarr")
	}
	if isManagerConfigured(cfg, "jellyfin") {
		addManager("jellyfin", "Jellyfin")
	}

	// Add Jellyfin if enabled
	if cfg.Jellyfin.Enabled && cfg.Jellyfin.URL != "" && cfg.Jellyfin.APIKey != "" {
		id := "jellyfin"
		name := "Jellyfin"
		managerType := "jellyfin"
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
