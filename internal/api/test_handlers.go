package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/jellyfin"
	"github.com/Nomadcxx/jellywatch/internal/radarr"
	"github.com/Nomadcxx/jellywatch/internal/sonarr"
)

type TestHandlers struct{}

type testResult struct {
	OK      bool   `json:"ok"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (TestHandlers) Sonarr(w http.ResponseWriter, r *http.Request) {
	var c config.SonarrConfig
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeJSON(w, http.StatusOK, testResult{OK: false, Error: err.Error()})
		return
	}
	cli := sonarr.NewClient(sonarr.Config{URL: c.URL, APIKey: c.APIKey, Timeout: 5 * time.Second})
	status, err := cli.GetSystemStatus()
	if err != nil {
		writeJSON(w, http.StatusOK, testResult{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, testResult{OK: true, Version: status.Version})
}

func (TestHandlers) Radarr(w http.ResponseWriter, r *http.Request) {
	var c config.RadarrConfig
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeJSON(w, http.StatusOK, testResult{OK: false, Error: err.Error()})
		return
	}
	cli := radarr.NewClient(radarr.Config{URL: c.URL, APIKey: c.APIKey, Timeout: 5 * time.Second})
	status, err := cli.GetSystemStatus()
	if err != nil {
		writeJSON(w, http.StatusOK, testResult{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, testResult{OK: true, Version: status.Version})
}

func (TestHandlers) Jellyfin(w http.ResponseWriter, r *http.Request) {
	var c config.JellyfinConfig
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeJSON(w, http.StatusOK, testResult{OK: false, Error: err.Error()})
		return
	}
	cli := jellyfin.NewClient(jellyfin.Config{URL: c.URL, APIKey: c.APIKey, Timeout: 5 * time.Second})
	info, err := cli.GetSystemInfo()
	if err != nil {
		writeJSON(w, http.StatusOK, testResult{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, testResult{OK: true, Version: info.Version})
}
