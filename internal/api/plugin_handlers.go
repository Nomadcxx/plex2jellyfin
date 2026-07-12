package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/api"
	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin/plugininstall"
)

func boolPtr(v bool) *bool       { return &v }
func strPtr(v string) *string    { return &v }

func (s *Server) jellyfinPluginEngine() (*plugininstall.Engine, string) {
	if s.cfg == nil || !s.cfg.Jellyfin.Enabled || strings.TrimSpace(s.cfg.Jellyfin.URL) == "" || strings.TrimSpace(s.cfg.Jellyfin.APIKey) == "" {
		return nil, "Jellyfin is not configured"
	}
	return plugininstall.New(s.cfg.Jellyfin.URL, s.cfg.Jellyfin.APIKey, &http.Client{Timeout: 15 * time.Second}), ""
}

// GetJellyfinPluginStatus reports companion plugin readiness (Inspect).
func (s *Server) GetJellyfinPluginStatus(w http.ResponseWriter, r *http.Request) {
	engine, reason := s.jellyfinPluginEngine()
	if engine == nil {
		writeJSON(w, http.StatusOK, api.JellyfinPluginStatus{
			Healthy:   boolPtr(false),
			Installed: boolPtr(false),
			Message:   strPtr(reason),
		})
		return
	}

	insp, err := engine.Inspect(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, api.JellyfinPluginStatus{
			Healthy:   boolPtr(false),
			Installed: boolPtr(false),
			Message:   strPtr(err.Error()),
		})
		return
	}

	installed := insp.InstalledVersion != ""
	healthy := installed && insp.PluginResponding && insp.ABISupported
	msg := fmt.Sprintf("Jellyfin %s (ABI supported: %v); repo registered: %v",
		insp.ServerVersion, insp.ABISupported, insp.RepoRegistered)
	if !installed {
		msg += "; plugin not installed"
	} else {
		msg += fmt.Sprintf("; plugin %s (responding: %v)", insp.InstalledVersion, insp.PluginResponding)
	}

	status := api.JellyfinPluginStatus{
		Healthy:   boolPtr(healthy),
		Installed: boolPtr(installed),
		Message:   strPtr(msg),
	}
	if installed {
		status.Version = strPtr(insp.InstalledVersion)
	}
	writeJSON(w, http.StatusOK, status)
}

// VerifyJellyfinPlugin triggers the plugin's signed test-webhook round-trip.
// Soft-fails with HTTP 200 and ok=false so the UI can show the reason.
func (s *Server) VerifyJellyfinPlugin(w http.ResponseWriter, r *http.Request) {
	engine, reason := s.jellyfinPluginEngine()
	if engine == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": reason})
		return
	}

	insp, err := engine.Inspect(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if !insp.PluginResponding {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": "plugin is not responding — install/restart Jellyfin, then retry",
		})
		return
	}

	res, err := engine.Verify(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	ok := res.Sent && res.Authenticated
	out := map[string]any{
		"ok":               ok,
		"sent":             res.Sent,
		"authenticated":    res.Authenticated,
		"daemon_url":       res.DaemonURL,
		"daemon_status":    res.DaemonStatusCode,
		"error":            res.Error,
	}
	if !ok && res.Error == "" {
		if res.Sent {
			out["error"] = fmt.Sprintf("test event reached daemon but HTTP %d", res.DaemonStatusCode)
		} else {
			out["error"] = "Jellyfin could not reach the daemon callback URL"
		}
	}
	writeJSON(w, http.StatusOK, out)
}
