package radarr

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMockRadarrServer(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/api/v3/config/mediamanagement":
			json.NewEncoder(w).Encode(MediaManagementConfig{
				ID:                      1,
				CreateEmptyMovieFolders: true,
			})
		case "/api/v3/config/naming":
			json.NewEncoder(w).Encode(NamingConfig{
				ID:                  1,
				RenameMovies:        false,
				ReplaceIllegalChars: true,
			})
		case "/api/v3/config/downloadClient", "/api/v3/config/downloadClient/1":
			json.NewEncoder(w).Encode(DownloadClientConfig{
				ID:                              1,
				EnableCompletedDownloadHandling: true,
				AutoRedownloadFailed:            true,
			})
		case "/api/v3/rootfolder":
			json.NewEncoder(w).Encode([]RootFolder{
				{
					ID:         1,
					Path:       "/movies",
					FreeSpace:  1000000000000,
					TotalSpace: 2000000000000,
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestGetMediaManagementConfig(t *testing.T) {
	server := newMockRadarrServer(t)
	defer server.Close()

	client := NewClient(Config{
		URL:    server.URL,
		APIKey: "test-key",
	})

	config, err := client.GetMediaManagementConfig()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if config == nil {
		t.Fatal("expected config, got nil")
	}
	if config.ID != 1 {
		t.Errorf("expected ID=1, got %d", config.ID)
	}
}

func TestGetNamingConfig(t *testing.T) {
	server := newMockRadarrServer(t)
	defer server.Close()

	client := NewClient(Config{
		URL:    server.URL,
		APIKey: "test-key",
	})

	config, err := client.GetNamingConfig()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if config == nil {
		t.Fatal("expected config, got nil")
	}
}

func TestGetRootFolders(t *testing.T) {
	server := newMockRadarrServer(t)
	defer server.Close()

	client := NewClient(Config{
		URL:    server.URL,
		APIKey: "test-key",
	})

	folders, err := client.GetRootFolders()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(folders) == 0 {
		t.Fatal("expected at least one root folder")
	}
}

func TestUpdateMediaManagementConfig(t *testing.T) {
	server := newMockRadarrServer(t)
	defer server.Close()

	client := NewClient(Config{
		URL:    server.URL,
		APIKey: "test-key",
	})

	config := &MediaManagementConfig{
		ID:                      1,
		CreateEmptyMovieFolders: true,
	}

	err := client.UpdateMediaManagementConfig(config)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestUpdateNamingConfig(t *testing.T) {
	server := newMockRadarrServer(t)
	defer server.Close()

	client := NewClient(Config{
		URL:    server.URL,
		APIKey: "test-key",
	})

	config := &NamingConfig{
		ID:                  1,
		RenameMovies:        false,
		ReplaceIllegalChars: true,
	}

	err := client.UpdateNamingConfig(config)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestDeleteRootFolder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]RootFolder{
				{
					ID:         1,
					Path:       "/movies",
					FreeSpace:  1000000000000,
					TotalSpace: 2000000000000,
				},
			})
		} else if r.Method == http.MethodDelete && r.URL.Path == "/api/v3/rootfolder/1" {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		URL:    server.URL,
		APIKey: "test-key",
	})

	err := client.DeleteRootFolder(1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestGetDownloadClientConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v3/config/downloadClient" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(DownloadClientConfig{
				ID:                              1,
				EnableCompletedDownloadHandling: true,
				AutoRedownloadFailed:            true,
			})
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL, APIKey: "test"})
	cfg, err := client.GetDownloadClientConfig()
	require.NoError(t, err)
	assert.True(t, cfg.EnableCompletedDownloadHandling)
	assert.True(t, cfg.AutoRedownloadFailed)
	assert.Equal(t, int64(1), cfg.ID)
}

func TestUpdateDownloadClientConfig(t *testing.T) {
	var received DownloadClientConfig
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v3/config/downloadClient/1" && r.Method == http.MethodPut {
			w.Header().Set("Content-Type", "application/json")
			json.NewDecoder(r.Body).Decode(&received)
			json.NewEncoder(w).Encode(received)
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL, APIKey: "test"})
	cfg := DownloadClientConfig{
		ID:                              1,
		EnableCompletedDownloadHandling: false,
		AutoRedownloadFailed:            true,
	}
	result, err := client.UpdateDownloadClientConfig(cfg)
	require.NoError(t, err)
	assert.False(t, result.EnableCompletedDownloadHandling)
}
