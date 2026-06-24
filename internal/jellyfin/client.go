package jellyfin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Config struct {
	URL        string
	APIKey     string
	Timeout    time.Duration
	HTTPClient *http.Client
}

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	deviceID   string
	hostname   string
}

func NewClient(cfg Config) *Client {
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

	return &Client{
		baseURL:    strings.TrimRight(cfg.URL, "/"),
		apiKey:     cfg.APIKey,
		httpClient: httpClient,
		hostname:   hostname,
		deviceID:   fmt.Sprintf("jellywatch-%s-%d", hostname, time.Now().UnixNano()),
	}
}

func (c *Client) authHeader() string {
	return fmt.Sprintf(`MediaBrowser Token="%s", Client="jellywatch", Device="%s", DeviceId="%s", Version="1.0.0"`,
		c.apiKey, c.hostname, c.deviceID)
}

func (c *Client) request(method, endpoint string, body io.Reader) (*http.Response, error) {
	return c.requestWithAuth(method, endpoint, body, true)
}

func (c *Client) requestWithAuth(method, endpoint string, body io.Reader, withAuth bool) (*http.Response, error) {
	return c.requestCtx(context.Background(), method, endpoint, body, withAuth)
}

func (c *Client) requestCtx(ctx context.Context, method, endpoint string, body io.Reader, withAuth bool) (*http.Response, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	rel, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint: %w", err)
	}

	fullURL := base.ResolveReference(rel)

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, fullURL.String(), body)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		if withAuth && c.apiKey != "" {
			req.Header.Set("Authorization", c.authHeader())
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("executing request (attempt %d/3): %w", attempt+1, err)
			if attempt < 2 {
				time.Sleep(time.Duration(1<<uint(attempt)) * time.Second)
				continue
			}
			return nil, lastErr
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			defer resp.Body.Close()
			bodyBytes, _ := io.ReadAll(resp.Body)
			lastErr = fmt.Errorf("API error (status %d, attempt %d/3): %s", resp.StatusCode, attempt+1, string(bodyBytes))
			if resp.StatusCode >= 500 && attempt < 2 {
				time.Sleep(time.Duration(1<<uint(attempt)) * time.Second)
				continue
			}
			return nil, lastErr
		}

		return resp, nil
	}

	return nil, lastErr
}

func (c *Client) getCtx(ctx context.Context, endpoint string, result interface{}) error {
	resp, err := c.requestCtx(ctx, http.MethodGet, endpoint, nil, true)
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

func (c *Client) get(endpoint string, result interface{}) error {
	resp, err := c.request(http.MethodGet, endpoint, nil)
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

func (c *Client) post(endpoint string, payload, result interface{}) error {
	var body io.Reader
	if payload != nil {
		jsonBytes, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encoding payload: %w", err)
		}
		body = bytes.NewReader(jsonBytes)
	}

	resp, err := c.request(http.MethodPost, endpoint, body)
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

func (c *Client) GetSystemInfo() (*SystemInfo, error) {
	var info SystemInfo
	if err := c.get("/System/Info", &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func (c *Client) GetPublicInfo() (*PublicSystemInfo, error) {
	resp, err := c.requestWithAuth(http.MethodGet, "/System/Info/Public", nil, false)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var info PublicSystemInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &info, nil
}

// RemoteSearchResult mirrors a single Jellyfin RemoteSearch hit. Only
// fields we actually use are decoded; Jellyfin returns many more.
type RemoteSearchResult struct {
Name           string `json:"Name"`
ProductionYear int    `json:"ProductionYear"`
TmdbID         string `json:"-"`
ImdbID         string `json:"-"`
TvdbID         string `json:"-"`
ProviderIDs    map[string]string `json:"ProviderIds"`
}

// RemoteSearch queries Jellyfin's metadata-provider search for a title.
// kind must be "movie" or "series". Returns matches with ProviderIDs
// populated (Tmdb/Imdb/Tvdb).
func (c *Client) RemoteSearch(ctx context.Context, kind, name string) ([]RemoteSearchResult, error) {
endpoint := "/Items/RemoteSearch/Movie"
if kind == "series" || kind == "tv" {
endpoint = "/Items/RemoteSearch/Series"
}
payload := map[string]any{
"SearchInfo": map[string]any{
"Name": name,
},
}
body, err := json.Marshal(payload)
if err != nil {
return nil, err
}
resp, err := c.requestCtx(ctx, http.MethodPost, endpoint, bytes.NewReader(body), true)
if err != nil {
return nil, err
}
defer resp.Body.Close()

var out []RemoteSearchResult
if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
return nil, fmt.Errorf("decode RemoteSearch: %w", err)
}
for i := range out {
if out[i].ProviderIDs != nil {
out[i].TmdbID = out[i].ProviderIDs["Tmdb"]
out[i].ImdbID = out[i].ProviderIDs["Imdb"]
out[i].TvdbID = out[i].ProviderIDs["Tvdb"]
}
}
return out, nil
}
