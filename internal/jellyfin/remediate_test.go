package jellyfin

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemediateOrphans_DryRun(t *testing.T) {
	t.Parallel()

	refreshCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			refreshCalls++
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL, APIKey: "test-api-key"})
	orphans := []Item{
		{ID: "ep-1", Name: "Orphan One", Path: "/media/orphan1.mkv"},
		{ID: "ep-2", Name: "Orphan Two", Path: "/media/orphan2.mkv"},
	}

	results, err := client.RemediateOrphans(orphans, true)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, 0, refreshCalls)
	assert.Equal(t, "skipped", results[0].Action)
	assert.Equal(t, "skipped", results[1].Action)
}

func TestRemediateOrphans_RefreshesOrphans(t *testing.T) {
	t.Parallel()

	var (
		mu          sync.Mutex
		refreshPath []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			mu.Lock()
			refreshPath = append(refreshPath, r.URL.Path)
			mu.Unlock()
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL, APIKey: "test-api-key"})
	orphans := []Item{
		{ID: "ep-1", Name: "Orphan One", Path: "/media/orphan1.mkv"},
		{ID: "ep-2", Name: "Orphan Two", Path: "/media/orphan2.mkv"},
	}

	results, err := client.RemediateOrphans(orphans, false)
	require.NoError(t, err)
	require.Len(t, results, 2)

	assert.Equal(t, []string{"/Items/ep-1/Refresh", "/Items/ep-2/Refresh"}, refreshPath)
	assert.Equal(t, "refreshed", results[0].Action)
	assert.Equal(t, "refreshed", results[1].Action)
	assert.NoError(t, results[0].Error)
	assert.NoError(t, results[1].Error)
}

func TestRemediateOrphans_ContinuesOnError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Items/ep-2/Refresh":
			http.Error(w, "fail", http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL, APIKey: "test-api-key"})
	orphans := []Item{
		{ID: "ep-1", Name: "Orphan One", Path: "/media/orphan1.mkv"},
		{ID: "ep-2", Name: "Orphan Two", Path: "/media/orphan2.mkv"},
		{ID: "ep-3", Name: "Orphan Three", Path: "/media/orphan3.mkv"},
	}

	results, err := client.RemediateOrphans(orphans, false)
	require.NoError(t, err)
	require.Len(t, results, 3)

	assert.Equal(t, "refreshed", results[0].Action)
	assert.Equal(t, "failed", results[1].Action)
	assert.Error(t, results[1].Error)
	assert.Equal(t, "refreshed", results[2].Action)
}
