package jellyfin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetLatestItemsFiltersVirtualAndUsesLimit(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Users":
			require.Equal(t, http.MethodGet, r.Method)
			_, _ = w.Write([]byte(`[{"Id":"admin-1","Name":"Admin","Policy":{"IsAdministrator":true}}]`))
		case "/Users/admin-1/Items/Latest":
			require.Equal(t, http.MethodGet, r.Method)
			q := r.URL.Query()
			require.Equal(t, "5", q.Get("Limit"))
			require.Contains(t, q.Get("Fields"), "DateCreated")
			require.Contains(t, q.Get("Fields"), "SeriesId")
			items := []Item{
				{ID: "m1", Name: "Movie", Type: "Movie", LocationType: "FileSystem"},
				{ID: "v1", Name: "Virtual", Type: "Movie", LocationType: "Virtual"},
				{ID: "e1", Name: "Ep", Type: "Episode", SeriesID: "s1", LocationType: "FileSystem"},
			}
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(items))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL, APIKey: "key"})
	got, err := client.GetLatestItems(context.Background(), 5)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "m1", got[0].ID)
	assert.Equal(t, "e1", got[1].ID)
}

func TestGetPrimaryImageProxiesBytes(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/Items/abc/Images/Primary", r.URL.Path)
		q := r.URL.Query()
		require.Equal(t, "320", q.Get("fillHeight"))
		require.Equal(t, "213", q.Get("fillWidth"))
		require.Equal(t, "50", q.Get("quality"))
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte{0xff, 0xd8, 0xff})
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL, APIKey: "key"})
	ct, body, err := client.GetPrimaryImage(context.Background(), "abc", 320, 213, 50)
	require.NoError(t, err)
	assert.Equal(t, "image/jpeg", ct)
	assert.Equal(t, []byte{0xff, 0xd8, 0xff}, body)
}
