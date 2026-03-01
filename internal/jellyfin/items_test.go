package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetOrphanedEpisodes(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/Items", r.URL.Path)
		require.Equal(t, http.MethodGet, r.Method)

		response := ItemsResponse{
			Items: []Item{
				{ID: "1", Name: "Linked Episode", Type: "Episode", SeriesID: "series-1"},
				{ID: "2", Name: "Orphaned Episode", Type: "Episode", SeriesID: ""},
				{ID: "3", Name: "Another Orphan", Type: "Episode", SeriesID: ""},
			},
			TotalRecordCount: 3,
		}

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(response))
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL, APIKey: "test-api-key"})
	orphans, err := client.GetOrphanedEpisodes()

	require.NoError(t, err)
	assert.Len(t, orphans, 2)
	assert.Equal(t, "2", orphans[0].ID)
	assert.Equal(t, "3", orphans[1].ID)
}
