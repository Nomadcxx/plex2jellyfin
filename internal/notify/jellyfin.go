package notify

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
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

	err := n.targetedRefresh(event)
	if err != nil {
		err = n.refreshLibrary()
	}

	result.Success = err == nil
	result.Error = err
	result.Duration = time.Since(start)
	return result
}

type jellyfinSearchResponse struct {
	Items []struct {
		ID             string `json:"Id"`
		Name           string `json:"Name"`
		Path           string `json:"Path"`
		ProductionYear int    `json:"ProductionYear"`
	} `json:"Items"`
}

func (n *JellyfinNotifier) targetedRefresh(event OrganizationEvent) error {
	term := strings.TrimSpace(event.Title)
	if term == "" {
		term = deriveSearchTermFromPath(event.TargetPath)
	}
	if term == "" {
		return fmt.Errorf("unable to determine search term for targeted refresh")
	}

	items, err := n.searchItems(term)
	if err != nil {
		return err
	}

	eventPath := normalizePath(event.TargetPath)
	eventYear, _ := strconv.Atoi(strings.TrimSpace(event.Year))

	for _, item := range items {
		if eventPath != "" && normalizePath(item.Path) == eventPath {
			return n.refreshItem(item.ID)
		}
		if event.Title != "" && strings.EqualFold(strings.TrimSpace(item.Name), strings.TrimSpace(event.Title)) {
			if eventYear == 0 || item.ProductionYear == 0 || item.ProductionYear == eventYear {
				return n.refreshItem(item.ID)
			}
		}
	}

	return n.refreshLibrary()
}

func (n *JellyfinNotifier) searchItems(term string) ([]struct {
	ID             string `json:"Id"`
	Name           string `json:"Name"`
	Path           string `json:"Path"`
	ProductionYear int    `json:"ProductionYear"`
}, error) {
	q := url.Values{}
	q.Set("SearchTerm", term)
	q.Set("Recursive", "true")
	q.Set("Fields", "Path,ProductionYear")

	body, err := n.doJSONRequest(http.MethodGet, "/Items?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}

	var resp jellyfinSearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to decode jellyfin search response: %w", err)
	}

	return resp.Items, nil
}

func (n *JellyfinNotifier) refreshItem(itemID string) error {
	if strings.TrimSpace(itemID) == "" {
		return fmt.Errorf("item id is required")
	}
	_, err := n.doJSONRequest(http.MethodPost, "/Items/"+url.PathEscape(itemID)+"/Refresh", nil)
	return err
}

func (n *JellyfinNotifier) refreshLibrary() error {
	_, err := n.doJSONRequest(http.MethodPost, "/Library/Refresh", nil)
	return err
}

func (n *JellyfinNotifier) doJSONRequest(method, path string, body []byte) ([]byte, error) {
	req, err := http.NewRequest(method, n.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", n.authHeader())
	req.Header.Set("Accept", "application/json")

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
	return fmt.Sprintf(`MediaBrowser Token="%s", Client="jellywatch", Device="jellywatchd", DeviceId="jellywatchd", Version="1.0.0"`, n.apiKey)
}

func deriveSearchTermFromPath(path string) string {
	base := strings.TrimSpace(filepath.Base(path))
	if base == "" {
		return ""
	}
	base = strings.TrimSuffix(base, filepath.Ext(base))
	base = strings.ReplaceAll(base, ".", " ")
	base = strings.ReplaceAll(base, "_", " ")
	return strings.TrimSpace(base)
}

func normalizePath(path string) string {
	return filepath.Clean(strings.TrimSpace(path))
}
