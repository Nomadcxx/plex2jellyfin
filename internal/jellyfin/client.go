package jellyfin

import (
	"bytes"
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
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	rel, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint: %w", err)
	}

	fullURL := base.ResolveReference(rel)
	req, err := http.NewRequest(method, fullURL.String(), body)
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
		return nil, fmt.Errorf("executing request: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	return resp, nil
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

func (c *Client) Ping() error {
	_, err := c.GetSystemInfo()
	if err != nil {
		return fmt.Errorf("ping failed: %w", err)
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
