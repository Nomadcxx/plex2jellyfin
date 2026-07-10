package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin"
	"github.com/Nomadcxx/plex2jellyfin/internal/radarr"
	"github.com/Nomadcxx/plex2jellyfin/internal/jellystat"
	"github.com/Nomadcxx/plex2jellyfin/internal/sonarr"
)

type TestHandlers struct {
	Cfg *config.Config
}

type testResult struct {
	OK      bool   `json:"ok"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

// connectionTestPayload mirrors the JSON the UI sends. Config structs use
// mapstructure tags only, so we decode the wire format directly here.
type connectionTestPayload struct {
	URL    string `json:"url"`
	APIKey string `json:"api_key"`
}

// resolveSecret returns the request-supplied secret unless it's a mask,
// in which case the live config secret is substituted.
func (h *TestHandlers) resolveSecret(supplied, live string) string {
	if isMaskedSecret(supplied) {
		return live
	}
	return supplied
}

func decodeTestPayload(r *http.Request) (connectionTestPayload, error) {
	var p connectionTestPayload
	err := json.NewDecoder(r.Body).Decode(&p)
	return p, err
}

func (h *TestHandlers) Sonarr(w http.ResponseWriter, r *http.Request) {
	p, err := decodeTestPayload(r)
	if err != nil {
		writeJSON(w, http.StatusOK, testResult{OK: false, Error: err.Error()})
		return
	}
	if h.Cfg != nil {
		p.APIKey = h.resolveSecret(p.APIKey, h.Cfg.Sonarr.APIKey)
	}
	cli := sonarr.NewClient(sonarr.Config{URL: p.URL, APIKey: p.APIKey, Timeout: 5 * time.Second})
	status, err := cli.GetSystemStatus()
	if err != nil {
		writeJSON(w, http.StatusOK, testResult{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, testResult{OK: true, Version: status.Version})
}

func (h *TestHandlers) Radarr(w http.ResponseWriter, r *http.Request) {
	p, err := decodeTestPayload(r)
	if err != nil {
		writeJSON(w, http.StatusOK, testResult{OK: false, Error: err.Error()})
		return
	}
	if h.Cfg != nil {
		p.APIKey = h.resolveSecret(p.APIKey, h.Cfg.Radarr.APIKey)
	}
	cli := radarr.NewClient(radarr.Config{URL: p.URL, APIKey: p.APIKey, Timeout: 5 * time.Second})
	status, err := cli.GetSystemStatus()
	if err != nil {
		writeJSON(w, http.StatusOK, testResult{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, testResult{OK: true, Version: status.Version})
}

func (h *TestHandlers) Jellyfin(w http.ResponseWriter, r *http.Request) {
	p, err := decodeTestPayload(r)
	if err != nil {
		writeJSON(w, http.StatusOK, testResult{OK: false, Error: err.Error()})
		return
	}
	if h.Cfg != nil {
		p.APIKey = h.resolveSecret(p.APIKey, h.Cfg.Jellyfin.APIKey)
	}
	cli := jellyfin.NewClient(jellyfin.Config{URL: p.URL, APIKey: p.APIKey, Timeout: 5 * time.Second})
	info, err := cli.GetSystemInfo()
	if err != nil {
		writeJSON(w, http.StatusOK, testResult{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, testResult{OK: true, Version: info.Version})
}

func (h *TestHandlers) Jellystat(w http.ResponseWriter, r *http.Request) {
	p, err := decodeTestPayload(r)
	if err != nil {
		writeJSON(w, http.StatusOK, testResult{OK: false, Error: err.Error()})
		return
	}
	if h.Cfg != nil {
		p.APIKey = h.resolveSecret(p.APIKey, h.Cfg.Jellystat.APIKey)
	}
	cli := jellystat.NewClient(jellystat.Config{URL: p.URL, APIKey: p.APIKey, Timeout: 5 * time.Second})
	if err := cli.Test(); err != nil {
		writeJSON(w, http.StatusOK, testResult{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, testResult{OK: true})
}
