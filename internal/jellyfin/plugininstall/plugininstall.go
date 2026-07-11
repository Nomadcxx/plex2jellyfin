// Package plugininstall drives installation, configuration, and
// verification of the companion Jellyfin plugin through Jellyfin's own
// package API, so it works identically for native and containerized
// Jellyfin servers.
package plugininstall

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	PluginGUID  = "f4eda3a1-c062-49b3-a958-7cf9ca80c269"
	PluginName  = "Plex2Jellyfin"
	RepoName    = "Plex2Jellyfin (official)"
	ManifestURL = "https://raw.githubusercontent.com/Nomadcxx/plex2jellyfin-plugin/main/manifest.json"

	// The plugin targets ABI 10.11.0.0; older servers cannot load it and
	// newer major ABIs are unknown until tested.
	requiredVersionPrefix = "10.11."
)

// Engine drives plugin installation against one Jellyfin server.
type Engine struct {
	baseURL      string
	apiKey       string
	http         *http.Client
	pollInterval time.Duration
}

func New(baseURL, apiKey string, client *http.Client) *Engine {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &Engine{
		baseURL:      strings.TrimRight(baseURL, "/"),
		apiKey:       apiKey,
		http:         client,
		pollInterval: 2 * time.Second,
	}
}

// Inspection is the read-only state the wizard renders before acting.
type Inspection struct {
	ServerVersion    string
	ABISupported     bool
	RepoRegistered   bool
	InstalledVersion string // empty when not installed
	PluginResponding bool   // /plex2jellyfin/health answered 200
}

// VerifyResult mirrors the plugin's test-webhook response.
type VerifyResult struct {
	Sent             bool   `json:"Sent"`
	DaemonURL        string `json:"DaemonUrl"`
	DaemonStatusCode int    `json:"DaemonStatusCode"`
	Authenticated    bool   `json:"Authenticated"`
	Error            string `json:"Error"`
}

type repositoryInfo struct {
	Name    string `json:"Name"`
	URL     string `json:"Url"`
	Enabled bool   `json:"Enabled"`
}

func (e *Engine) do(ctx context.Context, method, path string, body, result any) (int, error) {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, e.baseURL+path, reader)
	if err != nil {
		return 0, err
	}
	req.Header.Set("X-Emby-Token", e.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := e.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		detail := strings.TrimSpace(string(data))
		if detail == "" {
			return resp.StatusCode, fmt.Errorf("%s %s: %s", method, path, resp.Status)
		}
		if len(detail) > 240 {
			detail = detail[:240] + "…"
		}
		return resp.StatusCode, fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, detail)
	}
	if result != nil && len(data) > 0 {
		if err := json.Unmarshal(data, result); err != nil {
			return resp.StatusCode, fmt.Errorf("decode %s response: %w", path, err)
		}
	}
	return resp.StatusCode, nil
}

func guidsEqual(a, b string) bool {
	norm := func(s string) string { return strings.ToLower(strings.ReplaceAll(s, "-", "")) }
	return norm(a) == norm(b)
}

func (e *Engine) Inspect(ctx context.Context) (*Inspection, error) {
	var info struct {
		Version string `json:"Version"`
	}
	if _, err := e.do(ctx, http.MethodGet, "/System/Info", nil, &info); err != nil {
		return nil, fmt.Errorf("jellyfin system info: %w", err)
	}
	insp := &Inspection{
		ServerVersion: info.Version,
		ABISupported:  strings.HasPrefix(info.Version, requiredVersionPrefix),
	}

	var repos []repositoryInfo
	if _, err := e.do(ctx, http.MethodGet, "/Repositories", nil, &repos); err != nil {
		return nil, fmt.Errorf("list plugin repositories: %w", err)
	}
	for _, r := range repos {
		if r.URL == ManifestURL {
			insp.RepoRegistered = true
		}
	}

	var plugins []struct {
		ID      string `json:"Id"`
		Version string `json:"Version"`
	}
	if _, err := e.do(ctx, http.MethodGet, "/Plugins", nil, &plugins); err != nil {
		return nil, fmt.Errorf("list installed plugins: %w", err)
	}
	for _, p := range plugins {
		if guidsEqual(p.ID, PluginGUID) {
			insp.InstalledVersion = p.Version
		}
	}

	status, err := e.do(ctx, http.MethodGet, "/plex2jellyfin/health", nil, nil)
	insp.PluginResponding = err == nil && status == http.StatusOK
	return insp, nil
}

// RegisterRepo adds our manifest URL to the server's repository list.
// POST /Repositories replaces the entire list, so the existing entries
// are always carried over.
func (e *Engine) RegisterRepo(ctx context.Context) (bool, error) {
	var repos []repositoryInfo
	if _, err := e.do(ctx, http.MethodGet, "/Repositories", nil, &repos); err != nil {
		return false, err
	}
	for _, r := range repos {
		if r.URL == ManifestURL {
			return false, nil
		}
	}
	repos = append(repos, repositoryInfo{Name: RepoName, URL: ManifestURL, Enabled: true})
	if _, err := e.do(ctx, http.MethodPost, "/Repositories", repos, nil); err != nil {
		return false, err
	}
	return true, nil
}

func (e *Engine) Install(ctx context.Context) error {
	path := "/Packages/Installed/" + PluginName + "?assemblyGuid=" + PluginGUID
	_, err := e.do(ctx, http.MethodPost, path, nil, nil)
	return err
}

func (e *Engine) Restart(ctx context.Context) error {
	_, err := e.do(ctx, http.MethodPost, "/System/Restart", nil, nil)
	return err
}

// WaitReady polls the plugin's anonymous health endpoint until it
// answers, which proves the server is back AND the plugin loaded.
func (e *Engine) WaitReady(ctx context.Context, timeout time.Duration) error {
	deadline, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(e.pollInterval)
	defer ticker.Stop()
	for {
		if status, err := e.do(deadline, http.MethodGet, "/plex2jellyfin/health", nil, nil); err == nil && status == http.StatusOK {
			return nil
		}
		select {
		case <-deadline.Done():
			return fmt.Errorf("plugin did not respond within %s", timeout)
		case <-ticker.C:
		}
	}
}

func (e *Engine) Configure(ctx context.Context, daemonBaseURL, sharedSecret string) error {
	cfg := map[string]any{
		"Plex2JellyfinUrl":      strings.TrimRight(daemonBaseURL, "/"),
		"SharedSecret":          sharedSecret,
		"EnableEventForwarding": true,
		"EnableCustomEndpoints": true,
		"ForwardLibraryEvents":  true,
		"ForwardPlaybackEvents": true,
		"RequestTimeoutSeconds": 30,
		"RetryCount":            3,
	}
	_, err := e.do(ctx, http.MethodPost, "/Plugins/"+PluginGUID+"/Configuration", cfg, nil)
	return err
}

func (e *Engine) Verify(ctx context.Context) (*VerifyResult, error) {
	var result VerifyResult
	if _, err := e.do(ctx, http.MethodPost, "/plex2jellyfin/test-webhook", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
