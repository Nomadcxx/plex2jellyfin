package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin"
	"github.com/Nomadcxx/plex2jellyfin/internal/jellystat"
	"github.com/Nomadcxx/plex2jellyfin/internal/radarr"
	"github.com/Nomadcxx/plex2jellyfin/internal/service"
	"github.com/Nomadcxx/plex2jellyfin/internal/sonarr"
)

type TestHandlers struct {
	Cfg *config.Config
}

type testResult struct {
	OK                 bool     `json:"ok"`
	Version            string   `json:"version,omitempty"`
	Error              string   `json:"error,omitempty"`
	UnmappedLocations  []string `json:"unmapped_locations,omitempty"`
	PathMappingWarning string   `json:"path_mapping_warning,omitempty"`
}

type compatibilityResult struct {
	OK      bool                  `json:"ok"`
	Healthy bool                  `json:"healthy"`
	Issues  []service.HealthIssue `json:"issues"`
	Fixed   int                   `json:"fixed,omitempty"`
	Error   string                `json:"error,omitempty"`
}

// connectionTestPayload mirrors the JSON the UI sends. Config structs use
// mapstructure tags only, so we decode the wire format directly here.
type connectionTestPayload struct {
	URL              string `json:"url"`
	APIKey           string `json:"api_key"`
	PathMappings     []struct {
		Jellyfin string `json:"jellyfin"`
		Daemon   string `json:"daemon"`
	} `json:"path_mappings,omitempty"`
	LibrariesMovies []string `json:"libraries_movies,omitempty"`
	LibrariesTV     []string `json:"libraries_tv,omitempty"`
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
	result := testResult{OK: true, Version: info.Version}
	mappings := make([]jellyfin.PathMapping, 0)
	libs := []string{}
	if h.Cfg != nil {
		libs = append(append(libs, h.Cfg.Libraries.Movies...), h.Cfg.Libraries.TV...)
		for _, m := range h.Cfg.Jellyfin.PathMappings {
			mappings = append(mappings, jellyfin.PathMapping{Jellyfin: m.Jellyfin, Daemon: m.Daemon})
		}
	}
	if len(p.LibrariesMovies) > 0 || len(p.LibrariesTV) > 0 {
		libs = append(append([]string{}, p.LibrariesMovies...), p.LibrariesTV...)
	}
	if len(p.PathMappings) > 0 {
		mappings = mappings[:0]
		for _, m := range p.PathMappings {
			mappings = append(mappings, jellyfin.PathMapping{Jellyfin: m.Jellyfin, Daemon: m.Daemon})
		}
	}
	if folders, ferr := cli.GetVirtualFolders(); ferr == nil {
		if unmapped := jellyfin.UnmappedJellyfinLocations(folders, libs, mappings); len(unmapped) > 0 {
			result.UnmappedLocations = unmapped
			result.PathMappingWarning = "Jellyfin library paths do not match P2J library roots. Add path mappings or the feedback loop cannot confirm organizes."
		}
	}
	writeJSON(w, http.StatusOK, result)
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

func (h *TestHandlers) SonarrCompatibility(w http.ResponseWriter, r *http.Request) {
	h.sonarrCompatibility(w, r, false)
}

func (h *TestHandlers) FixSonarrCompatibility(w http.ResponseWriter, r *http.Request) {
	h.sonarrCompatibility(w, r, true)
}

func (h *TestHandlers) RadarrCompatibility(w http.ResponseWriter, r *http.Request) {
	h.radarrCompatibility(w, r, false)
}

func (h *TestHandlers) FixRadarrCompatibility(w http.ResponseWriter, r *http.Request) {
	h.radarrCompatibility(w, r, true)
}

func (h *TestHandlers) sonarrCompatibility(w http.ResponseWriter, r *http.Request, fix bool) {
	p, err := decodeTestPayload(r)
	if err != nil {
		writeJSON(w, http.StatusOK, compatibilityResult{Error: err.Error()})
		return
	}
	if h.Cfg != nil {
		p.APIKey = h.resolveSecret(p.APIKey, h.Cfg.Sonarr.APIKey)
	}
	client := sonarr.NewClient(sonarr.Config{URL: p.URL, APIKey: p.APIKey, Timeout: 5 * time.Second})
	issues, err := service.CheckSonarrConfig(client)
	if err != nil {
		writeJSON(w, http.StatusOK, compatibilityResult{Error: err.Error()})
		return
	}
	fixed := 0
	if fix && len(issues) > 0 {
		changes, err := service.FixSonarrIssues(client, issues, false)
		if err != nil {
			writeJSON(w, http.StatusOK, compatibilityResult{Issues: issues, Fixed: len(changes), Error: err.Error()})
			return
		}
		fixed = len(changes)
		issues, err = service.CheckSonarrConfig(client)
		if err != nil {
			writeJSON(w, http.StatusOK, compatibilityResult{Fixed: fixed, Error: err.Error()})
			return
		}
	}
	writeJSON(w, http.StatusOK, compatibilityResult{OK: true, Healthy: len(issues) == 0, Issues: issues, Fixed: fixed})
}

func (h *TestHandlers) radarrCompatibility(w http.ResponseWriter, r *http.Request, fix bool) {
	p, err := decodeTestPayload(r)
	if err != nil {
		writeJSON(w, http.StatusOK, compatibilityResult{Error: err.Error()})
		return
	}
	if h.Cfg != nil {
		p.APIKey = h.resolveSecret(p.APIKey, h.Cfg.Radarr.APIKey)
	}
	client := radarr.NewClient(radarr.Config{URL: p.URL, APIKey: p.APIKey, Timeout: 5 * time.Second})
	issues, err := service.CheckRadarrConfig(client)
	if err != nil {
		writeJSON(w, http.StatusOK, compatibilityResult{Error: err.Error()})
		return
	}
	fixed := 0
	if fix && len(issues) > 0 {
		changes, err := service.FixRadarrIssues(client, issues, false)
		if err != nil {
			writeJSON(w, http.StatusOK, compatibilityResult{Issues: issues, Fixed: len(changes), Error: err.Error()})
			return
		}
		fixed = len(changes)
		issues, err = service.CheckRadarrConfig(client)
		if err != nil {
			writeJSON(w, http.StatusOK, compatibilityResult{Fixed: fixed, Error: err.Error()})
			return
		}
	}
	writeJSON(w, http.StatusOK, compatibilityResult{OK: true, Healthy: len(issues) == 0, Issues: issues, Fixed: fixed})
}
