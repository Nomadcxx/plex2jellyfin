package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
)

type aiSettingsApplyRequest struct {
	Enabled       *bool   `json:"enabled,omitempty"`
	Endpoint      *string `json:"endpoint,omitempty"`
	PrimaryModel  *string `json:"primaryModel,omitempty"`
	FallbackModel *string `json:"fallbackModel,omitempty"`
}

type aiTestConnectionRequest struct {
	Endpoint *string `json:"endpoint,omitempty"`
}

type aiPromptTestRequest struct {
	Endpoint *string `json:"endpoint,omitempty"`
	Model    *string `json:"model,omitempty"`
}

func (s *Server) UpdateAISettings(w http.ResponseWriter, r *http.Request) {
	var req aiSettingsApplyRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request payload")
			return
		}
	}

	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_load_error", fmt.Sprintf("Failed to load config: %v", err))
		return
	}

	if req.Enabled != nil {
		cfg.AI.Enabled = *req.Enabled
	}
	if req.Endpoint != nil {
		cfg.AI.OllamaEndpoint = strings.TrimSpace(*req.Endpoint)
	}
	if req.PrimaryModel != nil {
		cfg.AI.Model = strings.TrimSpace(*req.PrimaryModel)
	}
	if req.FallbackModel != nil {
		cfg.AI.FallbackModel = strings.TrimSpace(*req.FallbackModel)
	}
	if cfg.AI.FallbackModel != "" && cfg.AI.FallbackModel == cfg.AI.Model {
		cfg.AI.FallbackModel = ""
	}

	if err := cfg.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, "config_save_error", fmt.Sprintf("Failed to save config: %v", err))
		return
	}

	s.cfg = cfg
	writeJSON(w, http.StatusOK, currentAISettingsPayload(cfg))
}

func (s *Server) TestAIConnection(w http.ResponseWriter, r *http.Request) {
	var req aiTestConnectionRequest
	if r.Body != nil && r.ContentLength != 0 {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	endpoint := ""
	if req.Endpoint != nil {
		endpoint = strings.TrimSpace(*req.Endpoint)
	}
	if endpoint == "" && s.cfg != nil {
		endpoint = strings.TrimSpace(s.cfg.AI.OllamaEndpoint)
	}
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	endpoint = strings.TrimRight(endpoint, "/")

	start := time.Now()
	models, err := fetchOllamaModels(endpoint)
	latencyMs := time.Since(start).Milliseconds()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success":   false,
			"message":   err.Error(),
			"latencyMs": latencyMs,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"message":    fmt.Sprintf("Connected to Ollama (%d models available)", len(models)),
		"latencyMs":  latencyMs,
		"modelCount": len(models),
		"models":     models,
	})
}

// ListAIModels returns the list of locally-available Ollama models for the
// given endpoint (or the configured one when none is supplied). Used by the
// Settings → AI page to populate model picker dropdowns.
func (s *Server) ListAIModels(w http.ResponseWriter, r *http.Request) {
	endpoint := strings.TrimSpace(r.URL.Query().Get("endpoint"))
	if endpoint == "" && s.cfg != nil {
		endpoint = strings.TrimSpace(s.cfg.AI.OllamaEndpoint)
	}
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	endpoint = strings.TrimRight(endpoint, "/")

	models, err := fetchOllamaModels(endpoint)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success":  false,
			"endpoint": endpoint,
			"message":  err.Error(),
			"models":   []string{},
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"endpoint": endpoint,
		"models":   models,
	})
}

func (s *Server) TestAIPrompt(w http.ResponseWriter, r *http.Request) {
	var req aiPromptTestRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request payload")
			return
		}
	}

	endpoint := ""
	if req.Endpoint != nil {
		endpoint = strings.TrimSpace(*req.Endpoint)
	}
	if endpoint == "" && s.cfg != nil {
		endpoint = strings.TrimSpace(s.cfg.AI.OllamaEndpoint)
	}
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	endpoint = strings.TrimRight(endpoint, "/")

	model := ""
	if req.Model != nil {
		model = strings.TrimSpace(*req.Model)
	}
	if model == "" && s.cfg != nil {
		model = strings.TrimSpace(s.cfg.AI.Model)
	}
	if model == "" {
		writeError(w, http.StatusBadRequest, "missing_model", "No model selected for prompt test")
		return
	}

	payload := map[string]interface{}{
		"model":  model,
		"prompt": "Parse this filename and return JSON with title, year, type (movie or tv), and confidence (0-1): The.Matrix.1999.2160p.UHD.BluRay.x265-GROUP. Return only JSON.",
		"stream": false,
	}
	body, _ := json.Marshal(payload)

	start := time.Now()
	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Post(endpoint+"/api/generate", "application/json", bytes.NewReader(body))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success":    false,
			"result":     fmt.Sprintf("Request failed: %v", err),
			"durationMs": time.Since(start).Milliseconds(),
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success":    false,
			"result":     fmt.Sprintf("Ollama returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw))),
			"durationMs": time.Since(start).Milliseconds(),
		})
		return
	}

	var parsed struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success":    false,
			"result":     fmt.Sprintf("Invalid Ollama response: %v", err),
			"durationMs": time.Since(start).Milliseconds(),
		})
		return
	}

	result := strings.TrimSpace(parsed.Response)
	if len(result) > 320 {
		result = result[:320] + "..."
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"result":     result,
		"durationMs": time.Since(start).Milliseconds(),
	})
}

func fetchOllamaModels(endpoint string) ([]string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(endpoint + "/api/tags")
	if err != nil {
		return nil, fmt.Errorf("failed to reach Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var parsed struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("invalid ollama response: %w", err)
	}

	out := make([]string, 0, len(parsed.Models))
	for _, m := range parsed.Models {
		name := strings.TrimSpace(m.Name)
		if name != "" {
			out = append(out, name)
		}
	}
	return out, nil
}

func currentAISettingsPayload(cfg *config.Config) map[string]interface{} {
	if cfg == nil {
		return map[string]interface{}{}
	}

	return map[string]interface{}{
		"enabled":             cfg.AI.Enabled,
		"defaultProvider":     "ollama",
		"confidenceThreshold": float32(cfg.AI.ConfidenceThreshold),
		"autoApply":           cfg.AI.AutoResolveRisky,
		"endpoint":            cfg.AI.OllamaEndpoint,
		"primaryModel":        cfg.AI.Model,
		"fallbackModel":       cfg.AI.FallbackModel,
	}
}
