package jellyfin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
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

func TestGetOrphanedEpisodesPaginatesPastFirstPage(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/Items", r.URL.Path)
		require.Equal(t, "Episode", r.URL.Query().Get("IncludeItemTypes"))
		require.Equal(t, "10000", r.URL.Query().Get("Limit"))
		startIndex := r.URL.Query().Get("StartIndex")
		var items []Item
		switch startIndex {
		case "0":
			items = make([]Item, 10000)
			for i := range items {
				items[i] = Item{ID: "ep-" + strconv.Itoa(i), Type: "Episode", SeriesID: "series"}
			}
		case "10000":
			items = []Item{{ID: "ep-10000", Type: "Episode"}}
		default:
			t.Errorf("unexpected page start index %q", startIndex)
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(ItemsResponse{Items: items, TotalRecordCount: 10001}))
	}))
	defer server.Close()

	orphans, err := NewClient(Config{URL: server.URL}).GetOrphanedEpisodes()
	require.NoError(t, err)
	require.Len(t, orphans, 1)
	assert.Equal(t, "ep-10000", orphans[0].ID)
}

func TestGetItemsByIDsUsesSingleRequest(t *testing.T) {
	t.Parallel()

	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/Items", r.URL.Path)
		require.Equal(t, "id1,id2", r.URL.Query().Get("Ids"))
		require.Equal(t, "Path,ProviderIds,SeriesId,SeriesName,ParentId,IndexNumber,ParentIndexNumber,Overview,ImageTags,PremiereDate", r.URL.Query().Get("Fields"))

		response := ItemsResponse{
			Items: []Item{
				{ID: "id1", Name: "Movie", Type: "Movie", ProviderIDs: map[string]string{"Tmdb": "123"}},
				{ID: "id2", Name: "Episode", Type: "Episode", ParentID: "season-1", SeriesID: "series-1"},
			},
			TotalRecordCount: 2,
		}

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(response))
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL, APIKey: "test-api-key"})
	items, err := client.GetItemsByIDs(context.Background(), []string{"id1", "id2"})

	require.NoError(t, err)
	require.NotNil(t, items)
	assert.Equal(t, 1, requestCount)
	assert.Len(t, items.Items, 2)
	assert.Equal(t, "id1", items.Items[0].ID)
	assert.Equal(t, "id2", items.Items[1].ID)
}

func TestGetItemUsesBatchedItemsEndpoint(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/Items", r.URL.Path)
		require.Equal(t, "item-1", r.URL.Query().Get("Ids"))

		response := ItemsResponse{
			Items: []Item{{ID: "item-1", Name: "Resolved Name", Type: "Movie"}},
		}

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(response))
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL, APIKey: "test-api-key"})
	item, err := client.GetItem("item-1")

	require.NoError(t, err)
	require.NotNil(t, item)
	assert.Equal(t, "Resolved Name", item.Name)
}

func TestGetItemReturnsErrorWhenMissing(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/Items", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(ItemsResponse{}))
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL, APIKey: "test-api-key"})
	item, err := client.GetItem("missing")

	require.Error(t, err)
	assert.Nil(t, item)
	assert.True(t, errors.Is(err, ErrItemNotFound))
	assert.Contains(t, err.Error(), "not found")
}
