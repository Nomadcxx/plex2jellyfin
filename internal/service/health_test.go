package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/radarr"
	"github.com/Nomadcxx/jellywatch/internal/sonarr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckSonarrConfig_AllGood(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/config/downloadClient":
			json.NewEncoder(w).Encode(sonarr.DownloadClientConfig{
				ID:                              1,
				EnableCompletedDownloadHandling: false,
			})
		case "/api/v3/config/naming":
			json.NewEncoder(w).Encode(sonarr.NamingConfig{
				ID:             1,
				RenameEpisodes: true,
			})
		}
	}))
	defer server.Close()

	client := sonarr.NewClient(sonarr.Config{URL: server.URL, APIKey: "test"})
	issues, err := CheckSonarrConfig(client)
	require.NoError(t, err)
	assert.Empty(t, issues)
}

func TestCheckSonarrConfig_CompletedDownloadEnabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/config/downloadClient":
			json.NewEncoder(w).Encode(sonarr.DownloadClientConfig{
				ID:                              1,
				EnableCompletedDownloadHandling: true,
			})
		case "/api/v3/config/naming":
			json.NewEncoder(w).Encode(sonarr.NamingConfig{
				ID:             1,
				RenameEpisodes: true,
			})
		}
	}))
	defer server.Close()

	client := sonarr.NewClient(sonarr.Config{URL: server.URL, APIKey: "test"})
	issues, err := CheckSonarrConfig(client)
	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, "sonarr", issues[0].Service)
	assert.Equal(t, "enableCompletedDownloadHandling", issues[0].Setting)
	assert.Equal(t, "critical", issues[0].Severity)
}

func TestCheckSonarrConfig_RenameDisabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/config/downloadClient":
			json.NewEncoder(w).Encode(sonarr.DownloadClientConfig{
				ID:                              1,
				EnableCompletedDownloadHandling: false,
			})
		case "/api/v3/config/naming":
			json.NewEncoder(w).Encode(sonarr.NamingConfig{
				ID:             1,
				RenameEpisodes: false,
			})
		}
	}))
	defer server.Close()

	client := sonarr.NewClient(sonarr.Config{URL: server.URL, APIKey: "test"})
	issues, err := CheckSonarrConfig(client)
	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, "sonarr", issues[0].Service)
	assert.Equal(t, "renameEpisodes", issues[0].Setting)
	assert.Equal(t, "warning", issues[0].Severity)
}

func TestCheckSonarrConfig_BothBad(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/config/downloadClient":
			json.NewEncoder(w).Encode(sonarr.DownloadClientConfig{
				ID:                              1,
				EnableCompletedDownloadHandling: true,
			})
		case "/api/v3/config/naming":
			json.NewEncoder(w).Encode(sonarr.NamingConfig{
				ID:             1,
				RenameEpisodes: false,
			})
		}
	}))
	defer server.Close()

	client := sonarr.NewClient(sonarr.Config{URL: server.URL, APIKey: "test"})
	issues, err := CheckSonarrConfig(client)
	require.NoError(t, err)
	assert.Len(t, issues, 2)
}

func TestCheckRadarrConfig_AllGood(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/config/downloadClient":
			json.NewEncoder(w).Encode(radarr.DownloadClientConfig{
				ID:                              1,
				EnableCompletedDownloadHandling: false,
			})
		case "/api/v3/config/naming":
			json.NewEncoder(w).Encode(radarr.NamingConfig{
				ID:           1,
				RenameMovies: true,
			})
		}
	}))
	defer server.Close()

	client := radarr.NewClient(radarr.Config{URL: server.URL, APIKey: "test"})
	issues, err := CheckRadarrConfig(client)
	require.NoError(t, err)
	assert.Empty(t, issues)
}

func TestCheckRadarrConfig_CompletedDownloadEnabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/config/downloadClient":
			json.NewEncoder(w).Encode(radarr.DownloadClientConfig{
				ID:                              1,
				EnableCompletedDownloadHandling: true,
			})
		case "/api/v3/config/naming":
			json.NewEncoder(w).Encode(radarr.NamingConfig{
				ID:           1,
				RenameMovies: true,
			})
		}
	}))
	defer server.Close()

	client := radarr.NewClient(radarr.Config{URL: server.URL, APIKey: "test"})
	issues, err := CheckRadarrConfig(client)
	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, "radarr", issues[0].Service)
	assert.Equal(t, "enableCompletedDownloadHandling", issues[0].Setting)
	assert.Equal(t, "critical", issues[0].Severity)
}

func TestCheckRadarrConfig_RenameDisabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/config/downloadClient":
			json.NewEncoder(w).Encode(radarr.DownloadClientConfig{
				ID:                              1,
				EnableCompletedDownloadHandling: false,
			})
		case "/api/v3/config/naming":
			json.NewEncoder(w).Encode(radarr.NamingConfig{
				ID:           1,
				RenameMovies: false,
			})
		}
	}))
	defer server.Close()

	client := radarr.NewClient(radarr.Config{URL: server.URL, APIKey: "test"})
	issues, err := CheckRadarrConfig(client)
	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, "radarr", issues[0].Service)
	assert.Equal(t, "renameMovies", issues[0].Setting)
	assert.Equal(t, "warning", issues[0].Severity)
}

func TestFixSonarrIssues_DryRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/config/downloadClient":
			json.NewEncoder(w).Encode(sonarr.DownloadClientConfig{
				ID:                              1,
				EnableCompletedDownloadHandling: true,
			})
		}
	}))
	defer server.Close()

	client := sonarr.NewClient(sonarr.Config{URL: server.URL, APIKey: "test"})
	issues := []HealthIssue{
		{Service: "sonarr", Setting: "enableCompletedDownloadHandling"},
	}

	fixed, err := FixSonarrIssues(client, issues, true)
	require.NoError(t, err)
	assert.Len(t, fixed, 1)
}

func TestFixSonarrIssues_RealFix(t *testing.T) {
	var putCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			putCalled = true
			var cfg sonarr.DownloadClientConfig
			json.NewDecoder(r.Body).Decode(&cfg)
			json.NewEncoder(w).Encode(cfg)
			return
		}
		switch r.URL.Path {
		case "/api/v3/config/downloadClient":
			json.NewEncoder(w).Encode(sonarr.DownloadClientConfig{
				ID:                              1,
				EnableCompletedDownloadHandling: true,
			})
		}
	}))
	defer server.Close()

	client := sonarr.NewClient(sonarr.Config{URL: server.URL, APIKey: "test"})
	issues := []HealthIssue{
		{Service: "sonarr", Setting: "enableCompletedDownloadHandling"},
	}

	fixed, err := FixSonarrIssues(client, issues, false)
	require.NoError(t, err)
	assert.Len(t, fixed, 1)
	assert.True(t, putCalled)
}

func TestFixRadarrIssues_DryRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/config/downloadClient":
			json.NewEncoder(w).Encode(radarr.DownloadClientConfig{
				ID:                              1,
				EnableCompletedDownloadHandling: true,
			})
		}
	}))
	defer server.Close()

	client := radarr.NewClient(radarr.Config{URL: server.URL, APIKey: "test"})
	issues := []HealthIssue{
		{Service: "radarr", Setting: "enableCompletedDownloadHandling"},
	}

	fixed, err := FixRadarrIssues(client, issues, true)
	require.NoError(t, err)
	assert.Len(t, fixed, 1)
}
