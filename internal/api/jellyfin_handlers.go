package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

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

// IdentificationItem is the per-row JSON shape returned by /jellyfin/identification/items.
type IdentificationItem struct {
	ID            int64   `json:"id"`
	SourcePath    string  `json:"source_path"`
	TargetPath    string  `json:"target_path"`
	ParsedTitle   string  `json:"parsed_title,omitempty"`
	ParsedYear    *int    `json:"parsed_year,omitempty"`
	MediaType     string  `json:"media_type,omitempty"`
	JellyfinItem  string  `json:"jellyfin_item_id,omitempty"`
	ImdbID        string  `json:"imdb_id,omitempty"`
	TmdbID        string  `json:"tmdb_id,omitempty"`
	TvdbID        string  `json:"tvdb_id,omitempty"`
	ResolvedAt    string  `json:"resolved_at,omitempty"`
	FirstSeenAt   string  `json:"first_seen_at,omitempty"`
	TargetAt      string  `json:"target_at,omitempty"`
	AutoLabel     string  `json:"auto_label,omitempty"`
	Identified    *bool   `json:"identified,omitempty"`
}

// IdentificationItems lists parse_decisions rows for one of the
// identification statuses surfaced on the dashboard. Status is taken
// from the "status" query param: unidentified|pending|failed|identified.
// Limit defaults to 200, max 1000.
func (h *JellyfinHandlers) IdentificationItems(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.DB == nil {
		http.Error(w, "db unavailable", http.StatusServiceUnavailable)
		return
	}
	status := database.IdentificationStatusFilter(r.URL.Query().Get("status"))
	limit := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	rows, err := h.DB.ListIdentificationItems(status, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	out := make([]IdentificationItem, 0, len(rows))
	for _, d := range rows {
		item := IdentificationItem{
			ID:           d.ID,
			SourcePath:   d.SourcePath,
			TargetPath:   d.TargetPath,
			ParsedTitle:  d.ParsedTitle,
			ParsedYear:   d.ParsedYear,
			MediaType:    d.MediaTypeGuessed,
			JellyfinItem: d.JellyfinItemID,
			ImdbID:       d.JellyfinImdbID,
			TmdbID:       d.JellyfinTmdbID,
			TvdbID:       d.JellyfinTvdbID,
			AutoLabel:    d.AutoLabel,
			Identified:   d.JellyfinIdentified,
		}
		if d.JellyfinResolvedAt != nil {
			item.ResolvedAt = d.JellyfinResolvedAt.Format(time.RFC3339)
		}
		if d.JellyfinFirstSeenAt != nil {
			item.FirstSeenAt = d.JellyfinFirstSeenAt.Format(time.RFC3339)
		}
		if d.TargetAt != nil {
			item.TargetAt = d.TargetAt.Format(time.RFC3339)
		}
		out = append(out, item)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": string(status),
		"count":  len(out),
		"items":  out,
	})
}
