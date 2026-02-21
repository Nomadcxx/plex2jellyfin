package jellyfin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// PluginClient communicates with the JellyWatch companion plugin endpoints.
type PluginClient struct {
	baseURL      string
	apiKey       string
	httpClient   *http.Client
	sharedSecret string
	timeout      time.Duration
	hostname     string
	deviceID     string
}

// NewPluginClient creates a new plugin client with the given configuration.
func NewPluginClient(cfg Config) *PluginClient {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "jellywatch"
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: timeout,
		}
	} else if httpClient.Timeout == 0 {
		httpClient.Timeout = timeout
	}

	return &PluginClient{
		baseURL:    strings.TrimRight(cfg.URL, "/"),
		apiKey:     cfg.APIKey,
		httpClient: httpClient,
		timeout:    timeout,
		hostname:   hostname,
		deviceID:   fmt.Sprintf("jellywatch-%s-%d", hostname, time.Now().UnixNano()),
	}
}

// authHeader returns the Jellyfin Authorization header value.
func (pc *PluginClient) authHeader() string {
	return fmt.Sprintf(`MediaBrowser Token="%s", Client="jellywatch", Device="%s", DeviceId="%s", Version="1.0.0"`,
		pc.apiKey, pc.hostname, pc.deviceID)
}

// request performs an HTTP request with retry logic (max 3 attempts with exponential backoff).
func (pc *PluginClient) request(method, endpoint string, body io.Reader) (*http.Response, error) {
	base, err := url.Parse(pc.baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	rel, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint: %w", err)
	}

	fullURL := base.ResolveReference(rel)

	var lastErr error
	maxRetries := 3

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Clone body for retry attempts
		var bodyReader io.Reader
		if body != nil {
			if seeker, ok := body.(io.Seeker); ok {
				seeker.Seek(0, io.SeekStart)
				bodyReader = body
			} else {
				// If body isn't seekable and we're retrying, we can't reuse it
				if attempt > 0 {
					return nil, fmt.Errorf("cannot retry request with non-seekable body")
				}
				bodyReader = body
			}
		}

		req, err := http.NewRequest(method, fullURL.String(), bodyReader)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		if pc.apiKey != "" {
			req.Header.Set("Authorization", pc.authHeader())
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := pc.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("executing request (attempt %d/%d): %w", attempt+1, maxRetries, err)
			if attempt < maxRetries-1 {
				backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
				time.Sleep(backoff)
				continue
			}
			return nil, lastErr
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			defer resp.Body.Close()
			bodyBytes, _ := io.ReadAll(resp.Body)
			lastErr = fmt.Errorf("API error (status %d, attempt %d/%d): %s", resp.StatusCode, attempt+1, maxRetries, string(bodyBytes))
			
			// Retry on 5xx errors
			if resp.StatusCode >= 500 && attempt < maxRetries-1 {
				backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
				time.Sleep(backoff)
				continue
			}
			return nil, lastErr
		}

		return resp, nil
	}

	return nil, lastErr
}

// get performs a GET request and decodes the response.
func (pc *PluginClient) get(endpoint string, result interface{}) error {
	resp, err := pc.request(http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}

	return nil
}

// post performs a POST request and decodes the response.
func (pc *PluginClient) post(endpoint string, payload, result interface{}) error {
	var body io.Reader
	if payload != nil {
		jsonBytes, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encoding payload: %w", err)
		}
		body = bytes.NewReader(jsonBytes)
	}

	resp, err := pc.request(http.MethodPost, endpoint, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}

	return nil
}

// Health checks if the plugin is responsive.
func (pc *PluginClient) Health() (*PluginHealth, error) {
	var health PluginHealth
	if err := pc.get("/JellyWatch/health", &health); err != nil {
		return nil, err
	}
	return &health, nil
}

// GetItemByPath retrieves a Jellyfin item by its file system path.
func (pc *PluginClient) GetItemByPath(path string) (*PluginItem, error) {
	endpoint := fmt.Sprintf("/JellyWatch/item-by-path?path=%s", url.QueryEscape(path))
	var item PluginItem
	if err := pc.get(endpoint, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

// GetLibraryByPath finds which library a file path belongs to.
func (pc *PluginClient) GetLibraryByPath(path string) (*PluginLibrary, error) {
	endpoint := fmt.Sprintf("/JellyWatch/library-by-path?path=%s", url.QueryEscape(path))
	var library PluginLibrary
	if err := pc.get(endpoint, &library); err != nil {
		return nil, err
	}
	return &library, nil
}

// GetActiveScans returns currently running library scans.
func (pc *PluginClient) GetActiveScans() (*ActiveScansResponse, error) {
	var scans ActiveScansResponse
	if err := pc.get("/JellyWatch/active-scans", &scans); err != nil {
		return nil, err
	}
	return &scans, nil
}

// TriggerScan initiates a library scan for the specified library and optional path.
func (pc *PluginClient) TriggerScan(libraryID, path string) error {
	payload := map[string]string{
		"libraryId": libraryID,
	}
	if path != "" {
		payload["path"] = path
	}
	return pc.post("/JellyWatch/scan-library", payload, nil)
}

// GetUnidentifiableItems retrieves items that Jellyfin couldn't identify.
func (pc *PluginClient) GetUnidentifiableItems(libraryID string) (*UnidentifiableResponse, error) {
	endpoint := "/JellyWatch/unidentifiable"
	if libraryID != "" {
		endpoint = fmt.Sprintf("%s?libraryId=%s", endpoint, url.QueryEscape(libraryID))
	}
	var unidentifiable UnidentifiableResponse
	if err := pc.get(endpoint, &unidentifiable); err != nil {
		return nil, err
	}
	return &unidentifiable, nil
}

// GetActivePlayback returns currently active playback sessions.
func (pc *PluginClient) GetActivePlayback() (*ActivePlaybackResponse, error) {
	var playback ActivePlaybackResponse
	if err := pc.get("/JellyWatch/active-playback", &playback); err != nil {
		return nil, err
	}
	return &playback, nil
}

// PluginHealth represents the health status of the plugin.
type PluginHealth struct {
	Status  string    `json:"status"`
	Version string    `json:"version"`
	Uptime  int64     `json:"uptime"`
	Time    time.Time `json:"time"`
}

// PluginItem represents a Jellyfin item returned by the plugin.
type PluginItem struct {
	ID             string            `json:"Id"`
	Name           string            `json:"Name"`
	Path           string            `json:"Path"`
	Type           string            `json:"Type"`
	LibraryID      string            `json:"LibraryId"`
	LibraryName    string            `json:"LibraryName"`
	ParentID       string            `json:"ParentId,omitempty"`
	SeriesID       string            `json:"SeriesId,omitempty"`
	SeriesName     string            `json:"SeriesName,omitempty"`
	SeasonNumber   *int              `json:"SeasonNumber,omitempty"`
	EpisodeNumber  *int              `json:"EpisodeNumber,omitempty"`
	ProductionYear int               `json:"ProductionYear,omitempty"`
	ProviderIDs    map[string]string `json:"ProviderIds,omitempty"`
}

// PluginLibrary represents a Jellyfin library.
type PluginLibrary struct {
	ID             string   `json:"Id"`
	Name           string   `json:"Name"`
	CollectionType string   `json:"CollectionType"`
	Locations      []string `json:"Locations"`
}

// ActiveScansResponse contains information about active library scans.
type ActiveScansResponse struct {
	Scans []LibraryScan `json:"Scans"`
}

// LibraryScan represents an active library scan.
type LibraryScan struct {
	LibraryID    string    `json:"LibraryId"`
	LibraryName  string    `json:"LibraryName"`
	StartedAt    time.Time `json:"StartedAt"`
	ItemsScanned int       `json:"ItemsScanned"`
	CurrentPath  string    `json:"CurrentPath,omitempty"`
}

// UnidentifiableResponse contains items that couldn't be identified.
type UnidentifiableResponse struct {
	Items []UnidentifiableItem `json:"Items"`
	Count int                  `json:"Count"`
}

// UnidentifiableItem represents a file that Jellyfin couldn't identify.
type UnidentifiableItem struct {
	Path        string    `json:"Path"`
	LibraryID   string    `json:"LibraryId"`
	LibraryName string    `json:"LibraryName"`
	FileName    string    `json:"FileName"`
	AddedAt     time.Time `json:"AddedAt"`
	Reason      string    `json:"Reason,omitempty"`
}

// ActivePlaybackResponse contains active playback sessions.
type ActivePlaybackResponse struct {
	Sessions []PluginSession `json:"Sessions"`
	Count    int             `json:"Count"`
}

// PluginSession represents an active playback session.
type PluginSession struct {
	ID               string    `json:"Id"`
	UserName         string    `json:"UserName"`
	UserID           string    `json:"UserId"`
	Client           string    `json:"Client"`
	DeviceName       string    `json:"DeviceName"`
	ItemID           string    `json:"ItemId"`
	ItemName         string    `json:"ItemName"`
	ItemPath         string    `json:"ItemPath"`
	ItemType         string    `json:"ItemType"`
	IsPaused         bool      `json:"IsPaused"`
	PositionTicks    *int64    `json:"PositionTicks,omitempty"`
	PlaybackStarted  time.Time `json:"PlaybackStarted"`
	LastActivityDate time.Time `json:"LastActivityDate"`
}
