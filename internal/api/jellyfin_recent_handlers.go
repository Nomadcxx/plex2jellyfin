package api

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin"
	"github.com/go-chi/chi/v5"
)

var jellyfinItemIDRE = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type recentlyAddedItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	SeriesName  string `json:"series_name,omitempty"`
	DateCreated string `json:"date_created,omitempty"`
	ImageItemID string `json:"image_item_id"`
}

func (s *Server) jellyfinClientFromConfig() *jellyfin.Client {
	if s.cfg == nil || !s.cfg.Jellyfin.Enabled || strings.TrimSpace(s.cfg.Jellyfin.URL) == "" || strings.TrimSpace(s.cfg.Jellyfin.APIKey) == "" {
		return nil
	}
	return jellyfin.NewClient(jellyfin.Config{
		URL:     s.cfg.Jellyfin.URL,
		APIKey:  s.cfg.Jellyfin.APIKey,
		Timeout: 12 * time.Second,
	})
}

// GetJellyfinRecentlyAdded returns recent library items from Jellyfin.
// Soft-disables with {"enabled":false} when Jellyfin is not configured.
func (s *Server) GetJellyfinRecentlyAdded(w http.ResponseWriter, r *http.Request) {
	client := s.jellyfinClientFromConfig()
	if client == nil {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}

	limit := 24
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	items, err := client.GetLatestItems(r.Context(), limit)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": true,
			"error":   err.Error(),
			"items":   []recentlyAddedItem{},
		})
		return
	}

	out := make([]recentlyAddedItem, 0, len(items))
	for _, item := range items {
		imageID := item.ID
		if item.Type == "Episode" && strings.TrimSpace(item.SeriesID) != "" {
			imageID = item.SeriesID
		}
		out = append(out, recentlyAddedItem{
			ID:          item.ID,
			Name:        item.Name,
			Type:        item.Type,
			SeriesName:  item.SeriesName,
			DateCreated: item.DateCreated,
			ImageItemID: imageID,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "items": out})
}

// ProxyJellyfinPrimaryImage streams a Primary image through p2j so the browser
// never sees the Jellyfin API key.
func (s *Server) ProxyJellyfinPrimaryImage(w http.ResponseWriter, r *http.Request) {
	client := s.jellyfinClientFromConfig()
	if client == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "jellyfin not configured"})
		return
	}

	id := chi.URLParam(r, "id")
	if !jellyfinItemIDRE.MatchString(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid item id"})
		return
	}

	ct, body, err := client.GetPrimaryImage(r.Context(), id, 320, 213, 50)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
