package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/daemon/ipc"
	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin/plugininstall"
	setupdomain "github.com/Nomadcxx/plex2jellyfin/internal/setup"
)

type SetupStatusResponse struct {
	Required           bool                    `json:"required"`
	Complete           bool                    `json:"complete"`
	DaemonState        string                  `json:"daemon_state"`
	Runtime            setupdomain.RuntimeInfo `json:"runtime"`
	Draft              setupdomain.Draft       `json:"draft"`
	AdvertiseIP        string                  `json:"advertise_ip,omitempty"`
	DefaultCallbackURL string                  `json:"default_callback_url,omitempty"`
}

type setupApplyResponse struct {
	Applied       bool   `json:"applied"`
	Complete      bool   `json:"complete"`
	DaemonState   string `json:"daemon_state"`
	PluginWarning string `json:"plugin_warning,omitempty"`
}

func (s *Server) GetSetupStatus(w http.ResponseWriter, r *http.Request) {
	s.configMu.RLock()
	cfg := cloneConfig(s.cfg)
	s.configMu.RUnlock()

	required := setupdomain.NeedsSetup(cfg)
	advertise := setupdomain.DetectAdvertiseIP()
	callback := ""
	if advertise != "" {
		callback = "http://" + advertise + ":5522"
	}
	writeJSON(w, http.StatusOK, SetupStatusResponse{
		Required: required, Complete: !required,
		DaemonState: s.currentDaemonState(r.Context()),
		Runtime:     setupdomain.DetectRuntime(), Draft: setupdomain.DraftFromConfig(cfg),
		AdvertiseIP: advertise, DefaultCallbackURL: callback,
	})
}

func (s *Server) ApplySetup(w http.ResponseWriter, r *http.Request) {
	var draft setupdomain.Draft
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&draft); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	runtime := setupdomain.DetectRuntime()
	if errs := setupdomain.ValidateDraft(draft, runtime); len(errs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "validation failed", "fields": errs})
		return
	}
	if errs := validateSetupPaths(draft); len(errs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "path preflight failed", "fields": errs})
		return
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()

	current, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_load_error", err.Error())
		return
	}
	candidate := setupdomain.ApplyDraft(current, draft)
	if candidate.Jellyfin.Enabled && candidate.Jellyfin.WebhookSecret == "" {
		secret, err := generateToken()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "secret_generation_error", err.Error())
			return
		}
		candidate.Jellyfin.WebhookSecret = secret
		candidate.Jellyfin.PluginSharedSecret = secret
	}
	if err := candidate.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, "config_save_error", err.Error())
		return
	}
	s.replaceConfigLocked(candidate)

	if err := s.activateSetupDaemon(r.Context()); err != nil {
		writeError(w, http.StatusBadGateway, "daemon_activation_error", err.Error())
		return
	}

	pluginWarning := ""
	if candidate.Jellyfin.Enabled && draft.Jellyfin.PluginInstall {
		if warn := s.setupCompanionPlugin(r.Context(), candidate, draft); warn != "" {
			pluginWarning = warn
		}
	}

	candidate.Setup.Completed = true
	if err := candidate.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, "config_save_error", err.Error())
		return
	}
	s.replaceConfigLocked(candidate)
	writeJSON(w, http.StatusOK, setupApplyResponse{
		Applied: true, Complete: true, DaemonState: "running", PluginWarning: pluginWarning,
	})
}

func validateSetupPaths(draft setupdomain.Draft) []setupdomain.FieldError {
	var errs []setupdomain.FieldError
	check := func(field string, paths []string, writable bool) {
		for _, path := range paths {
			result := PreflightPath(path)
			if !result.Exists || !result.IsDir || !result.Readable || (writable && !result.Writable) {
				errs = append(errs, setupdomain.FieldError{Field: field, Message: fmt.Sprintf("%s is not an accessible directory", path)})
			}
		}
	}
	check("watch.tv", draft.Watch.TV, draft.Runtime.DeleteSource)
	check("watch.movies", draft.Watch.Movies, draft.Runtime.DeleteSource)
	check("libraries.tv", draft.Libraries.TV, true)
	check("libraries.movies", draft.Libraries.Movies, true)
	return errs
}

func (s *Server) activateSetupDaemon(parent context.Context) error {
	if s.ipc == nil {
		return errors.New("daemon IPC is unavailable")
	}
	if _, err := s.ipc.Call(parent, ipc.CmdStatus, nil); err == nil {
		raw, err := s.ipc.Call(parent, ipc.CmdReload, nil)
		if err != nil {
			return fmt.Errorf("reload daemon: %w", err)
		}
		var result ReloadResult
		if err := json.Unmarshal(raw, &result); err == nil && !result.OK {
			return errors.New("daemon rejected the setup configuration")
		}
	} else {
		if s.launcher == nil {
			return errors.New("daemon is stopped and no launcher is available")
		}
		if err := s.launcher.Start(); err != nil {
			return fmt.Errorf("start daemon: %w", err)
		}
	}

	timeout := s.setupTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	interval := s.setupPollInterval
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := s.ipc.Call(ctx, ipc.CmdStatus, nil); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("daemon did not become ready within %s", timeout)
		case <-ticker.C:
		}
	}
}

func (s *Server) currentDaemonState(ctx context.Context) string {
	if s.ipc == nil {
		return "stopped"
	}
	raw, err := s.ipc.Call(ctx, ipc.CmdStatus, nil)
	if err != nil {
		return "stopped"
	}
	var status struct {
		State string `json:"state"`
	}
	if json.Unmarshal(raw, &status) == nil && status.State != "" {
		return status.State
	}
	return "running"
}

func (s *Server) replaceConfigLocked(cfg *config.Config) {
	if s.cfg == nil {
		s.cfg = cfg
		return
	}
	*s.cfg = *cfg
}

func cloneConfig(cfg *config.Config) *config.Config {
	if cfg == nil {
		return config.DefaultConfig()
	}
	copy := *cfg
	return &copy
}

// setupCompanionPlugin installs/configures the Jellyfin plugin after the
// daemon is up. Soft-fails: setup.completed is independent of plugin success.
func (s *Server) setupCompanionPlugin(ctx context.Context, cfg *config.Config, draft setupdomain.Draft) string {
	engine := plugininstall.New(cfg.Jellyfin.URL, cfg.Jellyfin.APIKey, &http.Client{Timeout: 15 * time.Second})
	insp, err := engine.Inspect(ctx)
	if err != nil {
		return "companion plugin inspect failed: " + err.Error()
	}
	if !insp.ABISupported {
		return "jellyfin " + insp.ServerVersion + " does not support the companion plugin"
	}
	if insp.InstalledVersion == "" {
		if _, err := engine.RegisterRepo(ctx); err != nil {
			return "register plugin repository: " + err.Error()
		}
		if err := engine.Install(ctx); err != nil {
			return "install companion plugin: " + err.Error()
		}
	}
	if draft.Jellyfin.PluginRestart {
		if err := engine.Restart(ctx); err != nil {
			return "restart jellyfin: " + err.Error() + "; run: plex2jellyfin plugin verify"
		}
		if err := engine.WaitReady(ctx, 60*time.Second); err != nil {
			return "plugin did not load after restart: " + err.Error()
		}
	} else if !insp.PluginResponding {
		return "plugin installed; restart Jellyfin, then run: plex2jellyfin plugin verify"
	}

	secret := cfg.Jellyfin.PluginSharedSecret
	if secret == "" {
		secret = cfg.Jellyfin.WebhookSecret
	}
	daemonURL := strings.TrimRight(cfg.Jellyfin.PluginDaemonURL, "/")
	if secret == "" || daemonURL == "" {
		return "plugin loaded but callback URL/secret missing; run: plex2jellyfin plugin verify"
	}
	if err := engine.Configure(ctx, daemonURL, secret); err != nil {
		return "configure plugin: " + err.Error() + "; retry: plex2jellyfin plugin verify"
	}
	if _, err := engine.Verify(ctx); err != nil {
		return "plugin verify: " + err.Error() + "; retry: plex2jellyfin plugin verify"
	}
	return ""
}

// ValidateSetupPaths exposes the setup path preflight for reuse outside the
// HTTP layer (the CLI wizard shares it).
func ValidateSetupPaths(draft setupdomain.Draft) []setupdomain.FieldError {
	return validateSetupPaths(draft)
}
