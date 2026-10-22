package organizer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin"
	"github.com/Nomadcxx/plex2jellyfin/internal/transfer"
	"github.com/stretchr/testify/require"
)

func TestTransferSourceStopsWhenJellyfinScanDoesNotFinish(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/Plex2Jellyfin/active-scans", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(jellyfin.ActiveScansResponse{
			Scans: []jellyfin.LibraryScan{{LibraryID: "tv"}},
		}))
	}))
	defer server.Close()

	source := filepath.Join(t.TempDir(), "episode.mkv")
	require.NoError(t, os.WriteFile(source, []byte("keep source"), 0o644))
	transferer := &recordingTransferer{}
	org, err := NewOrganizer([]string{t.TempDir()},
		WithPluginClient(jellyfin.NewPluginClient(jellyfin.Config{URL: server.URL}), true),
		WithTimeout(time.Nanosecond),
		WithTransferer(transferer),
	)
	require.NoError(t, err)

	result, sourceRetained, err := org.transferSource(source, filepath.Join(t.TempDir(), "target.mkv"), transfer.TransferOptions{})
	require.ErrorContains(t, err, "timeout waiting for Jellyfin scan to complete")
	require.NotNil(t, result)
	require.ErrorIs(t, result.Error, err)
	require.False(t, result.Success)
	require.False(t, sourceRetained)
	require.Zero(t, transferer.moves)
	require.FileExists(t, source)
}

func TestTransferSourceStopsWhenScanStatusCheckFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	source := filepath.Join(t.TempDir(), "episode.mkv")
	require.NoError(t, os.WriteFile(source, []byte("keep source"), 0o644))
	transferer := &recordingTransferer{}
	org, err := NewOrganizer([]string{t.TempDir()},
		WithPluginClient(jellyfin.NewPluginClient(jellyfin.Config{URL: server.URL}), true),
		WithTransferer(transferer),
	)
	require.NoError(t, err)

	result, _, err := org.transferSource(source, filepath.Join(t.TempDir(), "target.mkv"), transfer.TransferOptions{})
	require.ErrorContains(t, err, "checking Jellyfin scan status")
	require.ErrorContains(t, err, "API error (status 401")
	require.NotNil(t, result)
	require.ErrorIs(t, result.Error, err)
	require.Zero(t, transferer.moves)
	require.FileExists(t, source)
}
