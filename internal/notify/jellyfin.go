package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// JellyfinNotifier notifies Jellyfin about newly organized media.
type JellyfinNotifier struct {
	baseURL string
	apiKey  string
	enabled bool
	client  *http.Client
}

func NewJellyfinNotifier(baseURL, apiKey string, enabled bool) *JellyfinNotifier {
	return &JellyfinNotifier{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:  strings.TrimSpace(apiKey),
		enabled: enabled && strings.TrimSpace(baseURL) != "" && strings.TrimSpace(apiKey) != "",
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (n *JellyfinNotifier) Name() string {
	return "jellyfin"
}

func (n *JellyfinNotifier) Enabled() bool {
	return n.enabled
}

func (n *JellyfinNotifier) Ping() error {
	if !n.enabled {
		return nil
	}
	_, err := n.doJSONRequest(http.MethodGet, "/System/Info", nil)
	return err
}

func (n *JellyfinNotifier) Notify(event OrganizationEvent) *NotifyResult {
	start := time.Now()
	result := &NotifyResult{Service: n.Name()}

	if !n.enabled {
		result.Success = true
		result.Duration = time.Since(start)
		return result
	}

	err := n.notifyMediaUpdated(event)

	result.Success = err == nil
	result.Error = err
	result.Duration = time.Since(start)
	return result
}

func (n *JellyfinNotifier) notifyMediaUpdated(event OrganizationEvent) error {
	targetDir := strings.TrimSpace(event.JellyfinTargetDir)
	if targetDir == "" {
		targetDir = strings.TrimSpace(event.TargetDir)
	}
	if targetDir == "" && strings.TrimSpace(event.TargetPath) != "" {
		targetDir = filepath.Dir(event.TargetPath)
	}
	if targetDir == "" || targetDir == "." {
		return fmt.Errorf("unable to determine committed Jellyfin folder")
	}

	payload, err := json.Marshal(map[string]any{
		"Updates": []map[string]string{{
			"Path":       targetDir,
			"UpdateType": "Created",
		}},
	})
	if err != nil {
		return err
	}
	_, err = n.doJSONRequest(http.MethodPost, "/Library/Media/Updated", payload)
	return err
}

func (n *JellyfinNotifier) doJSONRequest(method, path string, body []byte) ([]byte, error) {
	req, err := http.NewRequest(method, n.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", n.authHeader())
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jellyfin request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("jellyfin returned status %d for %s %s", resp.StatusCode, method, path)
	}

	var out json.RawMessage
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out, nil
}

func (n *JellyfinNotifier) authHeader() string {
	return fmt.Sprintf(`MediaBrowser Token="%s", Client="plex2jellyfin", Device="plex2jellyfin-daemon", DeviceId="plex2jellyfin-daemon", Version="1.0.0"`, n.apiKey)
}
