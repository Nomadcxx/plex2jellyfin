package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/jellystat"
)

func (s *Server) jellystatClient() *jellystat.Client {
	if s.cfg == nil || !s.cfg.Jellystat.Enabled || s.cfg.Jellystat.URL == "" {
		return nil
	}
	return jellystat.NewClient(jellystat.Config{
		URL:     s.cfg.Jellystat.URL,
		APIKey:  s.cfg.Jellystat.APIKey,
		Timeout: 8 * time.Second,
	})
}

// GetJellystatOverview aggregates the dashboard-card data from Jellystat.
// Returns {"enabled": false} when the integration is not configured so the
// frontend can hide the cards without treating it as an error.
func (s *Server) GetJellystatOverview(w http.ResponseWriter, r *http.Request) {
	client := s.jellystatClient()
	if client == nil {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}

	resp := map[string]any{"enabled": true}
	var firstErr string

	collect := func(key string, fetch func() (json.RawMessage, error)) {
		raw, err := fetch()
		if err != nil {
			if firstErr == "" {
				firstErr = err.Error()
			}
			return
		}
		resp[key] = raw
	}

	collect("libraries", client.LibraryOverview)
	collect("most_viewed_movies", func() (json.RawMessage, error) {
		return client.MostViewedByType(30, "Movie")
	})
	collect("most_viewed_series", func() (json.RawMessage, error) {
		return client.MostViewedByType(30, "Series")
	})

	if firstErr != "" {
		resp["error"] = firstErr
	}
	writeJSON(w, http.StatusOK, resp)
}

// GetJellystatItemStats proxies per-item play stats
// (?item_id=<jellyfin item>&hours=720).
func (s *Server) GetJellystatItemStats(w http.ResponseWriter, r *http.Request) {
	client := s.jellystatClient()
	if client == nil {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}

	itemID := r.URL.Query().Get("item_id")
	if itemID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "item_id is required"})
		return
	}
	hours := 720
	if raw := r.URL.Query().Get("hours"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			hours = n
		}
	}

	raw, err := client.GlobalItemStats(hours, itemID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "stats": raw})
}
