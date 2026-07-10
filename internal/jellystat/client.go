// Package jellystat is a minimal client for a Jellystat instance
// (https://github.com/CyferShepard/Jellystat). Authentication uses the
// x-api-token header with an API key generated in Jellystat's settings.
package jellystat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Config struct {
	URL     string
	APIKey  string
	Timeout time.Duration
}

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func NewClient(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &Client{
		baseURL: strings.TrimRight(cfg.URL, "/"),
		apiKey:  cfg.APIKey,
		http:    &http.Client{Timeout: timeout},
	}
}

func (c *Client) do(method, path string, body any) (json.RawMessage, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("jellystat: no URL configured")
	}
	if _, err := url.Parse(c.baseURL); err != nil {
		return nil, fmt.Errorf("jellystat: invalid URL: %w", err)
	}

	var reqBody io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(payload)
	}

	req, err := http.NewRequest(method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-token", c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("jellystat: authentication failed (check the API key)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jellystat: %s %s returned %d", method, path, resp.StatusCode)
	}
	return json.RawMessage(raw), nil
}

// Test verifies connectivity and authentication.
func (c *Client) Test() error {
	_, err := c.LibraryOverview()
	return err
}

// LibraryOverview returns Jellystat's per-library overview rows.
func (c *Client) LibraryOverview() (json.RawMessage, error) {
	return c.do(http.MethodGet, "/stats/getLibraryOverview", nil)
}

// MostViewedByType returns the top viewed items of mediaType ("Movie" or
// "Series") over the trailing number of days.
func (c *Client) MostViewedByType(days int, mediaType string) (json.RawMessage, error) {
	return c.do(http.MethodPost, "/stats/getMostViewedByType", map[string]any{
		"days": days,
		"type": mediaType,
	})
}

// GlobalItemStats returns play count and duration for one Jellyfin item ID
// over the trailing number of hours.
func (c *Client) GlobalItemStats(hours int, itemID string) (json.RawMessage, error) {
	return c.do(http.MethodPost, "/stats/getGlobalItemStats", map[string]any{
		"hours":  hours,
		"itemid": itemID,
	})
}
