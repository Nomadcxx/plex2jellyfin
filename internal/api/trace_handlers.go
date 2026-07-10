package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
)

const (
	traceDefaultLimit = 25
	traceMaxLimit     = 200
)

// TraceJellyfin is the Jellyfin-resolution portion of a trace item.
type TraceJellyfin struct {
	ItemID     string     `json:"item_id,omitempty"`
	ImdbID     string     `json:"imdb_id,omitempty"`
	TmdbID     string     `json:"tmdb_id,omitempty"`
	TvdbID     string     `json:"tvdb_id,omitempty"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	Identified *bool      `json:"identified,omitempty"`
}

// TraceItem is one file's journey through the pipeline, assembled from a
// parse_decisions row.
type TraceItem struct {
	ID             int64          `json:"id"`
	SourcePath     string         `json:"source_path"`
	SourceFilename string         `json:"source_filename"`
	EventAt        time.Time      `json:"event_at"`
	MediaType      string         `json:"media_type,omitempty"`
	ParseMethod    string         `json:"parse_method,omitempty"`
	ParsedTitle    string         `json:"parsed_title,omitempty"`
	ParsedYear     *int           `json:"parsed_year,omitempty"`
	ParsedSeason   *int           `json:"parsed_season,omitempty"`
	ParsedEpisode  *int           `json:"parsed_episode,omitempty"`
	StrippedTokens string         `json:"stripped_tokens,omitempty"`
	TargetPath     string         `json:"target_path,omitempty"`
	TargetAt       *time.Time     `json:"target_at,omitempty"`
	Outcome        string         `json:"outcome,omitempty"`
	Error          string         `json:"error,omitempty"`
	Jellyfin       *TraceJellyfin `json:"jellyfin,omitempty"`
	Label          string         `json:"label,omitempty"`
}

func traceItemFromDecision(d *database.ParseDecision) TraceItem {
	item := TraceItem{
		ID:             d.ID,
		SourcePath:     d.SourcePath,
		SourceFilename: d.SourceFilename,
		EventAt:        d.EventAt,
		MediaType:      d.MediaTypeGuessed,
		ParseMethod:    d.ParseMethod,
		ParsedTitle:    d.ParsedTitle,
		ParsedYear:     d.ParsedYear,
		ParsedSeason:   d.ParsedSeason,
		ParsedEpisode:  d.ParsedEpisode,
		StrippedTokens: d.ParserStrippedTokens,
		TargetPath:     d.TargetPath,
		TargetAt:       d.TargetAt,
		Outcome:        d.OrganizeOutcome,
		Error:          d.OrganizeError,
		Label:          d.AutoLabel,
	}
	if d.HumanLabelOverride != "" {
		item.Label = d.HumanLabelOverride
	}
	if d.JellyfinItemID != "" || d.JellyfinResolvedAt != nil {
		item.Jellyfin = &TraceJellyfin{
			ItemID:     d.JellyfinItemID,
			ImdbID:     d.JellyfinImdbID,
			TmdbID:     d.JellyfinTmdbID,
			TvdbID:     d.JellyfinTvdbID,
			ResolvedAt: d.JellyfinResolvedAt,
			Identified: d.JellyfinIdentified,
		}
	}
	return item
}

// GetFileTrace returns recent per-file pipeline traces, newest first,
// optionally filtered by a source-path substring (?q=).
func (s *Server) GetFileTrace(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "database unavailable",
		})
		return
	}

	limit := traceDefaultLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = min(n, traceMaxLimit)
		}
	}

	rows, err := s.db.QueryDecisions(database.QueryFilter{
		SourceContains: r.URL.Query().Get("q"),
		Limit:          limit,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	items := make([]TraceItem, 0, len(rows))
	for _, d := range rows {
		items = append(items, traceItemFromDecision(d))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": items,
	})
}
