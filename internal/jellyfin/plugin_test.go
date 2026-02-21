package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewPluginClient(t *testing.T) {
	cfg := Config{
		URL:       "http://localhost:8096",
		APIKey:    "test-key",
		Timeout:   30 * time.Second,
	}

	client := NewPluginClient(cfg)
	if client == nil {
		t.Fatal("NewPluginClient returned nil")
	}

	if client.baseURL != "http://localhost:8096" {
		t.Errorf("expected baseURL to be 'http://localhost:8096', got '%s'", client.baseURL)
	}

	if client.apiKey != "test-key" {
		t.Errorf("expected apiKey to be 'test-key', got '%s'", client.apiKey)
	}
}

func TestPluginClientHealth(t *testing.T) {
	mockResponse := PluginHealth{
		Status:  "healthy",
		Version: "1.0.0",
		Time:    time.Now(),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/JellyWatch/health" {
			t.Errorf("expected path '/JellyWatch/health', got '%s'", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	cfg := Config{
		URL:    server.URL,
		APIKey: "test-key",
	}

	client := NewPluginClient(cfg)
	health, err := client.Health()
	if err != nil {
		t.Fatalf("Health() error: %v", err)
	}

	if health.Status != "healthy" {
		t.Errorf("expected status 'healthy', got '%s'", health.Status)
	}
}

func TestPluginClientGetItemByPath(t *testing.T) {
	mockItem := PluginItem{
		ID:          "test-id",
		Name:        "Test Movie",
		Path:        "/movies/test.mkv",
		Type:        "Movie",
		LibraryID:   "lib-1",
		LibraryName: "Movies",
		ProviderIDs: map[string]string{"imdb": "tt12345"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/JellyWatch/item-by-path" {
			t.Errorf("expected path '/JellyWatch/item-by-path', got '%s'", r.URL.Path)
		}
		path := r.URL.Query().Get("path")
		if path == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockItem)
	}))
	defer server.Close()

	cfg := Config{
		URL:    server.URL,
		APIKey: "test-key",
	}

	client := NewPluginClient(cfg)
	item, err := client.GetItemByPath("/movies/test.mkv")
	if err != nil {
		t.Fatalf("GetItemByPath() error: %v", err)
	}

	if item.Name != "Test Movie" {
		t.Errorf("expected name 'Test Movie', got '%s'", item.Name)
	}
}

func TestPluginClientTriggerScan(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/JellyWatch/scan-library" {
			t.Errorf("expected path '/JellyWatch/scan-library', got '%s'", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got '%s'", r.Method)
		}
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"message": "Scan started"})
	}))
	defer server.Close()

	cfg := Config{
		URL:    server.URL,
		APIKey: "test-key",
	}

	client := NewPluginClient(cfg)
	err := client.TriggerScan("", "/movies")
	if err != nil {
		t.Fatalf("TriggerScan() error: %v", err)
	}
}

func TestPluginClientGetActiveScans(t *testing.T) {
	mockResponse := ActiveScansResponse{
		Scans: []LibraryScan{
			{
				LibraryID: "lib-1",
				LibraryName: "Movies",
				StartedAt: time.Now().Add(-5 * time.Minute),
				ItemsScanned: 50,
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/JellyWatch/active-scans" {
			t.Errorf("expected path '/JellyWatch/active-scans', got '%s'", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	cfg := Config{
		URL:    server.URL,
		APIKey: "test-key",
	}

	client := NewPluginClient(cfg)
	scans, err := client.GetActiveScans()
	if err != nil {
		t.Fatalf("GetActiveScans() error: %v", err)
	}

	if len(scans.Scans) != 1 {
		t.Errorf("expected 1 scan, got %d", len(scans.Scans))
	}
}

func TestPluginClientRetryLogic(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PluginHealth{
			Status:  "healthy",
			Version: "1.0.0",
		})
	}))
	defer server.Close()

	cfg := Config{
		URL:    server.URL,
		APIKey: "test-key",
	}

	client := NewPluginClient(cfg)
	health, err := client.Health()
	if err != nil {
		t.Fatalf("Health() error: %v", err)
	}

	if attemptCount != 3 {
		t.Errorf("expected 3 attempts due to retries, got %d", attemptCount)
	}

	if health.Status != "healthy" {
		t.Errorf("expected status 'healthy', got '%s'", health.Status)
	}
}
