package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/daemon/ipc"
	setupdomain "github.com/Nomadcxx/plex2jellyfin/internal/setup"
)

type SetupStatusResponse struct {
	Required    bool                    `json:"required"`
	Complete    bool                    `json:"complete"`
	DaemonState string                  `json:"daemon_state"`
	Runtime     setupdomain.RuntimeInfo `json:"runtime"`
	Draft       setupdomain.Draft       `json:"draft"`
}

type setupApplyResponse struct {
	Applied     bool   `json:"applied"`
	Complete    bool   `json:"complete"`
	DaemonState string `json:"daemon_state"`
}

func (s *Server) GetSetupStatus(w http.ResponseWriter, r *http.Request) {
	s.configMu.RLock()
	cfg := cloneConfig(s.cfg)
	s.configMu.RUnlock()

	required := setupdomain.NeedsSetup(cfg)
	writeJSON(w, http.StatusOK, SetupStatusResponse{
		Required: required, Complete: !required,
		DaemonState: s.currentDaemonState(r.Context()),
		Runtime:     setupdomain.DetectRuntime(), Draft: setupdomain.DraftFromConfig(cfg),
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

	candidate.Setup.Completed = true
	if err := candidate.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, "config_save_error", err.Error())
		return
	}
	s.replaceConfigLocked(candidate)
	writeJSON(w, http.StatusOK, setupApplyResponse{Applied: true, Complete: true, DaemonState: "running"})
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
