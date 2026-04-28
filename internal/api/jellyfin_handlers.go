package api

import (
	"encoding/json"
	"net/http"

	"github.com/Nomadcxx/jellywatch/internal/database"
)

// JellyfinHandlers exposes Jellyfin bridge status endpoints.
type JellyfinHandlers struct {
	DB *database.MediaDB
}

// IdentificationStatus is the JSON shape returned by /jellyfin/identification.
// Counts are computed over parse_decisions rows that completed an organize
// (target_path is non-empty AND organize_outcome='success').
type IdentificationStatus struct {
	Total            int `json:"total"`
	Resolved         int `json:"resolved"`
	Identified       int `json:"identified"`
	Unidentified     int `json:"unidentified"`
	PendingNoSeen    int `json:"pending_no_seen"`
	FailedAutoLabel  int `json:"failed_auto_label"`
	IdentifiedPctX10 int `json:"identified_pct_x10"` // identified*1000/resolved (avoid float)
}

// Identification returns a count breakdown of the rename → identification
// pipeline so the web UI can render a single dashboard card.
func (h *JellyfinHandlers) Identification(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.DB == nil {
		http.Error(w, "db unavailable", http.StatusServiceUnavailable)
		return
	}

	stats, err := h.DB.IdentificationStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pct := 0
	if stats.Resolved > 0 {
		pct = stats.Identified * 1000 / stats.Resolved
	}
	resp := IdentificationStatus{
		Total:            stats.Total,
		Resolved:         stats.Resolved,
		Identified:       stats.Identified,
		Unidentified:     stats.Unidentified,
		PendingNoSeen:    stats.PendingNoSeen,
		FailedAutoLabel:  stats.FailedAutoLabel,
		IdentifiedPctX10: pct,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
