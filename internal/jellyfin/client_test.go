package jellyfin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClient_DefaultsAndConfig(t *testing.T) {
	client := NewClient(Config{URL: "http://localhost:8096/", APIKey: "token"})

	if client == nil {
		t.Fatalf("expected client")
	}
	if client.baseURL != "http://localhost:8096" {
		t.Fatalf("baseURL = %q, want %q", client.baseURL, "http://localhost:8096")
	}
	if client.httpClient == nil {
		t.Fatalf("expected http client")
	}
	if client.httpClient.Timeout != 30*time.Second {
		t.Fatalf("timeout = %v, want %v", client.httpClient.Timeout, 30*time.Second)
	}
	if client.apiKey != "token" {
		t.Fatalf("apiKey = %q, want %q", client.apiKey, "token")
	}
}

func TestGetSystemInfo_MakesExpectedHTTPRequest(t *testing.T) {
	var gotMethod, gotPath, gotAuth string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(SystemInfo{ServerName: "Jellyfin", Version: "10.9.0", ID: "server-1"})
	}))
	defer ts.Close()

	client := NewClient(Config{URL: ts.URL, APIKey: "secret-key", Timeout: 5 * time.Second})
	info, err := client.GetSystemInfo()
	if err != nil {
		t.Fatalf("GetSystemInfo() error = %v", err)
	}

	if gotMethod != http.MethodGet {
		t.Fatalf("method = %s, want GET", gotMethod)
	}
	if gotPath != "/System/Info" {
		t.Fatalf("path = %s, want /System/Info", gotPath)
	}
	if !strings.Contains(gotAuth, `Token="secret-key"`) {
		t.Fatalf("expected auth header to include API token, got %q", gotAuth)
	}
	if info == nil || info.ServerName != "Jellyfin" {
		t.Fatalf("unexpected response: %+v", info)
	}
}

func TestRefreshItem_MakesPostRequestWithJSONBody(t *testing.T) {
	var gotMethod, gotPath, gotContentType string
	var gotBody strings.Builder

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		bodyBytes, _ := io.ReadAll(r.Body)
		gotBody.Write(bodyBytes)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	client := NewClient(Config{URL: ts.URL, APIKey: "secret-key"})
	if err := client.RefreshItem("item-123"); err != nil {
		t.Fatalf("RefreshItem() error = %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Fatalf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/Items/item-123/Refresh" {
		t.Fatalf("path = %s, want /Items/item-123/Refresh", gotPath)
	}
	if gotContentType != "application/json" {
		t.Fatalf("content-type = %q, want application/json", gotContentType)
	}
	if !strings.Contains(gotBody.String(), `"Recursive":true`) {
		t.Fatalf("expected request body to include refresh payload, got %q", gotBody.String())
	}
}

func TestGetSystemInfo_InvalidURLAndHTTPError(t *testing.T) {
	badClient := NewClient(Config{URL: "://bad-url", APIKey: "secret-key"})
	if _, err := badClient.GetSystemInfo(); err == nil || !strings.Contains(err.Error(), "invalid base URL") {
		t.Fatalf("expected invalid base URL error, got %v", err)
	}

	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer errorServer.Close()

	client := NewClient(Config{URL: errorServer.URL, APIKey: "secret-key"})
	if _, err := client.GetSystemInfo(); err == nil || !strings.Contains(err.Error(), "API error (status 502)") {
		t.Fatalf("expected API status error, got %v", err)
	}
}
