package radarr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Config struct {
	URL     string
	APIKey  string
	Timeout time.Duration
}

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewClient(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &Client{
		baseURL: cfg.URL,
		apiKey:  cfg.APIKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) request(ctx context.Context, method, endpoint string, body io.Reader) (*http.Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	fullURL, err := joinURL(c.baseURL, endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		req.Header.Set("X-Api-Key", c.apiKey)
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

func (c *Client) get(endpoint string, result interface{}) error {
	return c.getContext(context.Background(), endpoint, result)
}

func (c *Client) getContext(ctx context.Context, endpoint string, result interface{}) error {
	resp, err := c.request(ctx, http.MethodGet, endpoint, nil)
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
	return c.postContext(context.Background(), endpoint, payload, result)
}

func (c *Client) postContext(ctx context.Context, endpoint string, payload, result interface{}) error {
	var body io.Reader
	if payload != nil {
		jsonBytes, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encoding payload: %w", err)
		}
		body = bytes.NewReader(jsonBytes)
	}

	resp, err := c.request(ctx, http.MethodPost, endpoint, body)
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

func (c *Client) put(endpoint string, payload, result interface{}) error {
	return c.putContext(context.Background(), endpoint, payload, result)
}

func (c *Client) putContext(ctx context.Context, endpoint string, payload, result interface{}) error {
	var body io.Reader
	if payload != nil {
		jsonBytes, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encoding payload: %w", err)
		}
		body = bytes.NewReader(jsonBytes)
	}

	resp, err := c.request(ctx, http.MethodPut, endpoint, body)
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

func (c *Client) delete(endpoint string) error {
	return c.deleteContext(context.Background(), endpoint)
}

func (c *Client) deleteContext(ctx context.Context, endpoint string) error {
	resp, err := c.request(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *Client) Ping() error {
	return c.PingContext(context.Background())
}

func (c *Client) PingContext(ctx context.Context) error {
	var status SystemStatus
	if err := c.getContext(ctx, "/api/v3/system/status", &status); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}
	return nil
}

func (c *Client) GetSystemStatus() (*SystemStatus, error) {
	return c.GetSystemStatusContext(context.Background())
}

func (c *Client) GetSystemStatusContext(ctx context.Context) (*SystemStatus, error) {
	var status SystemStatus
	if err := c.getContext(ctx, "/api/v3/system/status", &status); err != nil {
		return nil, err
	}
	return &status, nil
}
