package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/go-chi/chi/v5"
)

type SettingsHandlers struct {
	mu  sync.Mutex
	Cfg *config.Config
	IPC IPCCaller
}

func (h *SettingsHandlers) Get(w http.ResponseWriter, r *http.Request) {
	section := chi.URLParam(r, "section")

	h.mu.Lock()
	defer h.mu.Unlock()

	raw, err := config.GetSection(h.Cfg, section)
	if err != nil {
		writeError(w, http.StatusNotFound, "unknown_section", err.Error())
		return
	}

	if r.URL.Query().Get("reveal") != "1" || !h.ValidRevealToken(r.Header.Get("X-Reveal-Token")) {
		raw = h.maskSection(section, raw)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

func (h *SettingsHandlers) Put(w http.ResponseWriter, r *http.Request) {
	section := chi.URLParam(r, "section")
	body, err := readJSONBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	current, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_load_error", err.Error())
		return
	}
	body, err = preserveMaskedSectionSecrets(current, section, body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if err := config.SetSection(current, section, body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_section", err.Error())
		return
	}
	configPath, err := config.ConfigPath()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_path_error", err.Error())
		return
	}
	resp, err := SaveConfigAndReloadSection(r.Context(), configPath, current, h.IPC, section)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_save_error", err.Error())
		return
	}
	if resp.Reload.OK {
		h.Cfg = current
	} else if restored, err := config.Load(); err == nil {
		h.Cfg = restored
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *SettingsHandlers) ValidRevealToken(token string) bool {
	return false
}

func (h *SettingsHandlers) maskSection(section string, raw json.RawMessage) json.RawMessage {
	masked := *h.Cfg
	switch section {
	case "sonarr":
		config.MaskSecrets(&masked.Sonarr)
	case "radarr":
		config.MaskSecrets(&masked.Radarr)
	case "jellyfin":
		config.MaskSecrets(&masked.Jellyfin)
	default:
		return raw
	}
	out, err := config.GetSection(&masked, section)
	if err != nil {
		return raw
	}
	return out
}

func readJSONBody(r *http.Request) (json.RawMessage, error) {
	defer r.Body.Close()
	var raw json.RawMessage
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func preserveMaskedSectionSecrets(current *config.Config, section string, raw json.RawMessage) (json.RawMessage, error) {
	candidate := *current
	if err := config.SetSection(&candidate, section, raw); err != nil {
		return nil, err
	}
	switch section {
	case "sonarr":
		if isMaskedSecret(candidate.Sonarr.APIKey) {
			candidate.Sonarr.APIKey = current.Sonarr.APIKey
		}
	case "radarr":
		if isMaskedSecret(candidate.Radarr.APIKey) {
			candidate.Radarr.APIKey = current.Radarr.APIKey
		}
	case "jellyfin":
		if isMaskedSecret(candidate.Jellyfin.APIKey) {
			candidate.Jellyfin.APIKey = current.Jellyfin.APIKey
		}
		if isMaskedSecret(candidate.Jellyfin.WebhookSecret) {
			candidate.Jellyfin.WebhookSecret = current.Jellyfin.WebhookSecret
		}
		if isMaskedSecret(candidate.Jellyfin.PluginSharedSecret) {
			candidate.Jellyfin.PluginSharedSecret = current.Jellyfin.PluginSharedSecret
		}
	default:
		return raw, nil
	}
	return config.GetSection(&candidate, section)
}

func isMaskedSecret(v string) bool {
	return strings.HasPrefix(v, "****")
}
