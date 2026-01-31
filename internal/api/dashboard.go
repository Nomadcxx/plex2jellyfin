package api

import (
	"net/http"
)

// GetDashboard returns aggregate dashboard data
func (s *Server) GetDashboard(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"libraryStats": map[string]interface{}{
			"totalFiles":       0,
			"totalSize":        0,
			"movieCount":       0,
			"seriesCount":      0,
			"episodeCount":     0,
			"duplicateGroups":  0,
			"reclaimableBytes": 0,
			"scatteredSeries":  0,
		},
		"mediaManagers":  []interface{}{},
		"recentActivity": []interface{}{},
	}
	writeJSON(w, http.StatusOK, data)
}
