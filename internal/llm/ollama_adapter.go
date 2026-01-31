package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type OllamaAdapter struct {
	id       string
	name     string
	endpoint string
	model    string
	client   *http.Client
}

func NewOllamaAdapter(id, name, endpoint, model string) *OllamaAdapter {
	return &OllamaAdapter{
		id:       id,
		name:     name,
		endpoint: endpoint,
		model:    model,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (o *OllamaAdapter) ID() string         { return o.id }
func (o *OllamaAdapter) Type() ProviderType { return ProviderTypeOllama }
func (o *OllamaAdapter) Name() string       { return o.name }

func (o *OllamaAdapter) Info() ProviderInfo {
	return ProviderInfo{
		ID:           o.id,
		Type:         o.Type(),
		Name:         o.name,
		Endpoint:     o.endpoint,
		Enabled:      true,
		CurrentModel: o.model,
		Capabilities: o.Capabilities(),
	}
}

func (o *OllamaAdapter) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", o.endpoint+"/api/tags", nil)
	if err != nil {
		return err
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Ollama returned status %d", resp.StatusCode)
	}

	return nil
}

func (o *OllamaAdapter) Status(ctx context.Context) (*ProviderStatus, error) {
	status := &ProviderStatus{
		Model: o.model,
	}

	models, err := o.ListModels(ctx)
	if err != nil {
		status.Online = false
		status.Error = err.Error()
		return status, nil
	}

	status.Online = true
	status.ModelList = models

	return status, nil
}

func (o *OllamaAdapter) ListModels(ctx context.Context) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", o.endpoint+"/api/tags", nil)
	if err != nil {
		return nil, err
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name    string `json:"name"`
			Size    int64  `json:"size"`
			Details struct {
				Family            string `json:"family"`
				QuantizationLevel string `json:"quantization_level"`
			} `json:"details"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode models: %w", err)
	}

	models := make([]Model, len(result.Models))
	for i, m := range result.Models {
		models[i] = Model{
			ID:           m.Name,
			Name:         m.Name,
			Size:         formatSize(m.Size),
			Quantization: m.Details.QuantizationLevel,
			Family:       m.Details.Family,
		}
	}

	return models, nil
}

func (o *OllamaAdapter) CurrentModel() string {
	return o.model
}

func (o *OllamaAdapter) SetModel(model string) error {
	o.model = model
	return nil
}

func (o *OllamaAdapter) Complete(ctx context.Context, prompt string, opts CompletionOptions) (*Completion, error) {
	reqBody := map[string]interface{}{
		"model":  o.model,
		"prompt": prompt,
		"stream": false,
	}

	if opts.Temperature > 0 {
		reqBody["options"] = map[string]interface{}{
			"temperature": opts.Temperature,
		}
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.endpoint+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to complete: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Response string `json:"response"`
		Model    string `json:"model"`
		Done     bool   `json:"done"`
		Context  []int  `json:"context"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &Completion{
		Text:       result.Response,
		Model:      result.Model,
		UsedTokens: len(result.Context),
	}, nil
}

func (o *OllamaAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		SupportsStreaming:   true,
		SupportsVision:      false,
		SupportsModelSwitch: true,
		LocalOnly:           true,
	}
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
